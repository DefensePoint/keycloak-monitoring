package fx

import (
	"go.uber.org/fx"

	"repos.defensepoint.com/iam/oss-pmt/kmt/configcheck"
	"repos.defensepoint.com/iam/oss-pmt/kmt/eventscheck"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/metricscheck"
	"repos.defensepoint.com/iam/oss-pmt/kmt/notifications"
	notificationspostgres "repos.defensepoint.com/iam/oss-pmt/kmt/notifications/postgres"
	"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/database"
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
