package fx

import (
	"go.uber.org/fx"

	"repos.defensepoint.com/iam/oss-pmt/kmt/alerts"
	alertspostgres "repos.defensepoint.com/iam/oss-pmt/kmt/alerts/postgres"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/database"
)

// AlertsModule provides alerts domain dependencies.
var AlertsModule = fx.Module("alerts",
	fx.Provide(
		provideAlertsRepository,
		provideAlertsService,
	),
)

// provideAlertsRepository creates an alerts repository.
func provideAlertsRepository(db *database.Client) alerts.Repository {
	return alertspostgres.NewRepository(db.DB())
}

// provideAlertsService creates an alerts service.
func provideAlertsService(repo alerts.Repository, log *logger.Logger) alerts.Service {
	return alerts.NewService(repo, log)
}
