package postgres

import (
	"context"
	"fmt"
	"time"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// NewClient opens a read-only GORM connection to one tenant's AMFA database.
// The session is opened read-only at the Postgres level (see buildDSN), and
// AutoMigrate is NEVER called on this connection — the AMFA database schema
// is owned by AMFA, not KMT. A nil logger is tolerated for tests.
func NewClient(cfg config.AmfaDatabaseConfig, log *logger.Logger) (*gorm.DB, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("amfa client: database.host is required")
	}
	if cfg.Database == "" {
		return nil, fmt.Errorf("amfa client: database.database is required")
	}
	if cfg.User == "" {
		return nil, fmt.Errorf("amfa client: database.user is required")
	}

	dsn := buildDSN(cfg)

	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		return nil, fmt.Errorf("amfa client: connect: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("amfa client: extract sql.DB: %w", err)
	}

	if cfg.MaxConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxConns)
	}
	if cfg.MinConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MinConns)
	}
	// NOTE: cfg.Timeout is a per-request/dial timeout (used below for ping
	// and propagated via ctx in queries via GORM's WithContext). It must NOT
	// be passed to SetConnMaxLifetime, which would force every pooled
	// connection to recycle every cfg.Timeout seconds and defeat both pool
	// reuse and PrepareStmt caching. Leave connection lifetime at the
	// driver default (no expiry).

	pingTimeout := cfg.Timeout
	if pingTimeout <= 0 {
		pingTimeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("amfa client: ping: %w", err)
	}

	if log != nil {
		log.Info("AMFA database connected",
			logger.Str("host", hostUsed(cfg)),
			logger.Str("database", cfg.Database))
	}
	return db, nil
}

// buildDSN returns the Postgres DSN, preferring the read replica when
// configured. SSL mode defaults to "require" if not set.
//
// default_transaction_read_only makes the read-only intent something Postgres
// enforces rather than something this package promises: every statement runs
// in a read-only transaction, implicit single-statement ones included, so a
// write is refused by the server even where the configured AMFA credentials
// would allow it.
func buildDSN(cfg config.AmfaDatabaseConfig) string {
	host := hostUsed(cfg)
	sslmode := cfg.SSLMode
	if sslmode == "" {
		sslmode = "require"
	}
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s options='-c default_transaction_read_only=on'",
		host, cfg.Port, cfg.Database, cfg.User, cfg.Password, sslmode,
	)
}

// hostUsed returns the host that will be connected to. When a read replica is
// configured the replica host wins; otherwise the primary host is used.
func hostUsed(cfg config.AmfaDatabaseConfig) string {
	if cfg.ReadReplicaHost != "" {
		return cfg.ReadReplicaHost
	}
	return cfg.Host
}
