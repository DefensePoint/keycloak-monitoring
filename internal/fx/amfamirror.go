package fx

import (
	"context"
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
// amfamirror.Service for every tenant that has amfa.enabled=true AND
// keycloak.global.amfa_mirror.enabled=true. Each service owns one goroutine,
// started/stopped via fx lifecycle hooks — same shape as AmfaCheckModule.
var AmfaMirrorModule = fx.Module("amfamirror",
	fx.Provide(provideAmfaMirrorServices),
	fx.Invoke(startAmfaMirrorServices),
)

// amfaMirrorServices is a named holder so fx can distinguish the map from
// other providers.
type amfaMirrorServices struct {
	services map[string]amfamirror.Service
}

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

func provideAmfaMirrorServices(p AmfaMirrorParams) *amfaMirrorServices {
	return &amfaMirrorServices{
		services: buildAmfaMirrorServices(p.Config, p.Registry, p.Events, p.Enrichment, p.Keycloak, p.Logger),
	}
}

// buildAmfaMirrorServices is the unit-testable core. log may be nil in tests.
func buildAmfaMirrorServices(
	cfg *config.AppConfig,
	registry *amfa.Registry,
	eventStore amfamirror.EventStore,
	enricher amfamirror.Enricher,
	realms realmLister,
	log *logger.Logger,
) map[string]amfamirror.Service {
	mirrorCfg := cfg.Keycloak.Global.AmfaMirror
	if !mirrorCfg.Enabled {
		logInfo(log, "AMFA mirror globally disabled; no mirror services constructed")
		return nil
	}
	mirrorCfg.ApplyDefaults()

	adaptedLog := amfamirror.Logger(noopAmfamirrorLogger{})
	if log != nil {
		adaptedLog = &amfamirrorLoggerAdapter{log: log}
	}

	services := make(map[string]amfamirror.Service)
	for tenantID, tcfg := range cfg.Keycloak.Tenants {
		if tcfg.Amfa == nil || !tcfg.Amfa.Enabled {
			continue
		}
		tenantID := tenantID // capture

		repo, err := registry.RepositoryFor(tenantID)
		if err != nil {
			logWarn(log, "amfamirror: skipping tenant with no AMFA repository",
				logger.Str("tenant_id", tenantID), logger.Err(err))
			continue
		}

		realmsFn := func(ctx context.Context) ([]string, error) {
			rows, lerr := realms.ListRealms(ctx, tenantID)
			if lerr != nil {
				return nil, lerr
			}
			out := make([]string, 0, len(rows))
			for _, r := range rows {
				out = append(out, r.RealmName)
			}
			return out, nil
		}

		backfill := time.Duration(mirrorCfg.BackfillDays) * 24 * time.Hour
		services[tenantID] = amfamirror.NewService(
			tenantID, realmsFn, repo, eventStore, enricher,
			adaptedLog, mirrorCfg.PollInterval, backfill)
		logInfo(log, "amfamirror service constructed",
			logger.Str("tenant_id", tenantID),
			logger.Str("poll_interval", mirrorCfg.PollInterval.String()),
			logger.Int("backfill_days", mirrorCfg.BackfillDays))
	}
	logInfo(log, "AMFA mirror services constructed", logger.Int("count", len(services)))
	return services
}

func startAmfaMirrorServices(lc fx.Lifecycle, holder *amfaMirrorServices, log *logger.Logger) {
	for _, s := range holder.services {
		s := s
		// Long-lived context, not fx's OnStart context (which is cancelled once
		// startup completes) — same rationale as startAmfaCheckServices.
		var cancel context.CancelFunc
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				var runCtx context.Context
				runCtx, cancel = context.WithCancel(context.Background())
				return s.Start(runCtx)
			},
			OnStop: func(context.Context) error {
				if cancel != nil {
					cancel()
				}
				return s.Stop()
			},
		})
	}
	logInfo(log, "amfamirror lifecycle hooks registered", logger.Int("service_count", len(holder.services)))
}
