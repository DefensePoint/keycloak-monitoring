package database

import (
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// eventTenantIDBackfillKey marks the one attribution of pre-tenant events.
// A later startup that finds this row must not look at events again: a realm
// name that becomes unique only because another tenant was deleted is not
// evidence those rows belong to the survivor.
const eventTenantIDBackfillKey = "events.tenant_id"

// DataMigration records a one-shot data change. The row is written in the
// same transaction as the change, so a failed attempt leaves nothing to find
// and the next startup retries.
type DataMigration struct {
	Key       string    `gorm:"primaryKey;size:255"`
	AppliedAt time.Time `gorm:"not null"`
}

// TableName specifies the table name for DataMigration.
func (DataMigration) TableName() string {
	return "data_migrations"
}

const eventTenantIDBackfillSQL = `
	UPDATE events AS e
	SET tenant_id = unambiguous.tenant_id
	FROM (
		SELECT realm_name, MIN(tenant_id) AS tenant_id
		FROM keycloak_realms
		WHERE deleted_at IS NULL
		GROUP BY realm_name
		HAVING COUNT(DISTINCT tenant_id) = 1
	) AS unambiguous
	WHERE (e.tenant_id IS NULL OR e.tenant_id = '')
	  AND split_part(e.source, ':', 2) = unambiguous.realm_name`

// backfillEventTenantIDs attributes pre-existing event rows to a tenant where
// that can be established, and leaves the rest alone.
//
// Rows written before events.tenant_id existed recorded no tenant. The only
// tenant signal they carry is the realm name inside source
// ("keycloak:<realm>", "amfa:<realm>"), so a row is attributable exactly when
// its realm name belongs to one tenant and no other. Where a realm name is
// shared, and "master" is shared by every Keycloak, the owner is genuinely
// unrecoverable and the row keeps its empty tenant_id.
//
// The attribution runs once. Writers set tenant_id themselves, and a row that
// arrives after the marker commits stays unattributed rather than being
// guessed at on a later boot. That includes events a release from before per-tenant
// event ids wrote during a rollback. After rolling forward the marker is still
// there, so those empty-tenant rows stay empty. Clearing the marker to
// attribute them would also attribute a realm that became unique only because
// another tenant was deleted.
func (c *Client) backfillEventTenantIDs() error {
	applied, err := dataMigrationApplied(c.db, eventTenantIDBackfillKey)
	if err != nil || applied {
		return err
	}

	var ran bool
	var rows int64
	err = c.db.Transaction(func(tx *gorm.DB) error {
		// Held until this transaction ends, so a second startup waits and
		// then sees the marker instead of inserting it again.
		if err := tx.Exec(
			`SELECT pg_advisory_xact_lock(hashtext(?)::bigint)`,
			eventTenantIDBackfillKey,
		).Error; err != nil {
			return err
		}
		applied, err := dataMigrationApplied(tx, eventTenantIDBackfillKey)
		if err != nil || applied {
			return err
		}
		res := tx.Exec(eventTenantIDBackfillSQL)
		if res.Error != nil {
			return res.Error
		}
		if err := tx.Create(&DataMigration{
			Key:       eventTenantIDBackfillKey,
			AppliedAt: time.Now(),
		}).Error; err != nil {
			return err
		}
		ran = true
		rows = res.RowsAffected
		return nil
	})
	if err != nil || !ran {
		return err
	}
	c.logEventTenantBackfill(rows)
	return nil
}

func dataMigrationApplied(db *gorm.DB, key string) (bool, error) {
	var n int64
	err := db.Model(&DataMigration{}).Where("key = ?", key).Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (c *Client) logEventTenantBackfill(rows int64) {
	if rows > 0 {
		c.logger.Info("Attributed pre-existing events to a tenant",
			logger.Int64("rows", rows))
	}
	var unattributed int64
	if err := c.db.Raw(
		`SELECT count(*) FROM events WHERE tenant_id IS NULL OR tenant_id = ''`).
		Scan(&unattributed).Error; err != nil {
		c.logger.Warn("Could not count events left without a tenant", logger.Err(err))
		return
	}
	if unattributed > 0 {
		c.logger.Warn("Events remain unattributed to any tenant: their realm name is shared "+
			"between tenants so the owner cannot be established. They are excluded from "+
			"tenant-scoped queries and are not removed by tenant deletion",
			logger.Int64("rows", unattributed))
	}
}
