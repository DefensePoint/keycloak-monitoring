package fx

import (
	"context"
	"sync"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/lifecycle"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// tenantSource is the tenant service as the workers need it at startup.
// It holds config.yaml and interface tenants alike: config.yaml tenants are
// synced into the database at startup.
type tenantSource interface {
	LoadTenants(ctx context.Context) error
	ListEnabledTenants(ctx context.Context) ([]*domain.KeycloakTenant, error)
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

	return w.pool.Start()
}

// stop stops every service; a change that arrives afterwards starts nothing.
func (w *tenantCheckWorkers[S]) stop() error {
	w.mu.Lock()
	w.stopped = true
	w.running = nil
	w.mu.Unlock()
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
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.loaded {
		if w.changed == nil {
			w.changed = map[string]bool{}
		}
		w.changed[t.TenantID] = true
	}

	switch event {
	case tenant.EventRemoved, tenant.EventDisabled:
		w.removeLocked(t.TenantID)
	case tenant.EventAdded, tenant.EventEnabled, tenant.EventUpdated:
		if t.Enabled {
			w.putLocked(t.TenantID)
		} else {
			w.removeLocked(t.TenantID)
		}
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
