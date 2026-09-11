package fx

import (
	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/alerts"
	alertspostgres "github.com/DefensePoint/keycloak-monitoring/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
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
