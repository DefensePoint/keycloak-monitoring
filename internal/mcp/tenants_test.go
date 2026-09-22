package mcp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DefensePoint/keycloak-monitoring/internal/apitoken"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

func tokenFor(tenantIDs []string) *mockTokenValidator {
	return &mockTokenValidator{
		validateFn: func(_ context.Context, plaintext string) (*apitoken.Identity, error) {
			if plaintext == "pat_good" {
				return &apitoken.Identity{
					User:      &domain.User{ID: 7, Subject: "user-7", IsActive: true},
					TenantIDs: tenantIDs,
				}, nil
			}
			return nil, apitoken.ErrTokenNotFound
		},
	}
}

func globalRole() []*domain.UserRole {
	return []*domain.UserRole{{RoleID: 1, TenantID: nil}}
}

func tenantScopedRoles(tenantIDs ...string) []*domain.UserRole {
	roles := make([]*domain.UserRole, len(tenantIDs))
	for i := range tenantIDs {
		roles[i] = &domain.UserRole{RoleID: 1, TenantID: &tenantIDs[i]}
	}
	return roles
}

func permsWithRoles(roles []*domain.UserRole) *mockPermissionService {
	return &mockPermissionService{
		hasPermissionFn: func(context.Context, uint, string, *string) (bool, error) {
			return true, nil
		},
		getUserRolesFn: func(context.Context, uint) ([]*domain.UserRole, error) {
			return roles, nil
		},
	}
}

func enabledTenantsLister(tenants ...*domain.KeycloakTenant) *mockTenantReader {
	return &mockTenantReader{
		listEnabledFn: func(context.Context) ([]*domain.KeycloakTenant, error) {
			return tenants, nil
		},
	}
}

func callToolResult(t *testing.T, ts *httptest.Server, name string, args map[string]any) *mcpsdk.CallToolResult {
	t.Helper()
	session := connectClient(t, ts, "pat_good")
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s call failed: %v", name, err)
	}
	return res
}

func callToolOK(t *testing.T, ts *httptest.Server, name string, args map[string]any, out any) {
	t.Helper()
	res := callToolResult(t, ts, name, args)
	if res.IsError {
		t.Fatalf("%s returned tool error: %+v", name, res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("unmarshal %s output: %v", name, err)
	}
}

func callToolErrText(t *testing.T, ts *httptest.Server, name string, args map[string]any) string {
	t.Helper()
	return errTextOf(t, callToolResult(t, ts, name, args), name)
}

func tenantIDsOf(out listTenantsOutput) []string {
	ids := make([]string, len(out.Tenants))
	for i, t := range out.Tenants {
		ids[i] = t.TenantID
	}
	return ids
}

func healthyTenant(tenantID string) *domain.KeycloakTenant {
	return &domain.KeycloakTenant{TenantID: tenantID, Enabled: true, HealthStatus: "healthy"}
}

func TestListTenantsAllowlistNarrowerThanRBACWins(t *testing.T) {
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a"}),
		permsWithRoles(globalRole()),
		enabledTenantsLister(enabledTenant("tenant-a"), enabledTenant("tenant-b")),
		&mockRealmReader{})

	var out listTenantsOutput
	callToolOK(t, ts, "list_tenants", nil, &out)

	if ids := tenantIDsOf(out); len(ids) != 1 || ids[0] != "tenant-a" {
		t.Fatalf("tenants = %v, want [tenant-a]", ids)
	}
}

func TestListTenantsWithoutAllowlistDenied(t *testing.T) {
	ts := newTestServerWith(t,
		tokenFor(nil),
		permsWithRoles(tenantScopedRoles("tenant-b")),
		enabledTenantsLister(enabledTenant("tenant-a"), enabledTenant("tenant-b")),
		&mockRealmReader{})

	if text := callToolErrText(t, ts, "list_tenants", nil); text != ErrForbidden.Error() {
		t.Fatalf("error = %q, want %q: an unscoped token grants nothing", text, ErrForbidden.Error())
	}
}

func TestListTenantsSingleTenantCustomerToken(t *testing.T) {
	customer := &domain.KeycloakTenant{
		TenantID:     "tenant-a",
		Name:         "Customer A",
		Enabled:      true,
		HealthStatus: "healthy",
	}
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a"}),
		permsWithRoles(tenantScopedRoles("tenant-a")),
		enabledTenantsLister(customer, enabledTenant("tenant-b")),
		&mockRealmReader{})

	var out listTenantsOutput
	callToolOK(t, ts, "list_tenants", nil, &out)

	if len(out.Tenants) != 1 {
		t.Fatalf("tenants = %+v, want exactly one", out.Tenants)
	}
	got := out.Tenants[0]
	if got.TenantID != "tenant-a" || got.Name != "Customer A" || got.HealthStatus != "healthy" {
		t.Fatalf("tenant = %+v, want tenant-a / Customer A / healthy", got)
	}
}

func TestListTenantsDeniedWithoutPermission(t *testing.T) {
	perms := permsWithRoles(globalRole())
	perms.hasPermissionFn = func(context.Context, uint, string, *string) (bool, error) {
		return false, nil
	}
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a"}),
		perms,
		enabledTenantsLister(enabledTenant("tenant-a")),
		&mockRealmReader{})

	if text := callToolErrText(t, ts, "list_tenants", nil); text != ErrForbidden.Error() {
		t.Fatalf("error = %q, want %q", text, ErrForbidden.Error())
	}
}

func TestListTenantsAdminWithoutPermissionSeesAll(t *testing.T) {
	perms := &mockPermissionService{
		hasPermissionFn: func(context.Context, uint, string, *string) (bool, error) {
			return false, nil
		},
		isAdminFn: func(context.Context, uint) (bool, error) {
			return true, nil
		},
	}
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		perms,
		enabledTenantsLister(enabledTenant("tenant-a"), enabledTenant("tenant-b")),
		&mockRealmReader{})

	var out listTenantsOutput
	callToolOK(t, ts, "list_tenants", nil, &out)

	if ids := tenantIDsOf(out); !slices.Equal(ids, []string{"tenant-a", "tenant-b"}) {
		t.Fatalf("tenants = %v, want [tenant-a tenant-b]", ids)
	}
}

func TestSingleTenantTokenHasItsTenantInjected(t *testing.T) {
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a"}),
		permsWithRoles(globalRole()),
		readerFor(healthyTenant("tenant-a")),
		&mockRealmReader{})

	var out tenantHealthOutput
	callToolOK(t, ts, "get_tenant_health", nil, &out)
	if out.HealthStatus != "healthy" {
		t.Fatalf("health_status = %q, want healthy", out.HealthStatus)
	}
}

func TestSingleTenantTokenRejectsASuppliedTenant(t *testing.T) {
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a"}),
		permsWithRoles(globalRole()),
		readerFor(enabledTenant("tenant-a")),
		&mockRealmReader{})

	// Rejected even when it names the token's own tenant: the argument is
	// not the caller's to choose.
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		text := callToolErrText(t, ts, "get_tenant_health", map[string]any{"tenant": tenant})
		if !strings.Contains(text, "tenant must be omitted") {
			t.Fatalf("error for %q = %q, want the tenant-must-be-omitted message", tenant, text)
		}
	}
}

func TestMultiTenantTokenRequiresTheTenantArgument(t *testing.T) {
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		permsWithRoles(globalRole()),
		readerFor(enabledTenant("tenant-a")),
		&mockRealmReader{})

	text := callToolErrText(t, ts, "get_tenant_health", nil)
	if !strings.Contains(text, "tenant is required") {
		t.Fatalf("error = %q, want the tenant-is-required message", text)
	}
}

func TestMultiTenantTokenConstrainedToItsAllowlist(t *testing.T) {
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		permsWithRoles(globalRole()),
		readerFor(healthyTenant("tenant-a")),
		&mockRealmReader{})

	var out tenantHealthOutput
	callToolOK(t, ts, "get_tenant_health", map[string]any{"tenant": "tenant-a"}, &out)
	if out.HealthStatus != "healthy" {
		t.Fatalf("health_status = %q, want healthy", out.HealthStatus)
	}

	if text := callToolErrText(t, ts, "get_tenant_health",
		map[string]any{"tenant": "tenant-c"}); text != ErrForbidden.Error() {
		t.Fatalf("error = %q, want %q for a tenant outside the allowlist", text, ErrForbidden.Error())
	}
}

func TestGetTenantHealthDeniedForDisabledTenant(t *testing.T) {
	disabled := &domain.KeycloakTenant{TenantID: "tenant-a", Enabled: false}
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a"}),
		permsWithRoles(globalRole()),
		readerFor(disabled),
		&mockRealmReader{})

	text := callToolErrText(t, ts, "get_tenant_health", nil)
	if text != ErrTenantNotAvailable.Error() {
		t.Fatalf("error = %q, want %q", text, ErrTenantNotAvailable.Error())
	}
}

func TestGetTenantHealthDTOOmitsInternalFields(t *testing.T) {
	lastCheck := time.Date(2026, 9, 1, 10, 30, 0, 0, time.FixedZone("CET", 3600))
	tenant := &domain.KeycloakTenant{
		TenantID:        "tenant-a",
		Name:            "Customer A",
		Enabled:         true,
		HealthStatus:    "unhealthy",
		HealthMessage:   "connection refused to 10.0.0.5:8443",
		LastError:       "pq: password authentication failed",
		ServerURL:       "https://keycloak.internal:8443",
		LastHealthCheck: lastCheck,
	}
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a"}),
		permsWithRoles(globalRole()),
		readerFor(tenant),
		&mockRealmReader{})

	res := callToolResult(t, ts, "get_tenant_health", nil)
	if res.IsError {
		t.Fatalf("get_tenant_health returned tool error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}

	for _, leaked := range []string{"health_message", "last_error", "server_url", "10.0.0.5", "pq:", "keycloak.internal"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("health DTO leaks %q: %s", leaked, raw)
		}
	}

	var out tenantHealthOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal health output: %v", err)
	}
	if out.HealthStatus != "unhealthy" {
		t.Errorf("health_status = %q, want unhealthy", out.HealthStatus)
	}
	if out.LastHealthCheck != "2026-09-01T09:30:00Z" {
		t.Errorf("last_health_check = %q, want 2026-09-01T09:30:00Z", out.LastHealthCheck)
	}
}
