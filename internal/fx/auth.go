package fx

import (
	"context"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
)

// AuthModule provides authentication domain dependencies.
var AuthModule = fx.Module("auth",
	fx.Provide(
		provideCookieSessionManager,
		provideStateStore,
		provideSessionBlacklist,
		provideSimpleAuthService,
		provideOAuth2AdminService,
		provideAuthConfig,
		provideOIDCProvider,
		provideMemorySessionStore,
		provideAuthService,
	),
)

// provideCookieSessionManager creates the session manager for authentication.
func provideCookieSessionManager(cfg *config.AppConfig) *auth.CookieSessionManager {
	sessionMaxAge := int(cfg.Auth.Session.MaxAge / time.Second)
	return auth.NewCookieSessionManager(
		cfg.Auth.Session.Secret,
		cfg.Auth.Session.Name,
		sessionMaxAge,
		cfg.Auth.Session.Secure,
		cfg.Auth.Session.SameSite,
	)
}

// provideStateStore creates the OAuth2 state store for CSRF protection.
func provideStateStore() *auth.StateStore {
	// State tokens are valid for 10 minutes
	return auth.NewStateStore(10 * time.Minute)
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

// provideOIDCProvider creates the OIDC provider if OAuth2 is enabled.
// It initializes synchronously during DI container setup.
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

	// Use a background context with timeout for OIDC discovery
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider, err := auth.NewOIDCProvider(ctx, authCfg, log)
	if err != nil {
		log.Error("Failed to initialize OIDC provider - SSO will not work",
			logger.Err(err),
			logger.Str("provider_url", authCfg.ProviderURL))
		// Return nil to allow startup without SSO (if simple auth is enabled)
		return nil
	}

	log.Info("OAuth2/OIDC provider initialized successfully")
	return provider
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
