package fx

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// amfaTenantSync keeps the AMFA registry in step with tenants stored in the
// database.
//
// The registry is built from config.yaml at boot, which covers config-defined
// tenants but leaves a tenant created through the API with no AMFA repository
// until the next restart. This loads those tenants once at startup and then
// follows tenant changes, so configuring AMFA through the API takes effect
// immediately.
type amfaTenantSync struct {
	registry *amfa.Registry
	service  tenant.Service
	cfg      *config.AppConfig
	log      *logger.Logger

	// registeredMu guards registered: the settings each database tenant's
	// current repository was built from, so a save that leaves them as they
	// were keeps the repository, and with it the AMFA checker and mirror that
	// follow the registry.
	registeredMu sync.Mutex
	registered   map[string]config.TenantConfig
}

// startupLoadTimeout bounds the initial database read so a slow or unreachable
// database cannot hold up boot indefinitely.
const startupLoadTimeout = 15 * time.Second

// registerAmfaTenantSync loads database tenants into the registry and
// subscribes to tenant changes.
//
// The initial load runs from an OnStart hook, not directly in this Invoke:
// fx.Invoke functions run during app construction, strictly before any
// OnStart hook fires, and the tenant cache this depends on
// (tenant.Service.ListTenants reads it, never the repository) is itself only
// populated by another module's OnStart hook (registerMonitoringHooks). This
// function does not depend on running after that one — it loads the tenant
// cache itself before reading it, which is idempotent and cheap, rather than
// relying on fx.Invoke order between two unrelated modules.
func registerAmfaTenantSync(
	lc fx.Lifecycle,
	registry *amfa.Registry,
	service tenant.Service,
	cfg *config.AppConfig,
	log *logger.Logger,
) {
	if registry == nil || service == nil {
		return
	}
	s := &amfaTenantSync{registry: registry, service: service, cfg: cfg, log: log}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			loadCtx, cancel := context.WithTimeout(ctx, startupLoadTimeout)
			defer cancel()
			return s.loadExisting(loadCtx)
		},
	})

	service.RegisterCallback(s.handleTenantChange)
}

// loadExisting registers every database tenant that has AMFA configured.
//
// A failure fails startup rather than being logged and skipped. The AMFA
// checker and mirror follow the registry, so a skipped load would leave every
// database tenant without AMFA endpoints, alerts and mirrored events until each
// is saved again. The monitoring module fails startup on the same load.
func (s *amfaTenantSync) loadExisting(ctx context.Context) error {
	if err := s.service.LoadTenants(ctx); err != nil {
		return fmt.Errorf("load tenants for AMFA endpoints, alerts and mirroring: %w", err)
	}

	tenants, err := s.service.ListTenants(ctx)
	if err != nil {
		return fmt.Errorf("list tenants for AMFA endpoints, alerts and mirroring: %w", err)
	}

	loaded := 0
	for _, t := range tenants {
		if s.apply(t) {
			loaded++
		}
	}
	if loaded > 0 {
		logInfo(s.log, "AMFA registered for database tenants", logger.Int("count", loaded))
	}
	return nil
}

// handleTenantChange re-evaluates one tenant's AMFA repository.
//
// Removal and disabling unregister rather than leave the old repository in
// place: the endpoint a tenant used to point at must stop being queried the
// moment it stops pointing there.
func (s *amfaTenantSync) handleTenantChange(event tenant.Event, t *domain.KeycloakTenant) {
	if t == nil {
		return
	}
	switch event {
	case tenant.EventRemoved, tenant.EventDisabled:
		s.registeredMu.Lock()
		delete(s.registered, t.TenantID)
		s.registeredMu.Unlock()
		s.registry.Unregister(t.TenantID)
		logInfo(s.log, "AMFA unregistered for tenant", logger.Str("tenant_id", t.TenantID))
	case tenant.EventAdded, tenant.EventUpdated, tenant.EventEnabled:
		s.apply(t)
	}
}

// apply registers or unregisters one tenant based on its current settings, and
// reports whether it ended up registered. A tenant already registered with the
// same AMFA connection settings is left as it is.
func (s *amfaTenantSync) apply(t *domain.KeycloakTenant) bool {
	if t == nil {
		return false
	}
	s.registeredMu.Lock()
	defer s.registeredMu.Unlock()

	var settings config.TenantConfig
	eligible := !t.IsConfigDefined && t.Enabled && t.Amfa != nil && t.Amfa.Enabled && t.Amfa.APIBaseURL != ""
	if eligible {
		settings = amfaConnectionSettings(t)
		if prev, ok := s.registered[t.TenantID]; ok && reflect.DeepEqual(prev, settings) {
			if _, err := s.registry.RepositoryFor(t.TenantID); err == nil {
				return true
			}
		}
	}

	ok := applyAmfaTenant(s.registry, t, s.cfg, s.log)
	if s.registered == nil {
		s.registered = map[string]config.TenantConfig{}
	}
	if ok {
		s.registered[t.TenantID] = settings
	} else {
		delete(s.registered, t.TenantID)
	}
	return ok
}

// amfaConnectionSettings is what a tenant's AMFA repository is built from: the
// AMFA settings and the Keycloak client that issues its tokens. The display
// name is left out, so a rename does not restart anything.
func amfaConnectionSettings(t *domain.KeycloakTenant) config.TenantConfig {
	c := amfaTenantConfigFrom(t)
	c.Name = ""
	return c
}

// applyAmfaTenant registers or unregisters one database tenant based on its
// current settings, and reports whether it ended up registered. It is the
// single code path both assemblies (web tenant sync and MCP startup load) use
// for database-defined tenants.
//
// A config-defined tenant is skipped: it was already registered from
// config.yaml, which is the authoritative source for it, and re-registering
// from a database copy could override the file with stale values.
func applyAmfaTenant(registry *amfa.Registry, t *domain.KeycloakTenant, cfg *config.AppConfig, log *logger.Logger) bool {
	if t == nil {
		return false
	}
	if t.IsConfigDefined {
		return false
	}

	if t.Amfa == nil || !t.Amfa.Enabled || t.Amfa.APIBaseURL == "" || !t.Enabled {
		registry.Unregister(t.TenantID)
		return false
	}

	tcfg := amfaTenantConfigFrom(t)
	if err := tcfg.Amfa.Validate(); err != nil {
		logWarn(log, "AMFA config invalid for tenant; skipping",
			logger.Str("tenant_id", t.TenantID), logger.Err(err))
		registry.Unregister(t.TenantID)
		return false
	}

	repo, transport, err := newAmfaRepository(t.TenantID, tcfg, cfg, log)
	if err != nil {
		logWarn(log, "AMFA unavailable for tenant; tenant will not expose AMFA endpoints",
			logger.Str("tenant_id", t.TenantID), logger.Err(err))
		registry.Unregister(t.TenantID)
		return false
	}

	registry.Register(t.TenantID, repo)
	logInfo(log, "AMFA registered for tenant",
		logger.Str("tenant_id", t.TenantID),
		logger.Str("transport", transport),
		logger.Bool("is_config_defined", t.IsConfigDefined),
		logger.Int("events_lookback_days", tcfg.Amfa.EventsLookbackDays))
	return true
}

// amfaTenantConfigFrom converts a stored tenant into the config shape the
// repository constructor takes, so a database tenant and a config tenant are
// built through exactly the same path.
func amfaTenantConfigFrom(t *domain.KeycloakTenant) config.TenantConfig {
	var timeout time.Duration
	if t.Amfa.APITimeoutSeconds > 0 {
		timeout = time.Duration(t.Amfa.APITimeoutSeconds) * time.Second
	}
	return config.TenantConfig{
		Name:         t.Name,
		ServerURL:    t.ServerURL,
		AdminRealm:   t.AdminRealm,
		ClientID:     t.ClientID,
		ClientSecret: t.ClientSecret,
		Enabled:      t.Enabled,
		Amfa: &config.AmfaTenantConfig{
			Enabled:            t.Amfa.Enabled,
			EventsLookbackDays: t.Amfa.EventsLookbackDays,
			API: config.AmfaAPIConfig{
				BaseURL: t.Amfa.APIBaseURL,
				Timeout: timeout,
			},
		},
	}
}
