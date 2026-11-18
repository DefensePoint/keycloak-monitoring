package fx

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/lifecycle"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// checkAlertStore is the alert store as the workers need it to resolve a
// tenant's alerts. The configuration and event checkers' stores both have it.
type checkAlertStore interface {
	ListOpenAlertsByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error)
	ResolveAlert(ctx context.Context, tenantID, alertID string) error
}

// checkTypes returns the check types of checks, in order, for alertTypes.
func checkTypes[C interface{ GetCheckType() string }](checks []C) []string {
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		out = append(out, c.GetCheckType())
	}
	return out
}

// resolutionNotifier sends an alert's resolution notice.
type resolutionNotifier interface {
	NotifyAlertResolution(ctx context.Context, alert *domain.Alert) error
}

// tenantSource is the tenant service as the workers need it at startup.
// It holds config.yaml and interface tenants alike: config.yaml tenants are
// synced into the database at startup.
type tenantSource interface {
	LoadTenants(ctx context.Context) error
	ListEnabledTenants(ctx context.Context) ([]*domain.KeycloakTenant, error)
	ListTenants(ctx context.Context) ([]*domain.KeycloakTenant, error)
}

// tenantCheckWorkers runs one check service per enabled tenant (the
// configuration checker's or the event checker's) and keeps them in step with
// tenant changes, as the monitor pool does for the monitors: a tenant that is
// added or enabled gets its service at once, and one that is disabled or
// deleted has it stopped. Before, the services were built once at startup, so
// a tenant added in the interface went unchecked until a restart, and a
// disabled one was checked until then.
//
// An edit keeps a running service. The checks look the tenant's Keycloak
// client up on every call, so they already follow a changed URL or secret,
// and a fresh service would forget how many runs in a row have missed each
// open alert, delaying its resolution.
type tenantCheckWorkers[S lifecycle.Poller] struct {
	// component names the service in log lines: "configcheck" or "eventscheck".
	component string
	// build makes a tenant's service, and reports false when no check applies.
	build func(tenantID string) (S, bool)
	// loadTenants makes start load the tenant cache from the database first.
	// Only for workers that start before the monitoring module has loaded it
	// and before the HTTP server serves: the load writes the database rows
	// over the cache without the lock tenant edits take, so a tenant saved
	// meanwhile would be cached stale.
	loadTenants bool
	log         *logger.Logger

	// alertTypes are the check types of the alerts this worker's checks raise.
	// When a tenant is disabled, its open alerts of these types are resolved
	// once its checks have stopped, with a resolution notice, as the monitors
	// do for health alerts: nothing would ever resolve them otherwise. A
	// deleted tenant gets the same, though deleting a tenant normally removes
	// its alerts before the change reaches the workers, leaving none open.
	alertTypes  []string
	alertStore  checkAlertStore
	resolutions resolutionNotifier

	mu sync.Mutex
	// running holds the tenants that have a service.
	running map[string]bool
	// loaded is set once the startup list has been applied. Until then,
	// changed holds the tenants a change arrived for: the change is newer
	// than the list, which must not undo it.
	loaded  bool
	changed map[string]bool
	// stopped is set by stop; later changes start nothing.
	stopped bool
	// sweep resolves, in the background after start, the open alerts of the
	// tenants that were already disabled; stop cancels and waits for it.
	sweepCancel context.CancelFunc
	sweep       sync.WaitGroup

	pool lifecycle.Pool[S]
}

// follow subscribes to tenant changes. It must be called before start, so no
// change between the startup list and the subscription is lost.
func (w *tenantCheckWorkers[S]) follow(service tenant.Service) {
	service.RegisterCallback(w.tenantChanged)
}

// start gives every enabled tenant its service, then starts them all. With
// loadTenants it loads the tenant cache first: a tenant created in an earlier
// run is never announced as a change, so an unloaded cache would leave it out.
func (w *tenantCheckWorkers[S]) start(ctx context.Context, tenants tenantSource) error {
	if w.loadTenants {
		if err := tenants.LoadTenants(ctx); err != nil {
			return err
		}
	}
	list, err := tenants.ListEnabledTenants(ctx)
	if err != nil {
		return err
	}

	w.mu.Lock()
	for _, t := range list {
		if t != nil && t.Enabled && !w.changed[t.TenantID] {
			w.putLocked(t.TenantID)
		}
	}
	w.loaded = true
	w.changed = nil
	logInfo(w.log, w.component+" services constructed", logger.Int("tenants", len(w.running)))
	w.mu.Unlock()

	if err := w.pool.Start(); err != nil {
		return err
	}
	w.sweepDisabled(tenants)
	return nil
}

// sweepDisabled resolves, in the background, the open alerts of every tenant
// that is disabled when the server starts. A tenant disabled before an
// upgrade, or while the server was down, was never announced as disabled to
// this version, and its alerts would stay open. A tenant enabled meanwhile is
// left alone.
func (w *tenantCheckWorkers[S]) sweepDisabled(tenants tenantSource) {
	if w.alertStore == nil || len(w.alertTypes) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.mu.Lock()
	w.sweepCancel = cancel
	w.mu.Unlock()
	w.sweep.Add(1)
	go func() {
		defer w.sweep.Done()
		defer cancel()
		all, err := tenants.ListTenants(ctx)
		if err != nil {
			logWarn(w.log, w.component+": could not list the tenants to resolve the alerts of disabled ones",
				logger.Err(err))
			return
		}
		for _, t := range all {
			if t == nil || t.Enabled || ctx.Err() != nil {
				continue
			}
			w.mu.Lock()
			if !w.running[t.TenantID] && !w.stopped {
				w.resolveAlerts(ctx, t, "disabled")
			}
			w.mu.Unlock()
		}
	}()
}

// stop stops every service; a change that arrives afterwards starts nothing.
func (w *tenantCheckWorkers[S]) stop() error {
	w.mu.Lock()
	w.stopped = true
	w.running = nil
	cancel := w.sweepCancel
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.sweep.Wait()
	return w.pool.Stop()
}

// hook follows tenant changes and starts and stops the services with the app.
func (w *tenantCheckWorkers[S]) hook(lc fx.Lifecycle, service tenant.Service) {
	w.follow(service)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error { return w.start(ctx, service) },
		OnStop:  func(context.Context) error { return w.stop() },
	})
}

func (w *tenantCheckWorkers[S]) tenantChanged(event tenant.Event, t *domain.KeycloakTenant) {
	if t == nil {
		return
	}
	// Resolved after the lock is released, and after removeLocked has
	// drained the tenant's service, so no run still going reopens an alert.
	if how := w.apply(event, t); how != "" {
		ctx, cancel := context.WithTimeout(context.Background(), resolveAlertsTimeout)
		defer cancel()
		w.resolveAlerts(ctx, t, how)
	}
}

// apply starts or stops the tenant's service for one change. It reports why
// the tenant is no longer checked ("disabled" or "removed"), or "" if it
// still is.
func (w *tenantCheckWorkers[S]) apply(event tenant.Event, t *domain.KeycloakTenant) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.loaded {
		if w.changed == nil {
			w.changed = map[string]bool{}
		}
		w.changed[t.TenantID] = true
	}

	switch event {
	case tenant.EventRemoved:
		w.removeLocked(t.TenantID)
		return "removed"
	case tenant.EventDisabled:
		w.removeLocked(t.TenantID)
		return "disabled"
	case tenant.EventAdded, tenant.EventEnabled, tenant.EventUpdated:
		if t.Enabled {
			w.putLocked(t.TenantID)
			return ""
		}
		w.removeLocked(t.TenantID)
		return "disabled"
	}
	return ""
}

// resolveAlertsTimeout bounds closing one tenant's alerts, which runs on the
// tenant change callback.
const resolveAlertsTimeout = 30 * time.Second

// resolveAlerts resolves the tenant's open alerts of the worker's check types
// and sends each one's resolution notice, with a note that this is not a
// recovery. Re-enabled, the tenant starts afresh: a condition still there
// opens a new occurrence.
func (w *tenantCheckWorkers[S]) resolveAlerts(ctx context.Context, t *domain.KeycloakTenant, how string) {
	if w.alertStore == nil || len(w.alertTypes) == 0 {
		return
	}
	note := fmt.Sprintf("Tenant %s: it is no longer monitored, so this alert was closed without a recovery.", how)
	resolved := 0
	for _, checkType := range w.alertTypes {
		open, err := w.alertStore.ListOpenAlertsByCheck(ctx, t.TenantID, checkType)
		if err != nil {
			logWarn(w.log, w.component+": could not list the alerts of a tenant no longer monitored",
				logger.Str("tenant_id", t.TenantID), logger.Str("check_type", checkType), logger.Err(err))
			continue
		}
		for _, alert := range open {
			if err := w.alertStore.ResolveAlert(ctx, t.TenantID, alert.AlertID); err != nil {
				logWarn(w.log, w.component+": could not resolve an alert of a tenant no longer monitored",
					logger.Str("tenant_id", t.TenantID), logger.Str("alert_id", alert.AlertID), logger.Err(err))
				continue
			}
			resolved++
			if w.resolutions == nil {
				continue
			}
			now := time.Now()
			alert.Status = domain.AlertStatusResolved
			alert.ResolvedAt = &now
			alert.ResolutionNote = note
			if err := w.resolutions.NotifyAlertResolution(ctx, alert); err != nil {
				logWarn(w.log, w.component+": could not send the resolution of an alert of a tenant no longer monitored",
					logger.Str("tenant_id", t.TenantID), logger.Str("alert_id", alert.AlertID), logger.Err(err))
			}
		}
	}
	if resolved > 0 {
		logInfo(w.log, w.component+": resolved the open alerts of a tenant no longer monitored",
			logger.Str("tenant_id", t.TenantID), logger.Str("reason", how), logger.Int("alerts", resolved))
	}
}

// putLocked gives the tenant a service unless it has one. w.mu must be held.
func (w *tenantCheckWorkers[S]) putLocked(tenantID string) {
	if w.stopped || w.running[tenantID] {
		return
	}
	svc, ok := w.build(tenantID)
	if !ok {
		return
	}
	if w.running == nil {
		w.running = map[string]bool{}
	}
	w.running[tenantID] = true
	if err := w.pool.Put(tenantID, svc); err != nil {
		logWarn(w.log, w.component+": service failed to start",
			logger.Str("tenant_id", tenantID), logger.Err(err))
	}
	if w.loaded {
		logInfo(w.log, w.component+" service started for tenant", logger.Str("tenant_id", tenantID))
	}
}

// removeLocked stops the tenant's service, if it has one. w.mu must be held.
func (w *tenantCheckWorkers[S]) removeLocked(tenantID string) {
	if !w.running[tenantID] {
		return
	}
	delete(w.running, tenantID)
	w.pool.Remove(tenantID)
	logInfo(w.log, w.component+" service stopped for tenant", logger.Str("tenant_id", tenantID))
}
