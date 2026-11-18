package fx

import (
	"context"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfacheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// AmfaCheckModule wires the AMFA risk-alert checker: one per-tenant
// amfacheck.Service for every tenant that has AMFA enabled, when
// keycloak.global.amfa_checker.enabled=true. Each service owns one goroutine,
// started/stopped via fx lifecycle hooks, and follows tenant changes made
// through the API (see amfaTenantWorkers). Safe to install when no tenants
// qualify: there are simply no services.
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

func (a amfaCheckAlertStore) ListOpenAlertsByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error) {
	return a.repo.ListOpenByCheck(ctx, tenantID, checkType)
}

func (a amfaCheckAlertStore) AlertExists(ctx context.Context, tenantID, alertID string) (bool, error) {
	return a.repo.ExistsIncludingDeleted(ctx, tenantID, alertID)
}

func (a amfaCheckAlertStore) LatestFirstDetected(ctx context.Context, tenantID, checkType, realm string) (time.Time, bool, error) {
	return a.repo.LatestFirstDetected(ctx, tenantID, checkType, realm)
}

func (a amfaCheckAlertStore) ResolveAlert(ctx context.Context, tenantID, alertID string) error {
	return a.repo.Resolve(ctx, tenantID, alertID)
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
	Silences alerts.SilenceManager
	Tenants  tenant.Service
	Logger   *logger.Logger
}

// provideAmfaCheckRunner builds the per-tenant services and the Runner that
// holds them. The Runner is injected into the AMFA HTTP handlers for on-demand
// runs; the lifecycle starter (below) starts/stops the same Service instances.
func provideAmfaCheckRunner(p AmfaCheckParams) (*amfacheck.Runner, *amfaTenantWorkers[amfacheck.Service]) {
	return newAmfaCheckWorkers(p.Config, p.Registry, p.Keycloak,
		amfaCheckAlertStore{repo: p.Alerts}, p.Notifier, p.Silences, p.Tenants, p.Logger)
}

// newAmfaCheckWorkers is the unit-testable core of provideAmfaCheckRunner. It
// builds the config.yaml tenants' services and, when the checker is globally
// enabled, follows the registry for database tenants. log may be nil in tests.
func newAmfaCheckWorkers(
	cfg *config.AppConfig,
	registry *amfa.Registry,
	realms realmLister,
	alertStore amfacheck.AlertStore,
	notifier amfacheck.NotificationService,
	silencer amfacheck.Silencer,
	tenants interface {
		ListTenants(ctx context.Context) ([]*domain.KeycloakTenant, error)
	},
	log *logger.Logger,
) (*amfacheck.Runner, *amfaTenantWorkers[amfacheck.Service]) {
	runner := amfacheck.NewRunner(nil)
	workers := &amfaTenantWorkers[amfacheck.Service]{
		component:     "amfacheck",
		configTenants: cfg.Keycloak.Tenants,
		onPut:         runner.Set,
		onRemove:      runner.Remove,
		log:           log,
	}
	for tenantID, svc := range buildAmfaCheckServices(cfg, registry, realms, alertStore, notifier, silencer, log) {
		workers.put(tenantID, svc)
	}
	if cfg.Keycloak.Global.AmfaChecker.Enabled {
		workers.build = amfaCheckBuilder(cfg, realms, alertStore, notifier, silencer, log)
		workers.closer = &amfaAlertCloser{types: amfaCheckTypes(cfg.Keycloak.Global.AmfaChecker),
			store: alertStore, notifier: notifier, tenants: tenants}
		workers.follow(registry)
	}
	return runner, workers
}

// amfaCheckTypes returns the check types of the AMFA checks the configuration
// enables, the alerts a tenant's checker can raise.
func amfaCheckTypes(amfaCfg config.AmfaCheckerConfig) []string {
	amfaCfg.ApplyDefaults()
	checks := buildAmfaChecks("", nil, nil, amfaCfg, nil, noopAmfacheckLogger{})
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		out = append(out, c.GetCheckType())
	}
	return out
}

// buildAmfaCheckServices builds the config.yaml tenants' services. log may be
// nil in tests. It returns one Service per AMFA-enabled tenant, keyed by
// tenant ID.
func buildAmfaCheckServices(
	cfg *config.AppConfig,
	registry *amfa.Registry,
	realms realmLister,
	alertStore amfacheck.AlertStore,
	notifier amfacheck.NotificationService,
	silencer amfacheck.Silencer,
	log *logger.Logger,
) map[string]amfacheck.Service {
	if !cfg.Keycloak.Global.AmfaChecker.Enabled {
		logInfo(log, "AMFA checker globally disabled; no alert services constructed")
		return nil
	}
	build := amfaCheckBuilder(cfg, realms, alertStore, notifier, silencer, log)

	services := make(map[string]amfacheck.Service)
	for tenantID, tcfg := range cfg.Keycloak.Tenants {
		if tcfg.Amfa == nil || !tcfg.Amfa.Enabled {
			continue
		}

		repo, err := registry.RepositoryFor(tenantID)
		if err != nil {
			logWarn(log, "amfacheck: skipping tenant with no AMFA repository",
				logger.Str("tenant_id", tenantID), logger.Err(err))
			continue
		}
		services[tenantID] = build(tenantID, repo)
	}
	logInfo(log, "AMFA checker services constructed", logger.Int("count", len(services)))
	return services
}

// amfaCheckBuilder returns the function that builds one tenant's checker from
// its AMFA repository.
//
// Every checker it builds for a tenant shares the tenant's run lock. A saved
// tenant gets a new checker, which runs at once, while a manual run ("Reload
// AMFA alerts") may still be going on the old one; with a lock each, both
// could see an alert as new and both announce it.
func amfaCheckBuilder(
	cfg *config.AppConfig,
	realms realmLister,
	alertStore amfacheck.AlertStore,
	notifier amfacheck.NotificationService,
	silencer amfacheck.Silencer,
	log *logger.Logger,
) func(tenantID string, repo amfa.Repository) amfacheck.Service {
	amfaCfg := cfg.Keycloak.Global.AmfaChecker
	amfaCfg.ApplyDefaults()

	adaptedLog := amfacheck.Logger(noopAmfacheckLogger{})
	if log != nil {
		adaptedLog = &amfacheckLoggerAdapter{log: log}
	}

	var locksMu sync.Mutex
	runLocks := map[string]*sync.Mutex{}
	runLockFor := func(tenantID string) *sync.Mutex {
		locksMu.Lock()
		defer locksMu.Unlock()
		if runLocks[tenantID] == nil {
			runLocks[tenantID] = &sync.Mutex{}
		}
		return runLocks[tenantID]
	}

	return func(tenantID string, repo amfa.Repository) amfacheck.Service {
		var resume amfacheck.ResumeFunc
		if latest, ok := alertStore.(latestAlertFinder); ok {
			resume = func(ctx context.Context, checkType, realm string) (time.Time, bool, error) {
				return latest.LatestFirstDetected(ctx, tenantID, checkType, realm)
			}
		}
		realmsFn := amfacheck.RealmsFunc(tenantRealmsFunc(realms, tenantID))
		checks := buildAmfaChecks(tenantID, realmsFn, repo, amfaCfg, resume, adaptedLog)
		logInfo(log, "amfacheck service constructed",
			logger.Str("tenant_id", tenantID),
			logger.Int("checks", len(checks)),
			logger.Str("poll_interval", amfaCfg.PollInterval.String()))
		return amfacheck.NewService(tenantID, checks, alertStore, notifier, adaptedLog, amfaCfg.PollInterval,
			amfacheck.WithResolveAfter(amfaCfg.ResolveAfter), amfacheck.WithSilencer(silencer),
			amfacheck.WithRunLock(runLockFor(tenantID)))
	}
}

// latestAlertFinder lets a restarted per-event check resume from the newest
// alert it raised.
type latestAlertFinder interface {
	LatestFirstDetected(ctx context.Context, tenantID, checkType, realm string) (time.Time, bool, error)
}

// buildAmfaChecks builds the enabled checks for one tenant.
func buildAmfaChecks(
	tenantID string,
	realmsFn amfacheck.RealmsFunc,
	repo amfa.Repository,
	amfaCfg config.AmfaCheckerConfig,
	resume amfacheck.ResumeFunc,
	log amfacheck.Logger,
) []amfacheck.Check {
	c := amfaCfg.Checks
	var out []amfacheck.Check
	if c.RiskRejected.Enabled {
		out = append(out, amfacheck.NewRiskRejectedCheck(tenantID, realmsFn, repo, amfaCfg.PollInterval, log,
			amfacheck.WithResume(resume)))
	}
	if c.RepeatedRisky.Enabled {
		out = append(out, amfacheck.NewRepeatedRiskyCheck(tenantID, realmsFn, repo,
			c.RepeatedRisky.Threshold, c.RepeatedRisky.Window, c.RepeatedRisky.MinRiskLevel,
			repeatedRiskyActiveWithin(amfaCfg), log))
	}
	if c.VPNRisky.Enabled {
		out = append(out, amfacheck.NewVPNRiskyCheck(tenantID, realmsFn, repo, amfaCfg.PollInterval, log,
			amfacheck.WithResume(resume)))
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

// startAmfaCheckServices starts the checkers with the app and stops them with
// it. Each poll loop runs on its own long-lived context, not fx's OnStart
// context, which fx cancels once startup completes (see lifecycle.Pool).
func startAmfaCheckServices(lc fx.Lifecycle, workers *amfaTenantWorkers[amfacheck.Service], log *logger.Logger) {
	workers.hook(lc)
	logInfo(log, "amfacheck lifecycle hooks registered", logger.Int("service_count", workers.pool.Len()))
}

// repeatedRiskyActiveWithin is how recent a user's last risky attempt must be
// for rule 2 to count them as still active: the quiet period alerts resolve
// after, and never less than two polls, so every attempt is seen as recent by
// at least one poll.
func repeatedRiskyActiveWithin(cfg config.AmfaCheckerConfig) time.Duration {
	return max(cfg.ResolveAfter, 2*cfg.PollInterval)
}
