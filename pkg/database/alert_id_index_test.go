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

// The alert ID index has to be unique per tenant over live rows only, on every
// database: a deleted alert whose condition is detected again is saved as a
// new row beside the deleted one. These tests drop and rebuild the index, so
// they read the destructive DSN.

func newAlertIDIndexTestClient(t *testing.T) (*Client, *gorm.DB) {
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
	// Emptied first, not just of this test's rows: the wider starting shapes
	// below cannot be built over rows other tests left that only the partial
	// index allows, such as a deleted alert beside its live successor.
	if db.Migrator().HasTable(&Alert{}) {
		if err := db.Exec("TRUNCATE configuration_alerts").Error; err != nil {
			t.Fatalf("empty configuration_alerts: %v", err)
		}
	}
	if err := db.AutoMigrate(&Alert{}); err != nil {
		t.Fatalf("auto-migrate configuration_alerts: %v", err)
	}
	t.Cleanup(func() { db.Exec("TRUNCATE configuration_alerts") })

	return &Client{db: db, logger: logger.New(zerolog.New(io.Discard))}, db
}

func alertIDIndexDef(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var def string
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_alert_tenant'`).
		Scan(&def).Error; err != nil {
		t.Fatalf("read index definition: %v", err)
	}
	return def
}

func TestMigrateAlertIDIndex_LeavesEveryStartingShapePartialAndTenantScoped(t *testing.T) {
	starting := map[string]string{
		// Before the tenant-scope migration: an alert ID unique across every tenant.
		"global": `CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts (alert_id)`,
		// What AutoMigrate builds from the model tag on a fresh database.
		"full tenant-scoped": `CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts (tenant_id, alert_id)`,
		// What the migration has always produced; must be left as it is.
		"partial tenant-scoped": `CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts (tenant_id, alert_id) WHERE deleted_at IS NULL`,
	}
	for name, create := range starting {
		t.Run(name, func(t *testing.T) {
			c, db := newAlertIDIndexTestClient(t)
			db.Exec("DROP INDEX IF EXISTS idx_alert_tenant")
			if err := db.Exec(create).Error; err != nil {
				t.Fatalf("create starting index: %v", err)
			}

			// Twice: the migration runs on every boot.
			for run := 1; run <= 2; run++ {
				if err := c.migrateAlertIDIndex(); err != nil {
					t.Fatalf("migrate (run %d): %v", run, err)
				}
			}

			def := alertIDIndexDef(t, db)
			if !strings.Contains(def, "tenant_id") || !strings.Contains(def, "deleted_at IS NULL") {
				t.Errorf("index after migration = %q, want tenant-scoped over live rows", def)
			}
		})
	}
}

// Stated as behaviour: a recurring alert's new row fits beside its deleted one,
// and two live rows with one alert ID in one tenant still cannot.
func TestMigrateAlertIDIndex_LetsADeletedAlertBeRaisedAgain(t *testing.T) {
	c, db := newAlertIDIndexTestClient(t)
	db.Exec("DROP INDEX IF EXISTS idx_alert_tenant")
	db.Exec(`CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts (tenant_id, alert_id)`)
	if err := c.migrateAlertIDIndex(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	row := func() *Alert {
		now := time.Now()
		return &Alert{TenantID: "aix-t1", AlertID: "recurring", Type: "security", Severity: "warning",
			Status: "active", Title: "recurring", ResourceType: "realm", ResourceID: "r", ResourceName: "r",
			RealmName: "r", FirstDetected: now, LastSeen: now}
	}
	first := row()
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("insert alert: %v", err)
	}
	if err := db.Create(row()).Error; err == nil {
		t.Fatal("a second live row with the same alert ID was accepted; the index no longer enforces uniqueness")
	}
	if err := db.Delete(first).Error; err != nil {
		t.Fatalf("delete alert: %v", err)
	}
	if err := db.Create(row()).Error; err != nil {
		t.Errorf("the recurring alert could not be saved beside its deleted row: %v", err)
	}
}
