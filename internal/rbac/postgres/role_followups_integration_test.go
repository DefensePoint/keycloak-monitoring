// Role lifecycle follow-ups: a role created inactive stays inactive, a deleted
// role's name can be reused, /me lists only roles that grant, and the startup
// cleanup removes assignments left by roles deleted before DeleteRole did.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

func TestRoleFollowups_CreatedInactiveStaysInactive(t *testing.T) {
	f := newRoleLifeFixture(t, nil)
	ctx := context.Background()
	name := fmt.Sprintf("rolelife-%d-inactive", time.Now().UnixNano())

	role, err := f.svc.CreateRoleWithPermissions(ctx, name, "Inactive", "", []string{f.revoked.permission}, false)
	if err != nil {
		t.Fatalf("CreateRoleWithPermissions: %v", err)
	}
	t.Cleanup(func() {
		f.db.Exec(`DELETE FROM user_roles WHERE role_id = ?`, role.ID)
		f.db.Exec(`DELETE FROM role_permissions WHERE role_id = ?`, role.ID)
		f.db.Exec(`DELETE FROM roles WHERE id = ?`, role.ID)
	})
	if role.IsActive {
		t.Fatal("the returned role is active")
	}
	stored, err := f.roles.GetRoleByID(ctx, role.ID)
	if err != nil || stored == nil || stored.IsActive {
		t.Fatalf("stored role %+v, err %v; want inactive", stored, err)
	}
}

func TestRoleFollowups_DeletedNameCanBeReused(t *testing.T) {
	f := newRoleLifeFixture(t, nil)
	ctx := context.Background()
	name := fmt.Sprintf("rolelife-%d-reuse", time.Now().UnixNano())

	first, err := f.svc.CreateRoleWithPermissions(ctx, name, "First", "", nil, true)
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := f.svc.CreateRoleWithPermissions(ctx, name, "Duplicate", "", nil, true); !errors.Is(err, rbac.ErrRoleNameTaken) {
		t.Fatalf("second live role with the name: err = %v, want ErrRoleNameTaken", err)
	}
	if err := f.svc.DeleteRole(ctx, first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	second, err := f.svc.CreateRoleWithPermissions(ctx, name, "Second", "", nil, true)
	if err != nil {
		t.Fatalf("reuse the deleted role's name: %v", err)
	}
	t.Cleanup(func() { f.db.Exec(`DELETE FROM roles WHERE id IN (?, ?)`, first.ID, second.ID) })

	got, err := f.svc.GetRoleByName(ctx, name)
	if err != nil || got == nil || got.ID != second.ID {
		t.Fatalf("GetRoleByName = %+v, err %v; want the new role %d", got, err, second.ID)
	}
}

func TestRoleFollowups_MeListsOnlyRolesThatGrant(t *testing.T) {
	f := newRoleLifeFixture(t, nil)
	ctx := context.Background()
	f.setActive(t, f.revoked.id, false)

	me, err := f.svc.GetUserWithRolesAndPermissions(ctx, f.user, nil)
	if err != nil {
		t.Fatalf("GetUserWithRolesAndPermissions: %v", err)
	}
	for _, ur := range me.Roles {
		if ur.RoleID == f.revoked.id {
			t.Fatal("/me lists the inactive role")
		}
	}
	if len(me.Roles) != 1 || me.Roles[0].RoleID != f.kept.id {
		t.Fatalf("/me roles = %+v, want only the active role %d", me.Roles, f.kept.id)
	}
}

// The cleanup and the index rewrite run in NewClient's migrations. A second
// NewClient must find nothing left to do.
func TestRoleFollowups_StartupCleanupAndIndex(t *testing.T) {
	f := newRoleLifeFixture(t, nil)
	if err := f.db.Exec(`UPDATE roles SET deleted_at = NOW() WHERE id = ?`, f.revoked.id).Error; err != nil {
		t.Fatalf("soft-delete without removing assignments: %v", err)
	}
	if err := f.db.Exec(`DROP INDEX idx_roles_name`).Error; err != nil {
		t.Fatalf("drop index: %v", err)
	}
	if err := f.db.Exec(`CREATE UNIQUE INDEX idx_roles_name ON roles (name)`).Error; err != nil {
		t.Fatalf("recreate the old full index: %v", err)
	}

	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	for run := 1; run <= 2; run++ {
		client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
		if err != nil {
			t.Fatalf("NewClient run %d: %v", run, err)
		}
		_ = client.Close()

		if f.assignedRoleIDs(t)[f.revoked.id] {
			t.Fatalf("run %d: the deleted role is still assigned", run)
		}
		if !f.assignedRoleIDs(t)[f.kept.id] {
			t.Fatalf("run %d: the live role's assignment was removed", run)
		}
		var def string
		f.db.Raw(`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_roles_name'`).Scan(&def)
		if !strings.Contains(def, "deleted_at IS NULL") {
			t.Fatalf("run %d: idx_roles_name = %q, want it limited to live roles", run, def)
		}
	}
}
