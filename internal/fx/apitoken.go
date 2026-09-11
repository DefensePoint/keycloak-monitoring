package fx

import (
	"go.uber.org/fx"

	"repos.defensepoint.com/iam/oss-pmt/kmt/apitoken"
	apitokenpostgres "repos.defensepoint.com/iam/oss-pmt/kmt/apitoken/postgres"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/database"
	"repos.defensepoint.com/iam/oss-pmt/kmt/users"
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
