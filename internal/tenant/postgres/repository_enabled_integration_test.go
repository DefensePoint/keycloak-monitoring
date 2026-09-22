// Regression coverage for creating a tenant that must start out disabled.
//
// Requires a real PostgreSQL instance. The connection DSN is taken from the
// KMT_TEST_DATABASE_DSN environment variable; the tests are skipped when
// unset. Run with:
//
//	KMT_TEST_DATABASE_DSN="host=localhost port=5432 dbname=monitoring_test user=monitoring password=monitoring sslmode=disable" \
//	    go test ./tenant/postgres/... -run 'TestCreateStores|TestUpdateTogglesEnabled'
//
// The -run filter is not optional: the sibling purge tests in this package
// reset a hardcoded database with DROP SCHEMA.
package postgres

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// dsnConfig turns a libpq key=value DSN into the config NewClient builds its
// own DSN from. Nothing here is echoed back to the test log: the DSN carries a
// password.
func dsnConfig(t *testing.T, dsn string) *config.DatabaseConfig {
	t.Helper()

	cfg := &config.DatabaseConfig{MaxConns: 5, MinConns: 1, Timeout: 30 * time.Second}
	for _, pair := range strings.Fields(dsn) {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		switch key {
		case "host":
			cfg.Host = value
		case "port":
			port, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("KMT_TEST_DATABASE_DSN has a non-numeric port")
			}
			cfg.Port = port
		case "dbname":
			cfg.Database = value
		case "user":
			cfg.User = value
		case "password":
			cfg.Password = value
		case "sslmode":
			cfg.SSLMode = value
		}
	}
	return cfg
}

// newEnabledTestRepo opens a repository against KMT_TEST_DATABASE_DSN, plus
// the raw handle the assertions read the enabled column through.
func newEnabledTestRepo(t *testing.T) (tenant.Repository, *gorm.DB) {
	t.Helper()

	dsn := os.Getenv("KMT_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_TEST_DATABASE_DSN not set; skipping integration test")
	}

	client, err := database.NewClient(dsnConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return NewRepository(client, nil, logger.NewNoop()), client.DB()
}

// newTestTenant builds a valid tenant carrying a tenant_id unique to this run,
// so a shared test database can't collide on the unique index.
func newTestTenant(t *testing.T, db *gorm.DB, name string, enabled bool) *domain.KeycloakTenant {
	t.Helper()

	tenantID := fmt.Sprintf("enabled-test-%s-%d", name, time.Now().UnixNano())
	t.Cleanup(func() {
		if err := db.Unscoped().Where("tenant_id = ?", tenantID).
			Delete(&database.KeycloakTenant{}).Error; err != nil {
			t.Logf("cleanup: delete tenant %s: %v", tenantID, err)
		}
	})

	return &domain.KeycloakTenant{
		TenantID:     tenantID,
		Name:         tenantID,
		ServerURL:    "https://keycloak.invalid",
		AdminRealm:   "master",
		ClientID:     "admin-cli",
		ClientSecret: "s",
		Enabled:      enabled,
		HealthStatus: "unknown",
	}
}

// storedEnabled reads the column directly, bypassing every conversion between
// the row and the domain tenant.
func storedEnabled(t *testing.T, db *gorm.DB, tenantID string) bool {
	t.Helper()

	var enabled []bool
	if err := db.Raw(`SELECT enabled FROM keycloak_tenants WHERE tenant_id = ?`, tenantID).
		Scan(&enabled).Error; err != nil {
		t.Fatalf("read back enabled: %v", err)
	}
	if len(enabled) != 1 {
		t.Fatalf("expected exactly one row for tenant %s, got %d", tenantID, len(enabled))
	}
	return enabled[0]
}

// The bug this test exists for: keycloak_tenants.enabled defaults to true, and
// GORM substitutes that default for a false field on insert, so the API could
// accept "create this tenant disabled" and leave behind a row saying enabled.
// The running process kept the correct disabled value in its cache; the row
// only took effect on the next restart, when LoadTenants read it back.
func TestCreateStoresADisabledTenantDisabled(t *testing.T) {
	repo, db := newEnabledTestRepo(t)
	ctx := context.Background()

	newTenant := newTestTenant(t, db, "disabled", false)
	if err := repo.Create(ctx, newTenant); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if storedEnabled(t, db, newTenant.TenantID) {
		t.Error("tenant created with Enabled=false is enabled in the database; the next restart will monitor it against the operator's request")
	}

	readBack, err := repo.GetByTenantID(ctx, newTenant.TenantID)
	if err != nil {
		t.Fatalf("GetByTenantID: %v", err)
	}
	if readBack.Enabled {
		t.Error("tenant created with Enabled=false reads back enabled")
	}
}

func TestCreateStoresAnEnabledTenantEnabled(t *testing.T) {
	repo, db := newEnabledTestRepo(t)
	ctx := context.Background()

	newTenant := newTestTenant(t, db, "enabled", true)
	if err := repo.Create(ctx, newTenant); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !storedEnabled(t, db, newTenant.TenantID) {
		t.Error("tenant created with Enabled=true is disabled in the database")
	}
}

// Update writes every column, so it was never subject to the create bug. Pinned
// in both directions because the fix must not reach it.
func TestUpdateTogglesEnabledBothWays(t *testing.T) {
	repo, db := newEnabledTestRepo(t)
	ctx := context.Background()

	subject := newTestTenant(t, db, "toggle", true)
	if err := repo.Create(ctx, subject); err != nil {
		t.Fatalf("Create: %v", err)
	}

	subject.Enabled = false
	if err := repo.Update(ctx, subject); err != nil {
		t.Fatalf("Update to disabled: %v", err)
	}
	if storedEnabled(t, db, subject.TenantID) {
		t.Fatal("Update did not disable the tenant")
	}

	subject.Enabled = true
	if err := repo.Update(ctx, subject); err != nil {
		t.Fatalf("Update to enabled: %v", err)
	}
	if !storedEnabled(t, db, subject.TenantID) {
		t.Error("Update did not re-enable the tenant")
	}
}
