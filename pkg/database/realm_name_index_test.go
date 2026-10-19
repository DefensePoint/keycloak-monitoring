package database

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// The realm name index has to be unique per tenant over live rows only, on
// every database: a realm deleted and recreated in Keycloak keeps its name and
// gets a new id, and the old row is retired before the new one is inserted.
// These tests drop and rebuild the index, so they read the destructive DSN.

func newRealmNameIndexTestClient(t *testing.T) (*Client, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dsn := testdb.DSN(t, "KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	testdb.RequireDisposable(t, dsn)
	testdb.Lock(t, dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	closeOnCleanup(t, db)
	if err := db.AutoMigrate(&KeycloakRealmInfo{}); err != nil {
		t.Fatalf("auto-migrate keycloak_realms: %v", err)
	}

	clean := func() { db.Exec("DELETE FROM keycloak_realms WHERE tenant_id LIKE ?", "rix-%") }
	clean()
	t.Cleanup(clean)

	return &Client{db: db, logger: logger.New(zerolog.New(io.Discard))}, db
}

func realmNameIndexDef(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var def string
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_keycloak_realms_tenant_realm_name'`).
		Scan(&def).Error; err != nil {
		t.Fatalf("read index definition: %v", err)
	}
	return def
}

func TestMigrateRealmNameIndex_LeavesEveryStartingShapePartialAndTenantScoped(t *testing.T) {
	starting := map[string]string{
		// Before !74: a realm name unique across every tenant.
		"global": `CREATE UNIQUE INDEX idx_keycloak_realms_tenant_realm_name ON keycloak_realms (realm_name)`,
		// What AutoMigrate builds from the model tag on a fresh database.
		"full tenant-scoped": `CREATE UNIQUE INDEX idx_keycloak_realms_tenant_realm_name ON keycloak_realms (tenant_id, realm_name)`,
		// What the migration has always produced; must be left as it is.
		"partial tenant-scoped": `CREATE UNIQUE INDEX idx_keycloak_realms_tenant_realm_name ON keycloak_realms (tenant_id, realm_name) WHERE deleted_at IS NULL`,
	}
	for name, create := range starting {
		t.Run(name, func(t *testing.T) {
			c, db := newRealmNameIndexTestClient(t)
			db.Exec("DROP INDEX IF EXISTS idx_keycloak_realms_tenant_realm_name")
			if err := db.Exec(create).Error; err != nil {
				t.Fatalf("create starting index: %v", err)
			}

			// Twice: the migration runs on every boot.
			for run := 1; run <= 2; run++ {
				if err := c.migrateRealmNameIndex(); err != nil {
					t.Fatalf("migrate (run %d): %v", run, err)
				}
			}

			def := realmNameIndexDef(t, db)
			if !strings.Contains(def, "tenant_id") || !strings.Contains(def, "deleted_at IS NULL") {
				t.Errorf("index after migration = %q, want tenant-scoped over live rows", def)
			}
		})
	}
}

// Stated as behaviour: a recreated realm's new row fits beside the retired old
// one, and two live rows with one name still cannot.
func TestMigrateRealmNameIndex_LetsARecreatedRealmBeSaved(t *testing.T) {
	c, db := newRealmNameIndexTestClient(t)
	db.Exec("DROP INDEX IF EXISTS idx_keycloak_realms_tenant_realm_name")
	db.Exec(`CREATE UNIQUE INDEX idx_keycloak_realms_tenant_realm_name ON keycloak_realms (tenant_id, realm_name)`)
	if err := c.migrateRealmNameIndex(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	row := func(id string) *KeycloakRealmInfo {
		return &KeycloakRealmInfo{TenantID: "rix-t1", RealmID: id, RealmName: "recreated", LastChecked: time.Now()}
	}
	old := row("old-id")
	if err := db.Create(old).Error; err != nil {
		t.Fatalf("insert old realm: %v", err)
	}
	if err := db.Create(row("second-live-id")).Error; err == nil {
		t.Fatal("a second live row with the same name was accepted; the index no longer enforces uniqueness")
	}
	if err := db.Delete(old).Error; err != nil {
		t.Fatalf("retire old realm: %v", err)
	}
	if err := db.Create(row("new-id")).Error; err != nil {
		t.Errorf("the recreated realm could not be saved beside the retired row: %v", err)
	}
}
