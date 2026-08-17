package fx

import (
	"sync"
	"time"

	"go.uber.org/fx"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
	amfapg "repos.defensepoint.com/iam/oss-pmt/kmt/amfa/postgres"
	"repos.defensepoint.com/iam/oss-pmt/kmt/amfacheck"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
	chihttp "repos.defensepoint.com/iam/oss-pmt/kmt/internal/http/chi"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/keycloakadmin"
	"repos.defensepoint.com/iam/oss-pmt/kmt/rbac"
	"repos.defensepoint.com/iam/oss-pmt/kmt/tenant"
)

// AMFA enrichment cache TTLs. Successful Keycloak user lookups are cached
// for successTTL; 404s use the shorter notFoundTTL to prevent retry storms
// when an event references a deleted user. Kept as package consts so the
// numbers are visible in one place.
const (
	amfaEnrichmentSuccessTTL  = 5 * time.Minute
	amfaEnrichmentNotFoundTTL = 30 * time.Second
)

// AmfaModule wires the AMFA Events feature: per-tenant read-only Postgres
// repositories, the Keycloak enrichment cache (backed by the monitor pool's
// per-tenant admin clients), the service, and the HTTP handlers. The module
// is safe to install in deployments that have no AMFA-enabled tenants: the
// registry is simply empty. When the monitor pool is unavailable, enrichment
// degrades to raw-UUID display via the nil-Enrichment path in amfa.Service.
var AmfaModule = fx.Module("amfa",
	fx.Provide(
		provideAmfaRegistry,
		provideAmfaEnrichment,
		provideAmfaService,
		provideAmfaHandlers,
	),
	fx.Invoke(logAmfaStartup),
)

// provideAmfaRegistry builds the per-tenant Repository registry by walking
// the configured Keycloak tenants. Tenants without amfa.enabled=true are
// skipped silently. Tenants with invalid AMFA configuration or unreachable
// databases are logged and skipped — they never abort startup.
func provideAmfaRegistry(cfg *config.AppConfig, log *logger.Logger) *amfa.Registry {
	return buildAmfaRegistry(cfg.Keycloak.Tenants, log)
}

// buildAmfaRegistry is the unit-testable core of provideAmfaRegistry. It is
// intentionally non-fatal on per-tenant errors: AMFA is an optional add-on
// and a single misconfigured tenant must not bring down the platform.
func buildAmfaRegistry(tenants map[string]config.TenantConfig, log *logger.Logger) *amfa.Registry {
	reg := amfa.NewRegistry()
	for tenantID, tcfg := range tenants {
		if tcfg.Amfa == nil || !tcfg.Amfa.Enabled {
			continue
		}
		// Validate also fills in defaults (port, sslmode, pool sizes), so it
		// must run before NewClient.
		amfaCfg := tcfg.Amfa
		if err := amfaCfg.Validate(); err != nil {
			logWarn(log, "AMFA config invalid for tenant; skipping",
				logger.Str("tenant_id", tenantID), logger.Err(err))
			continue
		}
		db, err := amfapg.NewClient(amfaCfg.Database, log)
		if err != nil {
			logWarn(log, "AMFA database connection failed; tenant will not expose AMFA endpoints",
				logger.Str("tenant_id", tenantID), logger.Err(err))
			continue
		}
		// Schema drift is non-fatal; CheckAmfaSchema logs internally.
		if err := amfapg.CheckAmfaSchema(db, amfaCfg.ExpectedSchemaVersion, log); err != nil {
			logWarn(log, "AMFA schema check returned error",
				logger.Str("tenant_id", tenantID), logger.Err(err))
		}
		reg.Register(tenantID, amfapg.NewRepositoryWithLookback(db, amfaCfg.EventsLookbackDays))
		logInfo(log, "AMFA registered for tenant",
			logger.Str("tenant_id", tenantID),
			logger.Int("events_lookback_days", amfaCfg.EventsLookbackDays))
	}
	return reg
}

// provideAmfaEnrichment wires the AMFA Keycloak enrichment cache against
// the per-tenant *keycloakadmin.Client instances owned by the monitor pool.
// Resolution happens at call time so newly added tenants are picked up
// automatically; removed tenants degrade gracefully to raw-UUID display.
//
// When the pool is unavailable (nil) we return nil so amfa.Service falls
// back to its enrichment-disabled path. Constructing an Enrichment with a
// nil lookup would crash on the first BatchEnrich call, so nil is the
// correct degradation choice.
func provideAmfaEnrichment(pool *tenant.MonitorPoolManager, log *logger.Logger) *amfa.Enrichment {
	if pool == nil {
		logInfo(log, "AMFA Keycloak enrichment disabled: monitor pool not available; events will display raw user UUIDs",
			logger.Dur("success_ttl_when_enabled", amfaEnrichmentSuccessTTL),
			logger.Dur("not_found_ttl_when_enabled", amfaEnrichmentNotFoundTTL))
		return nil
	}
	lookup := &monitorPoolKeycloakAdminLookup{pool: pool, log: log}
	adapter := amfa.NewPoolKeycloakAdapter(lookup)
	logInfo(log, "AMFA Keycloak enrichment enabled via monitor pool",
		logger.Dur("success_ttl", amfaEnrichmentSuccessTTL),
		logger.Dur("not_found_ttl", amfaEnrichmentNotFoundTTL))
	return amfa.NewEnrichment(adapter, amfaEnrichmentSuccessTTL, amfaEnrichmentNotFoundTTL)
}

// monitorPoolKeycloakAdminLookup adapts *tenant.MonitorPoolManager to
// amfa.KeycloakClientLookup. The pool stores its admin clients behind the
// tenant.KeycloakClient interface (a deliberately narrow contract); the
// real *keycloakadmin.Client is held inside a private *keycloakClientAdapter
// defined in this same fx package. We replicate the same type-assert /
// unwrap dance that monitorPoolClientProvider uses for the keycloak.Service
// integration, so AMFA enrichment shares the exact same lifecycle as live
// Keycloak access.
type monitorPoolKeycloakAdminLookup struct {
	pool *tenant.MonitorPoolManager
	log  *logger.Logger
	// warned tracks tenants for which we've already logged the
	// type-assertion-miss warning, so a misconfigured pool doesn't spam the
	// logs on every BatchEnrich call (which can be 10s of times per second
	// under page-refresh load).
	warned sync.Map
}

// Compile-time check: *keycloakadmin.Client satisfies amfa.KeycloakAdmin
// (the package's exported single-method projection over pkg/keycloakadmin).
var _ amfa.KeycloakAdmin = (*keycloakadmin.Client)(nil)

// GetKeycloakAdmin resolves the admin client for tenantID by routing through
// the same monitor-pool plumbing the rest of KMT uses. Returns nil when
// the tenant has no active monitor; the caller (amfa.poolKeycloakAdapter)
// translates that into a transient error so Enrichment does not negative-
// cache the gap.
func (l *monitorPoolKeycloakAdminLookup) GetKeycloakAdmin(tenantID string) amfa.KeycloakAdmin {
	if l.pool == nil {
		return nil
	}
	client := l.pool.GetKeycloakClient(tenantID)
	if client == nil {
		// Common, expected case (monitor not started yet, or no admin client
		// for this tenant). Don't log — too noisy on the per-request path.
		return nil
	}
	adapter, ok := client.(*keycloakClientAdapter)
	if !ok || adapter == nil || adapter.client == nil {
		// Unexpected: the pool returned something that isn't our concrete
		// *keycloakClientAdapter. This means someone introduced a new wrapper
		// (circuit breaker, instrumentation, etc.) and forgot to update this
		// type assertion — AMFA enrichment would silently degrade to UUID-only
		// display. Log once per tenant so the regression is visible.
		if _, alreadyWarned := l.warned.LoadOrStore(tenantID, struct{}{}); !alreadyWarned {
			logWarn(l.log, "AMFA enrichment: unexpected KeycloakClient implementation in monitor pool; falling back to UUID display for this tenant. Update monitorPoolKeycloakAdminLookup's type assertion if a new wrapper was added.",
				logger.Str("tenant_id", tenantID))
		}
		return nil
	}
	return adapter.client
}

// provideAmfaService composes the registry and (optional) enrichment into
// the public Service interface consumed by handlers.
func provideAmfaService(reg *amfa.Registry, enr *amfa.Enrichment) amfa.Service {
	return amfa.NewService(reg, enr)
}

// AmfaHandlersParams gathers dependencies for the AMFA HTTP handlers,
// following the same fx.In pattern used by every other handler in
// handlers.go.
type AmfaHandlersParams struct {
	fx.In

	Service        amfa.Service
	TenantService  tenant.Service
	RBACService    rbac.Service
	Logger         *logger.Logger
	AuthMiddleware *chihttp.AuthMiddleware
	RBACMiddleware *chihttp.RBACMiddleware
	CheckerRunner  *amfacheck.Runner
}

// provideAmfaHandlers constructs *chihttp.AmfaHandlers, wrapping rbac.Service
// in the same rbacCheckerAdapter used by every other handler module.
func provideAmfaHandlers(p AmfaHandlersParams) *chihttp.AmfaHandlers {
	return chihttp.NewAmfaHandlers(
		p.Service,
		p.TenantService,
		&rbacCheckerAdapter{p.RBACService},
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
		p.CheckerRunner,
	)
}

// logAmfaStartup emits a single boot-time log line showing how many tenants
// ended up registered. Operators rely on this to confirm AMFA wiring picked
// up the tenants they expect.
func logAmfaStartup(reg *amfa.Registry, log *logger.Logger) {
	ids := reg.TenantIDs()
	logInfo(log, "AMFA module ready",
		logger.Int("enabled_tenant_count", len(ids)),
		logger.Strs("tenant_ids", ids))
}

// logInfo / logWarn tolerate a nil logger so unit tests can exercise
// buildAmfaRegistry without constructing a real logger. Production code
// always supplies one.
func logInfo(log *logger.Logger, msg string, fields ...logger.Field) {
	if log == nil {
		return
	}
	log.Info(msg, fields...)
}

func logWarn(log *logger.Logger, msg string, fields ...logger.Field) {
	if log == nil {
		return
	}
	log.Warn(msg, fields...)
}
