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

func provideDatabase(lc fx.Lifecycle, cfg *config.AppConfig, log *logger.Logger) (*database.Client, error) {
	client, err := database.NewClient(&cfg.Database, log)
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
