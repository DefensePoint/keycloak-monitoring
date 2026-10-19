package chi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/internal/reports"
)

type captureReports struct {
	got *reports.GenerateRequest
}

func (c *captureReports) AggregateData(ctx context.Context, req *reports.GenerateRequest) (*reports.Data, error) {
	c.got = req
	return &reports.Data{}, nil
}

func (c *captureReports) GeneratePDF(data *reports.Data) ([]byte, error) {
	return []byte("%PDF-test"), nil
}

// The report used to need only tenant access, so a caller without
// alerts:read or metrics:read still got alerts and operator metrics in it.
func TestGenerateReport_AsksOnlyForPermittedSections(t *testing.T) {
	for _, tc := range []struct {
		name     string
		granted  []string
		status   int
		wantSent reports.SectionAccess
	}{
		{"no read permission", nil, http.StatusForbidden, reports.SectionAccess{}},
		{"only keycloak:read", []string{rbac.PermissionKeycloakRead}, http.StatusForbidden, reports.SectionAccess{}},
		{"alerts:read", []string{rbac.PermissionAlertsRead}, http.StatusOK, reports.SectionAccess{Alerts: true}},
		{"metrics:read", []string{rbac.PermissionMetricsRead}, http.StatusOK, reports.SectionAccess{Operators: true}},
		{"all three", []string{rbac.PermissionAlertsRead, rbac.PermissionKeycloakRead, rbac.PermissionMetricsRead}, http.StatusOK,
			reports.SectionAccess{Alerts: true, Events: true, Operators: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// tenants:read is what RequireTenantAccess itself checks; every
			// case here has tenant access.
			granted := map[string]bool{rbac.PermissionTenantsRead: true}
			for _, p := range tc.granted {
				granted[p] = true
			}
			checker := &mockRBACChecker{
				isAdmin: true, // realm scope: all realms; permissions come from hasPermissionFn
				hasPermissionFn: func(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
					return granted[permission], nil
				},
			}
			log := logger.NewNoop()
			svc := &captureReports{}
			auth := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					ctx := context.WithValue(r.Context(), UserInfoKey, &UserDetails{ID: 5})
					// The real router puts the tenant in context before this group.
					ctx = context.WithValue(ctx, TenantIDKey, "t1")
					next.ServeHTTP(w, r.WithContext(ctx))
				})
			}
			h := NewReportHandlers(svc, nil, checker, log, auth, NewRBACMiddleware(checker, log))
			r := chi.NewRouter()
			r.Route("/api/tenants/{tenantID}", h.RegisterTenantRoutes)

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tenants/t1/reports/generate?start_date=2026-01-01&end_date=2026-01-31", nil))
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tc.status, rec.Body.String())
			}
			if tc.status != http.StatusOK {
				if svc.got != nil {
					t.Fatal("report data was gathered for a refused caller")
				}
				return
			}
			if svc.got == nil || svc.got.Access != tc.wantSent {
				t.Fatalf("access sent = %+v, want %+v", svc.got, tc.wantSent)
			}
		})
	}
}
