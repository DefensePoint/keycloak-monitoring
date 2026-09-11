package rbac

import (
	"context"
	"errors"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

type mockVisibilityChecker struct {
	isAdminFn      func(ctx context.Context, userID uint) (bool, error)
	getUserRolesFn func(ctx context.Context, userID uint) ([]*domain.UserRole, error)
}

func (m *mockVisibilityChecker) IsAdmin(ctx context.Context, userID uint) (bool, error) {
	if m.isAdminFn != nil {
		return m.isAdminFn(ctx, userID)
	}
	return false, nil
}

func (m *mockVisibilityChecker) GetUserRoles(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
	if m.getUserRolesFn != nil {
		return m.getUserRolesFn(ctx, userID)
	}
	return nil, nil
}

func visibilityTenants(ids ...string) []*domain.KeycloakTenant {
	tenants := make([]*domain.KeycloakTenant, len(ids))
	for i, id := range ids {
		tenants[i] = &domain.KeycloakTenant{TenantID: id}
	}
	return tenants
}

func tenantIDs(tenants []*domain.KeycloakTenant) []string {
	ids := make([]string, len(tenants))
	for i, t := range tenants {
		ids[i] = t.TenantID
	}
	return ids
}

func TestFilterTenantsByUserRolesAdminSeesAll(t *testing.T) {
	checker := &mockVisibilityChecker{
		isAdminFn: func(context.Context, uint) (bool, error) { return true, nil },
	}

	got := FilterTenantsByUserRoles(context.Background(), checker, 7, visibilityTenants("a", "b"), newTestLogger())
	if ids := tenantIDs(got); len(ids) != 2 {
		t.Fatalf("tenants = %v, want all", ids)
	}
}

func TestFilterTenantsByUserRolesGlobalRoleSeesAll(t *testing.T) {
	checker := &mockVisibilityChecker{
		getUserRolesFn: func(context.Context, uint) ([]*domain.UserRole, error) {
			return []*domain.UserRole{{RoleID: 1, TenantID: nil}}, nil
		},
	}

	got := FilterTenantsByUserRoles(context.Background(), checker, 7, visibilityTenants("a", "b"), newTestLogger())
	if ids := tenantIDs(got); len(ids) != 2 {
		t.Fatalf("tenants = %v, want all", ids)
	}
}

func TestFilterTenantsByUserRolesScopedRolesSeeSubset(t *testing.T) {
	tenantB := "b"
	checker := &mockVisibilityChecker{
		getUserRolesFn: func(context.Context, uint) ([]*domain.UserRole, error) {
			return []*domain.UserRole{{RoleID: 1, TenantID: &tenantB}}, nil
		},
	}

	got := FilterTenantsByUserRoles(context.Background(), checker, 7, visibilityTenants("a", "b", "c"), newTestLogger())
	if ids := tenantIDs(got); len(ids) != 1 || ids[0] != "b" {
		t.Fatalf("tenants = %v, want [b]", ids)
	}
}

func TestFilterTenantsByUserRolesFailsClosedOnRoleError(t *testing.T) {
	checker := &mockVisibilityChecker{
		getUserRolesFn: func(context.Context, uint) ([]*domain.UserRole, error) {
			return nil, errors.New("db down")
		},
	}

	got := FilterTenantsByUserRoles(context.Background(), checker, 7, visibilityTenants("a", "b"), newTestLogger())
	if len(got) != 0 {
		t.Fatalf("tenants = %v, want empty on error", tenantIDs(got))
	}
}

func TestFilterTenantsByUserRolesFailsClosedOnNilChecker(t *testing.T) {
	got := FilterTenantsByUserRoles(context.Background(), nil, 7, visibilityTenants("a"), newTestLogger())
	if len(got) != 0 {
		t.Fatalf("tenants = %v, want empty without checker", tenantIDs(got))
	}
}

func TestFilterTenantsByUserRolesNoRolesSeesNothing(t *testing.T) {
	got := FilterTenantsByUserRoles(context.Background(), &mockVisibilityChecker{}, 7, visibilityTenants("a", "b"), newTestLogger())
	if len(got) != 0 {
		t.Fatalf("tenants = %v, want empty without roles", tenantIDs(got))
	}
}
