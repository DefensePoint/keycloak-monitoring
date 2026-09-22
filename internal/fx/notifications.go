package fx

import (
	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/configcheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/eventscheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metricscheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
	notificationspostgres "github.com/DefensePoint/keycloak-monitoring/internal/notifications/postgres"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// NotificationsModule provides notification domain dependencies.
var NotificationsModule = fx.Module("notifications",
	fx.Provide(
		provideNotificationsRepository,
		provideNotificationsService,
		provideConfigcheckNotificationService,
		provideEventscheckNotificationService,
		provideMetricscheckNotificationService,
	),
)

func provideNotificationsRepository(db *database.Client) notifications.Repository {
	return notificationspostgres.NewRepository(db)
}

func provideNotificationsService(
	repo notifications.Repository,
	cfg *config.AppConfig,
	log *logger.Logger,
) notifications.Service {
	return notifications.NewFullService(repo, &cfg.Notifications, log)
}

// Since all check packages now use domain.Alert (via aliases), notifications.Service
// directly satisfies their NotificationService interfaces.

func provideConfigcheckNotificationService(svc notifications.Service) configcheck.NotificationService {
	return svc
}

func provideEventscheckNotificationService(svc notifications.Service) eventscheck.NotificationService {
	return svc
}

func provideMetricscheckNotificationService(svc notifications.Service) metricscheck.NotificationService {
	return svc
}
