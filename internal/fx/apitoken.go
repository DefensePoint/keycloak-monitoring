package fx

import (
	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/apitoken"
	apitokenpostgres "github.com/DefensePoint/keycloak-monitoring/internal/apitoken/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// APITokenModule provides API token domain dependencies.
var APITokenModule = fx.Module("apitoken",
	fx.Provide(
		provideAPITokenRepository,
		provideAPITokenService,
	),
)

// provideAPITokenRepository creates an apitoken repository.
func provideAPITokenRepository(db *database.Client) apitoken.Repository {
	return apitokenpostgres.NewRepository(db.DB())
}

// provideAPITokenService creates an apitoken service.
func provideAPITokenService(repo apitoken.Repository, userRepo users.Repository, log *logger.Logger) apitoken.Service {
	return apitoken.NewService(repo, userRepo, &apitokenLoggerAdapter{log: log})
}

// apitokenLoggerAdapter adapts *logger.Logger to apitoken.Logger.
type apitokenLoggerAdapter struct {
	log *logger.Logger
}

func (a *apitokenLoggerAdapter) Info(msg string, fields ...any) {
	a.log.Info(msg, convertToLoggerFields(fields)...)
}

func (a *apitokenLoggerAdapter) Error(msg string, fields ...any) {
	a.log.Error(msg, convertToLoggerFields(fields)...)
}

func (a *apitokenLoggerAdapter) Warn(msg string, fields ...any) {
	a.log.Warn(msg, convertToLoggerFields(fields)...)
}

func (a *apitokenLoggerAdapter) Debug(msg string, fields ...any) {
	a.log.Debug(msg, convertToLoggerFields(fields)...)
}
