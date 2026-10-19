// HasGlobalPermission counts only role grants that apply to every tenant.
// HasPermission with a nil tenant drops the tenant filter, so a grant made for
// one tenant used to satisfy checks on platform-wide resources such as the
// platform user accounts.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset. Only rows with a gperm- prefix are touched.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

type globalPermFixture struct {
	repo       *UserRoleRepository
	db         *gorm.DB
	permission string
	roleID     uint
}

func newGlobalPermFixture(t *testing.T) *globalPermFixture {
	t.Helper()

	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	db := client.DB()

	stamp := time.Now().UnixNano()
	permission := fmt.Sprintf("gperm-%d:write", stamp)
	roleName := fmt.Sprintf("gperm-%d-role", stamp)

	var permID, roleID uint
	if err := db.Raw(`INSERT INTO permissions (name, display_name, resource, action, created_at, updated_at)
		VALUES (?, 'test', 'gperm', 'write', NOW(), NOW()) RETURNING id`, permission).Scan(&permID).Error; err != nil {
		t.Fatalf("seed permission: %v", err)
	}
	if err := db.Raw(`INSERT INTO roles (name, display_name, is_active, created_at, updated_at)
		VALUES (?, 'test', true, NOW(), NOW()) RETURNING id`, roleName).Scan(&roleID).Error; err != nil {
		t.Fatalf("seed role: %v", err)
	}
	if err := db.Exec(`INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)`, roleID, permID).Error; err != nil {
		t.Fatalf("seed role permission: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM user_roles WHERE role_id = ?`, roleID)
		db.Exec(`DELETE FROM role_permissions WHERE role_id = ?`, roleID)
		db.Exec(`DELETE FROM roles WHERE id = ?`, roleID)
		db.Exec(`DELETE FROM permissions WHERE id = ?`, permID)
	})

	return &globalPermFixture{
		repo:       &UserRoleRepository{db: client},
		db:         db,
		permission: permission,
		roleID:     roleID,
	}
}

// userWithGrant creates a user holding the fixture role for tenantID (nil for
// every tenant), expiring at expiresAt when set.
func (f *globalPermFixture) userWithGrant(t *testing.T, tenantID *string, expiresAt *time.Time) uint {
	t.Helper()

	subject := fmt.Sprintf("gperm-%d", time.Now().UnixNano())
	var userID uint
	if err := f.db.Raw(`INSERT INTO users (subject, email, name, username, auth_method, is_active, is_blocked, login_count, created_at, updated_at)
		VALUES (?, ?, 'test', ?, 'simple', true, false, 0, NOW(), NOW()) RETURNING id`,
		subject, subject+"@example.invalid", subject).Scan(&userID).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { f.db.Exec(`DELETE FROM users WHERE id = ?`, userID) })

	if err := f.db.Exec(`INSERT INTO user_roles (user_id, role_id, tenant_id, assigned_by, assigned_at, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, 'test', NOW(), ?, NOW(), NOW())`, userID, f.roleID, tenantID, expiresAt).Error; err != nil {
		t.Fatalf("seed user role: %v", err)
	}
	return userID
}

func TestHasGlobalPermission_GlobalGrantCounts(t *testing.T) {
	f := newGlobalPermFixture(t)
	user := f.userWithGrant(t, nil, nil)

	got, err := f.repo.HasGlobalPermission(context.Background(), user, f.permission)
	if err != nil {
		t.Fatalf("HasGlobalPermission: %v", err)
	}
	if !got {
		t.Fatal("a grant for every tenant was not counted")
	}
}

func TestHasGlobalPermission_TenantScopedGrantDoesNotCount(t *testing.T) {
	f := newGlobalPermFixture(t)
	tenant := "gperm-tenant-a"
	user := f.userWithGrant(t, &tenant, nil)
	ctx := context.Background()

	// The behavior this guards against: with no tenant, HasPermission accepts
	// the grant made for tenant-a.
	loose, err := f.repo.HasPermission(ctx, user, f.permission, nil)
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if !loose {
		t.Fatal("precondition: HasPermission with a nil tenant should accept the tenant-scoped grant")
	}

	got, err := f.repo.HasGlobalPermission(ctx, user, f.permission)
	if err != nil {
		t.Fatalf("HasGlobalPermission: %v", err)
	}
	if got {
		t.Fatal("a grant for one tenant was counted as global")
	}
}

func TestHasGlobalPermission_ExpiredGrantDoesNotCount(t *testing.T) {
	f := newGlobalPermFixture(t)
	expired := time.Now().Add(-time.Hour)
	user := f.userWithGrant(t, nil, &expired)

	got, err := f.repo.HasGlobalPermission(context.Background(), user, f.permission)
	if err != nil {
		t.Fatalf("HasGlobalPermission: %v", err)
	}
	if got {
		t.Fatal("an expired global grant was counted")
	}
}

func TestHasGlobalPermission_OtherPermissionDoesNotCount(t *testing.T) {
	f := newGlobalPermFixture(t)
	user := f.userWithGrant(t, nil, nil)

	got, err := f.repo.HasGlobalPermission(context.Background(), user, f.permission+"-other")
	if err != nil {
		t.Fatalf("HasGlobalPermission: %v", err)
	}
	if got {
		t.Fatal("a permission the role does not grant was counted")
	}
}
