package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// testDBConfig restates a DSN as the config NewClient takes, so the raw
// connection and the client address the same database.
func testDBConfig(t *testing.T, dsn string) *config.DatabaseConfig {
	t.Helper()
	parsed, err := pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse KMT_DESTRUCTIVE_TEST_DATABASE_DSN: %v", err)
	}
	// pgconn resolves sslmode into a *tls.Config, so the original keyword is
	// gone by here; a nil TLSConfig means the DSN asked for no TLS.
	sslMode := "require"
	if parsed.TLSConfig == nil {
		sslMode = "disable"
	}
	return &config.DatabaseConfig{
		Host: parsed.Host, Port: int(parsed.Port), Database: parsed.Database,
		User: parsed.User, Password: parsed.Password, SSLMode: sslMode,
		MaxConns: 5, MinConns: 1, Timeout: 30 * time.Second,
	}
}

// newPurgeTestClient wipes the schema and runs the real AutoMigrate, giving
// each test a database shaped exactly like a fresh install.
//
// Follows the same opt-in convention as client_test.go's newTestClient: the DSN
// comes from KMT_DESTRUCTIVE_TEST_DATABASE_DSN and the test is skipped when
// it isn't set. Named distinctly from newTestClient (which only migrates the
// RBAC tables) to avoid a redeclaration in this package.
//
// Example:
//
//	KMT_DESTRUCTIVE_TEST_DATABASE_DSN="host=localhost port=55432 dbname=kmt_test user=kmt password=kmt sslmode=disable" \
//	    go test ./pkg/database/...
func newPurgeTestClient(t *testing.T) *Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dsn := os.Getenv("KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_DESTRUCTIVE_TEST_DATABASE_DSN not set; skipping destructive integration test")
	}
	testdb.RequireDisposable(t, dsn)
	raw, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := raw.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	c, err := NewClient(testDBConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestPendingTenantPurgeTableExists(t *testing.T) {
	c := newPurgeTestClient(t)

	var n int64
	if err := c.DB().Raw(
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema='public' AND table_name='pending_tenant_purges'`).
		Scan(&n).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Errorf("pending_tenant_purges table not created by AutoMigrate")
	}
}

func TestTenantScopedTablesCoversEveryTenantTable(t *testing.T) {
	c := newPurgeTestClient(t)

	// keycloak_tenants owns the tenant itself, and pending_tenant_purges is the
	// queue rather than tenant-owned data. Neither belongs in the registry.
	var actual []string
	if err := c.DB().Raw(
		`SELECT table_name FROM information_schema.columns
		 WHERE column_name='tenant_id' AND table_schema='public'
		   AND table_name NOT IN ('keycloak_tenants', 'pending_tenant_purges')
		 GROUP BY table_name ORDER BY table_name`).
		Scan(&actual).Error; err != nil {
		t.Fatalf("query: %v", err)
	}

	registered := map[string]bool{}
	for _, tt := range tenantScopedTables {
		registered[tt.name] = true
	}
	for _, name := range actual {
		if !registered[name] {
			t.Errorf("table %q has a tenant_id column but is missing from tenantScopedTables", name)
		}
	}
	if len(actual) != len(tenantScopedTables) {
		t.Errorf("registry has %d tables, schema has %d: %v", len(tenantScopedTables), len(actual), actual)
	}
}

// seedUserAndRole creates the rows that tenant_policies and user_roles point at.
// Both carry real foreign keys to users, and user_roles also to roles, so the
// tenant-scoped grants below cannot be inserted without them.
func seedUserAndRole(t *testing.T, c *Client) {
	t.Helper()
	if err := c.DB().Exec(`
		INSERT INTO users (id, email, created_at, updated_at)
		VALUES (1, 'seed@example.com', now(), now())
		ON CONFLICT (id) DO NOTHING`).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := c.DB().Exec(`
		INSERT INTO roles (id, name, display_name, created_at, updated_at)
		VALUES (1, 'seed-role', 'Seed Role', now(), now())
		ON CONFLICT (id) DO NOTHING`).Error; err != nil {
		t.Fatalf("seed role: %v", err)
	}
}

// seedTenant creates a tenant plus one owned row in every registry table.
func seedTenant(t *testing.T, c *Client, tenantID string) {
	t.Helper()
	seedUserAndRole(t, c)
	db := c.DB()
	if err := db.Create(&KeycloakTenant{
		TenantID: tenantID, Name: tenantID, ServerURL: "https://kc.example.com",
		AdminRealm: "master", ClientID: "monitoring-service", ClientSecret: "s", Enabled: true,
	}).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	stmts := []string{
		`INSERT INTO keycloak_realms (tenant_id, realm_id, realm_name, created_at, updated_at)
		 VALUES (?, ?, ?, now(), now())`,
		`INSERT INTO configuration_alerts (tenant_id, alert_id, source, type, severity, status, title,
		 resource_type, resource_id, resource_name, realm_name, first_detected, last_seen, created_at, updated_at)
		 VALUES (?, ?, 'configuration', 'security', 'warning', 'active', 'x', 'realm', 'x', 'x', 'x', now(), now(), now(), now())`,
		`INSERT INTO alert_rules (tenant_id, rule_id, name, source, severity, conditions, title_template, created_at, updated_at)
		 VALUES (?, ?, 'x', 'event', 'warning', '{}', 'x', now(), now())`,
		`INSERT INTO tenant_policies (tenant_id, user_id, granted_at, created_at, updated_at)
		 VALUES (?, 1, now(), now(), now())`,
		`INSERT INTO operator_actions (tenant_id, alert_id, operator_id, operator_email, action_type,
		 action_time, alert_severity, alert_type, realm_name, created_at, updated_at)
		 VALUES (?, 1, 1, 'seed@example.com', 'acknowledged', now(), 'warning', 'security', 'x', now(), now())`,
	}
	args := [][]interface{}{
		{tenantID, tenantID + "-realm", tenantID + "-realm"},
		{tenantID, tenantID + "-alert"},
		{tenantID, tenantID + "-rule"},
		{tenantID},
		{tenantID},
	}
	for i, s := range stmts {
		if err := db.Exec(s, args[i]...).Error; err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	if err := db.Exec(
		`INSERT INTO keycloak_metrics (tenant_id, time, realm_name) VALUES (?, now(), 'x')`, tenantID).Error; err != nil {
		t.Fatalf("seed metrics: %v", err)
	}
	// Tenant-scoped role grant, distinct from the global one used in
	// TestPurgeTenantSparesGlobalRows.
	if err := db.Exec(
		`INSERT INTO user_roles (user_id, role_id, tenant_id, assigned_at, created_at, updated_at)
		 VALUES (1, 1, ?, now(), now(), now())`, tenantID).Error; err != nil {
		t.Fatalf("seed user_role: %v", err)
	}
}

func countIn(t *testing.T, c *Client, table, tenantID string) int64 {
	t.Helper()
	var n int64
	if err := c.DB().Raw("SELECT count(*) FROM "+table+" WHERE tenant_id = ?", tenantID).
		Scan(&n).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestPurgeTenantRemovesOwnedRows(t *testing.T) {
	c := newPurgeTestClient(t)
	seedTenant(t, c, "doomed")
	seedTenant(t, c, "keeper")

	if _, err := c.PurgeTenant(context.Background(), "doomed"); err != nil {
		t.Fatalf("PurgeTenant: %v", err)
	}

	for _, table := range []string{"keycloak_realms", "configuration_alerts", "alert_rules", "operator_actions", "tenant_policies", "user_roles"} {
		if n := countIn(t, c, table, "doomed"); n != 0 {
			t.Errorf("%s still has %d rows for the purged tenant", table, n)
		}
		if n := countIn(t, c, table, "keeper"); n != 1 {
			t.Errorf("%s: other tenant lost rows, have %d want 1", table, n)
		}
	}

	// The tenant row is hard-deleted, so it is gone even unscoped.
	var tenants int64
	c.DB().Unscoped().Model(&KeycloakTenant{}).Where("tenant_id = ?", "doomed").Count(&tenants)
	if tenants != 0 {
		t.Errorf("tenant row survived as a soft-deleted tombstone")
	}
}

func TestPurgeTenantEnqueuesTelemetryDrain(t *testing.T) {
	c := newPurgeTestClient(t)
	seedTenant(t, c, "doomed")

	if _, err := c.PurgeTenant(context.Background(), "doomed"); err != nil {
		t.Fatalf("PurgeTenant: %v", err)
	}

	var queued int64
	c.DB().Model(&PendingTenantPurge{}).Where("tenant_id = ?", "doomed").Count(&queued)
	if queued != 1 {
		t.Errorf("tenant not enqueued for telemetry drain")
	}
	// Telemetry is deliberately left for the drain.
	if n := countIn(t, c, "keycloak_metrics", "doomed"); n != 1 {
		t.Errorf("telemetry should not be purged synchronously, have %d want 1", n)
	}
}

func TestPurgeTenantSparesGlobalRows(t *testing.T) {
	c := newPurgeTestClient(t)
	seedTenant(t, c, "doomed")

	// Global rows: NULL tenant_id means "not owned by any tenant".
	if err := c.DB().Exec(
		`INSERT INTO alert_rules (tenant_id, rule_id, name, source, severity, conditions, title_template, created_at, updated_at)
		 VALUES (NULL, 'global-rule', 'global', 'event', 'warning', '{}', 'x', now(), now())`).Error; err != nil {
		t.Fatalf("seed global rule: %v", err)
	}
	if err := c.DB().Exec(
		`INSERT INTO user_roles (user_id, role_id, tenant_id, assigned_at, created_at, updated_at)
		 VALUES (1, 1, NULL, now(), now(), now())`).Error; err != nil {
		t.Fatalf("seed global role: %v", err)
	}

	if _, err := c.PurgeTenant(context.Background(), "doomed"); err != nil {
		t.Fatalf("PurgeTenant: %v", err)
	}

	var rules, roles int64
	c.DB().Raw(`SELECT count(*) FROM alert_rules WHERE tenant_id IS NULL`).Scan(&rules)
	c.DB().Raw(`SELECT count(*) FROM user_roles WHERE tenant_id IS NULL`).Scan(&roles)
	if rules != 1 {
		t.Errorf("global alert rule was purged")
	}
	if roles != 1 {
		t.Errorf("global role grant was purged")
	}
}

func TestPurgeTenantIsAtomic(t *testing.T) {
	c := newPurgeTestClient(t)
	seedTenant(t, c, "doomed")

	// Break one registry table so the transaction must roll back.
	if err := c.DB().Exec("ALTER TABLE tenant_policies RENAME TO tenant_policies_moved").Error; err != nil {
		t.Fatalf("rename: %v", err)
	}
	t.Cleanup(func() {
		c.DB().Exec("ALTER TABLE tenant_policies_moved RENAME TO tenant_policies")
	})

	if _, err := c.PurgeTenant(context.Background(), "doomed"); err == nil {
		t.Fatal("expected PurgeTenant to fail")
	}

	var tenants int64
	c.DB().Unscoped().Model(&KeycloakTenant{}).Where("tenant_id = ?", "doomed").Count(&tenants)
	if tenants != 1 {
		t.Errorf("tenant row was deleted despite the purge failing: rollback did not happen")
	}
	if n := countIn(t, c, "keycloak_realms", "doomed"); n != 1 {
		t.Errorf("realms deleted despite the purge failing, have %d want 1", n)
	}
}

func TestDrainRemovesQueuedTelemetry(t *testing.T) {
	c := newPurgeTestClient(t)
	seedTenant(t, c, "doomed")
	seedTenant(t, c, "keeper")

	if _, err := c.PurgeTenant(context.Background(), "doomed"); err != nil {
		t.Fatalf("PurgeTenant: %v", err)
	}
	if _, err := c.DrainPendingPurges(context.Background()); err != nil {
		t.Fatalf("DrainPendingPurges: %v", err)
	}

	if n := countIn(t, c, "keycloak_metrics", "doomed"); n != 0 {
		t.Errorf("telemetry not drained, %d rows remain", n)
	}
	if n := countIn(t, c, "keycloak_metrics", "keeper"); n != 1 {
		t.Errorf("live tenant's telemetry was drained, have %d want 1", n)
	}

	var queued int64
	c.DB().Model(&PendingTenantPurge{}).Where("tenant_id = ?", "doomed").Count(&queued)
	if queued != 0 {
		t.Errorf("queue row not removed after a full drain")
	}
}

// The reason requested_at exists. A tenant can be deleted and recreated under
// the same ID before the queue is drained; the recreated tenant's fresh data
// must survive.
func TestDrainSparesTelemetryWrittenAfterDeletion(t *testing.T) {
	c := newPurgeTestClient(t)
	seedTenant(t, c, "recycled")

	if _, err := c.PurgeTenant(context.Background(), "recycled"); err != nil {
		t.Fatalf("PurgeTenant: %v", err)
	}

	// Recreated, and now writing new telemetry. The timestamp comes from the Go
	// clock, as it does in production (keycloak/monitor.go stamps every metric
	// with time.Now()), and as requested_at does. SQL now() would be read off
	// the database server's clock instead, which on a containerised test
	// database trails the host by a millisecond or so — enough for this row to
	// land fractionally *before* requested_at and be drained legitimately.
	if err := c.DB().Exec(
		`INSERT INTO keycloak_metrics (tenant_id, time, realm_name) VALUES ('recycled', ?, 'x')`,
		time.Now()).Error; err != nil {
		t.Fatalf("post-recreation metric: %v", err)
	}

	if _, err := c.DrainPendingPurges(context.Background()); err != nil {
		t.Fatalf("DrainPendingPurges: %v", err)
	}

	if n := countIn(t, c, "keycloak_metrics", "recycled"); n != 1 {
		t.Errorf("recreated tenant's new telemetry was destroyed, %d rows remain want 1", n)
	}
}

func TestDrainIsResumableWhenCapped(t *testing.T) {
	c := newPurgeTestClient(t)
	seedTenant(t, c, "big")
	if _, err := c.PurgeTenant(context.Background(), "big"); err != nil {
		t.Fatalf("PurgeTenant: %v", err)
	}
	// One row over the cap, backdated so the time bound includes them. Inserted
	// in a single statement: 50k individual round-trips would take minutes.
	//
	// Two deviations from the brief's literal statement, both required by the
	// live schema (confirmed via psql \d keycloak_metrics):
	//   - realm_name is included: it is NOT NULL with no default, so the
	//     literal statement fails with SQLSTATE 23502.
	//   - time is offset by the generate_series index: the primary key is
	//     (tenant_id, time, realm_name), and the literal statement gives every
	//     row the same "now() - interval '1 hour'" (evaluated once per
	//     statement), so every row collides on the same key
	//     (SQLSTATE 23505). The offset is in seconds, so all rows stay within
	//     the same hour, comfortably inside one drainWindow and below
	//     requested_at.
	if err := c.DB().Exec(`
		INSERT INTO keycloak_metrics (tenant_id, time, realm_name)
		SELECT 'big', now() - interval '1 hour' - (g * interval '1 second'), 'x'
		FROM generate_series(1, ?) AS g`, drainBatchLimit+1).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := c.DrainPendingPurges(context.Background()); err != nil {
		t.Fatalf("first drain: %v", err)
	}
	var queued int64
	c.DB().Model(&PendingTenantPurge{}).Where("tenant_id = ?", "big").Count(&queued)
	if queued != 1 {
		t.Fatalf("queue row removed while rows remain: drain is not resumable")
	}

	if _, err := c.DrainPendingPurges(context.Background()); err != nil {
		t.Fatalf("second drain: %v", err)
	}
	if n := countIn(t, c, "keycloak_metrics", "big"); n != 0 {
		t.Errorf("second drain did not finish the job, %d rows remain", n)
	}
	c.DB().Model(&PendingTenantPurge{}).Where("tenant_id = ?", "big").Count(&queued)
	if queued != 0 {
		t.Errorf("queue row survived a completed drain, tenant would be drained again every startup")
	}
}

func TestSeedEnqueuesPreExistingOrphans(t *testing.T) {
	c := newPurgeTestClient(t)
	seedTenant(t, c, "live")
	seedTenant(t, c, "ghost")
	// Simulate the old behaviour: tenant row removed, data left behind.
	if err := c.DB().Unscoped().Where("tenant_id = ?", "ghost").
		Delete(&KeycloakTenant{}).Error; err != nil {
		t.Fatalf("orphan the tenant: %v", err)
	}

	if err := c.migrateSeedTenantPurgeQueue(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var ghost, live int64
	c.DB().Model(&PendingTenantPurge{}).Where("tenant_id = ?", "ghost").Count(&ghost)
	c.DB().Model(&PendingTenantPurge{}).Where("tenant_id = ?", "live").Count(&live)
	if ghost != 1 {
		t.Errorf("pre-existing orphan not enqueued")
	}
	if live != 0 {
		t.Errorf("live tenant was enqueued for purge")
	}
}

// x NOT IN (empty set) is true for every row, so an empty tenants table would
// enqueue the entire database. "No tenants" is indistinguishable from "the
// tenant table failed to load", so seeding must skip.
func TestSeedSkipsWhenNoTenantsExist(t *testing.T) {
	c := newPurgeTestClient(t)
	seedTenant(t, c, "ghost")
	if err := c.DB().Unscoped().Where("1 = 1").Delete(&KeycloakTenant{}).Error; err != nil {
		t.Fatalf("clear tenants: %v", err)
	}

	if err := c.migrateSeedTenantPurgeQueue(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var queued int64
	c.DB().Model(&PendingTenantPurge{}).Count(&queued)
	if queued != 0 {
		t.Errorf("seeding enqueued %d tenants with an empty tenants table", queued)
	}
}
