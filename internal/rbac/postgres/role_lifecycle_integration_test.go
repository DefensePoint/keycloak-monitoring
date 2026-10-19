// A deleted or deactivated custom role must stop granting its permissions,
// while the user's other roles keep working.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset. Only rows with a rolelife- prefix are touched.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

const roleLifeTenant = "rolelife-tenant-a"

type roleLifeFixture struct {
	db      *gorm.DB
	roles   rbac.RoleRepository
	svc     rbac.Service
	user    uint
	revoked lifeRole
	kept    lifeRole
}

type lifeRole struct {
	id         uint
	permission string
}

func newRoleLifeFixture(t *testing.T, tenantID *string) *roleLifeFixture {
	t.Helper()

	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	f := &roleLifeFixture{
		db:    client.DB(),
		roles: NewRoleRepository(client),
		svc: rbac.NewService(NewRoleRepository(client), NewPermissionRepository(client),
			NewUserRoleRepository(client), NewTenantPolicyRepository(client), logger.NewNoop()),
	}
	stamp := time.Now().UnixNano()
	f.revoked = f.seedRole(t, fmt.Sprintf("rolelife-%d-revoked", stamp))
	f.kept = f.seedRole(t, fmt.Sprintf("rolelife-%d-kept", stamp))

	subject := fmt.Sprintf("rolelife-%d", stamp)
	if err := f.db.Raw(`INSERT INTO users (subject, email, name, username, auth_method, is_active, is_blocked, login_count, created_at, updated_at)
		VALUES (?, ?, 'test', ?, 'simple', true, false, 0, NOW(), NOW()) RETURNING id`,
		subject, subject+"@example.invalid", subject).Scan(&f.user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		f.db.Exec(`DELETE FROM user_roles WHERE user_id = ?`, f.user)
		f.db.Exec(`DELETE FROM users WHERE id = ?`, f.user)
	})
	for _, role := range []lifeRole{f.revoked, f.kept} {
		if err := f.svc.AssignRoleToUser(context.Background(), f.user, role.id, tenantID, "test", nil); err != nil {
			t.Fatalf("assign role: %v", err)
		}
	}
	return f
}

func (f *roleLifeFixture) seedRole(t *testing.T, name string) lifeRole {
	t.Helper()

	role := lifeRole{permission: name + ":write"}
	var permID uint
	if err := f.db.Raw(`INSERT INTO permissions (name, display_name, resource, action, created_at, updated_at)
		VALUES (?, 'test', 'rolelife', 'write', NOW(), NOW()) RETURNING id`, role.permission).Scan(&permID).Error; err != nil {
		t.Fatalf("seed permission: %v", err)
	}
	if err := f.db.Raw(`INSERT INTO roles (name, display_name, is_active, created_at, updated_at)
		VALUES (?, 'test', true, NOW(), NOW()) RETURNING id`, name).Scan(&role.id).Error; err != nil {
		t.Fatalf("seed role: %v", err)
	}
	if err := f.db.Exec(`INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)`, role.id, permID).Error; err != nil {
		t.Fatalf("seed role permission: %v", err)
	}
	t.Cleanup(func() {
		f.db.Exec(`DELETE FROM user_roles WHERE role_id = ?`, role.id)
		f.db.Exec(`DELETE FROM role_permissions WHERE role_id = ?`, role.id)
		f.db.Exec(`DELETE FROM roles WHERE id = ?`, role.id)
		f.db.Exec(`DELETE FROM permissions WHERE id = ?`, permID)
	})
	return role
}

func (f *roleLifeFixture) setActive(t *testing.T, roleID uint, active bool) {
	t.Helper()

	role, err := f.roles.GetRoleByID(context.Background(), roleID)
	if err != nil || role == nil {
		t.Fatalf("GetRoleByID(%d): %v, %v", roleID, role, err)
	}
	role.IsActive = active
	if err := f.roles.UpdateRole(context.Background(), role); err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
}

// grants reports whether each of the user's permission checks passes for
// permission: HasPermission for tenantID, GetUserPermissions for tenantID,
// and, for a global assignment, HasGlobalPermission.
func (f *roleLifeFixture) grants(t *testing.T, permission string, tenantID *string) bool {
	t.Helper()
	ctx := context.Background()

	has, err := f.svc.HasPermission(ctx, f.user, permission, tenantID)
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	perms, err := f.svc.GetUserPermissions(ctx, f.user, tenantID)
	if err != nil {
		t.Fatalf("GetUserPermissions: %v", err)
	}
	listed := false
	for _, p := range perms {
		listed = listed || p == permission
	}
	if listed != has {
		t.Fatalf("%s: HasPermission = %v but GetUserPermissions lists it = %v", permission, has, listed)
	}
	if tenantID == nil {
		global, err := f.svc.HasGlobalPermission(ctx, f.user, permission)
		if err != nil {
			t.Fatalf("HasGlobalPermission: %v", err)
		}
		if global != has {
			t.Fatalf("%s: HasPermission = %v but HasGlobalPermission = %v", permission, has, global)
		}
	}
	return has
}

func (f *roleLifeFixture) visibleTenants(t *testing.T) []string {
	t.Helper()

	tenants := []*domain.KeycloakTenant{{TenantID: roleLifeTenant}, {TenantID: "rolelife-tenant-b"}}
	var ids []string
	for _, tenant := range rbac.FilterTenantsByUserRoles(context.Background(), f.svc, f.user, tenants, logger.NewNoop()) {
		ids = append(ids, tenant.TenantID)
	}
	return ids
}

func (f *roleLifeFixture) assignedRoleIDs(t *testing.T) map[uint]bool {
	t.Helper()

	var ids []uint
	if err := f.db.Raw(`SELECT role_id FROM user_roles WHERE user_id = ?`, f.user).Scan(&ids).Error; err != nil {
		t.Fatalf("list assignments: %v", err)
	}
	set := make(map[uint]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

func expectGrant(t *testing.T, f *roleLifeFixture, tenantID *string, wantRevoked bool) {
	t.Helper()
	if got := f.grants(t, f.revoked.permission, tenantID); got != wantRevoked {
		t.Fatalf("revoked role's permission granted = %v, want %v", got, wantRevoked)
	}
	if !f.grants(t, f.kept.permission, tenantID) {
		t.Fatal("permission from the user's other, active role was lost")
	}
}

func TestRoleLifecycle_DeactivateAndReactivate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tenantID *string
	}{
		{name: "global assignment"},
		{name: "tenant assignment", tenantID: func() *string { s := roleLifeTenant; return &s }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRoleLifeFixture(t, tc.tenantID)
			expectGrant(t, f, tc.tenantID, true)

			f.setActive(t, f.revoked.id, false)
			expectGrant(t, f, tc.tenantID, false)

			f.setActive(t, f.revoked.id, true)
			expectGrant(t, f, tc.tenantID, true)
		})
	}
}

func TestRoleLifecycle_DeleteRevokesAndRemovesAssignments(t *testing.T) {
	f := newRoleLifeFixture(t, nil)
	expectGrant(t, f, nil, true)

	if err := f.svc.DeleteRole(context.Background(), f.revoked.id); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
	expectGrant(t, f, nil, false)

	assigned := f.assignedRoleIDs(t)
	if assigned[f.revoked.id] {
		t.Fatal("the deleted role is still assigned to the user")
	}
	if !assigned[f.kept.id] {
		t.Fatal("deleting one role removed the user's other assignment")
	}
}

// Assignments left behind by a role deleted before this fix must not grant
// anything either.
func TestRoleLifecycle_LeftoverAssignmentToDeletedRole(t *testing.T) {
	f := newRoleLifeFixture(t, nil)
	if err := f.db.Exec(`UPDATE roles SET deleted_at = NOW() WHERE id = ?`, f.revoked.id).Error; err != nil {
		t.Fatalf("soft-delete role: %v", err)
	}
	if !f.assignedRoleIDs(t)[f.revoked.id] {
		t.Fatal("precondition: the leftover assignment should still exist")
	}
	expectGrant(t, f, nil, false)
}

func TestRoleLifecycle_TenantVisibility(t *testing.T) {
	tenant := roleLifeTenant
	f := newRoleLifeFixture(t, &tenant)
	if err := f.svc.RemoveRoleFromUser(context.Background(), f.user, f.kept.id, &tenant); err != nil {
		t.Fatalf("RemoveRoleFromUser: %v", err)
	}
	if got := f.visibleTenants(t); len(got) != 1 || got[0] != roleLifeTenant {
		t.Fatalf("visible tenants = %v, want [%s]", got, roleLifeTenant)
	}

	f.setActive(t, f.revoked.id, false)
	if got := f.visibleTenants(t); len(got) != 0 {
		t.Fatalf("visible tenants with only an inactive role = %v, want none", got)
	}

	f.setActive(t, f.revoked.id, true)
	if err := f.svc.DeleteRole(context.Background(), f.revoked.id); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
	if got := f.visibleTenants(t); len(got) != 0 {
		t.Fatalf("visible tenants with only a deleted role = %v, want none", got)
	}
}

func TestRoleLifecycle_GlobalInactiveRoleDoesNotShowEveryTenant(t *testing.T) {
	f := newRoleLifeFixture(t, nil)
	if err := f.svc.RemoveRoleFromUser(context.Background(), f.user, f.kept.id, nil); err != nil {
		t.Fatalf("RemoveRoleFromUser: %v", err)
	}
	if got := f.visibleTenants(t); len(got) != 2 {
		t.Fatalf("visible tenants with a global role = %v, want both", got)
	}

	f.setActive(t, f.revoked.id, false)
	if got := f.visibleTenants(t); len(got) != 0 {
		t.Fatalf("visible tenants with only an inactive global role = %v, want none", got)
	}
}
