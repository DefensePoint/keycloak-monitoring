package postgres

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// A deleted alert whose condition is detected again must come back as a new,
// live alert, on every database. A fresh database gets idx_alert_tenant from
// the model tag rather than from the migration, so it has to be booted from an
// empty schema to see the index it really ends up with: these tests drop the
// public schema, so they read the destructive DSN.

func newFreshlyBootedRepo(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dsn := testdb.DSN(t, "KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	testdb.RequireDisposable(t, dsn)
	testdb.Lock(t, dsn)

	raw, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := raw.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if sqlDB, err := raw.DB(); err == nil {
		_ = sqlDB.Close()
	}

	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient on an empty schema: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return &Repository{db: client.DB()}, client.DB()
}

func recurringAlert(now time.Time) *domain.Alert {
	return &domain.Alert{
		TenantID: "fresh-db", AlertID: "ssl-not-required-blue", CheckType: "realm_security",
		Status: domain.AlertStatusActive, Source: domain.AlertSourceConfiguration,
		Type: domain.AlertTypeSecurity, Severity: domain.AlertSeverityCritical,
		Title: "SSL Not Required", ResourceType: "realm", ResourceID: "blue", ResourceName: "blue",
		RealmName: "blue", FirstDetected: now, LastSeen: now,
	}
}

func TestSave_ADeletedAlertComesBackOnAFreshDatabase(t *testing.T) {
	r, db := newFreshlyBootedRepo(t)
	ctx := context.Background()

	if err := r.Save(ctx, recurringAlert(time.Now().UTC())); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := r.Delete(ctx, "fresh-db", "ssl-not-required-blue"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// The condition is detected again.
	if err := r.Save(ctx, recurringAlert(time.Now().UTC())); err != nil {
		t.Fatalf("save after the delete: %v", err)
	}

	got, err := r.GetByAlertID(ctx, "fresh-db", "ssl-not-required-blue")
	if err != nil || got == nil {
		t.Fatalf("the alert did not come back after it was deleted and detected again: %v", err)
	}

	var live, deleted int64
	db.Model(&database.ConfigurationAlert{}).Where("tenant_id = ? AND alert_id = ?", "fresh-db", "ssl-not-required-blue").Count(&live)
	db.Unscoped().Model(&database.ConfigurationAlert{}).
		Where("tenant_id = ? AND alert_id = ? AND deleted_at IS NOT NULL", "fresh-db", "ssl-not-required-blue").Count(&deleted)
	if live != 1 || deleted != 1 {
		t.Errorf("want the deleted row kept and one new live row; got %d live, %d deleted", live, deleted)
	}
}
