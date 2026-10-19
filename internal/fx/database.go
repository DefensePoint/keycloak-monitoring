package fx

import (
	"context"

	"go.uber.org/fx"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// DatabaseModule provides database client
var DatabaseModule = fx.Module("database",
	fx.Provide(
		provideDatabase,
		provideGormDB,
	),
)

// provideGormDB extracts the underlying *gorm.DB from the database client.
// This allows domain modules to depend on *gorm.DB directly for cleaner interfaces.
func provideGormDB(client *database.Client) *gorm.DB {
	return client.DB()
}

// checkDatabasePassword refuses a database password that is not set, before
// any connection is attempted, and warns about the one this project publishes
// for local development databases.
func checkDatabasePassword(log *logger.Logger, key, password string) error {
	published, err := config.ValidateDatabasePassword(key, password)
	if err != nil {
		return err
	}
	if published {
		log.Warn("The database password is the one this project publishes for local development "+
			"databases; anywhere else, set a password of your own", logger.Str("setting", key))
	}
	return nil
}

func provideDatabase(lc fx.Lifecycle, cfg *config.AppConfig, log *logger.Logger) (*database.Client, error) {
	if err := checkDatabasePassword(log, "database.password", cfg.Database.Password); err != nil {
		return nil, err
	}
	client, err := database.NewClient(&cfg.Database, log, database.WithPreTenantOwner(preTenantOwner(cfg)))
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			log.Info("Closing database connection")
			return client.Close()
		},
	})

	return client, nil
}

// preTenantOwner is the tenant a database from a single-tenant release is given to: the
// one tenant the configuration defines, or none when it defines zero or
// several, in which case such a database is refused rather than guessed at.
func preTenantOwner(cfg *config.AppConfig) string {
	if len(cfg.Keycloak.Tenants) != 1 {
		return ""
	}
	for id := range cfg.Keycloak.Tenants {
		return id
	}
	return ""
}
