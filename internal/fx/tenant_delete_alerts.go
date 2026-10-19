package fx

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspostgres "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/lifecycle"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// closeDeletedTenantAlertsTimeout bounds how long closing a deleted tenant's
// alerts may take in the background, so a hung notification channel cannot
// keep the goroutine forever.
const closeDeletedTenantAlertsTimeout = 2 * time.Minute

// tenantAlertCloser closes, on its notification channels, every alert of a
// tenant that is deleted.
//
// Deleting a tenant purges its alerts with the rest of its rows, so the
// closing that would otherwise follow never happened: a deleted tenant's open
// alerts kept their Slack threads and GitLab issues open forever. This reads
// the tenant's unresolved alerts before the purge and, once the deletion has
// committed, sends each one's resolution, saying the tenant was deleted.
// Ignored alerts are included: the alert itself is gone, so nothing else
// would ever close an issue it opened.
//
// The resolutions go out in the background: a slow or unreachable channel
// must not hold up, or fail, the deletion. Shutdown waits for them, so a
// restart right after a deletion does not drop them: when it did, most of a
// tenant's issues stayed open. The notification service finds what was sent,
// and the GitLab issue to close, without the alert row.
type tenantAlertCloser struct {
	alertRepo alerts.Repository
	notifier  notifications.Service
	log       *logger.Logger
	workers   lifecycle.Workers
}

var _ tenant.DeletionHook = (*tenantAlertCloser)(nil)

func newTenantAlertCloser(repo alerts.Repository, notifier notifications.Service, log *logger.Logger) *tenantAlertCloser {
	return &tenantAlertCloser{alertRepo: repo, notifier: notifier, log: log.WithComponent("tenant_alert_closer")}
}

// provideTenantAlertCloser creates the closer and has shutdown wait for the
// closing it has under way. It takes the database client, so its stop hook
// is registered after the database's and runs before it: the closing writes
// to the notification log.
func provideTenantAlertCloser(lc fx.Lifecycle, db *database.Client, notifier notifications.Service, log *logger.Logger) *tenantAlertCloser {
	c := newTenantAlertCloser(alertspostgres.NewRepository(db.DB()), notifier, log)
	lc.Append(fx.Hook{OnStop: c.drain})
	return c
}

// drain waits, within the shutdown deadline, for closing under way.
func (c *tenantAlertCloser) drain(ctx context.Context) error {
	timeout := 15 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}
	if !c.workers.Drain(timeout) {
		c.log.Warn("Shutdown did not wait for every deleted tenant's alerts to be closed; some of their notifications may stay open")
	}
	return nil
}

// PrepareDeletion reads the tenant's unresolved alerts and returns what closes
// them once the deletion has committed.
func (c *tenantAlertCloser) PrepareDeletion(ctx context.Context, t *domain.KeycloakTenant) func(context.Context) {
	if c.notifier == nil {
		return nil
	}
	open, err := c.alertRepo.ListUnresolved(ctx, t.TenantID)
	if err != nil {
		c.log.Warn("Could not read a deleted tenant's alerts; their notifications stay open",
			logger.Str("tenant_id", t.TenantID), logger.Err(err))
		return nil
	}
	if len(open) == 0 {
		return nil
	}
	return func(ctx context.Context) {
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), closeDeletedTenantAlertsTimeout)
		c.workers.Go(func() {
			defer cancel()
			c.close(bg, t, open)
		})
	}
}

func (c *tenantAlertCloser) close(ctx context.Context, t *domain.KeycloakTenant, open []*domain.Alert) {
	note := fmt.Sprintf("Tenant '%s' was deleted, so this alert was closed without a recovery.", t.Name)
	now := time.Now()
	for _, a := range open {
		a.Status = domain.AlertStatusResolved
		a.ResolvedAt = &now
		a.ResolutionNote = note
		if err := c.notifier.NotifyAlertResolution(ctx, a); err != nil {
			c.log.Warn("Could not close a deleted tenant's alert on its notification channels",
				logger.Str("tenant_id", t.TenantID), logger.Str("alert_id", a.AlertID), logger.Err(err))
		}
	}
	// A channel that fails is logged by the notification service, per alert,
	// and does not surface here.
	c.log.Info("Requested closing the alerts of a deleted tenant; the notification service logs each channel's outcome",
		logger.Str("tenant_id", t.TenantID),
		logger.Int("alerts", len(open)))
}
