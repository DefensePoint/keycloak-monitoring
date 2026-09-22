package fx

import (
	"go.uber.org/fx"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/events"
	eventspostgres "github.com/DefensePoint/keycloak-monitoring/events/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/users"
	userspostgres "github.com/DefensePoint/keycloak-monitoring/users/postgres"
)

// RepositoriesModule provides all repositories
var RepositoriesModule = fx.Module("repositories",
	fx.Provide(
		// Event and User repositories
		provideEventRepository,
		provideUserRepository,
	),
)

// Event repository
func provideEventRepository(db *gorm.DB, log *logger.Logger) events.Repository {
	return eventspostgres.NewRepository(db, log)
}

// User repository
func provideUserRepository(db *gorm.DB) users.Repository {
	return userspostgres.NewRepository(db)
}
