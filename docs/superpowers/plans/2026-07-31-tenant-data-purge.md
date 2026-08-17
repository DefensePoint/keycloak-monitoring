# Tenant Data Purge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deleting a tenant removes every row that tenant owned, so orphaned data can no longer block future tenants from saving realms, alerts or rules.

**Architecture:** A table registry in `pkg/database` drives two operations that share one delete primitive (`WHERE tenant_id = $1`). Deletion hard-deletes the tenant row plus six non-telemetry tables in a single transaction and enqueues the tenant in `pending_tenant_purges`. Startup drains that queue, deleting from three TimescaleDB hypertables with a `time < requested_at` bound so a recreated tenant's fresh data survives.

**Tech Stack:** Go 1.26, GORM v1.31, PostgreSQL 16 / TimescaleDB, uber-go/fx, testify-free stdlib tests.

**Spec:** `docs/superpowers/specs/2026-07-31-tenant-data-purge-design.md`

## Global Constraints

- **Never use GORM's `Delete` on the registry tables.** Four carry `deleted_at` (keycloak_realms, configuration_alerts, alert_rules, tenant_policies); a GORM `Delete` soft-deletes and achieves nothing. Use `tx.Exec("DELETE FROM ...")`, or `Unscoped().Delete()` for the tenant row specifically.
- **Only ever match tenants by equality (`tenant_id = ?`).** `alert_rules.tenant_id` and `user_roles.tenant_id` are nullable, and NULL means a global row that must never be purged. Equality cannot match NULL. `NOT IN` appears exactly once, in the one-off seeding migration in Task 6.
- **Table names come only from the `tenantScopedTables` constant**, never from a parameter, since they are interpolated into SQL.
- **Database tests are guarded by `testing.Short()`**, matching `tests/alerts_integration_test.go:28`. CI runs `go test -short`, so they skip there.
- **Commit messages use conventional commits** (`fix:`, `feat:`, `refactor:`, `test:`). A commitlint `commit-msg` hook enforces this.
- **`gofmt` and `golangci-lint` must stay clean.** `development` is currently at 0 violations and 0 lint issues; do not regress it.
- Test database for local runs: TimescaleDB on **port 55433**, database `reviewdb`, user `postgres`, password `review`.

---

## File Structure

**Create:**
- `pkg/database/tenant_purge.go` — the registry, `PurgeTenant`, `DrainPendingPurges`, and the seeding migration. All four are the same concern (which tables hold tenant data and how they are emptied) and change together.
- `pkg/database/tenant_purge_test.go` — database-backed tests for the above.

**Modify:**
- `pkg/database/models.go` — add the `PendingTenantPurge` model
- `pkg/database/client.go` — register the new model in `AutoMigrate`, call the seeding migration
- `tenant/postgres/repository.go` — `Delete` delegates to `PurgeTenant`; remove `Restore` and `GetByTenantIDIncludingDeleted`
- `tenant/repository.go` — remove `Restore` and `GetByTenantIDIncludingDeleted` from the interface
- `tenant/service.go` — remove `GetTenantIncludingDeleted` and `RestoreTenant`
- `internal/fx/bootstrap.go` — add `DB` to `BootstrapParams`, drain before config sync, remove the restore branch
- `tenant/service_test.go`, `tenant/monitor_pool_test.go`, `internal/fx/bootstrap_test.go` — drop mocks for removed methods

---

## Task 1: Queue model and table registry

**Files:**
- Create: `pkg/database/tenant_purge.go`
- Create: `pkg/database/tenant_purge_test.go`
- Modify: `pkg/database/models.go` (append at end of file)
- Modify: `pkg/database/client.go:97` (AutoMigrate model list)

**Interfaces:**
- Consumes: nothing
- Produces: `PendingTenantPurge` struct; `tenantScopedTables []tenantTable` with fields `name string` and `telemetry bool`

- [ ] **Step 1: Write the failing test**

Create `pkg/database/tenant_purge_test.go`:

```go
package database

import (
	"testing"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const testDSN = "host=localhost port=55433 user=postgres password=review dbname=reviewdb sslmode=disable"

func testDBConfig() *config.DatabaseConfig {
	return &config.DatabaseConfig{
		Host: "localhost", Port: 55433, Database: "reviewdb",
		User: "postgres", Password: "review", SSLMode: "disable",
		MaxConns: 5, MinConns: 1, Timeout: 30 * time.Second,
	}
}

// newPurgeTestClient wipes the schema and runs the real AutoMigrate, giving each
// test a database shaped exactly like a fresh install.
func newPurgeTestClient(t *testing.T) *Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	raw, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := raw.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	c, err := NewClient(testDBConfig(), logger.NewNoop())
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
```

- [ ] **Step 2: Run the test to verify it fails**

Start the test database first:

```bash
docker run -d --rm --name point-purge-test \
  -e POSTGRES_PASSWORD=review -e POSTGRES_DB=reviewdb \
  -p 55433:5432 timescale/timescaledb:latest-pg16
until docker exec point-purge-test pg_isready -U postgres; do sleep 1; done
```

Run: `go test ./pkg/database/ -run 'TestPendingTenantPurge|TestTenantScopedTables' -v`
Expected: FAIL to compile with `undefined: tenantScopedTables`

- [ ] **Step 3: Add the model**

Append to `pkg/database/models.go`:

```go
// PendingTenantPurge is a work queue entry for telemetry belonging to a deleted
// tenant. Rows are written inside the deletion transaction and removed once the
// tenant's telemetry has been fully drained at startup.
type PendingTenantPurge struct {
	TenantID    string    `gorm:"primaryKey;size:100"`
	RequestedAt time.Time `gorm:"not null"`
}

// TableName specifies the table name for PendingTenantPurge.
func (PendingTenantPurge) TableName() string {
	return "pending_tenant_purges"
}
```

- [ ] **Step 4: Register the model in AutoMigrate**

In `pkg/database/client.go`, add `&PendingTenantPurge{}` to the first `AutoMigrate` call (currently line 97):

```go
err := c.db.AutoMigrate(&Event{}, &User{}, &KeycloakTenant{}, &KeycloakRealmInfo{}, &Alert{}, &AlertRule{}, &NotificationLog{}, &PendingTenantPurge{})
```

- [ ] **Step 5: Create the registry**

Create `pkg/database/tenant_purge.go`:

```go
package database

// tenantTable describes a table holding rows owned by a tenant.
//
// Telemetry tables are TimescaleDB hypertables with no retention policy, so
// they can grow without bound. They are excluded from the synchronous purge
// and drained at startup instead, because unlike the others they block nothing.
type tenantTable struct {
	name      string
	telemetry bool
}

// tenantScopedTables lists every table holding tenant-owned rows.
//
// Deletes against these tables always use raw SQL with an equality predicate on
// tenant_id. Raw SQL because four of them carry deleted_at, so a GORM Delete
// would soft-delete and free nothing. Equality because alert_rules.tenant_id and
// user_roles.tenant_id are nullable, where NULL denotes a global row that must
// never be purged, and an equality test can never match NULL.
var tenantScopedTables = []tenantTable{
	{name: "keycloak_realms"},
	{name: "configuration_alerts"},
	{name: "alert_rules"},
	{name: "operator_actions"},
	{name: "tenant_policies"},
	{name: "user_roles"},
	{name: "keycloak_events", telemetry: true},
	{name: "keycloak_metrics", telemetry: true},
	{name: "keycloak_health", telemetry: true},
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./pkg/database/ -run 'TestPendingTenantPurge|TestTenantScopedTables' -v`
Expected: PASS, both tests

- [ ] **Step 7: Verify the short-mode skip works**

Run: `go test ./pkg/database/ -short -run 'TestPendingTenantPurge|TestTenantScopedTables' -v`
Expected: PASS with `SKIP` for both, since CI runs in short mode

- [ ] **Step 8: Commit**

```bash
gofmt -l pkg/database/ && go vet ./pkg/database/
git add pkg/database/tenant_purge.go pkg/database/tenant_purge_test.go pkg/database/models.go pkg/database/client.go
git commit -m "feat(database): add tenant purge queue model and table registry"
```

---

## Task 2: PurgeTenant transaction

**Files:**
- Modify: `pkg/database/tenant_purge.go`
- Modify: `pkg/database/tenant_purge_test.go`

**Interfaces:**
- Consumes: `tenantScopedTables`, `PendingTenantPurge` from Task 1
- Produces: `func (c *Client) PurgeTenant(ctx context.Context, tenantID string) (map[string]int64, error)` returning per-table deleted row counts keyed by table name, including `"keycloak_tenants"`

- [ ] **Step 1: Write the failing tests**

Append to `pkg/database/tenant_purge_test.go`:

```go
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
		`INSERT INTO configuration_alerts (tenant_id, alert_id, source, type, severity, status, title, created_at, updated_at)
		 VALUES (?, ?, 'configuration', 'security', 'warning', 'active', 'x', now(), now())`,
		`INSERT INTO alert_rules (tenant_id, rule_id, name, created_at, updated_at)
		 VALUES (?, ?, 'x', now(), now())`,
		`INSERT INTO tenant_policies (tenant_id, user_id, created_at, updated_at)
		 VALUES (?, 1, now(), now())`,
	}
	args := [][]interface{}{
		{tenantID, tenantID + "-realm", tenantID + "-realm"},
		{tenantID, tenantID + "-alert"},
		{tenantID, tenantID + "-rule"},
		{tenantID},
	}
	for i, s := range stmts {
		if err := db.Exec(s, args[i]...).Error; err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	if err := db.Exec(
		`INSERT INTO keycloak_metrics (tenant_id, time) VALUES (?, now())`, tenantID).Error; err != nil {
		t.Fatalf("seed metrics: %v", err)
	}
	// Tenant-scoped role grant, distinct from the global one used in
	// TestPurgeTenantSparesGlobalRows.
	if err := db.Exec(
		`INSERT INTO user_roles (user_id, role_id, tenant_id, created_at, updated_at)
		 VALUES (1, 1, ?, now(), now())`, tenantID).Error; err != nil {
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

	for _, table := range []string{"keycloak_realms", "configuration_alerts", "alert_rules", "tenant_policies", "user_roles"} {
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
		`INSERT INTO alert_rules (tenant_id, rule_id, name, created_at, updated_at)
		 VALUES (NULL, 'global-rule', 'global', now(), now())`).Error; err != nil {
		t.Fatalf("seed global rule: %v", err)
	}
	if err := c.DB().Exec(
		`INSERT INTO user_roles (user_id, role_id, tenant_id, created_at, updated_at)
		 VALUES (1, 1, NULL, now(), now())`).Error; err != nil {
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
```

Add `"context"` to the import block.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/database/ -run TestPurgeTenant -v`
Expected: FAIL to compile with `c.PurgeTenant undefined`

- [ ] **Step 3: Implement PurgeTenant**

Append to `pkg/database/tenant_purge.go`:

```go
import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
)

// PurgeTenant hard-deletes a tenant and every non-telemetry row it owns, in a
// single transaction, and enqueues the tenant for telemetry drain at startup.
//
// Telemetry hypertables are excluded deliberately: they are unbounded and block
// nothing, so deleting them inside a request could take minutes. See
// DrainPendingPurges.
//
// On any failure the transaction rolls back and the tenant still exists, so the
// caller can retry. There is no partial state to reconcile.
func (c *Client) PurgeTenant(ctx context.Context, tenantID string) (map[string]int64, error) {
	counts := make(map[string]int64, len(tenantScopedTables)+1)

	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Unscoped().Where("tenant_id = ?", tenantID).Delete(&KeycloakTenant{})
		if res.Error != nil {
			return fmt.Errorf("failed to delete tenant row: %w", res.Error)
		}
		counts["keycloak_tenants"] = res.RowsAffected

		for _, table := range tenantScopedTables {
			if table.telemetry {
				continue
			}
			// table.name comes from a package constant, never from input.
			r := tx.Exec("DELETE FROM "+table.name+" WHERE tenant_id = ?", tenantID)
			if r.Error != nil {
				return fmt.Errorf("failed to purge %s: %w", table.name, r.Error)
			}
			counts[table.name] = r.RowsAffected
		}

		return tx.Exec(`
			INSERT INTO pending_tenant_purges (tenant_id, requested_at)
			VALUES (?, ?)
			ON CONFLICT (tenant_id) DO UPDATE SET requested_at = EXCLUDED.requested_at`,
			tenantID, time.Now()).Error
	})
	if err != nil {
		return nil, err
	}

	fields := make([]logger.Field, 0, len(counts)+1)
	fields = append(fields, logger.Str("tenant_id", tenantID))
	for table, n := range counts {
		fields = append(fields, logger.Int64(table, n))
	}
	c.logger.Info("Purged tenant data", fields...)

	return counts, nil
}
```

If `logger.Int64` does not exist, check `internal/logger` for the available field constructors and use the integer one that does.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/database/ -run TestPurgeTenant -v`
Expected: PASS, all four tests

- [ ] **Step 5: Commit**

```bash
gofmt -l pkg/database/ && go vet ./pkg/database/
git add pkg/database/tenant_purge.go pkg/database/tenant_purge_test.go
git commit -m "feat(database): purge tenant-owned rows atomically on deletion"
```

---

## Task 3: Wire the purge into tenant deletion

**Files:**
- Modify: `tenant/postgres/repository.go` (the `Delete` method)
- Test: `tenant/postgres/repository_purge_test.go` (create)

**Interfaces:**
- Consumes: `(*database.Client).PurgeTenant` from Task 2
- Produces: no signature change. `tenant.Repository.Delete` keeps `Delete(ctx context.Context, tenantID string) error` and now purges.

- [ ] **Step 1: Write the failing test**

Create `tenant/postgres/repository_purge_test.go`:

```go
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
	repo := NewRepository(client)
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
	repo := NewRepository(client)
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tenant/postgres/ -run TestRepositoryDelete -v`
Expected: FAIL. `TestRepositoryDeletePurgesOwnedData` fails on surviving realms; `TestRepositoryDeleteFreesTenantID` fails with a duplicate key violation on `idx_keycloak_tenants_tenant_id`.

- [ ] **Step 3: Delegate Delete to PurgeTenant**

Replace the `Delete` method in `tenant/postgres/repository.go`:

```go
// Delete removes a tenant and every row it owns.
//
// This is a hard delete, not a soft delete: leftover rows from a deleted tenant
// hold unique keys (realm names, alert IDs, rule IDs) that would block any future
// tenant, and a soft-deleted tenant row would keep its tenant_id reserved.
// Telemetry is queued for drain at startup rather than deleted here.
func (r *Repository) Delete(ctx context.Context, tenantID string) error {
	if _, err := r.db.PurgeTenant(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}
	return nil
}
```

Remove the now-unused `time` import if nothing else in the file uses it.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./tenant/postgres/ -run TestRepositoryDelete -v`
Expected: PASS, both tests

- [ ] **Step 5: Run the existing tenant tests for regressions**

Run: `go test ./tenant/... -v`
Expected: PASS. `TestDeleteTenant` in `tenant/service_test.go` uses a mock repository and is unaffected.

- [ ] **Step 6: Commit**

```bash
gofmt -l tenant/ && go vet ./tenant/...
git add tenant/postgres/repository.go tenant/postgres/repository_purge_test.go
git commit -m "feat(tenants): purge owned data when a tenant is deleted"
```

---

## Task 4: Drain the telemetry queue

**Files:**
- Modify: `pkg/database/tenant_purge.go`
- Modify: `pkg/database/tenant_purge_test.go`

**Interfaces:**
- Consumes: `PendingTenantPurge`, `tenantScopedTables` from Task 1
- Produces: `func (c *Client) DrainPendingPurges(ctx context.Context) (map[string]int64, error)` returning per-table deleted row counts; `const drainBatchLimit = 50000`

**Before writing code, resolve the open item from the spec.** The delete predicate is settled; how to cap rows per pass is not.

- [ ] **Step 1: Determine the batching mechanism experimentally**

Create a scratch file `pkg/database/zz_batch_probe_test.go`:

```go
package database

import (
	"fmt"
	"testing"
	"time"
)

// Probe only. Delete this file once the batching approach is chosen.
func TestProbeHypertableBatching(t *testing.T) {
	c := newPurgeTestClient(t)
	db := c.DB()

	// Spread rows across several chunks; the default chunk interval is 7 days.
	for i := 0; i < 300; i++ {
		ts := time.Now().Add(-time.Duration(i) * 24 * time.Hour)
		if err := db.Exec(
			`INSERT INTO keycloak_metrics (tenant_id, time) VALUES ('probe', ?)`, ts).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	res := db.Exec(`
		DELETE FROM keycloak_metrics
		WHERE ctid IN (SELECT ctid FROM keycloak_metrics WHERE tenant_id = 'probe' LIMIT 50)`)
	fmt.Printf("ctid batching: err=%v rowsAffected=%d\n", res.Error, res.RowsAffected)

	var remaining int64
	db.Raw(`SELECT count(*) FROM keycloak_metrics WHERE tenant_id='probe'`).Scan(&remaining)
	fmt.Printf("remaining after ctid attempt: %d (started at 300)\n", remaining)
}
```

Run: `go test ./pkg/database/ -run TestProbeHypertableBatching -v`

Record the outcome. If `ctid` batching deletes exactly 50 rows and errors are nil, use it in Step 3. If it errors or deletes an unexpected count, use the time-window fallback shown in Step 3 instead. Delete the probe file either way before committing.

- [ ] **Step 2: Write the failing tests**

Append to `pkg/database/tenant_purge_test.go`:

```go
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

	// Recreated, and now writing new telemetry.
	if err := c.DB().Exec(
		`INSERT INTO keycloak_metrics (tenant_id, time) VALUES ('recycled', now())`).Error; err != nil {
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
	if err := c.DB().Exec(`
		INSERT INTO keycloak_metrics (tenant_id, time)
		SELECT 'big', now() - interval '1 hour'
		FROM generate_series(1, ?)`, drainBatchLimit+1).Error; err != nil {
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
}
```

- [ ] **Step 3: Implement DrainPendingPurges**

Append to `pkg/database/tenant_purge.go`:

```go
// drainBatchLimit caps how many rows are deleted per table per tenant per
// startup. A tenant with more telemetry than this converges over several
// restarts rather than delaying a boot. Deliberately a constant: make it
// configurable only when a deployment demonstrates it needs tuning.
const drainBatchLimit = 50000

// DrainPendingPurges deletes telemetry belonging to tenants queued by
// PurgeTenant, and removes each queue row once that tenant is fully drained.
//
// Deletes are bounded by time < requested_at. This is required for correctness,
// not speed: a tenant can be deleted and recreated under the same ID before the
// queue drains, and the recreated tenant's newer rows must survive.
func (c *Client) DrainPendingPurges(ctx context.Context) (map[string]int64, error) {
	var queued []PendingTenantPurge
	if err := c.db.WithContext(ctx).Find(&queued).Error; err != nil {
		return nil, fmt.Errorf("failed to read purge queue: %w", err)
	}
	if len(queued) == 0 {
		return nil, nil
	}

	counts := make(map[string]int64)
	for _, entry := range queued {
		drained := true
		for _, table := range tenantScopedTables {
			if !table.telemetry {
				continue
			}
			// table.name comes from a package constant, never from input.
			res := c.db.WithContext(ctx).Exec(
				"DELETE FROM "+table.name+
					" WHERE ctid IN (SELECT ctid FROM "+table.name+
					" WHERE tenant_id = ? AND time < ? LIMIT ?)",
				entry.TenantID, entry.RequestedAt, drainBatchLimit)
			if res.Error != nil {
				return counts, fmt.Errorf("failed to drain %s for %s: %w", table.name, entry.TenantID, res.Error)
			}
			counts[table.name] += res.RowsAffected
			if res.RowsAffected >= drainBatchLimit {
				drained = false
			}
		}
		if drained {
			if err := c.db.WithContext(ctx).
				Where("tenant_id = ?", entry.TenantID).
				Delete(&PendingTenantPurge{}).Error; err != nil {
				return counts, fmt.Errorf("failed to dequeue %s: %w", entry.TenantID, err)
			}
		}
	}

	c.logger.Info("Drained tenant telemetry",
		logger.Int("tenants", len(queued)))
	return counts, nil
}
```

If the Step 1 probe showed `ctid` batching misbehaves on hypertables, replace the delete with a time-window walk instead, deleting one chunk interval at a time backwards from `requested_at`:

```go
res := c.db.WithContext(ctx).Exec(
	"DELETE FROM "+table.name+" WHERE tenant_id = ? AND time >= ? AND time < ?",
	entry.TenantID, windowStart, windowEnd)
```

where `windowEnd` starts at `entry.RequestedAt` and each pass moves back by seven days, stopping after `drainBatchLimit` rows or when no rows remain below the window.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/database/ -run TestDrain -v`
Expected: PASS, all three tests

- [ ] **Step 5: Remove the probe file**

```bash
rm pkg/database/zz_batch_probe_test.go
```

- [ ] **Step 6: Commit**

```bash
gofmt -l pkg/database/ && go vet ./pkg/database/
git add pkg/database/tenant_purge.go pkg/database/tenant_purge_test.go
git commit -m "feat(database): drain queued tenant telemetry at startup"
```

---

## Task 5: Run the drain at startup

**Files:**
- Modify: `internal/fx/bootstrap.go` (`BootstrapParams` struct, `runBootstrap`)
- Modify: `internal/fx/bootstrap_test.go`

**Interfaces:**
- Consumes: `(*database.Client).DrainPendingPurges` from Task 4
- Produces: `BootstrapParams.DB *database.Client`

- [ ] **Step 1: Write the failing test**

Append to `internal/fx/bootstrap_test.go`:

```go
func TestRunBootstrapDrainsBeforeConfigSync(t *testing.T) {
	// A nil DB must not panic; bootstrap should skip the drain and continue.
	p := BootstrapParams{
		Logger:        logger.NewNoop(),
		TenantService: &stubTenantService{},
		DB:            nil,
	}
	if err := drainPurgeQueue(context.Background(), p); err != nil {
		t.Errorf("drainPurgeQueue with nil DB = %v, want nil", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fx/ -run TestRunBootstrapDrains -v`
Expected: FAIL to compile, `unknown field DB` and `undefined: drainPurgeQueue`

- [ ] **Step 3: Add DB to BootstrapParams**

In `internal/fx/bootstrap.go`, add to the `BootstrapParams` struct:

```go
	DB                *database.Client
```

Add the import `"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/database"`. `*database.Client` is already provided by `provideDatabase` in `internal/fx/database.go`, so fx will inject it without further wiring.

- [ ] **Step 4: Add the drain step**

Add to `internal/fx/bootstrap.go`:

```go
// drainPurgeQueue removes telemetry belonging to deleted tenants. It runs before
// syncTenantsFromConfig so a config-defined tenant that was deleted has its old
// telemetry removed before config recreates it. A failure here is never fatal:
// the queue rows survive and the next startup retries.
func drainPurgeQueue(ctx context.Context, p BootstrapParams) error {
	if p.DB == nil {
		return nil
	}
	counts, err := p.DB.DrainPendingPurges(ctx)
	if err != nil {
		p.Logger.Warn("Failed to drain tenant purge queue, will retry next startup", logger.Err(err))
		return nil
	}
	for table, n := range counts {
		if n > 0 {
			p.Logger.Info("Drained tenant telemetry",
				logger.Str("table", table), logger.Int64("rows", n))
		}
	}
	return nil
}
```

Call it in `runBootstrap`, immediately before step 3:

```go
	// 3. Remove telemetry belonging to deleted tenants, before config sync can
	//    recreate any of them.
	if err := drainPurgeQueue(ctx, p); err != nil {
		p.Logger.Error("Failed to drain purge queue", logger.Err(err))
	}

	// 4. Sync tenants from config to database
	if err := syncTenantsFromConfig(ctx, p); err != nil {
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/fx/ -v`
Expected: PASS, including the existing `TestSyncTenant_*` tests

- [ ] **Step 6: Commit**

```bash
gofmt -l internal/fx/ && go vet ./internal/fx/
git add internal/fx/bootstrap.go internal/fx/bootstrap_test.go
git commit -m "feat(bootstrap): drain tenant purge queue before config sync"
```

---

## Task 6: Seed the queue with pre-existing orphans

**Files:**
- Modify: `pkg/database/tenant_purge.go`
- Modify: `pkg/database/client.go` (call the migration in `AutoMigrate`)
- Modify: `pkg/database/tenant_purge_test.go`

**Interfaces:**
- Consumes: `tenantScopedTables`, `PendingTenantPurge`
- Produces: `func (c *Client) migrateSeedTenantPurgeQueue() error`

- [ ] **Step 1: Write the failing tests**

Append to `pkg/database/tenant_purge_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/database/ -run TestSeed -v`
Expected: FAIL to compile, `c.migrateSeedTenantPurgeQueue undefined`

- [ ] **Step 3: Implement the seeding migration**

Append to `pkg/database/tenant_purge.go`:

```go
// migrateSeedTenantPurgeQueue enqueues tenants that own rows but no longer have
// a tenant record. These are orphans left by the old delete behaviour, which
// removed only the tenant row.
//
// This is the only place NOT IN is used, and it runs once per upgrade. It is
// guarded by a non-zero tenant count because x NOT IN (empty set) is true for
// every row: with an empty tenants table this would enqueue every tenant in the
// database, and "no tenants exist" cannot be distinguished from "the tenants
// table failed to load".
func (c *Client) migrateSeedTenantPurgeQueue() error {
	var tenantCount int64
	if err := c.db.Model(&KeycloakTenant{}).Count(&tenantCount).Error; err != nil {
		return fmt.Errorf("failed to count tenants: %w", err)
	}
	if tenantCount == 0 {
		c.logger.Warn("Skipping tenant purge queue seeding: no tenants found, cannot determine ownership")
		return nil
	}

	for _, table := range tenantScopedTables {
		// table.name comes from a package constant, never from input.
		res := c.db.Exec(`
			INSERT INTO pending_tenant_purges (tenant_id, requested_at)
			SELECT DISTINCT tenant_id, now() FROM `+table.name+`
			WHERE tenant_id IS NOT NULL
			  AND tenant_id NOT IN (SELECT tenant_id FROM keycloak_tenants)
			ON CONFLICT (tenant_id) DO NOTHING`)
		if res.Error != nil {
			return fmt.Errorf("failed to seed purge queue from %s: %w", table.name, res.Error)
		}
		if res.RowsAffected > 0 {
			c.logger.Info("Enqueued orphaned tenants for purge",
				logger.Str("table", table.name),
				logger.Int64("tenants", res.RowsAffected))
		}
	}
	return nil
}
```

- [ ] **Step 4: Call it from AutoMigrate**

In `pkg/database/client.go`, after the `migrateRealmNameIndex` call:

```go
	// Data migration: enqueue orphaned data left by the old tenant delete
	if err := c.migrateSeedTenantPurgeQueue(); err != nil {
		c.logger.Warn("Failed to seed tenant purge queue, continuing", logger.Err(err))
	}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/database/ -run TestSeed -v`
Expected: PASS, both tests

- [ ] **Step 6: Run the whole package**

Run: `go test ./pkg/database/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
gofmt -l pkg/database/ && go vet ./pkg/database/
git add pkg/database/tenant_purge.go pkg/database/tenant_purge_test.go pkg/database/client.go
git commit -m "feat(database): enqueue pre-existing orphaned tenant data on upgrade"
```

---

## Task 7: Remove the unreachable restore path

Hard-deleting the tenant row means `GetTenantIncludingDeleted` can never find one, so !72's restore branch is dead code. Config-defined tenants are recreated by `CreateTenant`, which now succeeds because the ID is free.

**Files:**
- Modify: `internal/fx/bootstrap.go` (restore branch in `syncTenant`)
- Modify: `tenant/service.go` (interface and implementation)
- Modify: `tenant/repository.go` (interface)
- Modify: `tenant/postgres/repository.go` (implementations)
- Modify: `internal/fx/bootstrap_test.go`, `tenant/service_test.go`, `tenant/monitor_pool_test.go` (mocks)

**Interfaces:**
- Consumes: nothing
- Produces: `tenant.Service` without `GetTenantIncludingDeleted` and `RestoreTenant`; `tenant.Repository` without `Restore` and `GetByTenantIDIncludingDeleted`

- [ ] **Step 1: Write the failing test**

Replace `TestSyncTenant_RestoresSoftDeletedTenant` in `internal/fx/bootstrap_test.go` with:

```go
// A tenant that no longer exists is recreated, not restored. Hard deletion frees
// the tenant_id, so CreateTenant succeeds where it previously collided.
func TestSyncTenant_RecreatesDeletedTenant(t *testing.T) {
	svc := &stubTenantService{} // no live tenant, no deleted row

	if err := syncTenant(context.Background(), params(svc), "t1", testCfgTenant()); err != nil {
		t.Fatalf("syncTenant() unexpected error = %v", err)
	}
	if !contains(svc.calls, "CreateTenant") {
		t.Errorf("expected CreateTenant, calls = %v", svc.calls)
	}
	if contains(svc.calls, "RestoreTenant") || contains(svc.calls, "GetTenantIncludingDeleted") {
		t.Errorf("restore path should no longer exist, calls = %v", svc.calls)
	}
}
```

Delete `TestSyncTenant_RollsBackRestoreWhenUpdateFails`, `TestSyncTenant_ReportsUpdateErrorWhenRollbackFails` and `TestSyncTenant_RestoreFailureStopsThere`: they test a branch that no longer exists.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fx/ -run TestSyncTenant_RecreatesDeletedTenant -v`
Expected: FAIL, `calls` contains `GetTenantIncludingDeleted`

- [ ] **Step 3: Remove the restore branch from syncTenant**

In `internal/fx/bootstrap.go`, delete the whole block that begins:

```go
	if deletedTenant, derr := p.TenantService.GetTenantIncludingDeleted(ctx, tenantID); derr == nil && deletedTenant != nil {
```

and ends just before the `// Tenant doesn't exist - create it` comment. Control now falls straight from the live-tenant update branch to the create branch.

- [ ] **Step 4: Remove the service methods**

In `tenant/service.go`, delete from the `Service` interface:

```go
	GetTenantIncludingDeleted(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error)
	RestoreTenant(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error)
```

and delete both method implementations.

- [ ] **Step 5: Remove the repository methods**

In `tenant/repository.go`, delete `GetByTenantIDIncludingDeleted` from `Reader` and `Restore` from `Writer`. In `tenant/postgres/repository.go`, delete both implementations. Remove any import left unused.

- [ ] **Step 6: Update the mocks**

In `tenant/service_test.go`, remove the `restoreFn` and `getByTenantIDIncludingDeletedFn` fields from `mockRepository` and the two methods implementing them. Remove `TestRestoreTenant`, `TestRestoreTenantNotifiesCallbacks` and `TestGetTenantIncludingDeleted`.

In `tenant/monitor_pool_test.go`, remove the `GetTenantIncludingDeleted` and `RestoreTenant` methods from `mockService`.

In `internal/fx/bootstrap_test.go`, remove the `GetTenantIncludingDeleted` and `RestoreTenant` methods from `stubTenantService`, plus the `deletedRow`, `restoreErr` and `restoredThen` fields and every reference to them.

- [ ] **Step 7: Run the full build and test suite**

Run: `go build ./... && go test ./... 2>&1 | grep -E "^(FAIL|ok)"`
Expected: no FAIL lines. The database tests run if the container is up.

- [ ] **Step 8: Verify CI-equivalent mode**

Run: `go test -short ./... 2>&1 | grep -E "^FAIL"`
Expected: no output

- [ ] **Step 9: Commit**

```bash
gofmt -l . | grep -v '^$' ; go vet ./...
git add -A
git commit -m "refactor(tenants): remove unreachable restore path after hard delete"
```

---

## Final verification

- [ ] **Step 1: Full CI-equivalent run**

```bash
gofmt -l . | wc -l          # expect 0
go vet ./...                # expect clean
go test -short ./...        # expect no FAIL
```

- [ ] **Step 2: Full run with the database**

```bash
go test ./... 2>&1 | grep -E "^(FAIL|ok)"
```

- [ ] **Step 3: Lint with the CI version**

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0
golangci-lint run --timeout 5m     # expect "0 issues"
```

- [ ] **Step 4: Tear down the test database**

```bash
docker stop point-purge-test
```

- [ ] **Step 5: Confirm the original defect is fixed**

On a database with an orphaned tenant (for example `dev-kc` holding 28 realms while `dev-instance` shows none), start the service and confirm the seeding migration enqueues the orphan, the drain removes its telemetry, and the live tenant saves its realms on the next poll.

---

## Out of scope

Tracked separately, and deliberately not addressed here:

- `configuration_alerts.idx_alert_tenant` and `alert_rules.idx_alert_rules_rule_id` omit `tenant_id`, so two live tenants sharing a realm name silently share config alerts. The purge does not fix this, because it only touches tenants that no longer exist.
- !74's index divergence: the realm migration writes a partial index while the model tag produces a full one, so fresh and upgraded databases differ.
