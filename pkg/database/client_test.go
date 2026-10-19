package database

import (
	"io"
	"testing"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// closeOnCleanup closes db's pool when the test ends. Register it straight
// after opening, before any cleanup that still queries db: cleanups run last
// registered first, so this one then runs after them. Left open, each test's
// pool outlived it, and pkg/database alone held some fifty connections by the
// end of a run, which is the whole connection limit on the TimescaleDB image.
func closeOnCleanup(t *testing.T, db *gorm.DB) {
	t.Helper()
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

// newTestClient opens a postgres-backed GORM connection for migration tests.
//
// The DSN comes from KMT_DESTRUCTIVE_TEST_DATABASE_DSN, not from
// KMT_TEST_DATABASE_DSN: the helper deletes the admin, operator and viewer
// rows globally rather than only the fixtures it created, so it needs a
// database nobody minds losing. If unset, the test is skipped. The test
// connects to a real PostgreSQL instance (sqlite is not a dependency of this
// project), so these tests are integration-style and only run when an operator
// opts in.
//
// Example:
//
//	KMT_DESTRUCTIVE_TEST_DATABASE_DSN="host=localhost port=55432 dbname=kmt_test user=kmt password=kmt sslmode=disable" \
//	    go test ./pkg/database/...
func newTestClient(t *testing.T) *Client {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	dsn := testdb.DSN(t, "KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	testdb.RequireDisposable(t, dsn)
	testdb.Lock(t, dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	closeOnCleanup(t, db)

	// Migrate just the tables we need for these tests.
	if err := db.AutoMigrate(&Role{}, &Permission{}, &RolePermission{}, &DataMigration{}); err != nil {
		t.Fatalf("failed to auto-migrate RBAC tables: %v", err)
	}

	// Clean up any prior state for the rows this test inspects to ensure
	// each test starts from a known baseline.
	t.Cleanup(func() {
		db.Exec("DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE name = ?)", "amfa:read")
		db.Exec("DELETE FROM permissions WHERE name = ?", "amfa:read")
		db.Exec("DELETE FROM roles WHERE name IN (?, ?, ?)", "admin", "operator", "viewer")
		db.Exec("DELETE FROM data_migrations WHERE key = ?", amfaReadGrantKey)
	})

	// Pre-clean to avoid contamination from prior test runs.
	db.Exec("DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE name = ?)", "amfa:read")
	db.Exec("DELETE FROM permissions WHERE name = ?", "amfa:read")
	db.Exec("DELETE FROM roles WHERE name IN (?, ?, ?)", "admin", "operator", "viewer")
	db.Exec("DELETE FROM data_migrations WHERE key = ?", amfaReadGrantKey)

	log := logger.New(zerolog.New(io.Discard))
	return &Client{db: db, logger: log}
}

// TestMigrateAmfaReadPermission_CreatesPermissionAndGrantsToAdmin verifies that
// migrateAmfaReadPermission creates the amfa:read permission row and grants it
// to the admin role.
func TestMigrateAmfaReadPermission_CreatesPermissionAndGrantsToAdmin(t *testing.T) {
	c := newTestClient(t)

	// Seed the admin role (the migration only grants to existing roles).
	adminRole := Role{
		Name:        "admin",
		DisplayName: "Administrator",
		Description: "test admin role",
		IsSystem:    true,
		IsActive:    true,
	}
	if err := c.db.Create(&adminRole).Error; err != nil {
		t.Fatalf("failed to seed admin role: %v", err)
	}

	if err := c.migrateAmfaReadPermission(); err != nil {
		t.Fatalf("migrateAmfaReadPermission() returned error: %v", err)
	}

	// Assert: the permission row exists.
	var perm Permission
	if err := c.db.Where("name = ?", "amfa:read").First(&perm).Error; err != nil {
		t.Fatalf("expected amfa:read permission to exist after migration: %v", err)
	}
	if perm.Resource != "amfa" || perm.Action != "read" {
		t.Errorf("permission has unexpected resource/action: resource=%q action=%q", perm.Resource, perm.Action)
	}

	// Assert: the role_permissions join row exists.
	var rp RolePermission
	err := c.db.Where("role_id = ? AND permission_id = ?", adminRole.ID, perm.ID).First(&rp).Error
	if err != nil {
		t.Fatalf("expected role_permissions row linking admin -> amfa:read: %v", err)
	}
}

// TestMigrateAmfaReadPermission_Idempotent verifies that running the migration
// twice produces no duplicate rows and no errors.
func TestMigrateAmfaReadPermission_Idempotent(t *testing.T) {
	c := newTestClient(t)

	// Seed all three default roles.
	for _, name := range []string{"admin", "operator", "viewer"} {
		role := Role{
			Name:        name,
			DisplayName: name,
			IsSystem:    true,
			IsActive:    true,
		}
		if err := c.db.Create(&role).Error; err != nil {
			t.Fatalf("failed to seed role %q: %v", name, err)
		}
	}

	// First run.
	if err := c.migrateAmfaReadPermission(); err != nil {
		t.Fatalf("first migrateAmfaReadPermission() returned error: %v", err)
	}
	// Second run.
	if err := c.migrateAmfaReadPermission(); err != nil {
		t.Fatalf("second migrateAmfaReadPermission() returned error: %v", err)
	}

	// Assert: exactly one amfa:read permission row.
	var permCount int64
	if err := c.db.Model(&Permission{}).Where("name = ?", "amfa:read").Count(&permCount).Error; err != nil {
		t.Fatalf("failed to count amfa:read permissions: %v", err)
	}
	if permCount != 1 {
		t.Errorf("expected 1 amfa:read permission row, got %d", permCount)
	}

	// Assert: exactly one role_permissions row per role (3 total).
	var perm Permission
	if err := c.db.Where("name = ?", "amfa:read").First(&perm).Error; err != nil {
		t.Fatalf("failed to load amfa:read permission: %v", err)
	}
	var rpCount int64
	if err := c.db.Model(&RolePermission{}).
		Where("permission_id = ?", perm.ID).
		Count(&rpCount).Error; err != nil {
		t.Fatalf("failed to count role_permissions: %v", err)
	}
	if rpCount != 3 {
		t.Errorf("expected 3 role_permissions rows for amfa:read (admin/operator/viewer), got %d", rpCount)
	}
}
