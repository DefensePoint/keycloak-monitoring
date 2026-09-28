package chi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/operator"
)

// stubOperatorService records the realm filter every query ran with, so a test
// can assert the scope the handler applied.
type stubOperatorService struct {
	actions   []*domain.OperatorAction
	summaries []*domain.OperatorMetricsSummary

	calls     int
	gotRealms []string
}

func (s *stubOperatorService) RecordAction(context.Context, *domain.OperatorAction) error { return nil }

func (s *stubOperatorService) GetMetrics(_ context.Context, _, _ string, _, _ time.Time, realmNames []string) (*domain.OperatorMetricsSummary, error) {
	s.calls++
	s.gotRealms = realmNames
	if len(s.summaries) == 0 {
		return nil, nil
	}
	return s.summaries[0], nil
}

func (s *stubOperatorService) GetAllOperatorsMetrics(_ context.Context, _ string, _, _ time.Time, realmNames []string) ([]*domain.OperatorMetricsSummary, error) {
	s.calls++
	s.gotRealms = realmNames
	return s.summaries, nil
}

func (s *stubOperatorService) GetActions(_ context.Context, _, _ string, _, _ time.Time, _ int, realmNames []string) ([]*domain.OperatorAction, error) {
	s.calls++
	s.gotRealms = realmNames
	return s.actions, nil
}

func (s *stubOperatorService) GetLastActionForAlert(context.Context, string, uint) (*domain.OperatorAction, error) {
	return nil, nil
}

var _ operator.Service = (*stubOperatorService)(nil)

func setupTestOperatorRouter(h *OperatorHandlers) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/tenants/{tenantID}", func(r chi.Router) {
		r.Use(TenantIDMiddleware)
		h.RegisterTenantRoutes(r)
	})
	return r
}

// operatorRoute is one read in the /metrics group. assertEmpty is the shape a
// caller allowed no realm must get back.
type operatorRoute struct {
	name        string
	path        string
	assertEmpty func(t *testing.T, body []byte)
}

func operatorRoutes() []operatorRoute {
	return []operatorRoute{
		{
			name:        "GET /metrics/actions",
			path:        "/metrics/actions",
			assertEmpty: assertEmptyJSONArray,
		},
		{
			name:        "GET /metrics/operators",
			path:        "/metrics/operators",
			assertEmpty: assertEmptyJSONArray,
		},
		{
			name: "GET /metrics/operators/details",
			path: "/metrics/operators/details?email=operator@example.com",
			assertEmpty: func(t *testing.T, body []byte) {
				t.Helper()
				var summary domain.OperatorMetricsSummary
				if err := json.Unmarshal(body, &summary); err != nil {
					t.Fatalf("decode: %v; body = %s", err, body)
				}
				if summary.TotalAlertsHandled != 0 || summary.TotalWorkTimeHours != 0 || len(summary.AlertsByType) != 0 {
					t.Errorf("summary carries numbers for a caller allowed no realm: %+v", summary)
				}
			},
		},
	}
}

func assertEmptyJSONArray(t *testing.T, body []byte) {
	t.Helper()
	var rows []json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode: %v; body = %s", err, body)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want an empty result", len(rows))
	}
}

func TestOperatorHandlers_MetricsRoutes_RealmPolicy(t *testing.T) {
	tests := []struct {
		name       string
		rbac       *mockRBACChecker
		wantStatus int
		wantRealms []string
		wantCalled bool
	}{
		{
			name:       "caller with no policy row reads every realm",
			rbac:       &mockRBACChecker{},
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "admin reads every realm despite a restricting policy",
			rbac:       &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-a", "realmB")},
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "policy restricted to one realm narrows the query",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			wantStatus: http.StatusOK,
			wantRealms: []string{"realmB"},
			wantCalled: true,
		},
		{
			name:       "policy restricted to several realms keeps all of them",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-a", "realmB", "realmC")},
			wantStatus: http.StatusOK,
			wantRealms: []string{"realmB", "realmC"},
			wantCalled: true,
		},
		{
			name:       "policy with an empty realm list returns nothing",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-a")},
			wantStatus: http.StatusOK,
		},
		{
			name:       "a failed policy lookup refuses the query",
			rbac:       &mockRBACChecker{policiesErr: errors.New("policy lookup failed")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, route := range operatorRoutes() {
		t.Run(route.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					svc := &stubOperatorService{
						actions: []*domain.OperatorAction{{RealmName: "realmB", Comment: "looks like a false positive"}},
						summaries: []*domain.OperatorMetricsSummary{
							{OperatorEmail: "operator@example.com", TotalAlertsHandled: 42, TotalWorkTimeHours: 7},
						},
					}
					h := NewOperatorHandlers(svc, tt.rbac, newTestLogger(), testAuthMiddleware, NewRBACMiddleware(tt.rbac, newTestLogger()))

					req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a"+route.path, nil)
					rec := httptest.NewRecorder()
					setupTestOperatorRouter(h).ServeHTTP(rec, req)

					if rec.Code != tt.wantStatus {
						t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
					}
					if !tt.wantCalled {
						if svc.calls != 0 {
							t.Fatalf("queried %d times for a refused caller, want 0", svc.calls)
						}
						if tt.wantStatus == http.StatusOK {
							route.assertEmpty(t, rec.Body.Bytes())
						}
						return
					}
					if svc.calls == 0 {
						t.Fatalf("never queried, want realms %v", tt.wantRealms)
					}
					if !reflect.DeepEqual(svc.gotRealms, tt.wantRealms) {
						t.Errorf("realms = %v, want %v", svc.gotRealms, tt.wantRealms)
					}
				})
			}
		})
	}
}

func TestOperatorHandlers_MetricsRoutes_RequireAuthenticatedCaller(t *testing.T) {
	handlers := map[string]func(*OperatorHandlers, http.ResponseWriter, *http.Request){
		"actions": (*OperatorHandlers).handleGetOperatorActions,
		"all":     (*OperatorHandlers).handleGetAllOperatorsMetrics,
		"details": (*OperatorHandlers).handleGetOperatorMetrics,
	}

	for name, handle := range handlers {
		t.Run(name, func(t *testing.T) {
			svc := &stubOperatorService{}
			h := NewOperatorHandlers(svc, &mockRBACChecker{}, newTestLogger(), nil, nil)

			req := httptest.NewRequest(http.MethodGet, "/metrics?email=operator@example.com", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("tenantID", "tenant-a")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rec := httptest.NewRecorder()
			handle(h, rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			if svc.calls != 0 {
				t.Errorf("queried %d times without an authenticated caller, want 0", svc.calls)
			}
		})
	}
}
