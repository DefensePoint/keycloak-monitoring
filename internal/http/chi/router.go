// Package chi provides a Chi-based HTTP router implementation.
// This is an alternative to the standard library http.ServeMux router.
// All routes are preserved exactly as they were - this is a drop-in replacement.
package chi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"

	// Import generated swagger docs
	_ "github.com/DefensePoint/keycloak-monitoring/docs/swagger"
)

// RouteRegistrar is the interface for handlers that can register their routes.
type RouteRegistrar interface {
	RegisterRoutes(r chi.Router)
}

// TenantSubRouter is implemented by handlers that register routes under /api/tenants/{tenantID}.
// These routes are automatically nested under the tenant route group and have access to the tenantID path parameter.
type TenantSubRouter interface {
	RegisterTenantRoutes(r chi.Router)
}

// Router manages HTTP routing using Chi.
type Router struct {
	chi            chi.Router
	logger         *logger.Logger
	authMiddleware func(http.Handler) http.Handler
	rbacMiddleware *RBACMiddleware
	handlers       []RouteRegistrar
	metrics        *metrics.Registry
	metricsConfig  config.MetricsConfig
}

// SetMetrics enables the /metrics endpoint and request instrumentation. It must
// be called before RegisterAllRoutes: Chi requires every middleware to be
// installed before the first route is defined.
func (r *Router) SetMetrics(m *metrics.Registry, cfg config.MetricsConfig) {
	r.metrics = m
	r.metricsConfig = cfg

	if m != nil && cfg.Enabled {
		r.chi.Use(MetricsMiddleware(m))
	}
}

// NewRouter creates a new Chi Router with the given handlers.
func NewRouter(
	log *logger.Logger,
	authMiddleware func(http.Handler) http.Handler,
	rbacMiddleware *RBACMiddleware,
	handlers ...RouteRegistrar,
) *Router {
	r := chi.NewRouter()

	// Add Chi's built-in middleware (optional, can be customized)
	r.Use(middleware.RequestID)
	// chi v5.3.0 deprecated RealIP without changing its behavior: it still
	// overwrites RemoteAddr from True-Client-IP / X-Real-IP / the leftmost
	// X-Forwarded-For whether or not a proxy set them, so the client IP here
	// remains attacker-controlled. Kept as-is for now because the documented
	// deployment does front this with nginx/traefik, and the safe replacements
	// (ClientIPFromXFFTrustedProxies + GetClientIP) need a trusted-proxy CIDR
	// setting this repo does not have yet. The only consumer on this router is
	// the remote_addr field in the request log, so the exposure is spoofable
	// log entries, not an authorization or rate-limit decision.
	r.Use(middleware.RealIP) //nolint:staticcheck // SA1019: see the comment above

	return &Router{
		chi:            r,
		logger:         log,
		authMiddleware: authMiddleware,
		rbacMiddleware: rbacMiddleware,
		handlers:       handlers,
	}
}

// RegisterAllRoutes registers all handler routes on the Chi router.
func (r *Router) RegisterAllRoutes() {
	// Register health check endpoints (no auth required)
	r.chi.Get("/health", r.handleHealth)
	r.chi.Get("/ready", r.handleReady)

	if r.metrics != nil && r.metricsConfig.Enabled {
		r.chi.Get("/metrics", r.handleMetrics())
	}

	// Swagger UI endpoint
	r.chi.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// Register all handlers
	for _, h := range r.handlers {
		h.RegisterRoutes(r.chi)
	}
}

// Handler returns the underlying http.Handler.
func (r *Router) Handler() http.Handler {
	return r.chi
}

// Chi returns the underlying chi.Router for direct manipulation.
func (r *Router) Chi() chi.Router {
	return r.chi
}

// GetAuthMiddleware returns the auth middleware for use by handlers.
func (r *Router) GetAuthMiddleware() func(http.Handler) http.Handler {
	return r.authMiddleware
}

// GetRBACMiddleware returns the RBAC middleware for use by handlers.
func (r *Router) GetRBACMiddleware() *RBACMiddleware {
	return r.rbacMiddleware
}

// handleHealth returns a simple health check response.
func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"healthy"}`))
}

// handleReady returns a readiness check response.
func (r *Router) handleReady(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

// handleMetrics serves the Prometheus exposition endpoint, optionally behind a
// bearer token.
func (r *Router) handleMetrics() http.HandlerFunc {
	return metrics.GuardedHandler(r.metrics, r.metricsConfig).ServeHTTP
}

// AddHandler adds a handler to be registered.
func (r *Router) AddHandler(h RouteRegistrar) {
	r.handlers = append(r.handlers, h)
}

// Use appends a middleware handler to the router middleware stack.
func (r *Router) Use(middlewares ...func(http.Handler) http.Handler) {
	r.chi.Use(middlewares...)
}
