package fx

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
)

// AuthModule provides authentication domain dependencies.
var AuthModule = fx.Module("auth",
	fx.Provide(
		provideCookieSessionManager,
		provideLoginCookie,
		provideSessionBlacklist,
		provideSimpleAuthService,
		provideOAuth2AdminService,
		provideAuthConfig,
		provideOIDCProvider,
		provideMemorySessionStore,
		provideAuthService,
	),
	fx.Invoke(registerOIDCProviderRetry),
)

// provideCookieSessionManager creates the session manager for authentication.
// It refuses an empty or published session secret, which stops the server at
// startup rather than serving forgeable sessions.
func provideCookieSessionManager(cfg *config.AppConfig) (*auth.CookieSessionManager, error) {
	if err := auth.ValidateSessionSecret(cfg.Auth.Session.Secret); err != nil {
		// The variable binds only to a key config.yaml already carries, so a
		// config without the key ignores it. Say so rather than calling a
		// secret the operator did set "empty".
		if strings.TrimSpace(cfg.Auth.Session.Secret) == "" && os.Getenv("MONITORING_AUTH_SESSION_SECRET") != "" {
			return nil, fmt.Errorf("%w (MONITORING_AUTH_SESSION_SECRET is set but has no effect: "+
				"config.yaml has no auth.session.secret key for it to fill; add `secret: \"\"` "+
				"under auth.session)", err)
		}
		return nil, err
	}
	sessionMaxAge := int(cfg.Auth.Session.MaxAge / time.Second)
	return auth.NewCookieSessionManager(
		cfg.Auth.Session.Secret,
		cfg.Auth.Session.Name,
		sessionMaxAge,
		cfg.Auth.Session.Secure,
		cfg.Auth.Session.SameSite,
	), nil
}

// defaultLoginMaxAge is how long an SSO login may take when
// auth.session.state_max_age is not positive.
const defaultLoginMaxAge = 10 * time.Minute

// provideLoginCookie creates the cookie that carries an SSO login in progress
// between its start and its callback, for auth.session.state_max_age.
func provideLoginCookie(cfg *config.AppConfig) (*auth.LoginCookie, error) {
	maxAge := cfg.Auth.Session.StateMaxAge
	if maxAge <= 0 {
		maxAge = defaultLoginMaxAge
	}
	return auth.NewLoginCookie(
		cfg.Auth.Session.Secret,
		cfg.Auth.Session.Name,
		maxAge,
		cfg.Auth.Session.Secure,
	)
}

// provideSessionBlacklist creates the session blacklist for logout revocation.
// This addresses the security finding where tokens could be reused after logout.
func provideSessionBlacklist(cfg *config.AppConfig) *auth.SessionBlacklist {
	// Use the same maxAge as the session to know when to clean up revoked sessions
	return auth.NewSessionBlacklist(cfg.Auth.Session.MaxAge)
}

// provideSimpleAuthService creates the simple auth service if enabled.
func provideSimpleAuthService(
	cfg *config.AppConfig,
	userRepo users.Repository,
	sessionManager *auth.CookieSessionManager,
	log *logger.Logger,
) auth.SimpleAuthService {
	if !cfg.Auth.Simple.Enabled {
		return nil
	}
	// users.Repository implements auth.UserRepository
	return auth.NewSimpleAuthService(userRepo, sessionManager, log)
}

// ProvideSimpleAuthAdapter creates an adapter for the HTTP handlers.
// This is no longer needed since SimpleAuthService now returns auth.UserInfo directly.
func ProvideSimpleAuthAdapter(service auth.SimpleAuthService) chihttp.SimpleAuthService {
	if service == nil {
		return nil
	}
	return service
}

// ProvideAuthService creates the auth service with dependencies.
func ProvideAuthService(
	provider auth.Provider,
	sessionStore auth.SessionStore,
	userRepository auth.UserRepository,
	log *logger.Logger,
	config *auth.Config,
) auth.Service {
	return auth.NewService(provider, sessionStore, userRepository, log, config)
}

// OAuth2AdminServiceParams contains dependencies for OAuth2AdminService.
type OAuth2AdminServiceParams struct {
	fx.In

	RBACService rbac.Service `optional:"true"`
	Config      *config.AppConfig
	Logger      *logger.Logger
}

// provideOAuth2AdminService creates the OAuth2 admin service if OAuth2 is enabled.
func provideOAuth2AdminService(p OAuth2AdminServiceParams) *auth.OAuth2AdminService {
	if !p.Config.Auth.OAuth2.Enabled {
		return nil
	}

	if p.RBACService == nil {
		return nil
	}

	adminEmails := p.Config.Auth.OAuth2.AdminUsers
	if len(adminEmails) == 0 {
		return nil
	}

	return auth.NewOAuth2AdminService(
		&oauth2AdminRBACAdapter{p.RBACService},
		adminEmails,
		p.Logger,
	)
}

// oauth2AdminRBACAdapter adapts rbac.Service to auth.OAuth2AdminRBACService.
type oauth2AdminRBACAdapter struct {
	service rbac.Service
}

func (a *oauth2AdminRBACAdapter) IsAdmin(ctx context.Context, userID uint) (bool, error) {
	return a.service.IsAdmin(ctx, userID)
}

func (a *oauth2AdminRBACAdapter) GetRoleByName(ctx context.Context, name string) (*auth.Role, error) {
	role, err := a.service.GetRoleByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return &auth.Role{
		ID:   role.ID,
		Name: role.Name,
	}, nil
}

func (a *oauth2AdminRBACAdapter) AssignRoleToUser(ctx context.Context, userID, roleID uint, tenantID *string, assignedBy string, expiresAt *time.Time) error {
	return a.service.AssignRoleToUser(ctx, userID, roleID, tenantID, assignedBy, expiresAt)
}

// provideAuthConfig creates the auth.Config from application configuration.
func provideAuthConfig(cfg *config.AppConfig) *auth.Config {
	if !cfg.Auth.OAuth2.Enabled {
		return nil
	}

	scopes := cfg.Auth.OAuth2.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}

	return &auth.Config{
		ProviderURL:     cfg.Auth.OAuth2.ProviderURL,
		ClientID:        cfg.Auth.OAuth2.ClientID,
		ClientSecret:    cfg.Auth.OAuth2.ClientSecret,
		RedirectURL:     cfg.Auth.OAuth2.RedirectURL,
		Scopes:          scopes,
		SessionSecret:   cfg.Auth.Session.Secret,
		SessionMaxAge:   cfg.Auth.Session.MaxAge,
		SessionSecure:   cfg.Auth.Session.Secure,
		SessionSameSite: cfg.Auth.Session.SameSite,
		SkipIssuerCheck: cfg.Auth.OAuth2.SkipIssuerCheck,
		SkipExpiryCheck: cfg.Auth.OAuth2.SkipExpiryCheck,
	}
}

// provideOIDCProvider creates the OIDC provider if OAuth2 is enabled. It tries
// discovery once during DI setup; registerOIDCProviderRetry and the next login
// retry it after a failure.
func provideOIDCProvider(cfg *config.AppConfig, authCfg *auth.Config, log *logger.Logger) auth.Provider {
	if !cfg.Auth.OAuth2.Enabled || authCfg == nil {
		log.Info("OAuth2/SSO authentication is disabled")
		return nil
	}

	// Validate required configuration
	if authCfg.ProviderURL == "" {
		log.Warn("OAuth2 enabled but provider_url is not configured")
		return nil
	}
	if authCfg.ClientID == "" {
		log.Warn("OAuth2 enabled but client_id is not configured")
		return nil
	}
	if authCfg.ClientSecret == "" {
		log.Warn("OAuth2 enabled but client_secret is not configured")
		return nil
	}
	if authCfg.RedirectURL == "" {
		log.Warn("OAuth2 enabled but redirect_url is not configured")
		return nil
	}

	log.Info("Initializing OAuth2/OIDC provider",
		logger.Str("provider_url", authCfg.ProviderURL),
		logger.Str("client_id", authCfg.ClientID),
		logger.Str("redirect_url", authCfg.RedirectURL))

	provider, err := auth.NewRetryingOIDCProvider(authCfg, log)
	if err != nil {
		log.Error("Invalid OAuth2/OIDC configuration - SSO will not work",
			logger.Err(err),
			logger.Str("provider_url", authCfg.ProviderURL))
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := provider.Ready(ctx); err != nil {
		log.Error("Failed to initialize OIDC provider - retrying in the background and on the next SSO login",
			logger.Err(err),
			logger.Str("provider_url", authCfg.ProviderURL))
	}
	return provider
}

// OIDCProviderRetryParams contains dependencies for the background retry.
type OIDCProviderRetryParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	Config    *config.AppConfig
	Provider  auth.Provider     `optional:"true"`
	Metrics   *metrics.Registry `optional:"true"`
}

// registerOIDCProviderRetry publishes sso_provider_up and keeps retrying
// discovery in the background while the identity provider is unreachable.
// With SSO enabled but no provider built, the configuration is incomplete or
// invalid, every login fails, and the gauge reads 0; it is absent only when
// SSO is disabled.
func registerOIDCProviderRetry(p OIDCProviderRetryParams) {
	provider, ok := p.Provider.(*auth.RetryingOIDCProvider)
	if !ok {
		if p.Metrics != nil && p.Config != nil && p.Config.Auth.OAuth2.Enabled {
			p.Metrics.SetSSOProviderUp(false)
		}
		return
	}
	if p.Metrics != nil {
		provider.SetMetrics(p.Metrics)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				provider.Run(ctx)
			}()
			return nil
		},
		OnStop: func(stopCtx context.Context) error {
			cancel()
			select {
			case <-done:
			case <-stopCtx.Done():
			}
			return nil
		},
	})
}

// provideMemorySessionStore creates an in-memory session store for OAuth2 sessions.
func provideMemorySessionStore(cfg *config.AppConfig) auth.SessionStore {
	if !cfg.Auth.OAuth2.Enabled {
		return nil
	}
	return auth.NewMemorySessionStore(cfg.Auth.Session.MaxAge)
}

// AuthServiceParams contains dependencies for auth.Service.
type AuthServiceParams struct {
	fx.In

	Provider       auth.Provider     `optional:"true"`
	SessionStore   auth.SessionStore `optional:"true"`
	UserRepository users.Repository
	Logger         *logger.Logger
	Config         *auth.Config `optional:"true"`
}

// provideAuthService creates the auth service for OAuth2 authentication.
func provideAuthService(p AuthServiceParams) auth.Service {
	if p.Provider == nil || p.Config == nil {
		return nil
	}

	return auth.NewService(
		p.Provider,
		p.SessionStore,
		p.UserRepository,
		p.Logger,
		p.Config,
	)
}
