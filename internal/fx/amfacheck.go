package fx

import (
	"context"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/alerts"
	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/amfacheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/notifications"
)

// AmfaCheckModule wires the AMFA risk-alert checker: one per-tenant
// amfacheck.Service for every tenant that has amfa.enabled=true AND
// keycloak.global.amfa_checker.enabled=true. Each service owns one goroutine,
// started/stopped via fx lifecycle hooks. Safe to install when no tenants
// qualify: the slice is simply empty.
var AmfaCheckModule = fx.Module("amfacheck",
	fx.Provide(provideAmfaCheckRunner),
	fx.Invoke(startAmfaCheckServices),
)

// realmLister is the slice of keycloak.Service that the per-tenant realmsFn
// needs. keycloak.Service satisfies it.
type realmLister interface {
	ListRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error)
}

// amfaCheckAlertStore adapts the real alerts.Repository method names
// (GetByAlertID / Save) to the amfacheck.AlertStore contract
// (GetAlertByID / SaveAlert).
type amfaCheckAlertStore struct{ repo alerts.Repository }

func (a amfaCheckAlertStore) GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	return a.repo.GetByAlertID(ctx, tenantID, alertID)
}

func (a amfaCheckAlertStore) SaveAlert(ctx context.Context, alert *domain.Alert) error {
	return a.repo.Save(ctx, alert)
}

// amfacheckLoggerAdapter adapts *logger.Logger to amfacheck.Logger, converting
// the variadic key/value args into logger.Field values so structured fields
// (tenant_id, alert_id, ...) survive.
type amfacheckLoggerAdapter struct{ log *logger.Logger }

func (a *amfacheckLoggerAdapter) toFields(args []any) []logger.Field {
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

func (a *amfacheckLoggerAdapter) Info(msg string, args ...any) { a.log.Info(msg, a.toFields(args)...) }
func (a *amfacheckLoggerAdapter) Error(msg string, args ...any) {
	a.log.Error(msg, a.toFields(args)...)
}
func (a *amfacheckLoggerAdapter) Warn(msg string, args ...any) { a.log.Warn(msg, a.toFields(args)...) }
func (a *amfacheckLoggerAdapter) Debug(msg string, args ...any) {
	a.log.Debug(msg, a.toFields(args)...)
}
func (a *amfacheckLoggerAdapter) WithComponent(name string) amfacheck.Logger {
	return &amfacheckLoggerAdapter{log: a.log.WithComponent(name)}
}

// noopAmfacheckLogger is used when log is nil (unit tests).
type noopAmfacheckLogger struct{}

func (noopAmfacheckLogger) Info(string, ...any)                     {}
func (noopAmfacheckLogger) Error(string, ...any)                    {}
func (noopAmfacheckLogger) Warn(string, ...any)                     {}
func (noopAmfacheckLogger) Debug(string, ...any)                    {}
func (n noopAmfacheckLogger) WithComponent(string) amfacheck.Logger { return n }

// AmfaCheckParams gathers the fx-provided dependencies. All of these types are
// already provided elsewhere in the graph (alerts.Repository, keycloak.Service,
// notifications.Service, *amfa.Registry, *config.AppConfig, *logger.Logger).
type AmfaCheckParams struct {
	fx.In

	Config   *config.AppConfig
	Registry *amfa.Registry
	Alerts   alerts.Repository
	Notifier notifications.Service
	Keycloak keycloak.Service
	Logger   *logger.Logger
}

// provideAmfaCheckRunner builds the per-tenant services once and wraps them in a
// Runner. The Runner is injected into the AMFA HTTP handlers for on-demand runs;
// the lifecycle starter (below) starts/stops the same Service instances.
func provideAmfaCheckRunner(p AmfaCheckParams) *amfacheck.Runner {
	services := buildAmfaCheckServices(p.Config, p.Registry, p.Keycloak,
		amfaCheckAlertStore{repo: p.Alerts}, p.Notifier, p.Logger)
	return amfacheck.NewRunner(services)
}

// buildAmfaCheckServices is the unit-testable core. log may be nil in tests.
// It returns one Service per AMFA-enabled tenant, keyed by tenant ID.
func buildAmfaCheckServices(
	cfg *config.AppConfig,
	registry *amfa.Registry,
	realms realmLister,
	alertStore amfacheck.AlertStore,
	notifier amfacheck.NotificationService,
	log *logger.Logger,
) map[string]amfacheck.Service {
	amfaCfg := cfg.Keycloak.Global.AmfaChecker
	if !amfaCfg.Enabled {
		logInfo(log, "AMFA checker globally disabled; no alert services constructed")
		return nil
	}
	amfaCfg.ApplyDefaults()

	adaptedLog := amfacheck.Logger(noopAmfacheckLogger{})
	if log != nil {
		adaptedLog = &amfacheckLoggerAdapter{log: log}
	}

	services := make(map[string]amfacheck.Service)
	for tenantID, tcfg := range cfg.Keycloak.Tenants {
		if tcfg.Amfa == nil || !tcfg.Amfa.Enabled {
			continue
		}
		tenantID := tenantID // capture

		repo, err := registry.RepositoryFor(tenantID)
		if err != nil {
			logWarn(log, "amfacheck: skipping tenant with no AMFA repository",
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

		checks := buildAmfaChecks(tenantID, amfacheck.RealmsFunc(realmsFn), repo, amfaCfg, adaptedLog)
		services[tenantID] = amfacheck.NewService(tenantID, checks, alertStore, notifier, adaptedLog, amfaCfg.PollInterval)
		logInfo(log, "amfacheck service constructed",
			logger.Str("tenant_id", tenantID),
			logger.Int("checks", len(checks)),
			logger.Str("poll_interval", amfaCfg.PollInterval.String()))
	}
	logInfo(log, "AMFA checker services constructed", logger.Int("count", len(services)))
	return services
}

// buildAmfaChecks builds the enabled checks for one tenant.
func buildAmfaChecks(
	tenantID string,
	realmsFn amfacheck.RealmsFunc,
	repo amfa.Repository,
	amfaCfg config.AmfaCheckerConfig,
	log amfacheck.Logger,
) []amfacheck.Check {
	c := amfaCfg.Checks
	var out []amfacheck.Check
	if c.RiskRejected.Enabled {
		out = append(out, amfacheck.NewRiskRejectedCheck(tenantID, realmsFn, repo, amfaCfg.PollInterval, log))
	}
	if c.RepeatedRisky.Enabled {
		out = append(out, amfacheck.NewRepeatedRiskyCheck(tenantID, realmsFn, repo,
			c.RepeatedRisky.Threshold, c.RepeatedRisky.Window, c.RepeatedRisky.MinRiskLevel, log))
	}
	if c.VPNRisky.Enabled {
		out = append(out, amfacheck.NewVPNRiskyCheck(tenantID, realmsFn, repo, amfaCfg.PollInterval, log))
	}
	if c.LoginError.Enabled {
		out = append(out, amfacheck.NewLoginErrorCheck(tenantID, realmsFn, repo,
			c.LoginError.Threshold, c.LoginError.Window, log))
	}
	if c.ClientLoginError.Enabled {
		out = append(out, amfacheck.NewClientLoginErrorCheck(tenantID, realmsFn, repo,
			c.ClientLoginError.Threshold, c.ClientLoginError.Window, log))
	}
	if c.LoginErrorByClient.Enabled {
		out = append(out, amfacheck.NewLoginErrorByClientCheck(tenantID, realmsFn, repo,
			c.LoginErrorByClient.Threshold, c.LoginErrorByClient.Window, log))
	}
	if c.RealmRejectBurst.Enabled {
		out = append(out, amfacheck.NewRealmRejectBurstCheck(tenantID, realmsFn, repo,
			c.RealmRejectBurst.Threshold, c.RealmRejectBurst.Window, log))
	}
	if c.LoginErrorByAccount.Enabled {
		out = append(out, amfacheck.NewLoginErrorByAccountCheck(tenantID, realmsFn, repo,
			c.LoginErrorByAccount.Threshold, c.LoginErrorByAccount.Window, log))
	}
	return out
}

func startAmfaCheckServices(lc fx.Lifecycle, runner *amfacheck.Runner, log *logger.Logger) {
	for _, s := range runner.Services() {
		s := s
		// The poll loop must run on a long-lived context, NOT fx's OnStart
		// context: fx cancels the OnStart context once startup completes, which
		// would immediately stop the ticker goroutine. We own a background
		// context here and cancel it in OnStop (alongside Stop()'s graceful
		// drain) so in-flight queries unwind on shutdown.
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
	logInfo(log, "amfacheck lifecycle hooks registered", logger.Int("service_count", len(runner.Services())))
}
