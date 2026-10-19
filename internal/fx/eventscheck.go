package fx

import (
	"context"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/eventscheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
)

// EventsCheckModule wires the generic Keycloak-event alert checker: one
// per-tenant eventscheck.Service for every enabled tenant when
// keycloak.global.event_checker.enabled is true and at least one check is
// enabled. Each service owns one goroutine, started/stopped via fx lifecycle
// hooks. Safe to install when the master switch is off or no tenants qualify:
// the slice is simply empty.
var EventsCheckModule = fx.Module("eventscheck",
	fx.Provide(provideEventsCheckRunner),
	fx.Invoke(startEventsCheckServices),
)

type eventsCheckAlertStore struct{ repo alerts.Repository }

func (a eventsCheckAlertStore) GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	return a.repo.GetByAlertID(ctx, tenantID, alertID)
}

func (a eventsCheckAlertStore) SaveAlert(ctx context.Context, alert *domain.Alert) error {
	return a.repo.Save(ctx, alert)
}

func (a eventsCheckAlertStore) ListOpenAlertsByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error) {
	return a.repo.ListOpenByCheck(ctx, tenantID, checkType)
}

func (a eventsCheckAlertStore) ResolveAlert(ctx context.Context, tenantID, alertID string) error {
	return a.repo.Resolve(ctx, tenantID, alertID)
}

type eventscheckLoggerAdapter struct{ log *logger.Logger }

func (a *eventscheckLoggerAdapter) toFields(args []any) []logger.Field {
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

func (a *eventscheckLoggerAdapter) Info(msg string, args ...any) {
	a.log.Info(msg, a.toFields(args)...)
}
func (a *eventscheckLoggerAdapter) Error(msg string, args ...any) {
	a.log.Error(msg, a.toFields(args)...)
}
func (a *eventscheckLoggerAdapter) Warn(msg string, args ...any) {
	a.log.Warn(msg, a.toFields(args)...)
}
func (a *eventscheckLoggerAdapter) Debug(msg string, args ...any) {
	a.log.Debug(msg, a.toFields(args)...)
}
func (a *eventscheckLoggerAdapter) WithComponent(name string) eventscheck.Logger {
	return &eventscheckLoggerAdapter{log: a.log.WithComponent(name)}
}

type noopEventscheckLogger struct{}

func (noopEventscheckLogger) Info(string, ...any)                       {}
func (noopEventscheckLogger) Error(string, ...any)                      {}
func (noopEventscheckLogger) Warn(string, ...any)                       {}
func (noopEventscheckLogger) Debug(string, ...any)                      {}
func (n noopEventscheckLogger) WithComponent(string) eventscheck.Logger { return n }

type EventsCheckParams struct {
	fx.In

	Config   *config.AppConfig
	Alerts   alerts.Repository
	Notifier notifications.Service
	Keycloak keycloak.Service
	Silences alerts.SilenceManager
	Logger   *logger.Logger
}

type eventsCheckRunner struct {
	services []eventscheck.Service
}

func (r *eventsCheckRunner) Services() []eventscheck.Service { return r.services }

func provideEventsCheckRunner(p EventsCheckParams) *eventsCheckRunner {
	services := buildEventsCheckServices(p.Config, p.Keycloak, p.Keycloak,
		eventsCheckAlertStore{repo: p.Alerts}, p.Notifier, p.Silences, p.Logger)
	return &eventsCheckRunner{services: services}
}

func buildEventsCheckServices(
	cfg *config.AppConfig,
	events eventscheck.KeycloakEventReader,
	realms realmLister,
	alertStore eventscheck.AlertStore,
	notifier eventscheck.NotificationService,
	silencer eventscheck.Silencer,
	log *logger.Logger,
) []eventscheck.Service {
	checkerCfg := cfg.Keycloak.Global.EventChecker
	if !checkerCfg.Enabled {
		logInfo(log, "eventscheck globally disabled; no alert services constructed")
		return nil
	}
	checkerCfg.ApplyDefaults()

	adaptedLog := eventscheck.Logger(noopEventscheckLogger{})
	if log != nil {
		adaptedLog = &eventscheckLoggerAdapter{log: log}
	}

	var services []eventscheck.Service
	for tenantID, tcfg := range cfg.Keycloak.Tenants {
		if !tcfg.Enabled {
			continue
		}
		tenantID := tenantID

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

		checks := buildEventsChecks(tenantID, eventscheck.RealmsFunc(realmsFn), events, checkerCfg, adaptedLog)
		if len(checks) == 0 {
			continue
		}
		svc := eventscheck.NewService(checks, alertStore, notifier, adaptedLog, checkerCfg.PollInterval,
			eventscheck.WithResolveAfter(checkerCfg.ResolveAfter), eventscheck.WithSilencer(silencer))
		services = append(services, svc)
		logInfo(log, "eventscheck service constructed",
			logger.Str("tenant_id", tenantID),
			logger.Int("checks", len(checks)),
			logger.Str("poll_interval", checkerCfg.PollInterval.String()))
	}
	logInfo(log, "eventscheck services constructed", logger.Int("count", len(services)))
	return services
}

func buildEventsChecks(
	tenantID string,
	realmsFn eventscheck.RealmsFunc,
	events eventscheck.KeycloakEventReader,
	checkerCfg config.EventCheckerConfig,
	log eventscheck.Logger,
) []eventscheck.Check {
	c := checkerCfg.Checks
	var out []eventscheck.Check
	if c.LoginError.Enabled {
		out = append(out, eventscheck.NewLoginErrorCheck(events, log, tenantID, realmsFn, c.LoginError.Threshold, c.LoginError.TimeWindow))
	}
	if c.ClientError.Enabled {
		out = append(out, eventscheck.NewClientErrorCheck(events, log, tenantID, realmsFn, c.ClientError.Threshold, c.ClientError.TimeWindow))
	}
	if c.CodeToToken.Enabled {
		out = append(out, eventscheck.NewCodeToTokenErrorCheck(events, log, tenantID, realmsFn, c.CodeToToken.Threshold, c.CodeToToken.TimeWindow))
	}
	if c.IdpError.Enabled {
		out = append(out, eventscheck.NewIdpErrorCheck(events, log, tenantID, realmsFn, c.IdpError.Threshold, c.IdpError.TimeWindow, c.IdpError.EventTypes))
	}
	if c.CommonErrors.Enabled {
		out = append(out, eventscheck.NewCommonErrorCheck(events, log, tenantID, realmsFn, c.CommonErrors.Threshold, c.CommonErrors.TimeWindow, c.CommonErrors.EventTypes))
	}
	return out
}

func startEventsCheckServices(lc fx.Lifecycle, runner *eventsCheckRunner, log *logger.Logger) {
	for _, s := range runner.Services() {
		s := s
		// The poll loop must run on a long-lived context, not fx's OnStart
		// context: fx cancels the OnStart context once startup completes, which
		// would immediately stop the ticker goroutine.
		var cancel context.CancelFunc
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				var runCtx context.Context
				runCtx, cancel = context.WithCancel(context.Background())
				logInfo(log, "eventscheck service starting")
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
	logInfo(log, "eventscheck lifecycle hooks registered", logger.Int("service_count", len(runner.Services())))
}
