package fx

import (
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfamirror"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// AmfaMirrorModule wires the AMFA events mirror: one per-tenant
// amfamirror.Service for every tenant that has AMFA enabled, when
// keycloak.global.amfa_mirror.enabled=true. Each service owns one goroutine,
// started/stopped via fx lifecycle hooks, and follows tenant changes made
// through the API, the same shape as AmfaCheckModule.
var AmfaMirrorModule = fx.Module("amfamirror",
	fx.Provide(provideAmfaMirrorServices),
	fx.Invoke(startAmfaMirrorServices),
)

// amfamirrorLoggerAdapter adapts *logger.Logger to amfamirror.Logger.
type amfamirrorLoggerAdapter struct{ log *logger.Logger }

func (a *amfamirrorLoggerAdapter) toFields(args []any) []logger.Field {
	fields := make([]logger.Field, 0, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		fields = append(fields, logger.Any(key, args[i+1]))
	}
	return fields
}

func (a *amfamirrorLoggerAdapter) Info(msg string, args ...any) {
	a.log.Info(msg, a.toFields(args)...)
}
func (a *amfamirrorLoggerAdapter) Error(msg string, args ...any) {
	a.log.Error(msg, a.toFields(args)...)
}
func (a *amfamirrorLoggerAdapter) Warn(msg string, args ...any) {
	a.log.Warn(msg, a.toFields(args)...)
}
func (a *amfamirrorLoggerAdapter) Debug(msg string, args ...any) {
	a.log.Debug(msg, a.toFields(args)...)
}
func (a *amfamirrorLoggerAdapter) WithComponent(name string) amfamirror.Logger {
	return &amfamirrorLoggerAdapter{log: a.log.WithComponent(name)}
}

// noopAmfamirrorLogger is used when log is nil (unit tests).
type noopAmfamirrorLogger struct{}

func (noopAmfamirrorLogger) Info(string, ...any)                      {}
func (noopAmfamirrorLogger) Error(string, ...any)                     {}
func (noopAmfamirrorLogger) Warn(string, ...any)                      {}
func (noopAmfamirrorLogger) Debug(string, ...any)                     {}
func (n noopAmfamirrorLogger) WithComponent(string) amfamirror.Logger { return n }

// AmfaMirrorParams gathers the fx-provided dependencies.
type AmfaMirrorParams struct {
	fx.In

	Config     *config.AppConfig
	Registry   *amfa.Registry
	Events     events.Repository
	Enrichment *amfa.Enrichment
	Keycloak   keycloak.Service
	Logger     *logger.Logger
}

func provideAmfaMirrorServices(p AmfaMirrorParams) *amfaTenantWorkers[amfamirror.Service] {
	return newAmfaMirrorWorkers(p.Config, p.Registry, p.Events, p.Enrichment, p.Keycloak, p.Logger)
}

// newAmfaMirrorWorkers is the unit-testable core of provideAmfaMirrorServices.
// It builds the config.yaml tenants' mirrors and, when the mirror is globally
// enabled, follows the registry for database tenants. log may be nil in tests.
func newAmfaMirrorWorkers(
	cfg *config.AppConfig,
	registry *amfa.Registry,
	eventStore amfamirror.EventStore,
	enricher amfamirror.Enricher,
	realms realmLister,
	log *logger.Logger,
) *amfaTenantWorkers[amfamirror.Service] {
	workers := &amfaTenantWorkers[amfamirror.Service]{
		component:     "amfamirror",
		configTenants: cfg.Keycloak.Tenants,
		log:           log,
	}
	for tenantID, svc := range buildAmfaMirrorServices(cfg, registry, eventStore, enricher, realms, log) {
		workers.put(tenantID, svc)
	}
	if cfg.Keycloak.Global.AmfaMirror.Enabled {
		workers.build = amfaMirrorBuilder(cfg, eventStore, enricher, realms, log)
		workers.follow(registry)
	}
	return workers
}

// buildAmfaMirrorServices builds the config.yaml tenants' mirrors. log may be
// nil in tests.
func buildAmfaMirrorServices(
	cfg *config.AppConfig,
	registry *amfa.Registry,
	eventStore amfamirror.EventStore,
	enricher amfamirror.Enricher,
	realms realmLister,
	log *logger.Logger,
) map[string]amfamirror.Service {
	if !cfg.Keycloak.Global.AmfaMirror.Enabled {
		logInfo(log, "AMFA mirror globally disabled; no mirror services constructed")
		return nil
	}
	build := amfaMirrorBuilder(cfg, eventStore, enricher, realms, log)

	services := make(map[string]amfamirror.Service)
	for tenantID, tcfg := range cfg.Keycloak.Tenants {
		if tcfg.Amfa == nil || !tcfg.Amfa.Enabled {
			continue
		}

		repo, err := registry.RepositoryFor(tenantID)
		if err != nil {
			logWarn(log, "amfamirror: skipping tenant with no AMFA repository",
				logger.Str("tenant_id", tenantID), logger.Err(err))
			continue
		}
		services[tenantID] = build(tenantID, repo)
	}
	logInfo(log, "AMFA mirror services constructed", logger.Int("count", len(services)))
	return services
}

// amfaMirrorBuilder returns the function that builds one tenant's mirror from
// its AMFA repository.
func amfaMirrorBuilder(
	cfg *config.AppConfig,
	eventStore amfamirror.EventStore,
	enricher amfamirror.Enricher,
	realms realmLister,
	log *logger.Logger,
) func(tenantID string, repo amfa.Repository) amfamirror.Service {
	mirrorCfg := cfg.Keycloak.Global.AmfaMirror
	mirrorCfg.ApplyDefaults()

	adaptedLog := amfamirror.Logger(noopAmfamirrorLogger{})
	if log != nil {
		adaptedLog = &amfamirrorLoggerAdapter{log: log}
	}
	backfill := time.Duration(mirrorCfg.BackfillDays) * 24 * time.Hour

	return func(tenantID string, repo amfa.Repository) amfamirror.Service {
		logInfo(log, "amfamirror service constructed",
			logger.Str("tenant_id", tenantID),
			logger.Str("poll_interval", mirrorCfg.PollInterval.String()),
			logger.Int("backfill_days", mirrorCfg.BackfillDays))
		return amfamirror.NewService(
			tenantID, tenantRealmsFunc(realms, tenantID), repo, eventStore, enricher,
			adaptedLog, mirrorCfg.PollInterval, backfill)
	}
}

// startAmfaMirrorServices starts the mirrors with the app and stops them with
// it, on their own long-lived contexts, as startAmfaCheckServices does.
func startAmfaMirrorServices(lc fx.Lifecycle, workers *amfaTenantWorkers[amfamirror.Service], log *logger.Logger) {
	workers.hook(lc)
	logInfo(log, "amfamirror lifecycle hooks registered", logger.Int("service_count", workers.pool.Len()))
}
