package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/pgdsn"
	"github.com/DefensePoint/keycloak-monitoring/pkg/secretcrypto"
)

// Client represents a PostgreSQL database client using GORM.
type Client struct {
	db     *gorm.DB
	logger *logger.Logger
	// preTenantOwner is the tenant a single-tenant database's rows are given
	// to; see migratePreTenantSchema.
	preTenantOwner string
}

// NewClient creates a new GORM database client and auto-migrates the schema.
func NewClient(cfg *config.DatabaseConfig, log *logger.Logger, opts ...ClientOption) (*Client, error) {
	client, err := openClient(cfg, log)
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt(client)
	}

	// Auto-migrate schema
	if err := client.AutoMigrate(); err != nil {
		if sqlDB, dbErr := client.db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, fmt.Errorf("failed to auto-migrate schema: %w", err)
	}

	log.Info("Database client initialized with GORM",
		logger.Str("host", cfg.Host),
		logger.Int("port", cfg.Port),
		logger.Str("database", cfg.Database))

	return client, nil
}

// NewReadOnlyClient opens the same GORM pool as NewClient but never migrates:
// no AutoMigrate, no TimescaleDB setup, no backfills. Constructing against an
// unmigrated database succeeds; queries against missing tables still fail.
// Enforcing read-only access is a deployment concern (a read-only database
// role); this constructor only guarantees it issues no schema changes itself.
func NewReadOnlyClient(cfg *config.DatabaseConfig, log *logger.Logger) (*Client, error) {
	client, err := openClient(cfg, log)
	if err != nil {
		return nil, err
	}

	log.Info("Read-only database client initialized with GORM",
		logger.Str("host", cfg.Host),
		logger.Int("port", cfg.Port),
		logger.Str("database", cfg.Database))

	return client, nil
}

// connMaxLifetime is how long a pooled connection is kept before it is closed
// and replaced. Long enough that reconnecting, and preparing statements again
// on the new connection, costs nothing noticeable; short enough that a
// connection a proxy, failover or DNS change has silently left dead does not
// linger.
const connMaxLifetime = 30 * time.Minute

// defaultTimeout bounds opening a connection and the startup check when the
// timeout is not set, as mcp.database.timeout usually is not: it has no
// default so that a missing mcp.database block still fails startup.
const defaultTimeout = 10 * time.Second

// effectiveTimeout is cfg.Timeout, or defaultTimeout when it is not set.
func effectiveTimeout(cfg *config.DatabaseConfig) time.Duration {
	if cfg.Timeout > 0 {
		return cfg.Timeout
	}
	return defaultTimeout
}

// connectionString is the driver DSN for cfg. The timeout becomes the
// driver's connect_timeout, which bounds every attempt to open a connection,
// including the one gorm.Open makes before openClient can pass a deadline;
// without it a database that accepts the connection and never answers holds
// startup forever. It is whole seconds, rounded up so a sub-second value
// still sets a limit.
func connectionString(cfg *config.DatabaseConfig) string {
	dsn := pgdsn.Format(cfg.Host, cfg.Port, cfg.Database, cfg.User, cfg.Password, cfg.SSLMode)
	seconds := int64((effectiveTimeout(cfg) + time.Second - 1) / time.Second)
	return dsn + fmt.Sprintf(" connect_timeout=%d", seconds)
}

// openClient opens the GORM connection, configures the pool and pings.
func openClient(cfg *config.DatabaseConfig, log *logger.Logger) (*Client, error) {
	dsn := connectionString(cfg)

	// Configure GORM
	gormConfig := &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent), // Use silent mode, we'll use our own logger
		SkipDefaultTransaction: true,                                          // Better performance
		PrepareStmt:            true,                                          // Prepare statements for better performance
	}

	// Open database connection
	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying sql.DB to configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying database: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(cfg.MaxConns)
	if cfg.MinConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MinConns)
	}
	// Not database.timeout: used as the lifetime, it closed and reopened every
	// connection each timeout period, 30s by default, and threw away its
	// prepared statements.
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), effectiveTimeout(cfg))
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Client{
		db:     db,
		logger: log,
	}, nil
}

// lockSchema holds a session advisory lock until the caller releases it.
// The lock sits on a pooled connection, so the pool needs a second connection
// for the migration. Unlock on that same connection: Close returns it to the
// pool and would otherwise keep the lock.
//
// It polls with pg_try_advisory_lock rather than blocking in pg_advisory_lock:
// a blocked statement keeps a snapshot open, and CREATE INDEX CONCURRENTLY run
// by the holder waits for every such snapshot, so the two would wait on each
// other forever without Postgres seeing a deadlock.
func (c *Client) lockSchema() (func(), error) {
	sqlDB, err := c.db.DB()
	if err != nil {
		return nil, fmt.Errorf("schema lock: %w", err)
	}
	if n := sqlDB.Stats().MaxOpenConnections; n > 0 && n < 2 {
		sqlDB.SetMaxOpenConns(2)
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("schema lock connection: %w", err)
	}
	start := time.Now()
	lastLog := start
	for {
		var locked bool
		if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1)::bigint)`, schemaAdvisoryLockKey).Scan(&locked); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("schema lock: %w", err)
		}
		if locked {
			break
		}
		if time.Since(lastLog) >= schemaLockLogEvery || lastLog.Equal(start) {
			c.logger.Info("Waiting for another instance to finish its schema migration",
				logger.Dur("waited", time.Since(start).Round(time.Second)))
			lastLog = time.Now()
		}
		time.Sleep(schemaLockPoll)
	}
	return func() {
		unlockCtx := context.Background()
		if _, err := conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock(hashtext($1)::bigint)`, schemaAdvisoryLockKey); err != nil {
			_, _ = conn.ExecContext(unlockCtx, `SELECT pg_terminate_backend(pg_backend_pid())`)
		}
		_ = conn.Close()
	}, nil
}

// schemaAdvisoryLockKey serializes AutoMigrate across processes. Two startups
// against an empty database both CREATE TABLE, and one fails on pg_class
// before the events backfill runs. The backfill takes its own transaction
// lock later, on a different connection.
const schemaAdvisoryLockKey = "kmt.schema"

const (
	schemaLockPoll     = 250 * time.Millisecond
	schemaLockLogEvery = 30 * time.Second
	indexBuildLogEvery = 30 * time.Second
)

// AutoMigrate automatically migrates the database schema.
func (c *Client) AutoMigrate() error {
	release, err := c.lockSchema()
	if err != nil {
		return err
	}
	defer release()

	c.logger.Info("Running GORM auto-migration")

	// A database from a single-tenant release has no tenant_id anywhere, which
	// AutoMigrate cannot add on its own.
	if err := c.migratePreTenantSchema(); err != nil {
		return err
	}

	// Migrate regular tables first (without OperatorAction to avoid FK issues)
	err = c.db.AutoMigrate(
		&Event{},
		&User{},
		&KeycloakTenant{},
		&KeycloakRealmInfo{},
		&Alert{},
		&AlertRule{},
		&NotificationLog{},
		&PendingTenantPurge{},
		&AmfaMirrorWatermark{},
		&DataMigration{},
	)
	if err != nil {
		return fmt.Errorf("failed to auto-migrate regular tables: %w", err)
	}

	// keycloak_tenants.amfa_enabled and .origin used to carry an `index` gorm
	// tag. AutoMigrate only adds schema, never removes it, so a database
	// migrated under the old tags keeps both indexes even after the tag is
	// gone from the struct. Drop them explicitly: both columns are 2-valued
	// and only ever compared in Go after the row is loaded, never filtered on
	// in SQL, so the index cost nothing but write overhead on every insert.
	for _, idx := range []string{"idx_keycloak_tenants_amfa_enabled", "idx_keycloak_tenants_origin"} {
		if c.db.Migrator().HasIndex(&KeycloakTenant{}, idx) {
			if err := c.db.Migrator().DropIndex(&KeycloakTenant{}, idx); err != nil {
				c.logger.Warn("Could not drop unused tenant index", logger.Str("index", idx), logger.Err(err))
			}
		}
	}

	// events.event_id used to be globally unique. Uniqueness is now per tenant
	// (idx_events_tenant_event), because two tenants pointed at the same
	// Keycloak see the same event ids and a global index made them silently
	// overwrite each other. AutoMigrate adds the composite index but never
	// removes the old one, so drop it explicitly or the old constraint keeps
	// rejecting the second tenant's rows.
	if c.db.Migrator().HasIndex(&Event{}, "idx_events_event_id") {
		if err := c.db.Migrator().DropIndex(&Event{}, "idx_events_event_id"); err != nil {
			c.logger.Warn("Could not drop the global unique index on events.event_id; "+
				"two tenants monitoring the same Keycloak will collide until it is removed",
				logger.Err(err))
		} else {
			c.logger.Info("Dropped the global unique index on events.event_id (now unique per tenant)")
		}
	}

	if err := c.backfillEventTenantIDs(); err != nil {
		c.logger.Warn("Could not backfill events.tenant_id; unattributed rows stay hidden from "+
			"tenant-scoped queries and will be retried next startup", logger.Err(err))
	}

	// Add new columns to existing configuration_alerts table if they don't exist
	// GORM AutoMigrate handles this automatically, but we explicitly check for data integrity
	if c.db.Migrator().HasTable(&Alert{}) {
		// Add source column with default value for existing rows
		if !c.db.Migrator().HasColumn(&Alert{}, "source") {
			if err := c.db.Migrator().AddColumn(&Alert{}, "source"); err != nil {
				c.logger.Warn("Could not add source column", logger.Err(err))
			}
		}
		// Add rule_id and event_id columns
		if !c.db.Migrator().HasColumn(&Alert{}, "rule_id") {
			if err := c.db.Migrator().AddColumn(&Alert{}, "rule_id"); err != nil {
				c.logger.Warn("Could not add rule_id column", logger.Err(err))
			}
		}
		if !c.db.Migrator().HasColumn(&Alert{}, "event_id") {
			if err := c.db.Migrator().AddColumn(&Alert{}, "event_id"); err != nil {
				c.logger.Warn("Could not add event_id column", logger.Err(err))
			}
		}
	}

	// Migrate OperatorAction separately after its dependencies exist
	err = c.db.AutoMigrate(&OperatorAction{})
	if err != nil {
		return fmt.Errorf("failed to auto-migrate operator actions table: %w", err)
	}

	// Migrate RBAC tables
	err = c.db.AutoMigrate(&Role{}, &Permission{}, &UserRole{}, &RolePermission{}, &TenantPolicy{})
	if err != nil {
		return fmt.Errorf("failed to auto-migrate RBAC tables: %w", err)
	}

	// Migrate API tokens after its User dependency exists
	err = c.db.AutoMigrate(&APIToken{})
	if err != nil {
		return fmt.Errorf("failed to auto-migrate api tokens table: %w", err)
	}

	if err := c.revokeCutOffUsersTokens(); err != nil {
		c.logger.Warn("Could not revoke the API tokens of users cut off before this release; "+
			"until it succeeds, reactivating such a user brings their old tokens back. "+
			"Retried next startup", logger.Err(err))
	}

	// Data migration: grant amfa:read to the default roles, once
	if err := c.migrateAmfaReadPermission(); err != nil {
		c.logger.Warn("amfa:read permission migration failed; retried next startup", logger.Err(err))
	}

	// Data migration: Set auth_method for existing users
	if err := c.migrateUserAuthMethod(); err != nil {
		c.logger.Warn("Failed to migrate user auth_method, continuing", logger.Err(err))
	}

	// Data migration: clear email_verified that was asserted rather than observed
	if err := c.migrateUnprovenEmailVerified(); err != nil {
		c.logger.Warn("Failed to clear unproven email_verified, continuing", logger.Err(err))
	}

	// Data migration: start the SSO sign-in clock for users who have none. After
	// the email_verified migration, which reads last_login_at.
	if err := c.baselineSSOLastLogin(); err != nil {
		c.logger.Warn("Failed to record a baseline SSO sign-in time; SSO users with none are not "+
			"held to the MCP sign-in limit until it succeeds, which is retried next startup", logger.Err(err))
	}

	// Data migration: Fix username unique index to allow multiple NULL/empty values
	if err := c.migrateUsernameIndex(); err != nil {
		c.logger.Warn("Failed to migrate username index, continuing", logger.Err(err))
	}

	// The same treatment for every unique index on users, not just username.
	if err := c.migrateUserUniqueIndexes(); err != nil {
		c.logger.Warn("Failed to migrate the unique indexes on users, continuing", logger.Err(err))
	}

	// ...but do not continue past the state those migrations exist to prevent.
	if err := c.assertUserUniqueIndexesAreSafe(); err != nil {
		return err
	}

	// Data migration: Detect InfiniSpan for existing tenants
	if err := c.migrateRoleNameIndex(); err != nil {
		return fmt.Errorf("failed to migrate role name index: %w", err)
	}

	if err := c.removeDeletedRoleAssignments(); err != nil {
		return fmt.Errorf("failed to remove deleted roles' assignments: %w", err)
	}

	if err := c.migrateInfinispanDetection(); err != nil {
		c.logger.Warn("Failed to migrate InfiniSpan detection, continuing", logger.Err(err))
	}

	// Data migration: Fix realm_name unique index to be scoped per-tenant
	if err := c.migrateRealmNameIndex(); err != nil {
		c.logger.Warn("Failed to migrate realm_name index, continuing", logger.Err(err))
	}

	// Data migration: Fix alert_id unique index to be scoped per-tenant
	if err := c.migrateAlertIDIndex(); err != nil {
		c.logger.Warn("Failed to migrate alert_id index, continuing", logger.Err(err))
	}

	if err := c.ensureAlertLookupIndex(); err != nil {
		c.logger.Warn("Failed to build idx_alerts_tenant_alert_id, continuing; the next start retries it", logger.Err(err))
	}

	// Data migration: enqueue orphaned data left by the old tenant delete
	if err := c.migrateSeedTenantPurgeQueue(); err != nil {
		c.logger.Warn("Failed to seed tenant purge queue, continuing", logger.Err(err))
	}

	// Drop NOT NULL constraint from legacy password-grant columns so that
	// existing rows don't block inserts after the columns are removed from
	// the GORM model. The columns are kept in the DB to preserve existing
	// data but are no longer written or read by the application.
	if err := c.migrateDropPasswordGrantColumns(); err != nil {
		c.logger.Warn("Failed to migrate password-grant column constraints, continuing", logger.Err(err))
	}

	// Migrate time-series tables (these will become hypertables)
	err = c.db.AutoMigrate(&KeycloakEvent{}, &KeycloakMetrics{}, &KeycloakHealth{})
	if err != nil {
		return fmt.Errorf("failed to auto-migrate time-series tables: %w", err)
	}

	// Enable TimescaleDB extension and create hypertables
	if err := c.setupTimescaleDB(); err != nil {
		c.logger.Error("Failed to setup TimescaleDB, continuing with regular PostgreSQL",
			logger.Err(err))
	}

	c.logger.Info("Database schema migrated successfully")
	return nil
}

// migrateUsernameIndex recreates the username unique index to allow multiple NULL/empty values.
// This is necessary because OAuth2 users don't have usernames (only simple auth users do).
// The partial index only enforces uniqueness for non-empty usernames.
func (c *Client) migrateUsernameIndex() error {
	c.logger.Info("Migrating username unique index to partial index")

	// Check if the old index exists
	var indexExists bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE indexname = 'idx_users_username'
		);
	`
	if err := c.db.Raw(query).Scan(&indexExists).Error; err != nil {
		return fmt.Errorf("failed to check if index exists: %w", err)
	}

	if !indexExists {
		c.logger.Info("Username index does not exist yet, will be created by GORM")
		// Create the partial index directly
		createIndexSQL := `CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username) WHERE username IS NOT NULL AND username != ''`
		if err := c.db.Exec(createIndexSQL).Error; err != nil {
			return fmt.Errorf("failed to create partial index: %w", err)
		}
		c.logger.Info("Created partial username index")
		return nil
	}

	// Check if it's already a partial index
	var indexDef string
	query = `SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_users_username'`
	if err := c.db.Raw(query).Scan(&indexDef).Error; err != nil {
		return fmt.Errorf("failed to get index definition: %w", err)
	}

	// If it's already a partial index (contains WHERE clause), skip
	if len(indexDef) > 0 && strings.Contains(indexDef, "WHERE") {
		c.logger.Info("Username index is already a partial index, skipping migration")
		return nil
	}

	// Drop the old index and create the new partial index
	c.logger.Info("Dropping old username index")
	if err := c.db.Exec("DROP INDEX IF EXISTS idx_users_username").Error; err != nil {
		return fmt.Errorf("failed to drop old index: %w", err)
	}

	c.logger.Info("Creating partial username index")
	createIndexSQL := `CREATE UNIQUE INDEX idx_users_username ON users(username) WHERE username IS NOT NULL AND username != ''`
	if err := c.db.Exec(createIndexSQL).Error; err != nil {
		return fmt.Errorf("failed to create partial index: %w", err)
	}

	c.logger.Info("Successfully migrated username index to partial index")
	return nil
}

// migrateRealmNameIndex recreates idx_keycloak_realms_tenant_realm_name as a
// composite (tenant_id, realm_name) unique index over live rows. It was
// originally created on realm_name alone, which made a realm name globally
// unique across the whole table instead of unique per tenant — any second
// tenant pointed at a Keycloak instance already monitored by another tenant
// would fail to save realm info for every realm name that tenant had already
// saved.
//
// The index must also ignore soft-deleted rows: a realm deleted and recreated
// in Keycloak keeps its name but gets a new id, and SaveRealm retires the old
// row before inserting the new one. AutoMigrate creates the index from the
// model tag without that predicate, so on a database it created, the index
// already named tenant_id and the tenant_id check alone skipped it, leaving
// fresh and upgraded databases with different indexes. Anything short of the
// tenant-scoped, live-rows-only form is rebuilt.
func (c *Client) migrateRealmNameIndex() error {
	c.logger.Info("Checking realm_name unique index scope")

	var indexDef string
	query := `SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_keycloak_realms_tenant_realm_name'`
	if err := c.db.Raw(query).Scan(&indexDef).Error; err != nil {
		return fmt.Errorf("failed to get index definition: %w", err)
	}

	if len(indexDef) == 0 {
		c.logger.Info("Realm name index does not exist yet, will be created by GORM")
		return nil
	}

	if strings.Contains(indexDef, "tenant_id") && strings.Contains(indexDef, "deleted_at IS NULL") {
		c.logger.Info("Realm name index is already scoped per tenant over live rows, skipping migration")
		return nil
	}

	c.logger.Info("Dropping realm_name index that is not tenant-scoped over live rows")
	if err := c.db.Exec("DROP INDEX IF EXISTS idx_keycloak_realms_tenant_realm_name").Error; err != nil {
		return fmt.Errorf("failed to drop old index: %w", err)
	}

	c.logger.Info("Creating tenant-scoped realm_name index over live rows")
	createIndexSQL := `CREATE UNIQUE INDEX idx_keycloak_realms_tenant_realm_name ON keycloak_realms(tenant_id, realm_name) WHERE deleted_at IS NULL`
	if err := c.db.Exec(createIndexSQL).Error; err != nil {
		return fmt.Errorf("failed to create tenant-scoped index: %w", err)
	}

	c.logger.Info("Successfully migrated realm_name index to be tenant-scoped")
	return nil
}

// MigrateEncryptClientSecrets encrypts every tenant's client_secret that
// isn't already encrypted. Unlike the other migrate* steps, this one is
// exported and NOT wired into AutoMigrate: it needs an *secretcrypto.Encryptor,
// which only exists once MONITORING_SECURITY_ENCRYPTION_KEY is configured, and
// AutoMigrate runs inside NewClient before the rest of the app (including the
// encryptor) is constructed. Callers run this explicitly, after the key is
// known to be present, typically once as a deliberate operational step when
// turning encryption on for an environment that already has tenants — not on
// every boot the way the other migrations are.
//
// Each row is updated individually and failures are collected rather than
// aborting: a single bad row (e.g. one that's somehow already corrupted)
// shouldn't block every other tenant from getting encrypted.
//
// Soft-deleted tenants are included (hence Unscoped on both the read and the
// write). Deleting a tenant is now a hard purge, so no new soft-deleted rows
// are created, but rows soft-deleted by earlier versions persist: the purge
// queue only enqueues orphaned data whose tenant row is already gone, and
// nothing removes the tenant row itself. Those leftovers still hold a
// client_secret that is valid at Keycloak and present in any database dump.
// Since a stolen dump is the threat this encryption exists for, skipping them
// would report a successful migration while leaving exactly the credentials it
// was meant to protect readable.
func (c *Client) MigrateEncryptClientSecrets(encryptor *secretcrypto.Encryptor) error {
	c.logger.Info("Encrypting existing tenant client_secret values")

	var tenants []KeycloakTenant
	if err := c.db.
		Unscoped().
		Find(&tenants).Error; err != nil {
		return fmt.Errorf("failed to list tenants: %w", err)
	}

	var encrypted, skipped, failed, softDeleted int
	for _, t := range tenants {
		if secretcrypto.IsEncrypted(t.ClientSecret) {
			skipped++
			continue
		}

		ciphertext, err := encryptor.Encrypt(t.ClientSecret)
		if err != nil {
			failed++
			c.logger.Error("Failed to encrypt client_secret for tenant, leaving as plaintext",
				logger.Str("tenant_id", t.TenantID), logger.Err(err))
			continue
		}

		// Unscoped here too: without it GORM appends "deleted_at IS NULL" to
		// the UPDATE, so a soft-deleted row matches nothing and the write is
		// silently a no-op while still counting as encrypted below.
		res := c.db.
			Unscoped().
			Model(&KeycloakTenant{}).
			Where("id = ?", t.ID).
			Update("client_secret", ciphertext)
		if res.Error != nil {
			failed++
			c.logger.Error("Failed to save encrypted client_secret for tenant",
				logger.Str("tenant_id", t.TenantID), logger.Err(res.Error))
			continue
		}
		// A write that matches no rows is not a success. Counting it as one
		// would report a secret as encrypted while leaving it readable, which
		// is worse than failing outright.
		if res.RowsAffected == 0 {
			failed++
			c.logger.Error("Encrypted client_secret update matched no rows, leaving as plaintext",
				logger.Str("tenant_id", t.TenantID))
			continue
		}

		encrypted++
		if t.DeletedAt.Valid {
			softDeleted++
		}
	}

	c.logger.Info("Finished encrypting tenant client_secret values",
		logger.Int("encrypted", encrypted),
		logger.Int("already_encrypted", skipped),
		logger.Int("failed", failed),
		logger.Int("of_which_soft_deleted", softDeleted))

	if failed > 0 {
		return fmt.Errorf("failed to encrypt client_secret for %d tenant(s)", failed)
	}
	return nil
}

// migrateAlertIDIndex recreates idx_alert_tenant as a composite
// (tenant_id, alert_id) unique index over live rows. It was originally created
// on alert_id alone, which made an alert_id globally unique across the whole
// table instead of unique per tenant. configuration_alerts IDs are a hash of
// check_type/realm_name/alert_type with no tenant in the input, so two
// tenants with a same-named realm produced identical alert_ids: the second
// tenant's Save (an upsert keyed on alert_id) silently updated the first
// tenant's row instead of creating its own, with no error and no ownership
// change.
//
// The index must also ignore soft-deleted rows. Save upserts with ON CONFLICT
// (tenant_id, alert_id) WHERE deleted_at IS NULL, and PostgreSQL may satisfy
// that from any unique index on the two columns, partial or not. AutoMigrate
// creates the index from the model tag without the predicate, so on a database
// it created the index already named tenant_id and the tenant_id check alone
// skipped it: there, a deleted alert detected again was written into its
// deleted row and never showed again. Anything short of the tenant-scoped,
// live-rows-only form is rebuilt, the same rule migrateRealmNameIndex applies.
//
// The drop and the create run in one transaction. The checkers save alerts on
// every poll, and between the two statements their upserts would find no
// index to resolve the conflict against and fail. Narrowing a unique index
// cannot fail on rows the wider one accepted, so the create cannot undo the
// drop.
//
// This migration only fixes the index so future writes can't collide again.
// It does not attempt to repair rows that already merged this way — that
// requires deciding which tenant a corrupted row should now belong to, which
// is a data question, not a schema one.
func (c *Client) migrateAlertIDIndex() error {
	c.logger.Info("Checking alert_id unique index scope")

	var indexDef string
	query := `SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_alert_tenant'`
	if err := c.db.Raw(query).Scan(&indexDef).Error; err != nil {
		return fmt.Errorf("failed to get index definition: %w", err)
	}

	if len(indexDef) == 0 {
		c.logger.Info("Alert ID index does not exist yet, will be created by GORM")
		return nil
	}

	if strings.Contains(indexDef, "tenant_id") && strings.Contains(indexDef, "deleted_at IS NULL") {
		c.logger.Info("Alert ID index is already scoped per tenant over live rows, skipping migration")
		return nil
	}

	c.logger.Info("Rebuilding alert_id index as tenant-scoped over live rows",
		logger.Str("current", indexDef))
	createIndexSQL := `CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts(tenant_id, alert_id) WHERE deleted_at IS NULL`
	err := c.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DROP INDEX IF EXISTS idx_alert_tenant").Error; err != nil {
			return fmt.Errorf("failed to drop old index: %w", err)
		}
		if err := tx.Exec(createIndexSQL).Error; err != nil {
			return fmt.Errorf("failed to create tenant-scoped index: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	c.logger.Info("Successfully migrated alert_id index to be tenant-scoped over live rows")
	return nil
}

// ensureAlertLookupIndex builds a plain (tenant_id, alert_id) index when
// idx_alert_tenant covers live rows only, as migrateAlertIDIndex leaves it on
// every database, fresh ones included. The per-event AMFA checks look an alert
// up including deleted rows, which the partial index cannot serve, so without
// this every lookup of a new event scans the table. A full index, left only
// when that migration failed, already serves it. CONCURRENTLY keeps alerts
// writable during the build; an index an interrupted build left invalid is
// dropped and built again.
func (c *Client) ensureAlertLookupIndex() error {
	var indexDef string
	if err := c.db.Raw(`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_alert_tenant'`).
		Scan(&indexDef).Error; err != nil {
		return fmt.Errorf("failed to inspect idx_alert_tenant: %w", err)
	}
	if !isLiveOnly(indexDef) {
		return nil
	}

	var valid []bool
	if err := c.db.Raw(`SELECT i.indisvalid FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid
		WHERE ic.relname = 'idx_alerts_tenant_alert_id'`).Scan(&valid).Error; err != nil {
		return fmt.Errorf("failed to inspect idx_alerts_tenant_alert_id: %w", err)
	}
	if len(valid) == 1 && valid[0] {
		return nil
	}
	if len(valid) == 1 {
		c.logger.Warn("Dropping an invalid idx_alerts_tenant_alert_id left by an interrupted build")
		if err := c.db.Exec(`DROP INDEX CONCURRENTLY IF EXISTS idx_alerts_tenant_alert_id`).Error; err != nil {
			return fmt.Errorf("failed to drop invalid idx_alerts_tenant_alert_id: %w", err)
		}
	}
	c.logger.Info("Building idx_alerts_tenant_alert_id for alert lookups that include deleted alerts")
	built := make(chan error, 1)
	go func() {
		built <- c.db.Exec(`CREATE INDEX CONCURRENTLY idx_alerts_tenant_alert_id ON configuration_alerts (tenant_id, alert_id)`).Error
	}()
	start := time.Now()
	ticker := time.NewTicker(indexBuildLogEvery)
	defer ticker.Stop()
	for {
		select {
		case err := <-built:
			return err
		case <-ticker.C:
			c.logger.Info("Still building idx_alerts_tenant_alert_id; the build waits for transactions already open on the database",
				logger.Dur("elapsed", time.Since(start).Round(time.Second)))
		}
	}
}

// emailVerifiedFixCutoff is the moment the OAuth login path started recording
// the IdP's own email_verified claim instead of hardcoding true.
//
// It is the pivot the migration below turns on, so it must never move. An OAuth
// row last touched before it carries the hardcoded value and says nothing about
// the address; one touched after carries whatever the IdP actually asserted and
// is authoritative. Moving this forward would discard real claims.
var emailVerifiedFixCutoff = time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)

// migrateUnprovenEmailVerified clears email_verified on OAuth users whose value
// was assumed rather than observed.
//
// The OAuth paths in auth/service.go set EmailVerified: true unconditionally,
// with the comment "From OAuth2, email is verified". That is not what OIDC
// means: an IdP issues email_verified per address, and Keycloak reports false
// for an account an admin created without confirming the address. The claim was
// parsed correctly in auth/provider.go and then discarded, so every OAuth user
// in the table reads as confirmed whether or not anyone confirmed them.
//
// Nothing gates access on this today — it reaches a badge in the users table and
// the /me response — so the cost of the stale value is a false statement rather
// than an outage. It matters because account linking by email is the next thing
// to be built here, and linking would rest on exactly this flag.
//
// The value cannot be recovered retroactively: a hardcoded true and a genuine
// true are the same byte. So this clears them and lets the truth return on each
// user's next login, where FindOrCreateBySubject writes the real claim. Users
// see an "unverified" badge in the meantime, which is honest — we do not know.
//
// Idempotent, and safe to run on every boot, because of the cutoff: a login
// after the fix updates last_login_at past it and puts the row permanently out
// of scope. A re-run can therefore never clobber a claim the IdP made. Simple
// auth users are untouched; their flag is set elsewhere and is a separate
// question.
func (c *Client) migrateUnprovenEmailVerified() error {
	res := c.db.Exec(`
		UPDATE users
		SET email_verified = false
		WHERE auth_method = 'oauth'
		  AND email_verified = true
		  AND (last_login_at IS NULL OR last_login_at < ?)
	`, emailVerifiedFixCutoff)
	if res.Error != nil {
		return fmt.Errorf("failed to clear unproven email_verified: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		c.logger.Info("Cleared email_verified that was assumed rather than asserted by the IdP; "+
			"each user's next login restores the real claim",
			logger.Int64("rows", res.RowsAffected))
	}
	return nil
}

// userUniqueIndex describes a unique index on users that must exclude rows the
// application does not consider live.
//
// Every one of these columns is legitimately empty for some kind of account,
// and every one of them can belong to a soft-deleted row, so a plain unique
// index over the bare column enforces more than the application means and
// blocks work it should allow.
type userUniqueIndex struct {
	name string
	// column is named only to build the repair statement in an error.
	column string
	// predicate is what keeps the index honest: live rows only, and only rows
	// that actually carry a value.
	predicate string
	// why explains, to whoever reads a refusal at three in the morning, which
	// accounts a plain index would break.
	why string
}

var userUniqueIndexes = []userUniqueIndex{
	{
		name:      "idx_users_email",
		column:    "email",
		predicate: "deleted_at IS NULL AND email IS NOT NULL AND email <> ''",
		why: "Keycloak accounts frequently have no email at all, and role_sync copies " +
			"that absence through verbatim, so a plain index admits the first such user " +
			"and rejects every one after them",
	},
	{
		name:      "idx_users_subject",
		column:    "subject",
		predicate: "deleted_at IS NULL AND subject IS NOT NULL AND subject <> ''",
		why: "simple auth derives the subject from the email address, so an account " +
			"without one carries an empty subject too",
	},
	{
		name:      "idx_users_username",
		column:    "username",
		predicate: "deleted_at IS NULL AND username IS NOT NULL AND username <> ''",
		why: "every OAuth user is created with an empty username, since only simple " +
			"auth sets one, so a plain index lets exactly one of them exist",
	},
}

func (i userUniqueIndex) createSQL() string {
	return fmt.Sprintf("CREATE UNIQUE INDEX %s ON users(%s) WHERE %s", i.name, i.column, i.predicate)
}

// isLiveOnly reports whether an existing index already excludes deleted rows.
//
// Deliberately not "does it have a predicate at all". migrateUsernameIndex,
// which runs earlier and predates this, gives the username index a predicate
// that excludes blank usernames but not deleted ones — so treating any WHERE
// clause as good enough silently left that index half-fixed, and a deleted
// user's username reserved forever. The catalogue renders the clause as
// `(deleted_at IS NULL)`, so match on the column and the test, not the exact
// spelling of the whole predicate.
func isLiveOnly(indexDef string) bool {
	return strings.Contains(indexDef, "deleted_at IS NULL")
}

// migrateRoleNameIndex rewrites idx_roles_name to cover live roles only, so a
// deleted role's name can be given to a new role. Role names are only looked
// up among live roles, so a deleted duplicate is never matched.
func (c *Client) migrateRoleNameIndex() error {
	var indexDef string
	if err := c.db.Raw(`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_roles_name'`).
		Scan(&indexDef).Error; err != nil {
		return fmt.Errorf("failed to inspect idx_roles_name: %w", err)
	}
	if isLiveOnly(indexDef) {
		return nil
	}
	c.logger.Info("Rewriting the role name index to cover live roles only")
	if err := c.db.Exec(`DROP INDEX IF EXISTS idx_roles_name`).Error; err != nil {
		return fmt.Errorf("failed to drop idx_roles_name: %w", err)
	}
	return c.db.Exec(`CREATE UNIQUE INDEX idx_roles_name ON roles (name) WHERE deleted_at IS NULL`).Error
}

// removeDeletedRoleAssignments deletes the user assignments of deleted roles.
// DeleteRole removes them now, but roles deleted before it did left theirs
// behind: they grant nothing, yet show as blank roles on users. Running it
// again finds nothing.
func (c *Client) removeDeletedRoleAssignments() error {
	result := c.db.Exec(`DELETE FROM user_roles USING roles
		WHERE user_roles.role_id = roles.id AND roles.deleted_at IS NOT NULL`)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		c.logger.Info("Removed user assignments of deleted roles",
			logger.Int64("assignments", result.RowsAffected))
	}
	return nil
}

// migrateUserUniqueIndexes rewrites the unique indexes on users so they cover
// live, non-empty rows only.
//
// Two separate problems share one fix. PostgreSQL treats the empty string as a
// value rather than as absent, so a plain unique index over a column that is
// blank for a whole class of account admits the first and rejects the rest.
// And a soft delete leaves the row in place, so a deleted user's address,
// subject and username stay reserved forever: the same person cannot be
// re-added, and the failure arrives as a constraint violation nowhere near the
// deletion that caused it.
//
// Narrowing a unique index can never fail on data that satisfied the wider one,
// so this is safe to apply to any existing database: every row that fits the
// plain index still fits the partial one.
//
// Idempotent. An index already carrying a predicate is left alone, so the
// common case is three catalogue reads and no writes.
func (c *Client) migrateUserUniqueIndexes() error {
	for _, idx := range userUniqueIndexes {
		var indexDef string
		if err := c.db.Raw(
			`SELECT indexdef FROM pg_indexes WHERE indexname = ?`, idx.name).
			Scan(&indexDef).Error; err != nil {
			return fmt.Errorf("failed to inspect %s: %w", idx.name, err)
		}

		if isLiveOnly(indexDef) {
			continue
		}

		if indexDef != "" {
			c.logger.Info("Rewriting a unique index on users to cover live rows only",
				logger.Str("index", idx.name))
			if err := c.db.Exec("DROP INDEX IF EXISTS " + idx.name).Error; err != nil {
				return fmt.Errorf("failed to drop %s: %w", idx.name, err)
			}
		}

		if err := c.db.Exec(idx.createSQL()).Error; err != nil {
			return fmt.Errorf("failed to create %s: %w", idx.name, err)
		}
	}
	return nil
}

// assertUserUniqueIndexesAreSafe refuses to start when a unique index on users
// would reject accounts the application considers valid.
//
// It is a backstop rather than the mechanism: migrateUserUniqueIndexes above
// puts the indexes right, and its failure is logged and tolerated so that an
// unrelated hiccup cannot keep the whole platform down. That tolerance is what
// makes this necessary. The migration failing is survivable; serving traffic
// afterwards, with an index that silently blocks sign-ins or lets duplicates
// through, is not. The two decisions were previously one.
//
// Both wrong states are refused. A plain index over the bare column rejects
// accounts that are legitimately blank in it, and rejects any user whose
// identifier belongs to a soft-deleted row. A missing index is refused too:
// every path through the migration ends with the index in place, so its absence
// means the migration failed, and nothing then enforces uniqueness at all.
func (c *Client) assertUserUniqueIndexesAreSafe() error {
	for _, idx := range userUniqueIndexes {
		var indexDef string
		if err := c.db.Raw(
			`SELECT indexdef FROM pg_indexes WHERE indexname = ?`, idx.name).
			Scan(&indexDef).Error; err != nil {
			return fmt.Errorf("failed to inspect %s: %w", idx.name, err)
		}

		switch {
		case isLiveOnly(indexDef):
			continue
		case indexDef == "":
			return fmt.Errorf(
				"refusing to start: there is no %s, so nothing enforces uniqueness of "+
					"users.%s and two accounts can share one. The migration that creates "+
					"it most often fails because the column is already duplicated: resolve "+
					"that, then `%s`", idx.name, idx.column, idx.createSQL())
		default:
			return fmt.Errorf(
				"refusing to start: %s does not exclude soft-deleted rows, so it also "+
					"covers rows the platform has deleted and rows with no %s — %s. "+
					"Recreate it as `%s`. Current definition: %s",
				idx.name, idx.column, idx.why, idx.createSQL(), indexDef)
		}
	}
	return nil
}

// migrateUserAuthMethod sets auth_method for existing users based on their authentication data.
// Users with password_hash are simple auth, users without are OAuth.
func (c *Client) migrateUserAuthMethod() error {
	c.logger.Info("Running user auth_method data migration")

	// Update users WITHOUT password_hash to 'oauth' (OAuth users have no password)
	// Check using LENGTH to handle NULL, empty strings, and whitespace
	result := c.db.Exec(`
		UPDATE users
		SET auth_method = 'oauth'
		WHERE (auth_method IS NULL OR auth_method = '' OR auth_method = 'simple')
		AND (password_hash IS NULL OR LENGTH(TRIM(COALESCE(password_hash, ''))) = 0)
	`)
	if result.Error != nil {
		return fmt.Errorf("failed to update OAuth users: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		c.logger.Info("Updated OAuth users", logger.Int64("count", result.RowsAffected))
	}

	// Update users WITH password_hash to 'simple' (Simple auth users have a password hash)
	result = c.db.Exec(`
		UPDATE users
		SET auth_method = 'simple'
		WHERE (auth_method IS NULL OR auth_method = '')
		AND password_hash IS NOT NULL
		AND LENGTH(TRIM(password_hash)) > 0
	`)
	if result.Error != nil {
		return fmt.Errorf("failed to update simple auth users: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		c.logger.Info("Updated simple auth users", logger.Int64("count", result.RowsAffected))
	}

	c.logger.Info("User auth_method migration completed")
	return nil
}

// setupTimescaleDB enables TimescaleDB extension and creates hypertables.
func (c *Client) setupTimescaleDB() error {
	c.logger.Info("Setting up TimescaleDB extension and hypertables")

	// Enable TimescaleDB extension
	if err := c.db.Exec("CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;").Error; err != nil {
		return fmt.Errorf("failed to enable TimescaleDB extension: %w", err)
	}

	// Create hypertables for time-series tables
	hypertables := []struct {
		table         string
		timeColumn    string
		chunkInterval string
		segmentBy     string // Column for compression segmentation (empty = no segmentation)
	}{
		{"keycloak_events", "time", "1 day", "tenant_id,realm_name"},
		{"keycloak_metrics", "time", "1 day", "tenant_id,realm_name"},
		{"keycloak_health", "time", "1 day", "tenant_id"},
	}

	for _, ht := range hypertables {
		// Check if hypertable already exists
		var exists bool
		query := `
			SELECT EXISTS (
				SELECT 1 FROM timescaledb_information.hypertables
				WHERE hypertable_name = ?
			);
		`
		if err := c.db.Raw(query, ht.table).Scan(&exists).Error; err != nil {
			c.logger.Warn("Failed to check if hypertable exists",
				logger.Str("table", ht.table),
				logger.Err(err))
			continue
		}

		if !exists {
			// Create hypertable
			createHypertableSQL := fmt.Sprintf(
				"SELECT create_hypertable('%s', '%s', chunk_time_interval => INTERVAL '%s', if_not_exists => TRUE);",
				ht.table, ht.timeColumn, ht.chunkInterval,
			)
			if err := c.db.Exec(createHypertableSQL).Error; err != nil {
				c.logger.Warn("Failed to create hypertable",
					logger.Str("table", ht.table),
					logger.Err(err))
				continue
			}
			c.logger.Info("Created hypertable",
				logger.Str("table", ht.table),
				logger.Str("time_column", ht.timeColumn),
				logger.Str("chunk_interval", ht.chunkInterval))
		} else {
			c.logger.Info("Hypertable already exists",
				logger.Str("table", ht.table))
		}

		// Set up compression (optional but recommended for storage efficiency)
		// Compress chunks older than 7 days
		var compressionSQL string
		if ht.segmentBy != "" {
			compressionSQL = fmt.Sprintf(
				"ALTER TABLE %s SET (timescaledb.compress, timescaledb.compress_segmentby = '%s');",
				ht.table, ht.segmentBy,
			)
		} else {
			compressionSQL = fmt.Sprintf(
				"ALTER TABLE %s SET (timescaledb.compress);",
				ht.table,
			)
		}

		if err := c.db.Exec(compressionSQL).Error; err != nil {
			c.logger.Debug("Failed to enable compression",
				logger.Str("table", ht.table),
				logger.Err(err))
		} else {
			if ht.segmentBy != "" {
				c.logger.Info("Enabled compression with segmentby",
					logger.Str("table", ht.table),
					logger.Str("segment_by", ht.segmentBy))
			} else {
				c.logger.Info("Enabled compression",
					logger.Str("table", ht.table))
			}
		}

		// Add compression policy to automatically compress old chunks
		compressionPolicySQL := fmt.Sprintf(
			"SELECT add_compression_policy('%s', INTERVAL '7 days', if_not_exists => TRUE);",
			ht.table,
		)
		if err := c.db.Exec(compressionPolicySQL).Error; err != nil {
			c.logger.Debug("Failed to add compression policy",
				logger.Str("table", ht.table),
				logger.Err(err))
		} else {
			c.logger.Info("Added compression policy",
				logger.Str("table", ht.table),
				logger.Str("compress_after", "7 days"))
		}

		// Add retention policy to automatically drop old data (optional)
		// Drop chunks older than 90 days
		retentionPolicySQL := fmt.Sprintf(
			"SELECT add_retention_policy('%s', INTERVAL '90 days', if_not_exists => TRUE);",
			ht.table,
		)
		if err := c.db.Exec(retentionPolicySQL).Error; err != nil {
			c.logger.Debug("Failed to add retention policy",
				logger.Str("table", ht.table),
				logger.Err(err))
		} else {
			c.logger.Info("Added retention policy",
				logger.Str("table", ht.table),
				logger.Str("drop_after", "90 days"))
		}
	}

	c.logger.Info("TimescaleDB setup completed successfully")
	return nil
}

// Close closes the database connection.
func (c *Client) Close() error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}
	c.logger.Info("Closing database connection")
	return sqlDB.Close()
}

// DB returns the underlying GORM DB instance.
func (c *Client) DB() *gorm.DB {
	return c.db
}

// Health checks if the database connection is healthy.
func (c *Client) Health(ctx context.Context) error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// migrateDropPasswordGrantColumns drops the NOT NULL constraint (and sets an
// empty-string default) on the legacy admin_username / admin_password columns
// so that the application can insert new tenants without providing these
// fields. The columns themselves are retained to avoid data loss.
func (c *Client) migrateDropPasswordGrantColumns() error {
	c.logger.Info("Migrating password-grant column constraints")

	type result struct{ Exists bool }

	for _, col := range []string{"admin_username", "admin_password"} {
		var r result
		query := `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'keycloak_tenants' AND column_name = $1
			) AS exists`
		if err := c.db.Raw(query, col).Scan(&r).Error; err != nil {
			return fmt.Errorf("failed to check column %s: %w", col, err)
		}
		if !r.Exists {
			continue
		}

		sql := fmt.Sprintf(`
			ALTER TABLE keycloak_tenants
				ALTER COLUMN %s DROP NOT NULL,
				ALTER COLUMN %s SET DEFAULT ''`, col, col)
		if err := c.db.Exec(sql).Error; err != nil {
			c.logger.Warn("Could not relax constraint on column",
				logger.Str("column", col),
				logger.Err(err))
		} else {
			c.logger.Info("Relaxed NOT NULL constraint on legacy column",
				logger.Str("column", col))
		}
	}

	c.logger.Info("Password-grant column migration completed")
	return nil
}

// migrateInfinispanDetection sets infinispan_enabled flag for existing tenants.
// Note: This is a simple migration that defaults to false. Administrators can
// manually update this field via the API or directly in the database for tenants
// that have InfiniSpan configured.
func (c *Client) migrateInfinispanDetection() error {
	c.logger.Info("Running InfiniSpan detection data migration")

	// Check if the column exists (in case this is a fresh install)
	var columnExists bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'keycloak_tenants'
			AND column_name = 'infinispan_enabled'
		);
	`
	if err := c.db.Raw(query).Scan(&columnExists).Error; err != nil {
		return fmt.Errorf("failed to check if infinispan_enabled column exists: %w", err)
	}

	if !columnExists {
		c.logger.Info("infinispan_enabled column does not exist yet, skipping migration")
		return nil
	}

	// Count tenants without infinispan_enabled set
	var unsetCount int64
	if err := c.db.Model(&KeycloakTenant{}).
		Where("infinispan_enabled IS NULL").
		Count(&unsetCount).Error; err != nil {
		return fmt.Errorf("failed to count tenants: %w", err)
	}

	if unsetCount == 0 {
		c.logger.Info("All tenants already have infinispan_enabled set")
		return nil
	}

	// Set infinispan_enabled to false for existing tenants where it's NULL
	// Administrators can update this manually later for tenants with InfiniSpan
	result := c.db.Model(&KeycloakTenant{}).
		Where("infinispan_enabled IS NULL").
		Update("infinispan_enabled", false)

	if result.Error != nil {
		return fmt.Errorf("failed to set infinispan_enabled for existing tenants: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		c.logger.Info("Set infinispan_enabled to false for existing tenants (can be updated manually)",
			logger.Int64("count", result.RowsAffected))
	}

	c.logger.Info("InfiniSpan detection migration completed")
	return nil
}

// amfaReadGrantKey marks the one-time amfa:read grant as applied.
const amfaReadGrantKey = "rbac.grant_amfa_read"

// migrateAmfaReadPermission creates the amfa:read permission and grants it to
// the default admin, operator, and viewer roles, which existed before it did.
//
// It runs once, marked in data_migrations. Run on every startup, it put back
// a grant that had been taken away since. On a fresh database the default
// roles do not exist yet; the seeder creates them, and GetSystemRoles gives
// them amfa:read.
func (c *Client) migrateAmfaReadPermission() error {
	applied, err := dataMigrationApplied(c.db, amfaReadGrantKey)
	if err != nil || applied {
		return err
	}

	var granted []string
	err = c.db.Transaction(func(tx *gorm.DB) error {
		// Held until this transaction ends, so a second startup waits and
		// then sees the marker instead of granting again.
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?)::bigint)`, amfaReadGrantKey).Error; err != nil {
			return err
		}
		applied, err := dataMigrationApplied(tx, amfaReadGrantKey)
		if err != nil || applied {
			return err
		}

		permission := Permission{
			Name:        "amfa:read",
			DisplayName: "Read AMFA Events",
			Description: "View AMFA (Adaptive MFA) events, risk metrics, and geolocation data",
			Resource:    "amfa",
			Action:      "read",
			IsSystem:    true,
		}
		if err := tx.Where(Permission{Name: "amfa:read"}).
			Attrs(permission).
			FirstOrCreate(&permission).Error; err != nil {
			return fmt.Errorf("failed to ensure amfa:read permission exists: %w", err)
		}

		for _, roleName := range []string{"admin", "operator", "viewer"} {
			var role Role
			err := tx.Where("name = ?", roleName).First(&role).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return fmt.Errorf("failed to look up role %q: %w", roleName, err)
			}

			var n int64
			if err := tx.Model(&RolePermission{}).
				Where("role_id = ? AND permission_id = ?", role.ID, permission.ID).
				Count(&n).Error; err != nil {
				return fmt.Errorf("failed to check role-permission for role %q: %w", roleName, err)
			}
			if n > 0 {
				continue
			}
			if err := tx.Create(&RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
				return fmt.Errorf("failed to grant amfa:read to role %q: %w", roleName, err)
			}
			granted = append(granted, roleName)
		}

		return tx.Create(&DataMigration{Key: amfaReadGrantKey, AppliedAt: time.Now()}).Error
	})
	if err != nil {
		return err
	}
	for _, roleName := range granted {
		c.logger.Info("Granted amfa:read permission to role", logger.Str("role", roleName))
	}
	return nil
}
