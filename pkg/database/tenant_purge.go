package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

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
//
// Order matters: children precede their parents. OperatorAction declares
// Alert *ConfigurationAlert with constraint:OnDelete:CASCADE; GORM does not
// currently create that foreign key, but if it ever did, deleting the parent
// first would fail every tenant delete.
var tenantScopedTables = []tenantTable{
	{name: "keycloak_realms"},
	{name: "operator_actions"},
	{name: "configuration_alerts"},
	{name: "alert_rules"},
	{name: "tenant_policies"},
	{name: "user_roles"},
	// Mirrored Keycloak and AMFA login events. Until events carried a
	// tenant_id there was nothing to purge by, so a deleted tenant's usernames,
	// emails, source IPs and coordinates outlived it indefinitely.
	{name: "events"},
	// One tiny row per (tenant, realm), so it is deleted inline rather than
	// drained: leaving it behind would hand a re-created tenant of the same id
	// a stale read position and skip its events.
	{name: "amfa_mirror_watermarks"},
	{name: "keycloak_events", telemetry: true},
	{name: "keycloak_metrics", telemetry: true},
	{name: "keycloak_health", telemetry: true},
}

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
	// Exported API: an empty ID would run six deletes with a meaningless
	// predicate rather than purging anything identifiable.
	if tenantID == "" {
		return nil, fmt.Errorf("cannot purge tenant: empty tenant ID")
	}

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

// drainBatchLimit caps how many rows are deleted per table per tenant per
// startup. A tenant with more telemetry than this converges over several
// restarts rather than delaying a boot. Deliberately a constant: make it
// configurable only when a deployment demonstrates it needs tuning.
const drainBatchLimit = 50000

// drainWindow is the size of each backward step through a tenant's telemetry
// history. It does not need to match a hypertable's chunk interval for
// correctness; it only bounds how much time-range a single DELETE covers.
const drainWindow = 7 * 24 * time.Hour

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

	// One tenant that always fails must not starve every other tenant in the
	// queue, so errors are accumulated and the walk continues.
	counts := make(map[string]int64)
	var errs []error
	for _, entry := range queued {
		drained := true
		failed := false
		for _, table := range tenantScopedTables {
			if !table.telemetry {
				continue
			}
			deleted, more, err := c.drainTelemetryTable(ctx, table.name, entry)
			counts[table.name] += deleted
			if err != nil {
				errs = append(errs, err)
				failed = true
				continue
			}
			if more {
				drained = false
			}
		}
		if drained && !failed {
			if err := c.db.WithContext(ctx).
				Where("tenant_id = ?", entry.TenantID).
				Delete(&PendingTenantPurge{}).Error; err != nil {
				errs = append(errs, fmt.Errorf("failed to dequeue %s: %w", entry.TenantID, err))
			}
		}
	}

	c.logger.Info("Purge queue drain completed", logger.Int("tenants", len(queued)))
	return counts, errors.Join(errs...)
}

// migrateSeedTenantPurgeQueue enqueues tenants that own rows in a scoped table
// but have no row at all in keycloak_tenants. Enqueueing has exactly one
// effect: the tenant's telemetry is drained at startup. It does not clean the
// orphan. The orphaned realms, alerts, rules and other non-telemetry rows that
// motivated this feature are never deleted by this migration, by the drain, or
// by anything else automatic; removing them still requires the separate manual
// script.
//
// It does NOT catch tenants soft-deleted by the pre-feature delete behaviour.
// That path only ever set deleted_at; the row is still physically present in
// keycloak_tenants, and the NOT IN subquery below is raw SQL that does not
// apply GORM's soft-delete scope, so it still sees that row and the tenant's
// data is never enqueued. Confirmed against a real database: a soft-deleted
// tenant with owned rows is enqueued 0 times. Cleaning up that pre-existing
// class of orphan needs a separate one-off script, not this migration.
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

	// The drain bounds deletes by time < requested_at, and telemetry timestamps
	// are written from the application clock. Use a Go timestamp here too, so
	// both this path and PurgeTenant compare against the same clock.
	requestedAt := time.Now()

	for _, table := range tenantScopedTables {
		// table.name comes from a package constant, never from input.
		res := c.db.Exec(`
			INSERT INTO pending_tenant_purges (tenant_id, requested_at)
			SELECT DISTINCT tenant_id, CAST(? AS timestamptz) FROM `+table.name+`
			WHERE tenant_id IS NOT NULL
			  AND tenant_id NOT IN (SELECT tenant_id FROM keycloak_tenants)
			ON CONFLICT (tenant_id) DO NOTHING`, requestedAt)
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

// drainTelemetryTable deletes one tenant's rows from one telemetry hypertable,
// walking backwards from requested_at in drainWindow-sized steps until either
// drainBatchLimit rows have been deleted (more=true, another pass is needed)
// or no rows older than the current window boundary remain (more=false).
//
// A window walk is used instead of the more obvious
// "DELETE ... WHERE ctid IN (SELECT ctid ... LIMIT n)" trick for capping rows,
// because Postgres DELETE has no native LIMIT. That trick was tried first and
// verified experimentally to misbehave on these hypertables: ctid is only
// unique within a single physical chunk, so when the LIMITed subquery pulls
// ctids from several chunks (likely, since these tables partition by day),
// the outer DELETE matches those same ctid values in every other chunk too,
// deleting far more rows than intended. In the probe, LIMIT 50 deleted all 300
// seeded rows spread across 300 chunks with no error. See task-4-report.md for
// the full probe output.
//
// table comes only from tenantScopedTables, never from a parameter.
func (c *Client) drainTelemetryTable(ctx context.Context, table string, entry PendingTenantPurge) (deleted int64, more bool, err error) {
	windowEnd := entry.RequestedAt
	for {
		windowStart := windowEnd.Add(-drainWindow)
		res := c.db.WithContext(ctx).Exec(
			"DELETE FROM "+table+" WHERE tenant_id = ? AND time >= ? AND time < ?",
			entry.TenantID, windowStart, windowEnd)
		if res.Error != nil {
			return deleted, false, fmt.Errorf("failed to drain %s for %s: %w", table, entry.TenantID, res.Error)
		}
		deleted += res.RowsAffected
		windowEnd = windowStart

		if deleted >= drainBatchLimit {
			return deleted, true, nil
		}

		// Jump straight to the newest row still below the cursor instead of
		// stepping through empty windows. Iterations then scale with the number
		// of populated windows rather than with elapsed calendar time: a single
		// row stamped 1970-01-01 (keycloak/monitor.go turns a zero event time
		// into exactly that) would otherwise cost thousands of round-trips.
		// Termination still holds: windowEnd strictly decreases and max(time)
		// is finite.
		var oldest sql.NullTime
		if err := c.db.WithContext(ctx).Raw(
			"SELECT max(time) FROM "+table+" WHERE tenant_id = ? AND time < ?",
			entry.TenantID, windowEnd).Scan(&oldest).Error; err != nil {
			return deleted, false, fmt.Errorf("failed to check remaining rows in %s for %s: %w", table, entry.TenantID, err)
		}
		if !oldest.Valid {
			return deleted, false, nil
		}
		// One microsecond, not one nanosecond: timestamptz resolves to
		// microseconds, so a nanosecond bump is truncated away, the cursor
		// lands exactly on the row's own timestamp, and the strict "time <"
		// bound then excludes that row from every subsequent window. Verified:
		// with +1ns an epoch-stamped row is never deleted. A microsecond is
		// the smallest step that cannot skip a row, since no stored value can
		// fall strictly between max(time) and max(time) + 1us.
		if next := oldest.Time.Add(time.Microsecond); next.Before(windowEnd) {
			windowEnd = next
		}
	}
}
