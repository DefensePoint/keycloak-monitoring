package chi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	_ "github.com/DefensePoint/keycloak-monitoring/internal/http/dto" // swagger
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/keycloak"
)

// KeycloakTenantService defines the tenant interface needed by keycloak handlers.
type KeycloakTenantService interface {
	GetTenant(ctx context.Context, tenantID string) (*KeycloakTenant, error)
}

// KeycloakTenant represents tenant info needed for validation.
type KeycloakTenant struct {
	TenantID string
	Name     string
	Enabled  bool
}

// KeycloakHandlers handles HTTP requests for keycloak operations.
// allRealms is what an aggregate reports in its realm field. It matches the
// sentinel the dashboard's realm selector already uses, so the response names
// the same thing the user picked.
const allRealms = "all"

type KeycloakHandlers struct {
	service        keycloak.Service
	rbacService    RBACChecker
	tenantService  KeycloakTenantService
	log            *logger.Logger
	authMiddleware func(http.Handler) http.Handler
	rbacMiddleware *RBACMiddleware
}

// NewKeycloakHandlers creates a new KeycloakHandlers.
func NewKeycloakHandlers(
	service keycloak.Service,
	rbacService RBACChecker,
	tenantService KeycloakTenantService,
	log *logger.Logger,
	authMiddleware func(http.Handler) http.Handler,
	rbacMiddleware *RBACMiddleware,
) *KeycloakHandlers {
	return &KeycloakHandlers{
		service:        service,
		rbacService:    rbacService,
		tenantService:  tenantService,
		log:            log,
		authMiddleware: authMiddleware,
		rbacMiddleware: rbacMiddleware,
	}
}

// RegisterRoutes registers all keycloak routes using Chi router.
// Routes are nested under /api/tenants/{tenantID}/keycloak
func (h *KeycloakHandlers) RegisterRoutes(r chi.Router) {
	// Routes are registered as part of tenant sub-routes
}

// RegisterTenantRoutes registers keycloak routes under a tenant route group.
// This should be called from the tenant router setup.
// Routes:
//   - GET /keycloak/health              - Keycloak health
//   - GET /keycloak/health/history      - Health history
//   - GET /keycloak/metrics             - Metrics
//   - GET /keycloak/metrics/history     - Metrics history
//   - GET /keycloak/events              - Events
//   - GET /keycloak/events/stats        - Event statistics
//   - GET /keycloak/realms              - List realms
//   - GET /keycloak/realms/info         - Realm info
//   - GET /keycloak/realms/users        - Realm users
//   - GET /keycloak/realms/clients      - Realm clients
//   - GET /keycloak/realms/users/details - User details
//   - GET /keycloak/version-check       - Version check
//   - GET /keycloak/dashboard           - Dashboard
//   - GET /keycloak/infinispan/metrics  - Infinispan metrics
//   - GET /realms/{realmName}/users/{userID} - User by path
func (h *KeycloakHandlers) RegisterTenantRoutes(r chi.Router) {
	r.Route("/keycloak", func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.validateTenantMiddleware)
		r.Use(h.rbacMiddleware.RequireTenantAccess())
		r.Use(h.requireKeycloakPermission)

		r.Get("/health", h.handleKeycloakHealth)
		r.Get("/health/history", h.handleKeycloakHealthHistory)
		r.Get("/metrics", h.handleKeycloakMetrics)
		r.Get("/metrics/history", h.handleKeycloakMetricsHistory)
		r.Get("/events", h.handleKeycloakEvents)
		r.Get("/events/stats", h.handleKeycloakEventStats)
		r.Get("/realms", h.handleKeycloakRealms)
		r.Get("/amfa-realms", h.handleKeycloakAmfaRealms)
		r.Get("/realms/info", h.handleKeycloakRealmInfo)
		r.Get("/realms/users", h.handleKeycloakUsers)
		r.Get("/realms/clients", h.handleKeycloakClients)
		r.Get("/realms/users/details", h.handleKeycloakUserDetails)
		r.Get("/version-check", h.handleKeycloakVersionCheck)
		r.Get("/dashboard", h.handleKeycloakDashboard)
		r.Get("/infinispan/metrics", h.handleInfinispanMetrics)
	})

	// Special route for frontend UserDetailsPage
	r.Route("/realms/{realmName}/users/{kcUserID}", func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.validateTenantMiddleware)
		r.Use(h.rbacMiddleware.RequireTenantAccess())
		r.Use(h.requireKeycloakPermission)
		r.Get("/", h.handleRealmUserByPath)
	})
}

// validateTenantMiddleware validates tenant exists and is enabled
func (h *KeycloakHandlers) validateTenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tenantID := chi.URLParam(r, "tenantID")

		if tenantID == "" {
			httputil.RespondBadRequest(w, h.log, "Tenant ID is required")
			return
		}

		// Skip validation if tenantService is not available
		if h.tenantService == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Get tenant from service
		tenant, err := h.tenantService.GetTenant(ctx, tenantID)
		if err != nil {
			h.log.Warn("Tenant not found",
				logger.Str("tenant_id", tenantID),
				logger.Str("path", r.URL.Path),
				logger.Err(err))
			httputil.RespondNotFound(w, h.log, "Tenant not found")
			return
		}

		// Check if tenant is enabled
		if !tenant.Enabled {
			h.log.Warn("Tenant is disabled",
				logger.Str("tenant_id", tenantID),
				logger.Str("path", r.URL.Path))
			httputil.RespondForbidden(w, h.log, "Tenant is disabled")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// requireKeycloakPermission middleware checks keycloak:read permission
func (h *KeycloakHandlers) requireKeycloakPermission(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tenantID := chi.URLParam(r, "tenantID")

		user := GetUserFromContext(ctx)
		if user == nil {
			h.log.Warn("No user info in context for keycloak permission check")
			httputil.RespondUnauthorized(w, h.log, "Authentication required")
			return
		}

		hasPermission, err := h.rbacService.HasPermission(ctx, user.ID, "keycloak:read", &tenantID)
		if err != nil {
			h.log.Error("Failed to check keycloak permission",
				logger.Err(err),
				logger.Uint("user_id", user.ID),
				logger.Str("tenant_id", tenantID))
			httputil.RespondInternalError(w, h.log, "Failed to check permissions")
			return
		}

		if !hasPermission {
			h.log.Warn("Keycloak permission denied",
				logger.Uint("user_id", user.ID),
				logger.Str("email", user.Email),
				logger.Str("tenant_id", tenantID))
			httputil.RespondForbidden(w, h.log, "Permission denied")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleKeycloakHealth returns the latest Keycloak health status.
//
//	@Summary		Get Keycloak health status
//	@Description	Returns the latest health status of the Keycloak instance
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/health [get]
func (h *KeycloakHandlers) handleKeycloakHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	health, err := h.service.GetLatestHealth(ctx, tenantID)
	if err != nil {
		h.log.Error("Failed to get health", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve health status")
		return
	}

	if health == nil {
		httputil.RespondSuccess(w, h.log, map[string]interface{}{
			"status":  "unknown",
			"message": "No health data available yet",
		})
		return
	}

	httputil.RespondSuccess(w, h.log, health)
}

// handleKeycloakHealthHistory returns Keycloak health check history.
//
//	@Summary		Get Keycloak health history
//	@Description	Returns health check history for the Keycloak instance
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			hours		query		int		false	"Number of hours to look back (default 24)"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/health/history [get]
func (h *KeycloakHandlers) handleKeycloakHealthHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	from, to := parseHoursParam(r, 24)

	history, err := h.service.GetHealthHistory(ctx, tenantID, from, to)
	if err != nil {
		h.log.Error("Failed to get health history", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve health history")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"health_checks": history,
		"count":         len(history),
		"start":         from,
		"end":           to,
	})
}

// handleKeycloakMetrics returns the latest metrics for all realms.
//
//	@Summary		Get Keycloak metrics
//	@Description	Returns metrics for all realms or a specific realm
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realm		query		string	false	"Filter by realm name"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/metrics [get]
func (h *KeycloakHandlers) handleKeycloakMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	realmName := r.URL.Query().Get("realm")

	if realmName != "" {
		// Check realm access when specific realm is requested
		if !h.checkRealmAccess(ctx, w, tenantID, realmName) {
			return
		}

		metrics, err := h.service.GetLatestMetricsByRealm(ctx, tenantID, realmName)
		if err != nil {
			h.log.Error("Failed to get metrics", logger.Err(err))
			httputil.RespondInternalError(w, h.log, "Failed to retrieve metrics")
			return
		}

		if metrics == nil {
			httputil.RespondSuccess(w, h.log, map[string]interface{}{
				"realm":   realmName,
				"message": "No metrics available for this realm",
			})
			return
		}

		httputil.RespondSuccess(w, h.log, metrics)
		return
	}

	scope, all, ok := h.callerRealmScope(ctx, w, tenantID)
	if !ok {
		return
	}

	allMetrics, err := h.service.GetAllRealmMetrics(ctx, tenantID)
	if err != nil {
		h.log.Error("Failed to get all realm metrics", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve metrics")
		return
	}
	if !all {
		allMetrics = narrowRealmMetrics(allMetrics, scope)
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"metrics": allMetrics,
		"count":   len(allMetrics),
	})
}

// narrowRealmMetrics keeps only the realms the caller's scope names. The result
// is driven from scope, so an empty scope yields no realm, never every realm.
func narrowRealmMetrics(metrics map[string]*domain.KeycloakMetrics, scope []string) map[string]*domain.KeycloakMetrics {
	narrowed := make(map[string]*domain.KeycloakMetrics, len(scope))
	for _, realm := range scope {
		if m, ok := metrics[realm]; ok {
			narrowed[realm] = m
		}
	}
	return narrowed
}

// handleKeycloakMetricsHistory returns metrics history for a realm.
//
//	@Summary		Get Keycloak metrics history
//	@Description	Returns metrics history for a specific realm
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realm		query		string	true	"Realm name"
//	@Param			hours		query		int		false	"Number of hours to look back (default 24)"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	dto.ErrorResponse	"Missing realm parameter"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/metrics/history [get]
func (h *KeycloakHandlers) handleKeycloakMetricsHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	realmName := r.URL.Query().Get("realm")
	if realmName == "" {
		httputil.RespondBadRequest(w, h.log, "realm query parameter is required")
		return
	}

	// Check realm access
	if !h.checkRealmAccess(ctx, w, tenantID, realmName) {
		return
	}

	from, to := parseHoursParam(r, 24)

	metrics, err := h.service.GetMetricsHistoryByRealm(ctx, tenantID, realmName, from, to)
	if err != nil {
		h.log.Error("Failed to get metrics history",
			logger.Str("realm", realmName),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve metrics history")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"realm":   realmName,
		"metrics": metrics,
		"count":   len(metrics),
		"start":   from,
		"end":     to,
	})
}

// handleKeycloakEvents returns recent Keycloak events.
//
//	@Summary		List Keycloak events
//	@Description	Returns recent Keycloak events for a realm
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realm		query		string	true	"Realm name"
//	@Param			type		query		string	false	"Filter by event type"
//	@Param			limit		query		int		false	"Limit results (default 100)"
//	@Param			hours		query		int		false	"Number of hours to look back (default 1)"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	dto.ErrorResponse	"Missing realm parameter"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/events [get]
func (h *KeycloakHandlers) handleKeycloakEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	realmName := r.URL.Query().Get("realm")
	if realmName == "" {
		httputil.RespondBadRequest(w, h.log, "realm query parameter is required")
		return
	}

	// Check realm access
	if !h.checkRealmAccess(ctx, w, tenantID, realmName) {
		return
	}

	eventType := r.URL.Query().Get("type")
	limit := parseIntParam(r, "limit", DefaultLimit)
	if limit < 1 {
		limit = 1
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	from, to := parseHoursParam(r, 1)

	var events []*domain.KeycloakEvent
	var err error

	if eventType != "" {
		events, err = h.service.ListEventsByType(ctx, tenantID, realmName, eventType, from, to, limit, 0)
	} else {
		events, err = h.service.ListEventsByRealm(ctx, tenantID, realmName, from, to, limit, 0)
	}

	if err != nil {
		h.log.Error("Failed to list events", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve events")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"realm":  realmName,
		"type":   eventType,
		"events": events,
		"start":  from,
		"end":    to,
		"limit":  limit,
	})
}

// handleKeycloakEventStats returns event statistics for a realm.
//
//	@Summary		Get Keycloak event statistics
//	@Description	Returns event statistics aggregated by type for a realm
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realm		query		string	true	"Realm name"
//	@Param			hours		query		int		false	"Number of hours to look back (default 24)"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	dto.ErrorResponse	"Missing realm parameter"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/events/stats [get]
func (h *KeycloakHandlers) handleKeycloakEventStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	realmName := r.URL.Query().Get("realm")
	from, to := parseTimeWindow(r, 24)

	if realmName != "" {
		// Check realm access
		if !h.checkRealmAccess(ctx, w, tenantID, realmName) {
			return
		}

		stats, err := h.service.GetEventStats(ctx, tenantID, realmName, from, to)
		if err != nil {
			h.log.Error("Failed to get event stats", logger.Err(err))
			httputil.RespondInternalError(w, h.log, "Failed to retrieve event statistics")
			return
		}

		httputil.RespondSuccess(w, h.log, stats)
		return
	}

	// No realm means All Realms, which is the dashboard's default view because
	// default_realm ships empty. Refusing it left the KPI strip showing zero
	// logins and zero failures while failures existed. Unlike the AMFA
	// transport, whose credentials are scoped to a single realm, this path can
	// aggregate: the platform already enumerates and polls every realm.
	realms, ok := h.statsRealmsForCaller(ctx, w, tenantID)
	if !ok {
		return
	}

	aggregate := &keycloak.EventStats{Realm: allRealms, Start: from, End: to}
	for _, realm := range realms {
		stats, err := h.service.GetEventStats(ctx, tenantID, realm, from, to)
		if err != nil {
			h.log.Error("Failed to get event stats",
				logger.Str("realm", realm),
				logger.Err(err))
			httputil.RespondInternalError(w, h.log, "Failed to retrieve event statistics")
			return
		}

		aggregate.LoginCount += stats.LoginCount
		aggregate.LoginErrorCount += stats.LoginErrorCount
		aggregate.LogoutCount += stats.LogoutCount
		aggregate.RegisterCount += stats.RegisterCount
		aggregate.CodeToTokenCount += stats.CodeToTokenCount
		aggregate.TotalEvents += stats.TotalEvents
	}

	httputil.RespondSuccess(w, h.log, aggregate)
}

// statsRealmsForCaller resolves which realms an All Realms aggregate may read.
//
// An unrestricted caller gets every realm the tenant has; a scoped caller gets
// only their own. An empty slice with all = false means no realm, never no
// restriction, so it is refused rather than widened: summing across realms the
// caller cannot open would leak their activity through a total.
func (h *KeycloakHandlers) statsRealmsForCaller(ctx context.Context, w http.ResponseWriter, tenantID string) ([]string, bool) {
	user := GetUserFromContext(ctx)
	if user == nil {
		httputil.RespondUnauthorized(w, h.log, "Authentication required")
		return nil, false
	}

	allowed, all, err := allowedRealms(ctx, h.rbacService, user.ID, tenantID)
	if err != nil {
		h.log.Error("Failed to resolve permitted realms", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve event statistics")
		return nil, false
	}

	if all {
		infos, err := h.service.ListRealms(ctx, tenantID)
		if err != nil {
			h.log.Error("Failed to list realms for stats aggregate", logger.Err(err))
			httputil.RespondInternalError(w, h.log, "Failed to retrieve event statistics")
			return nil, false
		}

		names := make([]string, 0, len(infos))
		for _, info := range infos {
			names = append(names, info.RealmName)
		}
		return names, true
	}

	if len(allowed) == 0 {
		httputil.RespondForbidden(w, h.log, "Access to realm denied")
		return nil, false
	}

	return allowed, true
}

// handleKeycloakRealms returns information about monitored realms.
//
//	@Summary		List Keycloak realms
//	@Description	Returns a list of all monitored realms
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/realms [get]
func (h *KeycloakHandlers) handleKeycloakRealms(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	scope, all, ok := h.callerRealmScope(ctx, w, tenantID)
	if !ok {
		return
	}

	realms, err := h.service.ListRealms(ctx, tenantID)
	if err != nil {
		h.log.Error("Failed to list realms", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve realms")
		return
	}
	if !all {
		narrowed := make([]*domain.KeycloakRealmInfo, 0, len(scope))
		for _, realm := range realms {
			if realm != nil && slices.Contains(scope, realm.RealmName) {
				narrowed = append(narrowed, realm)
			}
		}
		realms = narrowed
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"realms": realms,
		"count":  len(realms),
	})
}

// handleKeycloakAmfaRealms returns the names of realms whose Keycloak browser
// login flow uses an Adaptive MFA (AMFA) authenticator. The UI uses this
// to show/hide AMFA-only sections (KPI cards, geo map, Risk column) per realm.
//
//	@Summary		List AMFA-enabled realms
//	@Description	Returns realm names whose browser login flow uses an Adaptive MFA authenticator
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/amfa-realms [get]
func (h *KeycloakHandlers) handleKeycloakAmfaRealms(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	scope, all, ok := h.callerRealmScope(ctx, w, tenantID)
	if !ok {
		return
	}

	realms, err := h.service.ListAmfaEnabledRealms(ctx, tenantID)
	if err != nil {
		// Without a live Keycloak client we cannot determine AMFA config; return
		// an empty set (the UI fail-closes and hides AMFA sections) rather than a
		// 500, since this endpoint gates optional UI, not core data.
		if errors.Is(err, keycloak.ErrClientProviderNotConfigured) ||
			errors.Is(err, keycloak.ErrClientNotAvailable) {
			httputil.RespondSuccess(w, h.log, map[string]interface{}{
				"realms": []string{},
				"count":  0,
			})
			return
		}
		h.log.Error("Failed to list AMFA-enabled realms", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to determine AMFA-enabled realms")
		return
	}
	if !all {
		narrowed := make([]string, 0, len(scope))
		for _, realm := range realms {
			if slices.Contains(scope, realm) {
				narrowed = append(narrowed, realm)
			}
		}
		realms = narrowed
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"realms": realms,
		"count":  len(realms),
	})
}

// handleKeycloakRealmInfo returns detailed information about a specific realm.
//
//	@Summary		Get Keycloak realm info
//	@Description	Returns detailed information about a specific realm
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realm		query		string	true	"Realm name"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	dto.ErrorResponse	"Missing realm parameter"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Realm not found"
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/realms/info [get]
func (h *KeycloakHandlers) handleKeycloakRealmInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	realmName := r.URL.Query().Get("realm")
	if realmName == "" {
		httputil.RespondBadRequest(w, h.log, "realm query parameter is required")
		return
	}

	// Check realm access
	if !h.checkRealmAccess(ctx, w, tenantID, realmName) {
		return
	}

	realm, err := h.service.GetRealm(ctx, tenantID, realmName)
	if err != nil {
		h.log.Error("Failed to get realm", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve realm")
		return
	}

	if realm == nil {
		httputil.RespondNotFound(w, h.log, "Realm not found")
		return
	}

	httputil.RespondSuccess(w, h.log, realm)
}

// handleKeycloakVersionCheck returns the latest Keycloak version with comparison.
//
//	@Summary		Check Keycloak version
//	@Description	Returns current version info and comparison with latest releases
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/version-check [get]
func (h *KeycloakHandlers) handleKeycloakVersionCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	versionInfo, err := h.service.GetVersionInfo(ctx, tenantID)
	if err != nil {
		h.log.Error("Failed to get version info",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve version data from health check or external sources")
		return
	}

	if versionInfo == nil {
		httputil.RespondSuccess(w, h.log, map[string]interface{}{
			"message": "Version checker not available",
		})
		return
	}

	httputil.RespondSuccess(w, h.log, versionInfo)
}

// handleKeycloakDashboard returns aggregated dashboard data.
//
//	@Summary		Get Keycloak dashboard
//	@Description	Returns aggregated dashboard data for a tenant
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realm		query		string	false	"Filter by realm name"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/dashboard [get]
func (h *KeycloakHandlers) handleKeycloakDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	realmName := r.URL.Query().Get("realm")

	// Check realm access if realm specified
	if realmName != "" && !h.checkRealmAccess(ctx, w, tenantID, realmName) {
		return
	}

	scope, all, ok := h.callerRealmScope(ctx, w, tenantID)
	if !ok {
		return
	}

	dashboard, err := h.service.GetDashboard(ctx, tenantID, realmName)
	if err != nil {
		h.log.Error("Failed to get dashboard",
			logger.Str("tenant_id", tenantID),
			logger.Str("realm", realmName),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve dashboard data")
		return
	}
	if realmName == "" && !all {
		narrowDashboardRealms(dashboard, scope)
	}

	httputil.RespondSuccess(w, h.log, dashboard)
}

// narrowDashboardRealms keeps only the per-realm rows the caller's scope names
// and restates the realm count over what is left, so the total does not disclose
// the tenant's true realm count.
func narrowDashboardRealms(dashboard *keycloak.Dashboard, scope []string) {
	if dashboard == nil {
		return
	}
	narrowed := make([]*keycloak.RealmSummary, 0, len(scope))
	for _, summary := range dashboard.Realms {
		if summary != nil && slices.Contains(scope, summary.RealmName) {
			narrowed = append(narrowed, summary)
		}
	}
	dashboard.Realms = narrowed
	dashboard.TotalRealms = len(narrowed)
}

// handleKeycloakUsers returns users for a specific realm.
//
//	@Summary		List Keycloak users
//	@Description	Returns users for a specific realm with pagination
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realm		query		string	true	"Realm name"
//	@Param			first		query		int		false	"First result offset (default 0)"
//	@Param			max			query		int		false	"Max results (default 100)"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	dto.ErrorResponse	"Missing realm parameter"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/realms/users [get]
func (h *KeycloakHandlers) handleKeycloakUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	realmName := r.URL.Query().Get("realm")
	if realmName == "" {
		httputil.RespondBadRequest(w, h.log, "realm query parameter is required")
		return
	}

	// Check realm access
	if !h.checkRealmAccess(ctx, w, tenantID, realmName) {
		return
	}

	// Get pagination params with bounds (matching legacy behavior)
	first := httputil.GetIntQueryWithBounds(r, "first", 0, 0, 100000)
	max := httputil.GetIntQueryWithBounds(r, "max", 100, 1, MaxLimit)

	users, err := h.service.GetUsers(ctx, tenantID, realmName, first, max)
	if err != nil {
		h.log.Error("Failed to get users", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve users")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"realm": realmName,
		"users": users,
		"count": len(users),
		"first": first,
		"max":   max,
	})
}

// handleKeycloakClients returns clients for a specific realm.
//
//	@Summary		List Keycloak clients
//	@Description	Returns clients for a specific realm
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realm		query		string	true	"Realm name"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	dto.ErrorResponse	"Missing realm parameter"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/realms/clients [get]
func (h *KeycloakHandlers) handleKeycloakClients(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	realmName := r.URL.Query().Get("realm")
	if realmName == "" {
		httputil.RespondBadRequest(w, h.log, "realm query parameter is required")
		return
	}

	// Check realm access
	if !h.checkRealmAccess(ctx, w, tenantID, realmName) {
		return
	}

	clients, err := h.service.GetClients(ctx, tenantID, realmName)
	if err != nil {
		h.log.Error("Failed to get clients", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve clients")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"realm":   realmName,
		"clients": clients,
		"count":   len(clients),
	})
}

// handleKeycloakUserDetails returns complete user information including groups and roles.
//
//	@Summary		Get Keycloak user details
//	@Description	Returns complete user information including groups and roles
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realm		query		string	true	"Realm name"
//	@Param			userId		query		string	true	"User ID"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	dto.ErrorResponse	"Missing realm or userId parameter"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/realms/users/details [get]
func (h *KeycloakHandlers) handleKeycloakUserDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	realmName := r.URL.Query().Get("realm")
	if realmName == "" {
		httputil.RespondBadRequest(w, h.log, "realm query parameter is required")
		return
	}

	userID := r.URL.Query().Get("userId")
	if userID == "" {
		httputil.RespondBadRequest(w, h.log, "userId query parameter is required")
		return
	}

	// Check realm access
	if !h.checkRealmAccess(ctx, w, tenantID, realmName) {
		return
	}

	userDetails, err := h.service.GetUserDetails(ctx, tenantID, realmName, userID)
	if err != nil {
		h.log.Error("Failed to get user details", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve user details")
		return
	}

	httputil.RespondSuccess(w, h.log, userDetails)
}

// handleInfinispanMetrics fetches and returns Infinispan cache metrics.
//
//	@Summary		Get Infinispan cache metrics
//	@Description	Returns Infinispan cache metrics for the Keycloak instance
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/keycloak/infinispan/metrics [get]
func (h *KeycloakHandlers) handleInfinispanMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	metrics, err := h.service.GetInfinispanMetrics(ctx, tenantID)
	if err != nil {
		h.log.Error("Failed to get Infinispan metrics", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve Infinispan metrics")
		return
	}

	httputil.RespondSuccess(w, h.log, metrics)
}

// handleRealmUserByPath handles GET /api/tenants/{tenant_id}/realms/{realmName}/users/{userId}
//
//	@Summary		Get user by path
//	@Description	Returns user details using path parameters (for frontend UserDetailsPage)
//	@Tags			keycloak
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realmName	path		string	true	"Realm name"
//	@Param			kcUserID	path		string	true	"Keycloak User ID"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	dto.ErrorResponse	"Missing realm name or user ID"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/realms/{realmName}/users/{kcUserID} [get]
func (h *KeycloakHandlers) handleRealmUserByPath(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")
	realmName := chi.URLParam(r, "realmName")
	userID := chi.URLParam(r, "kcUserID")

	if realmName == "" || userID == "" {
		httputil.RespondBadRequest(w, h.log, "realm name and user ID are required")
		return
	}

	// Check realm access
	if !h.checkRealmAccess(ctx, w, tenantID, realmName) {
		return
	}

	userDetails, err := h.service.GetUserDetails(ctx, tenantID, realmName, userID)
	if err != nil {
		h.log.Error("Failed to get user details",
			logger.Str("realm", realmName),
			logger.Str("user_id", userID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve user details")
		return
	}

	httputil.RespondSuccess(w, h.log, userDetails)
}

// checkRealmAccess verifies that the user has access to the specified realm.
// An empty realm name names no realm to authorize, so it is denied.
func (h *KeycloakHandlers) checkRealmAccess(ctx context.Context, w http.ResponseWriter, tenantID, realmName string) bool {
	user := GetUserFromContext(ctx)
	if user == nil {
		h.log.Warn("No user info in context for realm access check")
		httputil.RespondUnauthorized(w, h.log, "Authentication required")
		return false
	}

	if realmName == "" {
		httputil.RespondForbidden(w, h.log, "Access to realm denied")
		return false
	}

	hasAccess, err := h.rbacService.HasAccessToRealm(ctx, user.ID, tenantID, realmName)
	if err != nil {
		h.log.Error("Failed to check realm access",
			logger.Err(err),
			logger.Uint("user_id", user.ID),
			logger.Str("tenant_id", tenantID),
			logger.Str("realm", realmName))
		httputil.RespondInternalError(w, h.log, "Failed to check realm access")
		return false
	}

	if !hasAccess {
		h.log.Warn("Realm access denied",
			logger.Uint("user_id", user.ID),
			logger.Str("email", user.Email),
			logger.Str("tenant_id", tenantID),
			logger.Str("realm", realmName))
		httputil.RespondForbidden(w, h.log, "Access to realm denied")
		return false
	}

	return true
}

// callerRealmScope resolves the realms the caller may read within a tenant,
// responding with the failure itself and returning ok = false. all = true means
// unrestricted, and scope is then empty rather than every realm.
func (h *KeycloakHandlers) callerRealmScope(ctx context.Context, w http.ResponseWriter, tenantID string) (scope []string, all, ok bool) {
	user := GetUserFromContext(ctx)
	if user == nil {
		h.log.Warn("No user info in context for realm access check")
		httputil.RespondUnauthorized(w, h.log, "Authentication required")
		return nil, false, false
	}

	scope, all, err := allowedRealms(ctx, h.rbacService, user.ID, tenantID)
	if err != nil {
		h.log.Error("Failed to check realm access",
			logger.Err(err),
			logger.Uint("user_id", user.ID),
			logger.Str("tenant_id", tenantID))
		httputil.RespondInternalError(w, h.log, "Failed to check realm access")
		return nil, false, false
	}

	return scope, all, true
}

// CanHandle returns true if this handler can handle keycloak routes (for TenantSubRouter compatibility).
func (h *KeycloakHandlers) CanHandle(resourcePath string) bool {
	if strings.HasPrefix(resourcePath, "keycloak") {
		return true
	}
	if strings.HasPrefix(resourcePath, "realms/") && strings.Contains(resourcePath, "/users/") {
		return true
	}
	return false
}

// Ensure KeycloakHandlers implements TenantSubRouter
var _ TenantSubRouter = (*KeycloakHandlers)(nil)
