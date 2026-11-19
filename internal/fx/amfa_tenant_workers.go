package fx

import (
	"context"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfacheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/lifecycle"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// amfaTenantWorkers runs one AMFA background service per tenant (the checker
// or the mirror) and keeps them in step with the AMFA registry.
//
// config.yaml tenants get their service once at startup, as before. Database
// tenants follow the registry, which amfaTenantSync updates on every tenant
// change: a tenant that gets an AMFA repository gets a service, a new
// repository (a save that changes its AMFA connection) replaces it, and a tenant
// that loses its repository (deleted, disabled, AMFA switched off) has it
// stopped. So the tenant's AMFA toggle takes effect without a restart.
type amfaTenantWorkers[S lifecycle.Poller] struct {
	// component names the service in log lines: "amfacheck" or "amfamirror".
	component string
	// build makes a tenant's service from its repository.
	build func(tenantID string, repo amfa.Repository) S
	// configTenants are left to the services built at startup: config.yaml is
	// authoritative for them, and amfaTenantSync never registers them.
	configTenants map[string]config.TenantConfig
	// onPut and onRemove, when set, keep a lookup of the running services in
	// step (the checker's Runner, for on-demand runs).
	onPut    func(tenantID string, svc S)
	onRemove func(tenantID string)
	log      *logger.Logger

	pool lifecycle.Pool[S]

	// closer, when set (the checker's workers), resolves the open AMFA alerts
	// of a tenant that no longer has its AMFA checked.
	closer *amfaAlertCloser

	// mu guards running and makes a tenant's resolution and a change to its
	// service take turns.
	mu      sync.Mutex
	running map[string]bool
	// sweep is the startup pass; stop cancels and waits for it.
	sweepCancel context.CancelFunc
	sweep       sync.WaitGroup
}

// amfaAlertCloser resolves a tenant's open alerts of the AMFA checks.
type amfaAlertCloser struct {
	types    []string
	store    amfacheck.AlertStore
	notifier amfacheck.NotificationService
	// tenants lists every tenant for the startup pass.
	tenants interface {
		ListTenants(ctx context.Context) ([]*domain.KeycloakTenant, error)
	}
}

// put makes svc the tenant's service, stopping the one it replaces.
func (w *amfaTenantWorkers[S]) put(tenantID string, svc S) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running == nil {
		w.running = map[string]bool{}
	}
	w.running[tenantID] = true
	if err := w.pool.Put(tenantID, svc); err != nil {
		logWarn(w.log, w.component+": service failed to start",
			logger.Str("tenant_id", tenantID), logger.Err(err))
	}
	if w.onPut != nil {
		w.onPut(tenantID, svc)
	}
}

// remove stops the tenant's service, if it has one, then resolves the
// tenant's open AMFA alerts (the checker's workers only). The service is
// stopped first, so no run still going reopens an alert just resolved.
func (w *amfaTenantWorkers[S]) remove(tenantID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.running, tenantID)
	if w.onRemove != nil {
		w.onRemove(tenantID)
	}
	w.pool.Remove(tenantID)
	if w.closer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), amfaResolveTimeout)
		defer cancel()
		w.closer.resolve(ctx, tenantID, w.log)
	}
}

// follow subscribes to registry changes. It must be called before the
// database tenants are loaded into the registry, which happens on start.
func (w *amfaTenantWorkers[S]) follow(registry *amfa.Registry) {
	registry.Subscribe(w.registryChanged)
}

func (w *amfaTenantWorkers[S]) registryChanged(tenantID string, repo amfa.Repository) {
	if _, ok := w.configTenants[tenantID]; ok {
		return
	}
	if repo == nil {
		w.remove(tenantID)
		logInfo(w.log, w.component+" service stopped for tenant", logger.Str("tenant_id", tenantID))
		return
	}
	w.put(tenantID, w.build(tenantID, repo))
}

// hook starts the services with the app and stops them with it.
func (w *amfaTenantWorkers[S]) hook(lc fx.Lifecycle) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error { return w.start() },
		OnStop:  func(context.Context) error { return w.stop() },
	})
}

// start starts the services, then, for the checker's workers, resolves in
// the background the open AMFA alerts of every tenant that has no checker:
// AMFA off or the tenant disabled, possibly since before this version.
func (w *amfaTenantWorkers[S]) start() error {
	if err := w.pool.Start(); err != nil {
		return err
	}
	if w.closer == nil || w.closer.tenants == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.mu.Lock()
	w.sweepCancel = cancel
	w.mu.Unlock()
	w.sweep.Add(1)
	go func() {
		defer w.sweep.Done()
		defer cancel()
		all, err := w.closer.tenants.ListTenants(ctx)
		if err != nil {
			logWarn(w.log, w.component+": could not list the tenants to resolve the alerts of those without AMFA",
				logger.Err(err))
			return
		}
		for _, t := range all {
			if t == nil || ctx.Err() != nil {
				continue
			}
			w.mu.Lock()
			if !w.running[t.TenantID] {
				w.closer.resolve(ctx, t.TenantID, w.log)
			}
			w.mu.Unlock()
		}
	}()
	return nil
}

// stop cancels the startup pass, waits for it, and stops the services.
func (w *amfaTenantWorkers[S]) stop() error {
	w.mu.Lock()
	cancel := w.sweepCancel
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.sweep.Wait()
	return w.pool.Stop()
}

// amfaResolveTimeout bounds resolving one tenant's AMFA alerts.
const amfaResolveTimeout = 30 * time.Second

// resolve resolves the tenant's open alerts of the AMFA checks and sends each
// one's resolution notice, with a note that this is not a recovery. If AMFA
// is checked again later, a condition still there opens a new occurrence.
func (c *amfaAlertCloser) resolve(ctx context.Context, tenantID string, log *logger.Logger) {
	const note = "AMFA is no longer checked for this tenant, so this alert was closed without a recovery."
	resolved := 0
	for _, checkType := range c.types {
		open, err := c.store.ListOpenAlertsByCheck(ctx, tenantID, checkType)
		if err != nil {
			logWarn(log, "amfacheck: could not list the AMFA alerts of a tenant no longer checked",
				logger.Str("tenant_id", tenantID), logger.Str("check_type", checkType), logger.Err(err))
			continue
		}
		for _, alert := range open {
			if err := c.store.ResolveAlert(ctx, tenantID, alert.AlertID); err != nil {
				logWarn(log, "amfacheck: could not resolve an AMFA alert of a tenant no longer checked",
					logger.Str("tenant_id", tenantID), logger.Str("alert_id", alert.AlertID), logger.Err(err))
				continue
			}
			resolved++
			if c.notifier == nil {
				continue
			}
			now := time.Now()
			alert.Status = domain.AlertStatusResolved
			alert.ResolvedAt = &now
			alert.ResolutionNote = note
			if err := c.notifier.NotifyAlertResolution(ctx, alert); err != nil {
				logWarn(log, "amfacheck: could not send the resolution of an AMFA alert",
					logger.Str("tenant_id", tenantID), logger.Str("alert_id", alert.AlertID), logger.Err(err))
			}
		}
	}
	if resolved > 0 {
		logInfo(log, "amfacheck: resolved the open AMFA alerts of a tenant no longer checked",
			logger.Str("tenant_id", tenantID), logger.Int("alerts", resolved))
	}
}

// tenantRealmsFunc lists the names of the realms KMT knows for the tenant.
func tenantRealmsFunc(realms realmLister, tenantID string) func(ctx context.Context) ([]string, error) {
	return func(ctx context.Context) ([]string, error) {
		rows, err := realms.ListRealms(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.RealmName)
		}
		return out, nil
	}
}
