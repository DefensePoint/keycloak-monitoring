package postgres

import (
	"context"
	"testing"
	"time"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/database"
)

const purgeTestDSN = "host=localhost port=55433 user=postgres password=review dbname=reviewdb sslmode=disable"

func newPurgeTestClient(t *testing.T) (*database.Client, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	raw, err := gorm.Open(gormpg.Open(purgeTestDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := raw.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	c, err := database.NewClient(&config.DatabaseConfig{
		Host: "localhost", Port: 55433, Database: "reviewdb",
		User: "postgres", Password: "review", SSLMode: "disable",
		MaxConns: 5, MinConns: 1, Timeout: 30 * time.Second,
	}, logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	after, err := gorm.Open(gormpg.Open(purgeTestDSN), &gorm.Config{})
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
