package fx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/internal/reports"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// The handler tests set the branding themselves; these build the handlers
// the way the app does, so dropping the line that hands them the config's
// branding fails here.

var wiredBranding = config.BrandingConfig{
	Name: "Acme Monitoring Platform", ShortName: "ACME", LogoText: "ACME",
	Vendor: "Acme", ContactEmail: "security@acme.example",
}

func TestProvideAuthHandlers_PublishesTheConfiguredBranding(t *testing.T) {
	log := logger.NewNoop()
	h := provideAuthHandlers(AuthHandlersParams{
		Config:         &config.AppConfig{Branding: wiredBranding},
		Logger:         log,
		AuthMiddleware: chihttp.NewAuthMiddleware(nil, nil, nil, nil, log),
	})
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/config", nil))
	var body struct {
		Branding map[string]any `json:"branding"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if got := body.Branding["name"]; got != wiredBranding.Name {
		t.Errorf("/auth/config branding name = %v, want %q; body %s", got, wiredBranding.Name, rec.Body.String())
	}
}

// bearerAuthService accepts any bearer token as user 1.
type bearerAuthService struct{ auth.Service }

func (bearerAuthService) ValidateSession(context.Context, string) (*auth.UserInfo, error) {
	return &auth.UserInfo{ID: 1, Subject: "user-1"}, nil
}

// adminRBAC lets user 1 do anything.
type adminRBAC struct{ rbac.Service }

func (adminRBAC) IsAdmin(context.Context, uint) (bool, error) { return true, nil }
func (adminRBAC) HasPermission(context.Context, uint, string, *string) (bool, error) {
	return true, nil
}
func (adminRBAC) HasAccessToTenant(context.Context, uint, string) (bool, error) { return true, nil }

type namedTenants struct{ tenant.Service }

func (namedTenants) GetTenant(_ context.Context, id string) (*domain.KeycloakTenant, error) {
	return &domain.KeycloakTenant{TenantID: id, Name: "tenant-a"}, nil
}

// recordingReports keeps the data the PDF was rendered from.
type recordingReports struct{ rendered *reports.Data }

func (s *recordingReports) AggregateData(_ context.Context, req *reports.GenerateRequest) (*reports.Data, error) {
	return &reports.Data{TenantID: req.TenantID, TenantName: req.TenantName}, nil
}

func (s *recordingReports) GeneratePDF(d *reports.Data) ([]byte, error) {
	s.rendered = d
	return []byte("%PDF-1.4"), nil
}

func TestProvideReportHandlers_NamesTheReportAfterTheConfiguredBranding(t *testing.T) {
	log := logger.NewNoop()
	svc := &recordingReports{}
	h := provideReportHandlers(ReportHandlersParams{
		Service:        svc,
		TenantService:  namedTenants{},
		RBACService:    adminRBAC{},
		Logger:         log,
		AuthMiddleware: chihttp.NewAuthMiddleware(nil, bearerAuthService{}, nil, nil, log),
		RBACMiddleware: provideRBACMiddleware(adminRBAC{}, log),
		Config:         &config.AppConfig{Branding: wiredBranding},
	})
	r := chi.NewRouter()
	r.Route("/api/tenants/{tenantID}", func(r chi.Router) {
		r.Use(chihttp.TenantIDMiddleware)
		h.RegisterTenantRoutes(r)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/reports/generate?start_date=2026-01-01&end_date=2026-01-31", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}

	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, `filename="acme-report-`) {
		t.Errorf("Content-Disposition = %q, want an acme-report- file name", got)
	}
	if svc.rendered == nil || svc.rendered.ProductName != wiredBranding.Name || svc.rendered.Vendor != "Acme" {
		t.Errorf("PDF rendered from %+v, want the configured product and vendor", svc.rendered)
	}
}
