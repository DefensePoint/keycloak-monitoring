package fx

import (
	"context"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
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
}

// put makes svc the tenant's service, stopping the one it replaces.
func (w *amfaTenantWorkers[S]) put(tenantID string, svc S) {
	if err := w.pool.Put(tenantID, svc); err != nil {
		logWarn(w.log, w.component+": service failed to start",
			logger.Str("tenant_id", tenantID), logger.Err(err))
	}
	if w.onPut != nil {
		w.onPut(tenantID, svc)
	}
}

// remove stops the tenant's service, if it has one.
func (w *amfaTenantWorkers[S]) remove(tenantID string) {
	if w.onRemove != nil {
		w.onRemove(tenantID)
	}
	w.pool.Remove(tenantID)
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
		OnStart: func(context.Context) error { return w.pool.Start() },
		OnStop:  func(context.Context) error { return w.pool.Stop() },
	})
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
