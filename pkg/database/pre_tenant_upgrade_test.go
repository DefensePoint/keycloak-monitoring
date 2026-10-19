package database

import (
	_ "embed"
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// A database an early single-tenant server created has no tenant_id anywhere, and
// AutoMigrate alone could not take it forward. These tests rebuild that schema
// from what such a server really created, give it rows and compressed
// history, and boot the current client on it. They drop the public schema, so
// they read the destructive DSN.

//go:embed testdata/single_tenant_schema.sql
var singleTenantSchema string

func newPreTenantDatabase(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dsn := testdb.DSN(t, "KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	testdb.RequireDisposable(t, dsn)
	testdb.Lock(t, dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	closeOnCleanup(t, db)

	// The server runs on plain PostgreSQL as well, where those releases left the
	// time-series tables as ordinary tables. Where TimescaleDB is installed
	// they are built as their setupTimescaleDB built them, compressed
	// history included: the shape that kept these databases from upgrading.
	var timescale int64
	if err := db.Raw(`SELECT COUNT(*) FROM pg_available_extensions WHERE name = 'timescaledb'`).
		Scan(&timescale).Error; err != nil {
		t.Fatalf("check for TimescaleDB: %v", err)
	}
	stmts := []string{"DROP SCHEMA public CASCADE", "CREATE SCHEMA public"}
	if timescale > 0 {
		stmts = append(stmts, "CREATE EXTENSION IF NOT EXISTS timescaledb")
	}
	stmts = append(stmts, singleTenantSchema)
	if timescale > 0 {
		stmts = append(stmts,
			`SELECT create_hypertable('keycloak_events', 'time', chunk_time_interval => INTERVAL '1 day')`,
			`SELECT create_hypertable('keycloak_metrics', 'time', chunk_time_interval => INTERVAL '1 day')`,
			`SELECT create_hypertable('keycloak_health', 'time', chunk_time_interval => INTERVAL '1 day')`,
			`ALTER TABLE keycloak_events SET (timescaledb.compress, timescaledb.compress_segmentby = 'realm_name')`,
			`ALTER TABLE keycloak_metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'realm_name')`,
			`ALTER TABLE keycloak_health SET (timescaledb.compress)`)
	} else {
		t.Log("TimescaleDB is not installed: building the time-series tables as plain tables")
	}
	stmts = append(stmts,
		// One Keycloak's rows.
		`INSERT INTO keycloak_realms (realm_id, realm_name, created_at, updated_at, last_checked) VALUES ('r-blue', 'blue', now(), now(), now())`,
		`INSERT INTO configuration_alerts (alert_id, type, severity, status, title, resource_type, resource_id, resource_name, realm_name, check_type, first_detected, last_seen)
		 VALUES ('ssl-blue', 'security', 'critical', 'active', 'SSL', 'realm', 'blue', 'blue', 'blue', 'realm_security', now(), now())`,
		`INSERT INTO events (event_id, type, category, severity, description, source, source_system, status, timestamp, created_at, updated_at)
		 VALUES ('ev-1', 'LOGIN', 'authentication', 'info', 'x', 'keycloak:blue', 'keycloak', 'processed', now(), now(), now())`,
		`INSERT INTO keycloak_events (time, event_id, realm_id, realm_name, event_type, success)
		 SELECT now() - (g || ' days')::interval, 'kev-' || g, 'r-blue', 'blue', 'LOGIN', true FROM generate_series(1, 10) g`,
		`INSERT INTO keycloak_metrics (time, realm_name, total_users, enabled_users, disabled_users, active_sessions, offline_sessions, total_clients, login_events, logout_events, failed_login_events, register_events)
		 SELECT now() - (g || ' days')::interval, 'blue', 1, 1, 0, 0, 0, 1, 0, 0, 0, 0 FROM generate_series(1, 10) g`,
		`INSERT INTO keycloak_health (time, status, response_time, memory_used, memory_max, memory_free, uptime_millis)
		 SELECT now() - (g || ' days')::interval, 'healthy', 1, 1, 1, 1, 1 FROM generate_series(1, 10) g`)
	if timescale > 0 {
		// Older history already compressed.
		stmts = append(stmts,
			`SELECT compress_chunk(c) FROM show_chunks('keycloak_events', older_than => INTERVAL '2 days') c`,
			`SELECT compress_chunk(c) FROM show_chunks('keycloak_metrics', older_than => INTERVAL '2 days') c`,
			`SELECT compress_chunk(c) FROM show_chunks('keycloak_health', older_than => INTERVAL '2 days') c`)
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("build the single-tenant database: %v\n%.200s", err, stmt)
		}
	}
	return dsn
}

func bootClient(t *testing.T, dsn string, opts ...ClientOption) (*Client, error) {
	t.Helper()
	c, err := NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop(), opts...)
	if err == nil {
		t.Cleanup(func() { _ = c.Close() })
	}
	return c, err
}

func scalar[T any](t *testing.T, db *gorm.DB, sql string) T {
	t.Helper()
	var v T
	if err := db.Raw(sql).Scan(&v).Error; err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return v
}

func TestPreTenantUpgrade_GivesOneKeycloaksRowsToTheConfiguredTenant(t *testing.T) {
	dsn := newPreTenantDatabase(t)

	c, err := bootClient(t, dsn, WithPreTenantOwner("acme"))
	if err != nil {
		t.Fatalf("a single-tenant database could not be upgraded: %v", err)
	}
	db := c.DB()

	for _, table := range []string{"keycloak_realms", "configuration_alerts", "events"} {
		if got := scalar[string](t, db, "SELECT string_agg(DISTINCT tenant_id, ',') FROM "+table); got != "acme" {
			t.Errorf("%s rows belong to %q, want acme", table, got)
		}
	}

	// History is kept, set aside, and the new tables are per tenant.
	for _, table := range []string{"keycloak_events", "keycloak_metrics", "keycloak_health"} {
		if n := scalar[int64](t, db, "SELECT COUNT(*) FROM "+table+"_pre_tenant"); n != 10 {
			t.Errorf("%s_pre_tenant holds %d rows, want the 10 that were there", table, n)
		}
		pk := scalar[string](t, db, "SELECT indexdef FROM pg_indexes WHERE indexname = '"+table+"_pkey'")
		if !containsAll(pk, "tenant_id") {
			t.Errorf("%s primary key = %q, want one naming tenant_id", table, pk)
		}
	}

	// No unique index is left making a value unique across tenants.
	for _, idx := range []string{"idx_keycloak_realms_realm_id", "idx_keycloak_realms_realm_name", "idx_configuration_alerts_alert_id", "idx_events_event_id"} {
		if n := scalar[int64](t, db, "SELECT COUNT(*) FROM pg_indexes WHERE indexname = '"+idx+"'"); n != 0 {
			t.Errorf("%s survived the upgrade", idx)
		}
	}
	if err := db.Exec(`INSERT INTO keycloak_realms (tenant_id, realm_id, realm_name, created_at, updated_at, last_checked)
		VALUES ('beta', 'r-blue', 'blue', now(), now(), now())`).Error; err != nil {
		t.Errorf("a second tenant could not have a realm of the same id and name: %v", err)
	}

	// Every later start finds nothing to do.
	if _, err := bootClient(t, dsn); err != nil {
		t.Errorf("the upgraded database did not start again without an owner: %v", err)
	}
}

func TestPreTenantUpgrade_RefusesWithoutExactlyOneTenantAndChangesNothing(t *testing.T) {
	dsn := newPreTenantDatabase(t)

	_, err := bootClient(t, dsn)
	if !errors.Is(err, ErrPreTenantOwnerUnknown) {
		t.Fatalf("want ErrPreTenantOwnerUnknown, got %v", err)
	}

	db, oerr := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if oerr != nil {
		t.Fatalf("connect: %v", oerr)
	}
	closeOnCleanup(t, db)
	if n := scalar[int64](t, db, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = 'public' AND column_name = 'tenant_id'"); n != 0 {
		t.Errorf("the refused upgrade left %d tenant_id columns behind", n)
	}

	// Configured properly, the same database then upgrades.
	if _, err := bootClient(t, dsn, WithPreTenantOwner("acme")); err != nil {
		t.Errorf("upgrade after the configuration was fixed: %v", err)
	}
}

// Databases from early multi-tenant releases have tenants everywhere except events.
// That is not a pre-tenant database, and must start with any number of
// tenants configured.
func TestPreTenantUpgrade_LeavesADatabaseWithTenantsAlone(t *testing.T) {
	dsn := newPreTenantDatabase(t)
	c, err := bootClient(t, dsn, WithPreTenantOwner("acme"))
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if err := c.DB().Exec(`ALTER TABLE events DROP COLUMN tenant_id`).Error; err != nil {
		t.Fatalf("give events its early multi-tenant shape: %v", err)
	}
	_ = c.Close()

	if _, err := bootClient(t, dsn); err != nil {
		t.Errorf("a database whose only untenanted table is events was refused: %v", err)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
