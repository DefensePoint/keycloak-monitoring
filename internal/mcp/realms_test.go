package mcp

import (
	"context"
	"slices"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
)

// mockRealmReader implements RealmReader for testing.
type mockRealmReader struct {
	getRealmsFn func(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error)
}

func (m *mockRealmReader) GetRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error) {
	if m.getRealmsFn != nil {
		return m.getRealmsFn(ctx, tenantID)
	}
	return nil, nil
}

func realmsNamed(names ...string) *mockRealmReader {
	return &mockRealmReader{
		getRealmsFn: func(_ context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error) {
			rows := make([]*domain.KeycloakRealmInfo, len(names))
			for i, name := range names {
				rows[i] = &domain.KeycloakRealmInfo{TenantID: tenantID, RealmName: name}
			}
			return rows, nil
		},
	}
}

func permsWithPolicies(policies []*domain.TenantPolicy) *mockPermissionService {
	perms := permsWithRoles(globalRole())
	perms.getUserPoliciesFn = func(context.Context, uint) ([]*domain.TenantPolicy, error) {
		return policies, nil
	}
	return perms
}

func TestListRealmsPolicyRestrictsToListedRealms(t *testing.T) {
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod"}}}
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		permsWithPolicies(policies),
		readerFor(enabledTenant("tenant-a")),
		realmsNamed("dev", "prod", "staging"))

	var out listRealmsOutput
	callToolOK(t, ts, "list_realms", map[string]any{"tenant": "tenant-a"}, &out)

	if !slices.Equal(out.Realms, []string{"prod"}) {
		t.Fatalf("realms = %v, want [prod]", out.Realms)
	}
}

func TestListRealmsEmptyPolicyRowDenied(t *testing.T) {
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{}}}
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		permsWithPolicies(policies),
		readerFor(enabledTenant("tenant-a")),
		realmsNamed("dev", "prod"))

	text := callToolErrText(t, ts, "list_realms", map[string]any{"tenant": "tenant-a"})
	if text != ErrForbidden.Error() {
		t.Fatalf("error = %q, want %q", text, ErrForbidden.Error())
	}
}

func TestListRealmsNoPolicyRowSeesAllRealms(t *testing.T) {
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		permsWithPolicies(nil),
		readerFor(enabledTenant("tenant-a")),
		realmsNamed("dev", "prod", "staging"))

	var out listRealmsOutput
	callToolOK(t, ts, "list_realms", map[string]any{"tenant": "tenant-a"}, &out)

	if !slices.Equal(out.Realms, []string{"dev", "prod", "staging"}) {
		t.Fatalf("realms = %v, want [dev prod staging]", out.Realms)
	}
}

func TestListRealmsDeniedWithoutTenantsRead(t *testing.T) {
	perms := permsWithPolicies(nil)
	perms.hasPermissionFn = func(_ context.Context, _ uint, permission string, _ *string) (bool, error) {
		return permission == rbac.PermissionKeycloakRead, nil
	}
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		perms,
		readerFor(enabledTenant("tenant-a")),
		realmsNamed("dev", "prod"))

	text := callToolErrText(t, ts, "list_realms", map[string]any{"tenant": "tenant-a"})
	if text != ErrForbidden.Error() {
		t.Fatalf("error = %q, want %q", text, ErrForbidden.Error())
	}
}

func TestListRealmsDeniedForDisabledTenant(t *testing.T) {
	disabled := &domain.KeycloakTenant{TenantID: "tenant-a", Enabled: false}
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		permsWithPolicies(nil),
		readerFor(disabled),
		realmsNamed("dev", "prod"))

	text := callToolErrText(t, ts, "list_realms", map[string]any{"tenant": "tenant-a"})
	if text != ErrTenantNotAvailable.Error() {
		t.Fatalf("error = %q, want %q", text, ErrTenantNotAvailable.Error())
	}
}

func TestListRealmsSkipsNilAndUnnamedRows(t *testing.T) {
	realms := &mockRealmReader{
		getRealmsFn: func(_ context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error) {
			return []*domain.KeycloakRealmInfo{
				nil,
				{TenantID: tenantID, RealmName: ""},
				{TenantID: tenantID, RealmName: "prod"},
			}, nil
		},
	}
	ts := newTestServerWith(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		permsWithPolicies(nil),
		readerFor(enabledTenant("tenant-a")),
		realms)

	var out listRealmsOutput
	callToolOK(t, ts, "list_realms", map[string]any{"tenant": "tenant-a"}, &out)

	if !slices.Equal(out.Realms, []string{"prod"}) {
		t.Fatalf("realms = %v, want [prod]", out.Realms)
	}
}
