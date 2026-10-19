// Coverage for the "exactly one default tenant" invariant.
//
// Requires a real PostgreSQL instance, taken from KMT_TEST_DATABASE_DSN the
// same way the enabled tests in this package do.
package postgres

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

func storedDefault(t *testing.T, db *gorm.DB, tenantID string) bool {
	t.Helper()

	var isDefault []bool
	if err := db.Raw(`SELECT is_default FROM keycloak_tenants WHERE tenant_id = ?`, tenantID).
		Scan(&isDefault).Error; err != nil {
		t.Fatalf("read back is_default: %v", err)
	}
	if len(isDefault) != 1 {
		t.Fatalf("expected exactly one row for tenant %s, got %d", tenantID, len(isDefault))
	}
	return isDefault[0]
}

func TestSavingADefaultTenantClearsThePreviousDefault(t *testing.T) {
	repo, db := newEnabledTestRepo(t)
	ctx := context.Background()

	previous := newTestTenant(t, db, "default-previous", true)
	previous.IsDefault = true
	if err := repo.Create(ctx, previous); err != nil {
		t.Fatalf("Create previous default: %v", err)
	}

	next := newTestTenant(t, db, "default-next", true)
	if err := repo.Create(ctx, next); err != nil {
		t.Fatalf("Create: %v", err)
	}

	next.IsDefault = true
	if err := repo.Update(ctx, next); err != nil {
		t.Fatalf("Update to default: %v", err)
	}

	if storedDefault(t, db, previous.TenantID) {
		t.Error("the previous default is still default; two tenants are marked default")
	}
	if !storedDefault(t, db, next.TenantID) {
		t.Error("the updated tenant is not default")
	}
}

// The unset and the save share one transaction, so a save the database refuses
// must not leave the installation with no default tenant at all.
func TestAFailedSaveLeavesThePreviousDefaultInPlace(t *testing.T) {
	repo, db := newEnabledTestRepo(t)
	ctx := context.Background()

	previous := newTestTenant(t, db, "rollback-previous", true)
	previous.IsDefault = true
	if err := repo.Create(ctx, previous); err != nil {
		t.Fatalf("Create previous default: %v", err)
	}

	next := newTestTenant(t, db, "rollback-next", true)
	if err := repo.Create(ctx, next); err != nil {
		t.Fatalf("Create: %v", err)
	}

	next.IsDefault = true
	next.Name = strings.Repeat("n", 300)
	if err := repo.Update(ctx, next); err == nil {
		t.Fatal("Update accepted a name longer than the column")
	}

	if !storedDefault(t, db, previous.TenantID) {
		t.Error("the previous default was cleared by an update that failed")
	}
	if storedDefault(t, db, next.TenantID) {
		t.Error("a tenant whose save failed is marked default")
	}
}

func TestCreatingADefaultTenantClearsThePreviousDefault(t *testing.T) {
	repo, db := newEnabledTestRepo(t)
	ctx := context.Background()

	previous := newTestTenant(t, db, "create-previous", true)
	previous.IsDefault = true
	if err := repo.Create(ctx, previous); err != nil {
		t.Fatalf("Create previous default: %v", err)
	}

	next := newTestTenant(t, db, "create-next", true)
	next.IsDefault = true
	if err := repo.Create(ctx, next); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if storedDefault(t, db, previous.TenantID) {
		t.Error("the previous default is still default after a new default tenant was created")
	}

	var defaults []domain.KeycloakTenant
	if err := db.Raw(`SELECT tenant_id FROM keycloak_tenants WHERE is_default = true`).
		Scan(&defaults).Error; err != nil {
		t.Fatalf("count defaults: %v", err)
	}
	if len(defaults) != 1 {
		t.Errorf("default tenants = %d, want 1", len(defaults))
	}
}
