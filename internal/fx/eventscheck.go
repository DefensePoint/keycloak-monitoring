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
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// EventsCheckModule wires the generic Keycloak-event alert checker: one
// per-tenant eventscheck.Service for every enabled tenant, config.yaml and
// interface tenants alike, when keycloak.global.event_checker.enabled is true
// and at least one check is enabled. The services follow tenant changes: see
// tenantCheckWorkers.
var EventsCheckModule = fx.Module("eventscheck",
	fx.Invoke(registerEventsCheckHooks),
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

	Lifecycle fx.Lifecycle
	Config    *config.AppConfig
	Alerts    alerts.Repository
	Notifier  notifications.Service
	Keycloak  keycloak.Service
	Silences  alerts.SilenceManager
	Tenants   tenant.Service
	Logger    *logger.Logger
}

func registerEventsCheckHooks(p EventsCheckParams) {
	checkerCfg := p.Config.Keycloak.Global.EventChecker
	if !checkerCfg.Enabled {
		logInfo(p.Logger, "eventscheck globally disabled; no alert services constructed")
		return
	}

	newEventsCheckWorkers(checkerCfg, p.Keycloak, p.Keycloak, eventsCheckAlertStore{repo: p.Alerts},
		p.Notifier, p.Silences, p.Logger).hook(p.Lifecycle, p.Tenants)
}

// newEventsCheckWorkers runs one event checker service per enabled tenant,
// with the checks the configuration enables.
func newEventsCheckWorkers(
	checkerCfg config.EventCheckerConfig,
	events eventscheck.KeycloakEventReader,
	realms realmLister,
	alertStore eventscheck.AlertStore,
	notifier eventscheck.NotificationService,
	silencer eventscheck.Silencer,
	log *logger.Logger,
) *tenantCheckWorkers[eventscheck.Service] {
	checkerCfg.ApplyDefaults()

	adaptedLog := eventscheck.Logger(noopEventscheckLogger{})
	if log != nil {
		adaptedLog = &eventscheckLoggerAdapter{log: log}
	}

	return &tenantCheckWorkers[eventscheck.Service]{
		component: "eventscheck",
		// The event checks start before the monitoring module loads the
		// tenant cache, and before the HTTP server serves.
		loadTenants: true,
		log:         log,
		build: func(tenantID string) (eventscheck.Service, bool) {
			realmsFn := eventscheck.RealmsFunc(tenantRealmsFunc(realms, tenantID))
			checks := buildEventsChecks(tenantID, realmsFn, events, checkerCfg, adaptedLog)
			if len(checks) == 0 {
				return nil, false
			}
			return eventscheck.NewService(checks, alertStore, notifier, adaptedLog, checkerCfg.PollInterval,
				eventscheck.WithResolveAfter(checkerCfg.ResolveAfter), eventscheck.WithSilencer(silencer)), true
		},
	}
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
