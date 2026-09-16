package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/secretcrypto"
)

// Client represents a PostgreSQL database client using GORM.
type Client struct {
	db     *gorm.DB
	logger *logger.Logger
}

// NewClient creates a new GORM database client and auto-migrates the schema.
func NewClient(cfg *config.DatabaseConfig, log *logger.Logger) (*Client, error) {
	client, err := openClient(cfg, log)
	if err != nil {
		return nil, err
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

// openClient opens the GORM connection, configures the pool and pings.
func openClient(cfg *config.DatabaseConfig, log *logger.Logger) (*Client, error) {
	// Build connection DSN
	dsn := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		cfg.Host,
		cfg.Port,
		cfg.Database,
		cfg.User,
		cfg.Password,
		cfg.SSLMode,
	)

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
	if cfg.Timeout > 0 {
		sqlDB.SetConnMaxLifetime(cfg.Timeout)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

// backfillEventTenantIDs attributes pre-existing event rows to a tenant where
// that can be established, and leaves the rest alone.
//
// Rows written before events.tenant_id existed recorded no tenant. The only
// tenant signal they carry is the realm name inside source
// ("keycloak:<realm>", "amfa:<realm>"), so a row is attributable exactly when
// its realm name belongs to one tenant and no other. Where a realm name is
// shared, and "master" is shared by every Keycloak, the owner is genuinely
// unrecoverable and the row keeps its empty tenant_id: excluded from every
// tenant-scoped query, and still awaiting a decision on whether to delete it.
//
// Deliberately not a guess. Attributing a shared-realm row to whichever tenant
// happens to hold that realm now would assert ownership of personal data that
// cannot be verified, which is the opposite of the point.
//
// One honest limit: "unique today" is not quite "always was". If a realm was
// previously owned by a tenant that has since been deleted, its rows attribute
// to the current holder of that name. Nothing recorded enough to do better.
//
// Idempotent: it only touches rows with no tenant, of which writers create
// none, so after the first successful run this is a no-op.
func (c *Client) backfillEventTenantIDs() error {
	const backfill = `
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

	res := c.db.Exec(backfill)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		c.logger.Info("Attributed pre-existing events to a tenant",
			logger.Int64("rows", res.RowsAffected))
	}

	var unattributed int64
	if err := c.db.Raw(
		`SELECT count(*) FROM events WHERE tenant_id IS NULL OR tenant_id = ''`).
		Scan(&unattributed).Error; err == nil && unattributed > 0 {
		c.logger.Warn("Events remain unattributed to any tenant: their realm name is shared "+
			"between tenants so the owner cannot be established. They are excluded from "+
			"tenant-scoped queries and are not removed by tenant deletion",
			logger.Int64("rows", unattributed))
	}
	return nil
}

// AutoMigrate automatically migrates the database schema.
func (c *Client) AutoMigrate() error {
	c.logger.Info("Running GORM auto-migration")

	// Migrate regular tables first (without OperatorAction to avoid FK issues)
	err := c.db.AutoMigrate(&Event{}, &User{}, &KeycloakTenant{}, &KeycloakRealmInfo{}, &Alert{}, &AlertRule{}, &NotificationLog{}, &PendingTenantPurge{}, &AmfaMirrorWatermark{})
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

	// Data migration: Ensure amfa:read permission exists and is granted to default roles
	if err := c.migrateAmfaReadPermission(); err != nil {
		c.logger.Warn("amfa:read permission migration failed", logger.Err(err))
	}

	// Data migration: Set auth_method for existing users
	if err := c.migrateUserAuthMethod(); err != nil {
		c.logger.Warn("Failed to migrate user auth_method, continuing", logger.Err(err))
	}

	// Data migration: clear email_verified that was asserted rather than observed
	if err := c.migrateUnprovenEmailVerified(); err != nil {
		c.logger.Warn("Failed to clear unproven email_verified, continuing", logger.Err(err))
	}

	// Data migration: Fix username unique index to allow multiple NULL/empty values
	if err := c.migrateUsernameIndex(); err != nil {
		c.logger.Warn("Failed to migrate username index, continuing", logger.Err(err))
	}

	// ...but do not continue past the state that migration exists to prevent.
	if err := c.assertUsernameIndexIsSafe(); err != nil {
		return err
	}

	// Data migration: Detect InfiniSpan for existing tenants
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
// composite (tenant_id, realm_name) unique index. It was originally created
// on realm_name alone, which made a realm name globally unique across the
// whole table instead of unique per tenant — any second tenant pointed at a
// Keycloak instance already monitored by another tenant would fail to save
// realm info for every realm name that tenant had already saved.
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

	if strings.Contains(indexDef, "tenant_id") {
		c.logger.Info("Realm name index is already scoped per tenant, skipping migration")
		return nil
	}

	c.logger.Info("Dropping globally-unique realm_name index")
	if err := c.db.Exec("DROP INDEX IF EXISTS idx_keycloak_realms_tenant_realm_name").Error; err != nil {
		return fmt.Errorf("failed to drop old index: %w", err)
	}

	c.logger.Info("Creating tenant-scoped realm_name index")
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
// (tenant_id, alert_id) unique index. It was originally created on alert_id
// alone, which made an alert_id globally unique across the whole table
// instead of unique per tenant. configuration_alerts IDs are a hash of
// check_type/realm_name/alert_type with no tenant in the input, so two
// tenants with a same-named realm produced identical alert_ids: the second
// tenant's Save (an upsert keyed on alert_id) silently updated the first
// tenant's row instead of creating its own, with no error and no ownership
// change.
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

	if strings.Contains(indexDef, "tenant_id") {
		c.logger.Info("Alert ID index is already scoped per tenant, skipping migration")
		return nil
	}

	c.logger.Info("Dropping globally-unique alert_id index")
	if err := c.db.Exec("DROP INDEX IF EXISTS idx_alert_tenant").Error; err != nil {
		return fmt.Errorf("failed to drop old index: %w", err)
	}

	c.logger.Info("Creating tenant-scoped alert_id index")
	createIndexSQL := `CREATE UNIQUE INDEX idx_alert_tenant ON configuration_alerts(tenant_id, alert_id) WHERE deleted_at IS NULL`
	if err := c.db.Exec(createIndexSQL).Error; err != nil {
		return fmt.Errorf("failed to create tenant-scoped index: %w", err)
	}

	c.logger.Info("Successfully migrated alert_id index to be tenant-scoped")
	return nil
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

// assertUsernameIndexIsSafe refuses to start when a non-partial unique index
// sits on users(username).
//
// Every OAuth user is created with an empty username — only simple auth sets
// one — and PostgreSQL treats the empty string as a value, not as absent. So a
// plain unique index admits the first OAuth user and rejects every one after
// them: SSO appears to work, because whoever signed in first is fine, and
// nobody else can be created. The failure surfaces as a constraint violation
// from a login, far from the index that caused it.
//
// migrateUsernameIndex above converts the index GORM creates into a partial
// one, and its failure is logged and tolerated so that an unrelated hiccup
// cannot keep the whole platform down. That tolerance is what makes this check
// necessary: the migration failing is survivable, serving traffic afterwards is
// not, and the two need separating.
//
// A missing index is refused too, and that is the case worth understanding.
// Every path through migrateUsernameIndex ends with the index in place, so its
// absence means the migration failed — and the way it fails in practice is
// CREATE UNIQUE INDEX rejecting duplicate usernames after the old index was
// already dropped. Continuing from there leaves nothing enforcing username
// uniqueness at all, so two simple-auth accounts can share a username and a
// lookup by username returns whichever the planner reaches first. Silently
// weaker than the state we started in.
func (c *Client) assertUsernameIndexIsSafe() error {
	var indexDef string
	err := c.db.Raw(
		`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_users_username'`).
		Scan(&indexDef).Error
	if err != nil {
		return fmt.Errorf("failed to inspect the username index: %w", err)
	}

	const repair = "`CREATE UNIQUE INDEX idx_users_username ON users(username) " +
		"WHERE username IS NOT NULL AND username != ''`"

	switch {
	case strings.Contains(indexDef, "WHERE"):
		return nil
	case indexDef == "":
		return fmt.Errorf(
			"refusing to start: there is no idx_users_username, so nothing enforces "+
				"username uniqueness and two accounts can share one. The migration that "+
				"creates it most often fails because usernames are already duplicated: "+
				"resolve those, then %s", repair)
	default:
		return fmt.Errorf(
			"refusing to start: idx_users_username is a plain unique index, so only one "+
				"OAuth user can exist (all of them have an empty username) and every "+
				"subsequent SSO sign-in fails on a constraint violation; recreate it as "+
				"%s. Current definition: %s", repair, indexDef)
	}
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

// migrateAmfaReadPermission ensures the amfa:read permission exists and is
// granted to the default admin, operator, and viewer roles. This is idempotent:
// running it multiple times will not produce duplicates or errors.
//
// If any of the default roles do not exist yet (e.g. fresh install before the
// seeder has run), this migration skips them silently. The seeder will create
// them with the permission included via GetSystemRoles().
func (c *Client) migrateAmfaReadPermission() error {
	c.logger.Info("Running amfa:read permission data migration")

	// Ensure the permission row exists
	permission := Permission{
		Name:        "amfa:read",
		DisplayName: "Read AMFA Events",
		Description: "View AMFA (Adaptive MFA) events, risk metrics, and geolocation data",
		Resource:    "amfa",
		Action:      "read",
		IsSystem:    true,
	}
	if err := c.db.Where(Permission{Name: "amfa:read"}).
		Attrs(permission).
		FirstOrCreate(&permission).Error; err != nil {
		return fmt.Errorf("failed to ensure amfa:read permission exists: %w", err)
	}

	// Grant the permission to each default role (admin, operator, viewer)
	defaultRoles := []string{"admin", "operator", "viewer"}
	for _, roleName := range defaultRoles {
		var role Role
		err := c.db.Where("name = ?", roleName).First(&role).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				c.logger.Debug("Default role not found yet, skipping (seeder will create it)",
					logger.Str("role", roleName))
				continue
			}
			return fmt.Errorf("failed to look up role %q: %w", roleName, err)
		}

		// Check if the role-permission join row already exists
		var existing RolePermission
		err = c.db.Where("role_id = ? AND permission_id = ?", role.ID, permission.ID).
			First(&existing).Error
		if err == nil {
			// Already granted; nothing to do.
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return fmt.Errorf("failed to check role-permission for role %q: %w", roleName, err)
		}

		// Insert the join row
		rp := RolePermission{
			RoleID:       role.ID,
			PermissionID: permission.ID,
		}
		if err := c.db.Create(&rp).Error; err != nil {
			return fmt.Errorf("failed to grant amfa:read to role %q: %w", roleName, err)
		}
		c.logger.Info("Granted amfa:read permission to role",
			logger.Str("role", roleName))
	}

	c.logger.Info("amfa:read permission migration completed")
	return nil
}
