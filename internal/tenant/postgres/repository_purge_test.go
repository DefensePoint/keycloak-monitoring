package postgres

import (
	"context"
	"testing"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// newPurgeTestClient wipes the schema and runs the real AutoMigrate, then hands
// back the client plus a raw connection for assertions the Repository does not
// expose.
//
// Follows the project's opt-in convention for destructive database-backed
// tests: the DSN comes from KMT_DESTRUCTIVE_TEST_DATABASE_DSN and the test is
// skipped when it isn't set.
//
// Example:
//
//	KMT_DESTRUCTIVE_TEST_DATABASE_DSN="host=localhost port=55432 dbname=kmt_test user=kmt password=kmt sslmode=disable" \
//	    go test ./tenant/postgres/...
func newPurgeTestClient(t *testing.T) (*database.Client, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dsn := testdb.DSN(t, "KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	testdb.RequireDisposable(t, dsn)
	testdb.Lock(t, dsn)
	raw, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := raw.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	c, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	after, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	return c, after
}

func TestRepositoryDeletePurgesOwnedData(t *testing.T) {
	client, raw := newPurgeTestClient(t)
	repo := NewRepository(client, nil, logger.NewNoop())
	ctx := context.Background()

	if err := raw.Exec(`
		INSERT INTO keycloak_tenants (tenant_id, name, server_url, admin_realm, client_id, client_secret, enabled, created_at, updated_at)
		VALUES ('t1','t1','https://kc','master','c','s',true, now(), now())`).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if err := raw.Exec(`
		INSERT INTO keycloak_realms (tenant_id, realm_id, realm_name, created_at, updated_at)
		VALUES ('t1','master','master', now(), now())`).Error; err != nil {
		t.Fatalf("seed realm: %v", err)
	}

	if err := repo.Delete(ctx, "t1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var realms, tenants int64
	raw.Raw(`SELECT count(*) FROM keycloak_realms WHERE tenant_id='t1'`).Scan(&realms)
	raw.Raw(`SELECT count(*) FROM keycloak_tenants WHERE tenant_id='t1'`).Scan(&tenants)
	if realms != 0 {
		t.Errorf("realms survived tenant deletion: %d", realms)
	}
	if tenants != 0 {
		t.Errorf("tenant row survived: %d", tenants)
	}
}

// The whole point of hard-deleting: the ID becomes reusable immediately.
func TestRepositoryDeleteFreesTenantID(t *testing.T) {
	client, raw := newPurgeTestClient(t)
	repo := NewRepository(client, nil, logger.NewNoop())
	ctx := context.Background()

	if err := raw.Exec(`
		INSERT INTO keycloak_tenants (tenant_id, name, server_url, admin_realm, client_id, client_secret, enabled, created_at, updated_at)
		VALUES ('t1','t1','https://kc','master','c','s',true, now(), now())`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := repo.Delete(ctx, "t1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	err := raw.Exec(`
		INSERT INTO keycloak_tenants (tenant_id, name, server_url, admin_realm, client_id, client_secret, enabled, created_at, updated_at)
		VALUES ('t1','t1 again','https://kc','master','c','s',true, now(), now())`).Error
	if err != nil {
		t.Errorf("could not recreate a deleted tenant under the same ID: %v", err)
	}
}
