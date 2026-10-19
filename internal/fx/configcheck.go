package fx

import (
	"context"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/configcheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
	"github.com/DefensePoint/keycloak-monitoring/pkg/saferequest"
)

// ConfigCheckModule runs the Keycloak configuration checks (SAML identity
// provider metadata and certificates, realm security, client security) for
// every enabled tenant when keycloak.global.config_checker.enabled is true.
// It must be installed after MonitoringModule: its start hook reads the
// tenants and Keycloak clients that the monitoring start hook loads.
var ConfigCheckModule = fx.Module("configcheck",
	fx.Invoke(registerConfigCheckHooks),
)

type configCheckAlertStore struct{ repo alerts.Repository }

func (a configCheckAlertStore) GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	return a.repo.GetByAlertID(ctx, tenantID, alertID)
}

func (a configCheckAlertStore) SaveAlert(ctx context.Context, alert *domain.Alert) error {
	return a.repo.Save(ctx, alert)
}

func (a configCheckAlertStore) ResolveAlert(ctx context.Context, tenantID, alertID string) error {
	return a.repo.Resolve(ctx, tenantID, alertID)
}

func (a configCheckAlertStore) ListOpenAlertsByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error) {
	return a.repo.ListOpenByCheck(ctx, tenantID, checkType)
}

// lateKeycloakClient looks the tenant's Keycloak client up on every call: the
// monitor pool creates it after startup and replaces it on reconnect.
type lateKeycloakClient struct {
	lookup   func(tenantID string) configcheck.KeycloakClient
	tenantID string
}

func (c lateKeycloakClient) client() (configcheck.KeycloakClient, error) {
	if kc := c.lookup(c.tenantID); kc != nil {
		return kc, nil
	}
	return nil, keycloak.ErrClientNotAvailable
}

func (c lateKeycloakClient) GetAllRealms(ctx context.Context) ([]*keycloakadmin.RealmRepresentation, error) {
	kc, err := c.client()
	if err != nil {
		return nil, err
	}
	return kc.GetAllRealms(ctx)
}

func (c lateKeycloakClient) GetRealmInfo(ctx context.Context, realmName string) (*keycloakadmin.RealmRepresentation, error) {
	kc, err := c.client()
	if err != nil {
		return nil, err
	}
	return kc.GetRealmInfo(ctx, realmName)
}

func (c lateKeycloakClient) GetClients(ctx context.Context, realmName string) ([]*keycloakadmin.ClientRepresentation, error) {
	kc, err := c.client()
	if err != nil {
		return nil, err
	}
	return kc.GetClients(ctx, realmName)
}

func (c lateKeycloakClient) GetIdentityProviders(ctx context.Context, realmName string) ([]*keycloakadmin.IdentityProviderRepresentation, error) {
	kc, err := c.client()
	if err != nil {
		return nil, err
	}
	return kc.GetIdentityProviders(ctx, realmName)
}

func poolKeycloakClient(pool *tenant.MonitorPoolManager) func(tenantID string) configcheck.KeycloakClient {
	return func(tenantID string) configcheck.KeycloakClient {
		if adapter, ok := pool.GetKeycloakClient(tenantID).(*keycloakClientAdapter); ok && adapter.client != nil {
			return adapter.client
		}
		return nil
	}
}

type ConfigCheckParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	Config    *config.AppConfig
	Alerts    alerts.Repository
	Notifier  configcheck.NotificationService
	Silences  alerts.SilenceManager
	Tenants   tenant.Service
	Pool      *tenant.MonitorPoolManager
	Logger    *logger.Logger
}

func registerConfigCheckHooks(p ConfigCheckParams) {
	checkerCfg := p.Config.Keycloak.Global.ConfigChecker
	if !checkerCfg.Enabled {
		logInfo(p.Logger, "Configuration checker globally disabled")
		return
	}

	var svc configcheck.Service
	var cancel context.CancelFunc
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			tenants, err := p.Tenants.ListEnabledTenants(ctx)
			if err != nil {
				return err
			}
			checks := buildConfigChecks(checkerCfg, tenants, poolKeycloakClient(p.Pool), keycloakSafeRequestPolicy(p.Config), p.Logger)
			if len(checks) == 0 {
				logInfo(p.Logger, "Configuration checker enabled but no checks apply")
				return nil
			}
			svc = configcheck.NewService(checks, configCheckAlertStore{repo: p.Alerts}, p.Notifier, p.Logger, checkerCfg.PollInterval,
				configcheck.WithSilencer(p.Silences))
			// fx cancels the OnStart context once startup completes, which
			// would stop the poll loop at once.
			var runCtx context.Context
			runCtx, cancel = context.WithCancel(context.Background())
			return svc.Start(runCtx)
		},
		OnStop: func(context.Context) error {
			if svc == nil {
				return nil
			}
			cancel()
			return svc.Stop()
		},
	})
}

func buildConfigChecks(
	checkerCfg config.ConfigCheckerConfig,
	tenants []*domain.KeycloakTenant,
	lookup func(tenantID string) configcheck.KeycloakClient,
	policy saferequest.Policy,
	log *logger.Logger,
) []configcheck.Check {
	c := checkerCfg.Checks
	var checks []configcheck.Check
	for _, t := range tenants {
		kc := lateKeycloakClient{lookup: lookup, tenantID: t.TenantID}
		if c.IdentityProviderMetadata.Enabled {
			checks = append(checks, configcheck.NewIdentityProviderMetadataCheck(kc, log, nil, t.TenantID,
				c.IdentityProviderMetadata.CertificateExpirationWarningDays,
				c.IdentityProviderMetadata.CertificateCheckTimeout, policy))
		}
		if c.RealmSecurity.Enabled {
			checks = append(checks, configcheck.NewRealmSecurityCheck(kc, log, nil, t.TenantID, configcheck.RealmSecurityCheckConfig{
				CheckSSLRequired:              c.RealmSecurity.CheckSSLRequired,
				CheckBruteForce:               c.RealmSecurity.CheckBruteForce,
				CheckPasswordPolicy:           c.RealmSecurity.CheckPasswordPolicy,
				CheckEmailVerification:        c.RealmSecurity.CheckEmailVerification,
				CheckAdminEvents:              c.RealmSecurity.CheckAdminEvents,
				CheckUserEvents:               c.RealmSecurity.CheckUserEvents,
				CheckDuplicateEmails:          c.RealmSecurity.CheckDuplicateEmails,
				MinPasswordLength:             c.RealmSecurity.MinPasswordLength,
				RequirePasswordComplexity:     c.RealmSecurity.RequirePasswordComplexity,
				MaxAccessTokenLifespanMinutes: c.RealmSecurity.MaxAccessTokenLifespanMinutes,
				MaxSSOSessionIdleMinutes:      c.RealmSecurity.MaxSSOSessionIdleMinutes,
			}))
		}
		if c.ClientSecurity.Enabled {
			checks = append(checks, configcheck.NewClientSecurityCheck(kc, log, nil, t.TenantID, configcheck.ClientSecurityCheckConfig{
				CheckRedirectURIs:       c.ClientSecurity.CheckRedirectURIs,
				CheckPublicClients:      c.ClientSecurity.CheckPublicClients,
				CheckDirectAccessGrants: c.ClientSecurity.CheckDirectAccessGrants,
				CheckClientSecret:       c.ClientSecurity.CheckClientSecret,
				CheckImplicitFlow:       c.ClientSecurity.CheckImplicitFlow,
				CheckStandardFlow:       c.ClientSecurity.CheckStandardFlow,
				AllowLocalhostRedirects: c.ClientSecurity.AllowLocalhostRedirects,
				ProductionEnvironment:   c.ClientSecurity.ProductionEnvironment,
			}))
		}
	}
	logInfo(log, "Configuration checks constructed",
		logger.Int("tenants", len(tenants)),
		logger.Int("checks", len(checks)))
	return checks
}
