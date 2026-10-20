package fx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// ServerModule provides the HTTP server
var ServerModule = fx.Module("server",
	fx.Provide(
		provideAuthMiddleware,
		provideHTTPServer,
	),
	fx.Invoke(registerServerHooks),
)

// AuthMiddlewareParams contains dependencies for auth middleware.
type AuthMiddlewareParams struct {
	fx.In

	SessionManager   *auth.CookieSessionManager
	AuthService      auth.Service `optional:"true"`
	UserRepo         users.Repository
	TenantService    tenant.Service `optional:"true"`
	SessionBlacklist *auth.SessionBlacklist
	Logger           *logger.Logger
}

func provideAuthMiddleware(p AuthMiddlewareParams) *chihttp.AuthMiddleware {
	var userLookup chihttp.UserLookupService
	var tenantLookup chihttp.TenantLookupService

	if p.UserRepo != nil {
		userLookup = &userLookupAdapter{p.UserRepo}
	}

	if p.TenantService != nil {
		tenantLookup = &tenantLookupAdapter{p.TenantService}
	}

	middleware := chihttp.NewAuthMiddleware(
		p.SessionManager,
		p.AuthService, // Can be nil if only using simple auth
		userLookup,
		tenantLookup,
		p.Logger,
	)

	// Set session blacklist for logout revocation (security fix)
	if p.SessionBlacklist != nil {
		middleware.SetSessionBlacklist(p.SessionBlacklist)
	}

	return middleware
}

// userLookupAdapter adapts users.Repository to chihttp.UserLookupService.
type userLookupAdapter struct {
	repo users.Repository
}

func (a *userLookupAdapter) GetUserBySubject(ctx context.Context, subject string) (*chihttp.UserDetails, error) {
	user, err := a.repo.GetBySubject(ctx, subject)
	if errors.Is(err, users.ErrUserNotFound) {
		return nil, chihttp.ErrUnknownAccount
	}
	if err != nil {
		return nil, err
	}
	return &chihttp.UserDetails{
		ID:                 user.ID,
		Subject:            user.Subject,
		Email:              user.Email,
		EmailVerified:      user.EmailVerified,
		Name:               user.Name,
		GivenName:          user.GivenName,
		FamilyName:         user.FamilyName,
		PreferredUsername:  user.PreferredUsername,
		Locale:             user.Locale,
		UpdatedAt:          user.UpdatedAt,
		MustChangePassword: user.MustChangePassword,
		SessionEpoch:       user.SessionEpoch,
		IsActive:           user.IsActive,
		IsBlocked:          user.IsBlocked,
	}, nil
}

// HTTPServerParams contains dependencies for the HTTP server.
type HTTPServerParams struct {
	fx.In

	Config         *config.AppConfig
	Router         *chihttp.Router
	AuthMiddleware *chihttp.AuthMiddleware
	Logger         *logger.Logger
	Metrics        *metrics.Registry
	// Database, when present, is what /ready requires to answer.
	Database *database.Client `optional:"true"`
}

func provideHTTPServer(p HTTPServerParams) (*http.Server, error) {
	if p.Database != nil {
		p.Router.SetReadinessCheck(p.Database.Health)
	}
	clientIP, err := chihttp.ClientIPMiddleware(p.Config.HTTP.TrustedProxies)
	if err != nil {
		return nil, err
	}
	p.Router.Use(clientIP)
	if p.Config.HTTP.SecurityHeaders.Enabled {
		p.Router.Use(chihttp.SecurityHeadersMiddleware(chihttp.SecurityHeaders{
			HSTS:                  p.Config.HTTP.SecurityHeaders.HSTS,
			ContentSecurityPolicy: p.Config.HTTP.SecurityHeaders.ContentSecurityPolicy,
			XContentTypeOptions:   p.Config.HTTP.SecurityHeaders.XContentTypeOptions,
			XFrameOptions:         p.Config.HTTP.SecurityHeaders.XFrameOptions,
			ReferrerPolicy:        p.Config.HTTP.SecurityHeaders.ReferrerPolicy,
		}))
	}
	p.Router.SetMetrics(p.Metrics, p.Config.HTTP.Metrics)
	crossOrigin, err := chihttp.CrossOriginMiddleware(p.Config.HTTP.CORS.AllowedOrigins, p.Logger)
	if err != nil {
		return nil, err
	}
	p.Router.Use(chihttp.RecoveryMiddleware(p.Logger))
	p.Router.Use(chihttp.LoggingMiddleware(p.Logger))
	p.Router.Use(crossOrigin)

	// Register all routes AFTER middleware
	p.Router.RegisterAllRoutes()

	addr := fmt.Sprintf(":%d", p.Config.HTTP.Server.Port)

	return &http.Server{
		Addr:              addr,
		Handler:           p.Router.Handler(),
		ReadHeaderTimeout: apiReadHeaderTimeout,
		ReadTimeout:       httputil.DefaultReadTimeout,
		WriteTimeout:      apiWriteTimeout,
		IdleTimeout:       httputil.DefaultIdleTimeout,
	}, nil
}

const (
	// apiReadHeaderTimeout closes a connection that sends its headers too
	// slowly, so idle sockets cannot be held open to exhaust the server.
	apiReadHeaderTimeout = 10 * time.Second

	// apiWriteTimeout bounds a whole request, from the end of its headers to
	// the last byte of the response. It must outlast the slowest legitimate
	// request: a PDF report aggregates its data and then gives the browser
	// that renders it up to 30 seconds.
	apiWriteTimeout = 2 * time.Minute
)

func registerServerHooks(lc fx.Lifecycle, server *http.Server, log *logger.Logger, sd fx.Shutdowner) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("Starting HTTP server", logger.Str("addr", server.Addr))
			go serveOrShutdown("HTTP server", server, log, sd)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("Stopping HTTP server")
			return server.Shutdown(ctx)
		},
	})
}

// tenantLookupAdapter adapts tenant.Service to chihttp.TenantLookupService.
type tenantLookupAdapter struct {
	service tenant.Service
}

func (a *tenantLookupAdapter) GetDefaultTenantID(ctx context.Context) (string, error) {
	t, err := a.service.GetDefault(ctx)
	if err != nil {
		return "", err
	}
	if t == nil {
		return "", nil
	}
	return t.TenantID, nil
}
