package database

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// Early single-tenant releases monitored a single Keycloak, and their schema has no
// tenant_id anywhere. AutoMigrate cannot take such a database forward on its
// own: it adds tenant_id as NOT NULL, which PostgreSQL refuses on a table with
// rows and TimescaleDB refuses on a compressed hypertable even when it is
// empty, so the server never started. And the unique indexes those releases
// created on realm_id, realm_name and alert_id, and the primary keys of the
// time-series tables, are global: they would make two tenants collide.
//
// migratePreTenantSchema takes such a database forward before AutoMigrate
// runs. Every row it finds belongs to the one Keycloak that release monitored,
// so it is given to the one tenant the configuration now names. The
// time-series tables are set aside as <table>_pre_tenant, data intact, and
// AutoMigrate creates them afresh in their per-tenant shape: their history
// was raw polling output, and rebuilding a compressed hypertable's primary key
// in place would mean decompressing all of it.

// ErrPreTenantOwnerUnknown is returned when a single-tenant database must be
// upgraded but the configuration does not name exactly one tenant to own its
// rows.
var ErrPreTenantOwnerUnknown = errors.New("cannot upgrade a database from a single-tenant release without exactly one configured tenant")

// preTenantTables gained tenant_id with multi-tenant support, so a database where one of them
// exists without it predates tenants. They gain it here, typed as their
// models declare it, so AutoMigrate finds nothing left to change.
var preTenantTables = []struct{ table, columnType string }{
	{"keycloak_realms", "varchar(100)"},
	{"configuration_alerts", "text"},
}

// preTenantEvents is events' tenant_id column. events gained it only
// later, so a database from an early multi-tenant release lacks it too and is not
// pre-tenant: backfillEventTenantIDs attributes those rows by realm. On a
// database that does predate tenants every event is the one Keycloak's, so
// they are given to the configured tenant here instead.
const preTenantEventsType = "text"

// preTenantTimeSeries are set aside rather than converted.
var preTenantTimeSeries = []string{"keycloak_events", "keycloak_metrics", "keycloak_health"}

// globalUniqueIndexes are the unique indexes the pre-tenant releases created
// over a single column that is unique only per tenant now. Each has a
// per-tenant replacement, which AutoMigrate creates.
var globalUniqueIndexes = []struct{ table, index, column string }{
	{"keycloak_realms", "idx_keycloak_realms_realm_id", "realm_id"},
	{"keycloak_realms", "idx_keycloak_realms_realm_name", "realm_name"},
	{"configuration_alerts", "idx_configuration_alerts_alert_id", "alert_id"},
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithPreTenantOwner names the tenant that rows from a single-tenant database
// belong to: the one tenant the configuration defines. Empty when it defines
// none or several, which is fine for any newer database.
func WithPreTenantOwner(tenantID string) ClientOption {
	return func(c *Client) { c.preTenantOwner = tenantID }
}

func (c *Client) hasColumn(table, column string) (bool, error) {
	var n int64
	err := c.db.Raw(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, table, column).Scan(&n).Error
	return n > 0, err
}

func (c *Client) hasTable(table string) (bool, error) {
	var n int64
	err := c.db.Raw(`SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = ?`, table).Scan(&n).Error
	return n > 0, err
}

// pendingPreTenant returns the tables that exist without tenant_id.
func (c *Client) pendingPreTenant(tables []string) ([]string, error) {
	var pending []string
	for _, table := range tables {
		exists, err := c.hasTable(table)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect %s: %w", table, err)
		}
		if !exists {
			continue
		}
		has, err := c.hasColumn(table, "tenant_id")
		if err != nil {
			return nil, fmt.Errorf("failed to inspect %s: %w", table, err)
		}
		if !has {
			pending = append(pending, table)
		}
	}
	return pending, nil
}

// migratePreTenantSchema upgrades a database from a single-tenant release, and does
// nothing on any other. Idempotent: what it changes is what it looks for.
func (c *Client) migratePreTenantSchema() error {
	names := make([]string, 0, len(preTenantTables))
	types := map[string]string{}
	for _, t := range preTenantTables {
		names = append(names, t.table)
		types[t.table] = t.columnType
	}
	rowTables, err := c.pendingPreTenant(names)
	if err != nil {
		return err
	}
	if len(rowTables) > 0 {
		events, err := c.pendingPreTenant([]string{"events"})
		if err != nil {
			return err
		}
		rowTables = append(rowTables, events...)
		types["events"] = preTenantEventsType
	}
	seriesTables, err := c.pendingPreTenant(preTenantTimeSeries)
	if err != nil {
		return err
	}

	if len(rowTables) > 0 && c.preTenantOwner == "" {
		return fmt.Errorf("%w: this database was created by an early release that monitored a single "+
			"Keycloak, and its rows in %s have no tenant. Define that Keycloak as the only tenant under "+
			"keycloak.tenants for this first start and its rows are given to it; further tenants can be "+
			"added afterwards", ErrPreTenantOwnerUnknown, strings.Join(rowTables, ", "))
	}

	for _, table := range rowTables {
		if err := c.db.Transaction(func(tx *gorm.DB) error {
			// The default fills existing rows without rewriting the table and
			// is dropped at once: new rows always name their tenant.
			if err := tx.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN tenant_id %s NOT NULL DEFAULT %s`,
				table, types[table], quoteLiteral(c.preTenantOwner))).Error; err != nil {
				return err
			}
			return tx.Exec(fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN tenant_id DROP DEFAULT`, table)).Error
		}); err != nil {
			return fmt.Errorf("failed to give %s rows to tenant %s: %w", table, c.preTenantOwner, err)
		}
		c.logger.Info("Gave the rows of a single-tenant table to the configured tenant",
			logger.Str("table", table), logger.Str("tenant_id", c.preTenantOwner))
	}

	for _, table := range seriesTables {
		if err := c.setAsidePreTenantSeries(table); err != nil {
			return err
		}
	}

	return c.dropGlobalUniqueIndexes()
}

// setAsidePreTenantSeries renames a pre-tenant time-series table, and every
// index on it, out of the way of the per-tenant table AutoMigrate creates.
// The indexes must move too: they keep their names through a table rename,
// and the new table's primary key would collide with the old one's.
func (c *Client) setAsidePreTenantSeries(table string) error {
	aside := table + "_pre_tenant"
	err := c.db.Transaction(func(tx *gorm.DB) error {
		var indexes []string
		if err := tx.Raw(`SELECT indexname FROM pg_indexes WHERE schemaname = current_schema() AND tablename = ?`,
			table).Scan(&indexes).Error; err != nil {
			return err
		}
		for _, idx := range indexes {
			if err := tx.Exec(fmt.Sprintf(`ALTER INDEX %s RENAME TO %s`,
				quoteIdent(idx), quoteIdent(asideName(idx)))).Error; err != nil {
				return err
			}
		}
		return tx.Exec(fmt.Sprintf(`ALTER TABLE %s RENAME TO %s`, quoteIdent(table), quoteIdent(aside))).Error
	})
	if err != nil {
		return fmt.Errorf("failed to set aside single-tenant table %s: %w", table, err)
	}
	c.logger.Warn("Set aside a single-tenant time-series table; its history is kept but not shown, and a "+
		"per-tenant table replaces it. Drop it once it is no longer wanted",
		logger.Str("table", aside))
	return nil
}

// dropGlobalUniqueIndexes removes the pre-tenant unique indexes over a single
// column. Only that exact shape is dropped, so an index of the same name that
// something else created stays.
func (c *Client) dropGlobalUniqueIndexes() error {
	for _, g := range globalUniqueIndexes {
		var def string
		if err := c.db.Raw(`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?`,
			g.index).Scan(&def).Error; err != nil {
			return fmt.Errorf("failed to inspect %s: %w", g.index, err)
		}
		if !strings.HasPrefix(def, "CREATE UNIQUE INDEX") || !strings.HasSuffix(def, "("+g.column+")") {
			continue
		}
		if err := c.db.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %s`, quoteIdent(g.index))).Error; err != nil {
			return fmt.Errorf("failed to drop %s: %w", g.index, err)
		}
		c.logger.Info("Dropped a unique index that made a value unique across every tenant",
			logger.Str("table", g.table), logger.Str("index", g.index))
	}
	return nil
}

// asideName is idx with _pre_tenant appended, within PostgreSQL's 63-byte
// identifier limit.
func asideName(idx string) string {
	const suffix = "_pre_tenant"
	if len(idx)+len(suffix) > 63 {
		idx = idx[:63-len(suffix)]
	}
	return idx + suffix
}

func quoteIdent(s string) string   { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func quoteLiteral(s string) string { return `'` + strings.ReplaceAll(s, `'`, `''`) + `'` }
