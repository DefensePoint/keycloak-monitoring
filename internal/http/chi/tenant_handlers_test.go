package chi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// stubTenantHandlerService is a minimal tenant.Service for handler tests.
// Each *Err field controls one method's error response so a test can
// exercise the config-defined-read-only path without a real repository.
type stubTenantHandlerService struct {
	updateErr error
	deleteErr error
}

func (s *stubTenantHandlerService) GetTenant(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
	return &domain.KeycloakTenant{TenantID: tenantID}, nil
}
func (s *stubTenantHandlerService) ListTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return nil, nil
}
func (s *stubTenantHandlerService) ListEnabledTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return nil, nil
}
func (s *stubTenantHandlerService) CreateTenant(ctx context.Context, req *tenant.CreateRequest) (*domain.KeycloakTenant, error) {
	return nil, nil
}
func (s *stubTenantHandlerService) UpdateTenant(ctx context.Context, tenantID string, req *tenant.UpdateRequest) (*domain.KeycloakTenant, error) {
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	return &domain.KeycloakTenant{TenantID: tenantID}, nil
}
func (s *stubTenantHandlerService) DeleteTenant(ctx context.Context, tenantID string) error {
	return s.deleteErr
}
func (s *stubTenantHandlerService) SyncFromConfig(ctx context.Context, tenantID string, createReq *tenant.CreateRequest, updateReq *tenant.UpdateRequest) (*domain.KeycloakTenant, error) {
	return nil, nil
}
func (s *stubTenantHandlerService) ListConfigDefinedTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return nil, nil
}
func (s *stubTenantHandlerService) UnmarkConfigDefined(ctx context.Context, tenantID string) error {
	return nil
}
func (s *stubTenantHandlerService) UpdateHealth(ctx context.Context, tenantID, status, message string) error {
	return nil
}
func (s *stubTenantHandlerService) LoadTenants(ctx context.Context) error { return nil }
func (s *stubTenantHandlerService) UpdateError(ctx context.Context, tenantID, errorMsg string) error {
	return nil
}
func (s *stubTenantHandlerService) GetHealth(ctx context.Context, tenantID string) (*tenant.HealthStatus, error) {
	return nil, nil
}
func (s *stubTenantHandlerService) RegisterCallback(callback tenant.ChangeCallback) {}
func (s *stubTenantHandlerService) GetDefault(ctx context.Context) (*domain.KeycloakTenant, error) {
	return nil, nil
}

// newTestTenantHandlers builds a TenantHandlers wired with the given stub
// service and no auth/RBAC middleware or connection tester — handlers are
// called directly so middleware is not exercised here.
func newTestTenantHandlers(svc tenant.Service) *TenantHandlers {
	return NewTenantHandlers(svc, nil, nil, newTestLogger(), nil, nil)
}

// newTenantRequest mimics a real Chi-routed request with a tenantID URL param.
func newTenantRequest(method, path, tenantID string, body *strings.Reader) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, body)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantID", tenantID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestTenantHandlers_UpdateTenant_RejectsConfigDefined(t *testing.T) {
	h := newTestTenantHandlers(&stubTenantHandlerService{updateErr: tenant.ErrConfigDefinedTenantReadOnly})

	body := strings.NewReader(`{"name":"New Name"}`)
	req := newTenantRequest(http.MethodPut, "/api/tenants/cfg-tenant", "cfg-tenant", body)
	rec := httptest.NewRecorder()
	h.updateTenant(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "config-defined") {
		t.Errorf("expected error to mention config-defined, got %s", rec.Body.String())
	}
}

func TestTenantHandlers_DeleteTenant_RejectsConfigDefined(t *testing.T) {
	h := newTestTenantHandlers(&stubTenantHandlerService{deleteErr: tenant.ErrConfigDefinedTenantReadOnly})

	req := newTenantRequest(http.MethodDelete, "/api/tenants/cfg-tenant", "cfg-tenant", nil)
	rec := httptest.NewRecorder()
	h.deleteTenant(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "config-defined") {
		t.Errorf("expected error to mention config-defined, got %s", rec.Body.String())
	}
}

func TestTenantHandlers_UpdateTenant_AllowsNonConfigDefined(t *testing.T) {
	h := newTestTenantHandlers(&stubTenantHandlerService{})

	body := strings.NewReader(`{"name":"New Name"}`)
	req := newTenantRequest(http.MethodPut, "/api/tenants/ui-tenant", "ui-tenant", body)
	rec := httptest.NewRecorder()
	h.updateTenant(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestTenantHandlers_DeleteTenant_AllowsNonConfigDefined(t *testing.T) {
	h := newTestTenantHandlers(&stubTenantHandlerService{})

	req := newTenantRequest(http.MethodDelete, "/api/tenants/ui-tenant", "ui-tenant", nil)
	rec := httptest.NewRecorder()
	h.deleteTenant(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
