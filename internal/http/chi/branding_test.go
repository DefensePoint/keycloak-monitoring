package chi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/reports"
)

var acmeBranding = config.BrandingConfig{Name: "Acme Monitoring Platform", ShortName: "ACME", LogoText: "ACME"}

// pdfRecordingReportService keeps the data the PDF was rendered from.
type pdfRecordingReportService struct{ rendered *reports.Data }

func (s *pdfRecordingReportService) AggregateData(_ context.Context, req *reports.GenerateRequest) (*reports.Data, error) {
	return &reports.Data{TenantID: req.TenantID, TenantName: req.TenantName}, nil
}

func (s *pdfRecordingReportService) GeneratePDF(d *reports.Data) ([]byte, error) {
	s.rendered = d
	return []byte("%PDF-1.4"), nil
}

func generateReport(t *testing.T, branding *config.BrandingConfig) (*httptest.ResponseRecorder, *reports.Data) {
	t.Helper()
	svc := &pdfRecordingReportService{}
	rbac := &mockRBACChecker{}
	h := NewReportHandlers(svc, nil, rbac, newTestLogger(), testAuthMiddleware, NewRBACMiddleware(rbac, newTestLogger()))
	if branding != nil {
		h.SetBranding(*branding)
	}
	rec := httptest.NewRecorder()
	setupTestReportRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, reportPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}
	return rec, svc.rendered
}

func TestReportHandlers_Generate_NamesTheReportAfterTheBranding(t *testing.T) {
	rec, rendered := generateReport(t, &acmeBranding)

	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, `attachment; filename="acme-report-`) {
		t.Errorf("Content-Disposition = %q, want an acme-report- file name", got)
	}
	if rendered == nil || rendered.ProductName != acmeBranding.Name {
		t.Errorf("PDF rendered with product %+v, want %q", rendered, acmeBranding.Name)
	}
}

func TestReportHandlers_Generate_WithoutBrandingKeepsTheDefaultName(t *testing.T) {
	rec, rendered := generateReport(t, nil)

	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, `attachment; filename="kmt-report-`) {
		t.Errorf("Content-Disposition = %q, want a kmt-report- file name", got)
	}
	if rendered == nil || rendered.ProductName != config.DefaultBrandingName {
		t.Errorf("PDF rendered with product %+v, want the default name", rendered)
	}
}

func authConfigBranding(t *testing.T, h *AuthHandlers) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	h.handleAuthConfig(rec, httptest.NewRequest(http.MethodGet, "/auth/config", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if data, ok := body["data"].(map[string]any); ok {
		body = data
	}
	branding, ok := body["branding"].(map[string]any)
	if !ok {
		t.Fatalf("no branding in %s", rec.Body.String())
	}
	return branding
}

func TestAuthConfig_PublishesTheBranding(t *testing.T) {
	h := NewAuthHandlers(nil, nil, nil, nil, nil, "kmt", logger.NewNoop(), true, false, nil, nil)
	h.SetBranding(acmeBranding)

	got := authConfigBranding(t, h)
	want := map[string]any{"name": "Acme Monitoring Platform", "short_name": "ACME", "logo_text": "ACME"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("branding[%q] = %v, want %v", k, got[k], v)
		}
	}
}

func TestAuthConfig_WithoutBrandingPublishesTheDefaults(t *testing.T) {
	h := NewAuthHandlers(nil, nil, nil, nil, nil, "kmt", logger.NewNoop(), true, false, nil, nil)

	got := authConfigBranding(t, h)
	if got["name"] != "Keycloak Monitoring Tool" || got["short_name"] != "KMT" || got["logo_text"] != "Keycloak" {
		t.Errorf("branding = %v, want the defaults", got)
	}
}
