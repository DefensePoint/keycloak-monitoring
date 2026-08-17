package fx

import (
	"go.uber.org/fx"
	"gorm.io/gorm"

	"repos.defensepoint.com/iam/oss-pmt/kmt/events"
	eventspostgres "repos.defensepoint.com/iam/oss-pmt/kmt/events/postgres"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/users"
	userspostgres "repos.defensepoint.com/iam/oss-pmt/kmt/users/postgres"
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
