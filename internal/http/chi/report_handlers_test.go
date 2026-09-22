package chi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/reports"
)

// stubReportService records the request it aggregated so a test can assert the
// realm scope the handler resolved for the caller.
type stubReportService struct {
	calls   int
	lastReq *reports.GenerateRequest
}

func (s *stubReportService) AggregateData(_ context.Context, req *reports.GenerateRequest) (*reports.Data, error) {
	s.calls++
	s.lastReq = req
	return &reports.Data{TenantID: req.TenantID}, nil
}

func (s *stubReportService) GeneratePDF(*reports.Data) ([]byte, error) {
	return []byte("%PDF-1.4"), nil
}

var _ reports.Service = (*stubReportService)(nil)

func setupTestReportRouter(h *ReportHandlers) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api/tenants/{tenantID}", func(r chi.Router) {
		r.Use(TenantIDMiddleware)
		h.RegisterTenantRoutes(r)
	})
	return r
}

const reportPath = "/api/tenants/tenant-a/reports/generate?start_date=2026-01-01&end_date=2026-01-31"

func TestReportHandlers_Generate_RealmScope(t *testing.T) {
	tests := []struct {
		name       string
		rbac       *mockRBACChecker
		wantStatus int
		wantScope  reports.RealmScope
	}{
		{
			name:       "caller with no policy row covers the whole tenant",
			rbac:       &mockRBACChecker{},
			wantStatus: http.StatusOK,
			wantScope:  reports.RealmScope{All: true},
		},
		{
			name:       "admin covers the whole tenant despite a restricting policy",
			rbac:       &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-a", "realmB")},
			wantStatus: http.StatusOK,
			wantScope:  reports.RealmScope{All: true},
		},
		{
			name:       "policy restricted to one realm covers only it",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			wantStatus: http.StatusOK,
			wantScope:  reports.RealmScope{Realms: []string{"realmB"}},
		},
		{
			name:       "policy with an empty realm list covers no realm",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-a")},
			wantStatus: http.StatusOK,
			wantScope:  reports.RealmScope{},
		},
		{
			name:       "a failed policy lookup refuses the report",
			rbac:       &mockRBACChecker{policiesErr: errors.New("policy lookup failed")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubReportService{}
			h := NewReportHandlers(svc, nil, tt.rbac, newTestLogger(), testAuthMiddleware, NewRBACMiddleware(tt.rbac, newTestLogger()))

			req := httptest.NewRequest(http.MethodGet, reportPath, nil)
			rec := httptest.NewRecorder()
			setupTestReportRouter(h).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				if svc.calls != 0 {
					t.Errorf("report aggregated %d times for a refused caller, want 0", svc.calls)
				}
				return
			}
			if svc.lastReq == nil {
				t.Fatal("report was never aggregated")
			}
			if !reflect.DeepEqual(svc.lastReq.Scope, tt.wantScope) {
				t.Errorf("scope = %+v, want %+v", svc.lastReq.Scope, tt.wantScope)
			}
		})
	}
}

func TestReportHandlers_Generate_RequiresAuthenticatedCaller(t *testing.T) {
	svc := &stubReportService{}
	h := NewReportHandlers(svc, nil, &mockRBACChecker{}, newTestLogger(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/reports/generate?start_date=2026-01-01&end_date=2026-01-31", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantID", "tenant-a")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.handleGenerateReport(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if svc.calls != 0 {
		t.Errorf("report aggregated %d times without an authenticated caller, want 0", svc.calls)
	}
}
