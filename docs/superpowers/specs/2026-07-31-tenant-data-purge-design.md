# Tenant Data Purge

Design for removing all data owned by a tenant when that tenant is deleted.

## Problem

Deleting a tenant soft-deletes the `keycloak_tenants` row and nothing else. No
other table is touched, so every row the tenant owned is left behind
permanently.

This is not only untidy. Three tenant-scoped tables carry a unique index that
omits `tenant_id`, so orphaned rows actively block future tenants:

| table | index | unique on |
|---|---|---|
| `keycloak_realms` | `idx_keycloak_realms_tenant_realm_name` | `(tenant_id, realm_name)` since !74 |
| `configuration_alerts` | `idx_alert_tenant` | `alert_id` alone |
| `alert_rules` | `idx_alert_rules_rule_id` | `rule_id` alone |

Observed on a development instance: the tenant `dev-kc` was deleted on
2026-07-23 and left 28 realms, 4 alerts, 165 events, 1271 metrics and 230
health rows. A live tenant `dev-instance`, pointing at the same Keycloak and
reporting healthy, shows **zero realms**, because every realm name it tries to
save is still held by the deleted tenant's rows.

A second consequence: `keycloak_tenants.tenant_id` is unique across
soft-deleted rows, so a deleted tenant's ID cannot be reused. !72 works around
this for config-defined tenants; UI-created tenants remain unable to be
recreated under the same ID.

## Decisions

Taken during design, with the reasoning that produced them.

1. **Deletion purges owned data permanently.** Not archived, not retained.
2. **The `keycloak_tenants` row is hard-deleted.** Consistent with purging
   everything, frees the ID immediately, and fixes UI recreation as a side
   effect.
3. **No audit record.** Deletion is logged as a structured log line with
   per-table row counts. `operator_actions` was considered and rejected: it
   requires a non-null `AlertID` and records alert workflow timings, so a
   tenant deletion cannot be represented in it.
4. **Every tenant-scoped table is purged**, including alerts, alert rules,
   operator actions and RBAC grants. Leaving RBAC grants would let a recreated
   tenant with the same ID silently inherit permissions nobody granted.
5. **Split execution.** Tables that block reuse are purged synchronously in the
   delete request. Telemetry hypertables are drained at startup, because they
   are unbounded and block nothing.
6. **Telemetry cleanup is driven by a work queue**, not by scanning for
   orphans. Production row counts are unknown, so the design must be safe at
   any scale.

## Architecture

A table registry in `pkg/database`, alongside the existing migration code,
which already owns every model and all schema knowledge.

```go
// tenantScopedTables lists every table holding tenant-owned rows.
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

Every delete uses the same primitive: `DELETE FROM <table> WHERE tenant_id = $1`.

Raw SQL is deliberate. Four of these tables carry `deleted_at`, so a GORM
`Delete` would soft-delete them and achieve nothing. A raw `DELETE FROM` is
unconditionally a hard delete, and there is no `Unscoped()` to forget.

Equality on `tenant_id` is also deliberate. `alert_rules.tenant_id` and
`user_roles.tenant_id` are nullable, and NULL denotes a global row that must
never be purged. An equality predicate can never match NULL, so global rows are
protected structurally rather than by SQL three-valued logic. See
[Rejected alternatives](#rejected-alternatives).

### Purge, on tenant deletion

One transaction:

1. Hard-delete the `keycloak_tenants` row using `Unscoped()`
2. Delete the six non-telemetry tables by `tenant_id = $1`
3. Upsert a row into `pending_tenant_purges`

```go
// pkg/database
func (c *Client) PurgeTenant(ctx context.Context, tenantID string) (map[string]int64, error)
```

`tenant.Repository.Delete` keeps its signature and delegates to this, so
`tenant.Service` and the HTTP handler are unchanged. `tenant/postgres.Repository`
already holds a `*database.Client`, so no new wiring is required.

The `gorm.DeletedAt` field stays on the `KeycloakTenant` model. Reads already
filter `deleted_at IS NULL`, which remains trivially true, and removing the
field would change query generation across the codebase for no benefit.

### Telemetry queue

```sql
CREATE TABLE pending_tenant_purges (
    tenant_id    text PRIMARY KEY,
    requested_at timestamptz NOT NULL DEFAULT now()
);
```

`tenant_id` as primary key makes enqueueing idempotent across
delete/recreate/delete cycles.

### Drain, at startup

Runs early in `runBootstrap`, before `syncTenantsFromConfig`. For each queued
tenant, delete from the three telemetry hypertables in bounded batches:

```sql
DELETE FROM <table> WHERE tenant_id = $1 AND time < $requested_at
```

When all three return zero rows for a tenant, remove its queue row. If the batch
cap is reached, leave the row and stop; the next startup resumes.

**The `time < requested_at` bound is required for correctness, not efficiency.**
A tenant can be deleted and then recreated under the same ID, by config sync or
by hand, before the queue is drained. Without the bound, the drain would delete
the *recreated* tenant's fresh telemetry along with the old, silently destroying
history belonging to a live and healthy tenant. All three hypertables partition
on a column named `time`, so the bound is available on every one of them.

The bound also aligns deletes with hypertable chunk boundaries, which makes them
substantially cheaper than an unbounded scan and gives batching a natural axis.

Default cap: **50,000 rows per table per startup**, as a named constant rather
than a config value until there is evidence a deployment needs to tune it. At
that size a drain is a few seconds of work, and a tenant with millions of rows
converges over a handful of restarts while never delaying a boot.

A tenant is always enqueued on deletion, even when it owns no telemetry. The
first drain finds nothing, removes the queue row, and costs three indexed
lookups. That is cheaper than conditionally enqueueing based on a count.

Because the queue names tenants explicitly, the drain stays correct even if
`syncTenantsFromConfig` recreates that tenant in the same boot. Ordering before
config sync is preferable but no longer load-bearing.

### One-off seeding of existing orphans

Orphans predating the queue are enqueued once by a migration, running in
`Client.AutoMigrate` alongside the existing `migrateX` functions. At that point
the tenants table already holds every tenant from previous runs, and config sync
has not yet run, which is the correct moment to identify what is genuinely
orphaned.

```sql
INSERT INTO pending_tenant_purges (tenant_id)
SELECT DISTINCT tenant_id FROM <each registry table>
WHERE tenant_id IS NOT NULL
  AND tenant_id NOT IN (SELECT tenant_id FROM keycloak_tenants)
ON CONFLICT DO NOTHING;
```

This is the only place `NOT IN` appears and it runs once.

**It must be guarded by a non-zero tenant count.** `x NOT IN (empty set)` is
true for every row, so with an empty `keycloak_tenants` this would enqueue every
tenant in the system. "No tenants exist" is indistinguishable from "the tenant
table failed to load", so an empty result skips the seeding and logs a warning.

All nine registry tables are scanned, telemetry included, so a tenant whose only
remaining rows are telemetry is still found. This is the one place a `DISTINCT`
scan over the hypertables is paid, and it is paid once on upgrade rather than on
every startup.

## Error handling

| Failure | Behaviour |
|---|---|
| Purge transaction fails | Rolls back; tenant is not deleted; error surfaces to the caller; operator retries |
| Drain fails | Logged, startup continues; queue row survives and retries next boot |
| Seeding fails | Logged, startup continues; matches every existing migration in `client.go` |

Both purge and drain log per-table row counts at Info, so an unexpectedly large
deletion is visible rather than silent.

## Dead code removal

Hard-deleting the tenant row makes !72's restore path unreachable, because
`GetTenantIncludingDeleted` can never find a row. Remove in the same change:

- `Service.GetTenantIncludingDeleted`, `Service.RestoreTenant`
- `Repository.Restore`, `Repository.GetByTenantIDIncludingDeleted`
- the restore branch in `syncTenant`, and its tests

Config-defined tenants are then recreated by the existing `CreateTenant` path,
which works because the ID is genuinely free.

## Testing

Against a throwaway TimescaleDB container, matching the deployment image.

**Purge**
- Deletes exactly the target tenant's rows in all six non-telemetry tables
- Leaves other tenants' rows untouched
- Enqueues the tenant for telemetry drain
- Rolls back completely when any statement fails, leaving the tenant present
- Hard-deletes the tenant row: it is absent even under `Unscoped()`

**Global rows survive** (the trap a future `NOT IN` rewrite would reintroduce)
- `alert_rules` rows with `tenant_id IS NULL` survive purge and drain
- `user_roles` rows with `tenant_id IS NULL` survive purge and drain

**Drain**
- Removes telemetry for queued tenants only
- Removes the queue row when a tenant is fully drained
- Leaves the queue row and resumes next run when the batch cap is hit
- **A tenant deleted, recreated, and then drained keeps every telemetry row
  written after `requested_at`, and loses every row written before it.** This is
  the case the time bound exists for; without it the test fails by deleting the
  live tenant's data.

**Seeding**
- Enqueues pre-existing orphans
- Skips entirely, and logs, when `keycloak_tenants` is empty

**Regression discipline**: each test must fail against current `development`
before the change, as was done for !72.

## Resolved during implementation

**Batch deletion against a hypertable: `ctid` batching is unsafe, rejected.**
The generic Postgres idiom

```sql
DELETE FROM t WHERE ctid IN (SELECT ctid FROM t WHERE tenant_id = $1 LIMIT $2);
```

must never be used against a hypertable. `ctid` is a physical row pointer
unique only within a single heap, and a hypertable's parent is a view over many
chunk heaps, so the selected pointers collide with unrelated rows in every other
chunk. The outer `DELETE` then filters on `ctid` alone, with no tenant
predicate. Reproduced twice against TimescaleDB:

| layout | before | after |
|---|---|---|
| one tenant, 300 rows, `LIMIT 50` | 300 | **0** (limit silently ignored) |
| two tenants interleaved, 400 rows, `LIMIT 50` | 200 + 200 | **0 + 0** (both wiped) |

Draining one deleted tenant would have destroyed live tenants' telemetry.

The implementation instead walks a time window backwards from `requested_at`,
which maps onto chunk boundaries and carries the tenant predicate on every
statement. Note the batch cap is therefore approximate: a single window may
contain more rows than the cap, since Postgres `DELETE` has no `LIMIT`.

## Open items

**How to bound each drain batch.** The delete predicate is settled
(`tenant_id = $1 AND time < $requested_at`); what is unsettled is how to cap the
rows removed per pass.

The generic Postgres idiom uses `ctid`:

```sql
DELETE FROM t WHERE ctid IN (SELECT ctid FROM t WHERE tenant_id = $1 LIMIT $2);
```

On a TimescaleDB hypertable `ctid` is per-chunk and the parent is a view over
many chunks, so this may work, may touch only one chunk, or may error. It is
unverified.

Because the predicate is already time-bounded, a likely better approach is to
narrow the time window per pass, walking backwards from `requested_at`, which
maps directly onto chunk boundaries. Resolve during implementation by testing
both against a real hypertable with data spanning several chunks.

## Rejected alternatives

**Sweep by `NOT IN` instead of a queue.** Scanning each table for tenant IDs
absent from `keycloak_tenants`. Rejected on two counts. It costs a scan per
hypertable at every startup with unknown row counts. And global rows survive
only because `NULL NOT IN (...)` evaluates to NULL rather than true; the
standard performance rewrites, `LEFT JOIN ... WHERE parent IS NULL` and
`NOT EXISTS`, both delete NULL rows. Demonstrated:

```
NOT IN                    would delete: orphan
LEFT JOIN ... IS NULL     would delete: orphan, <NULL>
```

A future optimisation would silently start deleting global alert rules and
global role grants.

**Per-domain repository methods.** A `DeleteByTenant` on each of six domain
repositories, coordinated at the fx layer. Rejected: six interface changes, six
mocks and six implementations to express one identical statement, with no
per-domain logic to justify the separation.

**Domain-registered purge callbacks.** Extending the existing `ChangeCallback`
mechanism. Rejected: those callbacks are fire-and-forget goroutines with no
error handling, so it would need a parallel synchronous mechanism, which is
real complexity for a problem that is not about decoupling.

**Foreign keys with `ON DELETE CASCADE`.** Rejected: soft deletes do not fire
cascades, and it would mean adding FKs to hypertables.

No foreign key references any purge target, so delete ordering within the purge
transaction is not FK-sensitive. Verified on both an existing database and a
freshly migrated one: each has exactly the same six constraints, all pointing at
`users` or `roles`. Note that `OperatorAction` *declares*
`Alert *ConfigurationAlert` with `constraint:OnDelete:CASCADE`, but GORM does not
create it in either environment, so it cannot be relied on.

## Out of scope

Related defects found during this work, to be handled separately.

- **`configuration_alerts` and `alert_rules` unique indexes omit `tenant_id`.**
  Two live tenants sharing a realm name silently share config alerts. The purge
  does not fix this, because it only touches tenants that no longer exist.
- **!74's index divergence.** The realm migration writes a partial index while
  the model tag produces a full one, so fresh installs and upgraded databases
  differ. This becomes material once realms can be deleted.

- **Two tables hold per-tenant data without a `tenant_id` column**, so they fall
  outside the registry and are never purged. `events` gets one row per Keycloak
  event per tenant, written via `generalEventRepo.Save`. `notification_logs` is
  keyed by `alert_id`, which belongs to a tenant, so when a purge frees an
  `alert_id` a later tenant reusing it silently inherits the old notification
  history. Neither blocks anything: `events.Save` upserts on `event_id`, so no
  unique key is held hostage. Fixing them means adding a tenant column, which is
  a schema change of its own. Decision 4 above ("every tenant-scoped table is
  purged") should be read as "every table with a `tenant_id` column".

- **Pre-existing orphans are not cleaned automatically.** The old delete was a
  soft delete, so those tenant rows are still present and the seeding migration's
  raw subquery still sees them. Deliberate decision: they are handled by a manual
  script (`scripts/purge-orphaned-tenant-data.sql`) rather than by a migration
  that deletes production rows at startup.
