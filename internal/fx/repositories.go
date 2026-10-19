package fx

import (
	"go.uber.org/fx"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
	eventspostgres "github.com/DefensePoint/keycloak-monitoring/internal/events/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
	userspostgres "github.com/DefensePoint/keycloak-monitoring/internal/users/postgres"
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
func provideUserRepository(db *gorm.DB, cfg *config.AppConfig, log *logger.Logger) users.Repository {
	link := cfg.Auth.OAuth2.LinkByVerifiedEmail
	if link {
		// Said at every start, so a setting meant for one migration is not
		// left on and forgotten.
		log.Warn("auth.oauth2.link_by_verified_email is on: a login with a new subject takes over " +
			"the SSO account holding its verified email address. Turn it off once the realm " +
			"rebuild or migration it was enabled for is done")
	}
	return userspostgres.NewRepository(db, userspostgres.WithLinkByVerifiedEmail(link))
}
