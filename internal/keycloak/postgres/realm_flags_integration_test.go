// Regression coverage for storing a realm's false flags.
//
// enabled, login_with_email_allowed and is_healthy all default to true, and
// GORM substitutes a column's default for a zero value, so a realm disabled
// in Keycloak used to be stored enabled and healthy, on insert and on every
// later sync.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

func newRealmFlagsDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.AutoMigrate(&database.KeycloakRealmInfo{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// realmWithFlags builds a realm in a tenant unique to this run, so a shared
// test database can't collide on the (tenant_id, realm_id) index.
func realmWithFlags(t *testing.T, db *gorm.DB, on bool) *domain.KeycloakRealmInfo {
	t.Helper()

	tenantID := fmt.Sprintf("realm-flags-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		db.Unscoped().Where("tenant_id = ?", tenantID).Delete(&database.KeycloakRealmInfo{})
	})
	return &domain.KeycloakRealmInfo{
		TenantID:              tenantID,
		RealmID:               "realm-flags",
		RealmName:             "realm-flags",
		Enabled:               on,
		LoginWithEmailAllowed: on,
		IsHealthy:             on,
		LastChecked:           time.Now().UTC(),
	}
}

// assertStoredFlags reads the columns directly, so no conversion between the
// row and the domain type can mask what was written.
func assertStoredFlags(t *testing.T, db *gorm.DB, realm *domain.KeycloakRealmInfo, want bool) {
	t.Helper()

	var row database.KeycloakRealmInfo
	if err := db.Where("tenant_id = ? AND realm_id = ?", realm.TenantID, realm.RealmID).
		First(&row).Error; err != nil {
		t.Fatalf("read realm: %v", err)
	}
	if row.Enabled != want || row.LoginWithEmailAllowed != want || row.IsHealthy != want {
		t.Errorf("stored enabled=%v login_with_email_allowed=%v is_healthy=%v, want all %v",
			row.Enabled, row.LoginWithEmailAllowed, row.IsHealthy, want)
	}
}

func TestSaveRealmStoresADisabledRealmDisabled(t *testing.T) {
	db := newRealmFlagsDB(t)
	repo := NewRealmRepository(db)
	realm := realmWithFlags(t, db, false)

	if err := repo.SaveRealm(context.Background(), realm); err != nil {
		t.Fatalf("SaveRealm: %v", err)
	}
	assertStoredFlags(t, db, realm, false)
}

func TestSaveRealmSyncTogglesTheFlagsBothWays(t *testing.T) {
	db := newRealmFlagsDB(t)
	repo := NewRealmRepository(db)
	realm := realmWithFlags(t, db, true)
	ctx := context.Background()

	for _, want := range []bool{true, false, true} {
		realm.Enabled, realm.LoginWithEmailAllowed, realm.IsHealthy = want, want, want
		if err := repo.SaveRealm(ctx, realm); err != nil {
			t.Fatalf("SaveRealm(%v): %v", want, err)
		}
		assertStoredFlags(t, db, realm, want)
	}
}

// A soft-deleted row still owns its (tenant_id, realm_id) slot, so the upsert
// lands on it. Saving must succeed and bring the row back: Keycloak listing
// the realm again means it exists, so a retired row must not stay hidden.
func TestSaveRealmOverASoftDeletedRowStillSucceeds(t *testing.T) {
	db := newRealmFlagsDB(t)
	repo := NewRealmRepository(db)
	realm := realmWithFlags(t, db, true)
	ctx := context.Background()

	if err := repo.SaveRealm(ctx, realm); err != nil {
		t.Fatalf("SaveRealm: %v", err)
	}
	if err := repo.DeleteRealm(ctx, realm.TenantID, realm.RealmName); err != nil {
		t.Fatalf("DeleteRealm: %v", err)
	}
	realm.Enabled, realm.LoginWithEmailAllowed, realm.IsHealthy = false, false, false
	if err := repo.SaveRealm(ctx, realm); err != nil {
		t.Fatalf("SaveRealm over a soft-deleted row: %v", err)
	}
	// Read through the default scope: a row still soft-deleted is not found.
	assertStoredFlags(t, db, realm, false)
}
