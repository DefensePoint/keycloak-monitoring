package fx

import (
	"context"
	"errors"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspostgres "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/lifecycle"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// retriedCheckTypes are the alerts notified only when they are raised: tenant
// health, event storage and single Keycloak events. The checkers re-request
// their own open alerts every hour, which already retries a failed send.
var retriedCheckTypes = []string{"health_check", "event_storage", "event_monitor"}

const defaultNotificationRetryInterval = 5 * time.Minute

type undeliveredLister interface {
	ListUndelivered(ctx context.Context, checkTypes []string) ([]notifications.AlertKey, error)
}

type failedNotificationRetrier interface {
	RetryFailedNotifications(ctx context.Context, alert *domain.Alert) error
}

// notificationRetrier periodically resends the open, unacknowledged alerts of
// retriedCheckTypes to each channel whose send failed during the current
// occurrence. A channel that keeps failing is called once per interval.
type notificationRetrier struct {
	undelivered undeliveredLister
	alerts      alerts.Repository
	notifier    failedNotificationRetrier
	interval    time.Duration
	log         *logger.Logger
	workers     lifecycle.Workers
}

func registerNotificationRetrier(
	lc fx.Lifecycle,
	repo notifications.Repository,
	db *database.Client,
	notifier notifications.Service,
	cfg *config.AppConfig,
	log *logger.Logger,
) {
	interval := cfg.Notifications.RetryInterval
	if interval <= 0 {
		interval = defaultNotificationRetryInterval
	}
	r := &notificationRetrier{
		undelivered: repo,
		alerts:      alertspostgres.NewRepository(db.DB()),
		notifier:    notifier,
		interval:    interval,
		log:         log.WithComponent("notification_retrier"),
	}
	var cancel context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			// fx cancels the OnStart context once startup completes.
			var ctx context.Context
			ctx, cancel = context.WithCancel(context.Background())
			r.workers.Go(func() { r.run(ctx) })
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			if !r.workers.Drain(10 * time.Second) {
				r.log.Warn("Notification retrier stop timeout")
			}
			return nil
		},
	})
}

func (r *notificationRetrier) run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.workers.Stopping():
			return
		case <-ticker.C:
			r.retryOnce(ctx)
		}
	}
}

// retryOnce resends every alert that has an undelivered channel and returns
// how many it retried.
func (r *notificationRetrier) retryOnce(ctx context.Context) int {
	keys, err := r.undelivered.ListUndelivered(ctx, retriedCheckTypes)
	if err != nil {
		r.log.Error("Failed to list alerts with undelivered notifications", logger.Err(err))
		return 0
	}
	retried := 0
	for _, key := range keys {
		alert, err := r.alerts.GetByAlertID(ctx, key.TenantID, key.AlertID)
		if err != nil {
			if !errors.Is(err, alerts.ErrNotFound) {
				r.log.Error("Failed to load alert to retry its notifications",
					logger.Str("alert_id", key.AlertID), logger.Err(err))
			}
			continue
		}
		if alert.Status != domain.AlertStatusActive || alert.AcknowledgedAt != nil {
			continue
		}
		if err := r.notifier.RetryFailedNotifications(ctx, alert); err != nil {
			r.log.Error("Failed to retry notifications",
				logger.Str("alert_id", key.AlertID), logger.Err(err))
			continue
		}
		retried++
	}
	return retried
}
