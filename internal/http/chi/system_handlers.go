package chi

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	_ "github.com/DefensePoint/keycloak-monitoring/internal/http/dto" // swagger
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
	"github.com/DefensePoint/keycloak-monitoring/internal/version"
)

// EventRepository defines the interface for event operations needed by system handlers.
type EventRepository interface {
	List(ctx context.Context, opts *events.ListOptions) ([]*domain.Event, error)
	CountWithFilter(ctx context.Context, opts *events.ListOptions) (int64, error)
}

// RealmLister lists the realms configured for a tenant. It is used to scope
// event queries to the sources (keycloak:<realm>, amfa:<realm>) that belong to
// the tenant in the request.
type RealmLister interface {
	ListRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error)
}

// SystemHandlers handles HTTP requests for system-level operations (events, stats, version).
type SystemHandlers struct {
	eventRepo      EventRepository
	tenantService  tenant.Service
	realmLister    RealmLister
	rbacService    RBACService
	log            *logger.Logger
	authMiddleware func(http.Handler) http.Handler
	rbacMiddleware *RBACMiddleware
}

// NewSystemHandlers creates a new SystemHandlers.
func NewSystemHandlers(
	eventRepo EventRepository,
	tenantService tenant.Service,
	realmLister RealmLister,
	rbacService RBACService,
	log *logger.Logger,
	authMiddleware func(http.Handler) http.Handler,
	rbacMiddleware *RBACMiddleware,
) *SystemHandlers {
	return &SystemHandlers{
		eventRepo:      eventRepo,
		tenantService:  tenantService,
		realmLister:    realmLister,
		rbacService:    rbacService,
		log:            log,
		authMiddleware: authMiddleware,
		rbacMiddleware: rbacMiddleware,
	}
}

// requireCaller returns the authenticated caller, responding with the 401 itself
// when there is none.
func (h *SystemHandlers) requireCaller(ctx context.Context, w http.ResponseWriter) (*UserDetails, bool) {
	user := GetUserFromContext(ctx)
	if user == nil {
		h.log.Warn("No user info in context for event scoping")
		httputil.RespondUnauthorized(w, h.log, "Authentication required")
		return nil, false
	}
	return user, true
}

// tenantEventSources returns the event sources a caller may read within a
// tenant: keycloak:<realm> and amfa:<realm> for each of the tenant's realms
// inside the caller's allowed realm set. requestedSource and requestedRealm only
// ever narrow that set, and an empty result means the query must return nothing.
func (h *SystemHandlers) tenantEventSources(ctx context.Context, userID uint, tenantID, requestedSource, requestedRealm string) ([]string, error) {
	if h.realmLister == nil {
		return nil, nil
	}

	scope, all, err := allowedRealms(ctx, h.rbacService, userID, tenantID)
	if err != nil {
		return nil, err
	}

	realms, err := h.realmLister.ListRealms(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	allowed := make([]string, 0, len(realms)*2)
	for _, realm := range realms {
		if realm == nil || realm.RealmName == "" {
			continue
		}
		if !all && !slices.Contains(scope, realm.RealmName) {
			continue
		}
		allowed = append(allowed, events.SourcesForRealm(realm.RealmName)...)
	}

	// Intersected with `allowed`, which is already narrowed to the caller's
	// realm scope, so a realm outside that scope contributes nothing.
	if requestedRealm != "" {
		allowedSet := make(map[string]struct{}, len(allowed))
		for _, a := range allowed {
			allowedSet[a] = struct{}{}
		}
		filtered := make([]string, 0, 2)
		for _, want := range events.SourcesForRealm(requestedRealm) {
			if _, ok := allowedSet[want]; !ok {
				continue
			}
			if requestedSource != "" && !strings.Contains(want, requestedSource) {
				continue
			}
			filtered = append(filtered, want)
		}
		return filtered, nil
	}

	if requestedSource == "" {
		return allowed, nil
	}

	filtered := make([]string, 0, len(allowed))
	for _, s := range allowed {
		if strings.Contains(s, requestedSource) {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

// RegisterRoutes registers global routes on the given Chi router.
// Routes:
//   - GET /api/version - Global version endpoint (no auth required)
func (h *SystemHandlers) RegisterRoutes(r chi.Router) {
	// Global version endpoint (no auth required)
	r.Get("/api/version", h.handleVersion)
}

// RegisterTenantRoutes registers tenant-scoped system routes.
// Routes:
//   - GET /events  - List events for tenant (requires keycloak:read permission)
//   - GET /stats   - Get statistics for tenant (requires keycloak:read permission)
//   - GET /version - Get version info (tenant-scoped, auth only)
func (h *SystemHandlers) RegisterTenantRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.validateTenantMiddleware)
		r.Use(h.rbacMiddleware.RequireTenantAccess())

		// Protected routes requiring keycloak:read permission
		r.Group(func(r chi.Router) {
			r.Use(h.rbacMiddleware.RequirePermission("keycloak:read"))
			r.Get("/events", h.handleEvents)
			r.Get("/stats", h.handleStats)
		})

		// Version endpoint only requires authentication and tenant access (no specific permission)
		// Returns same public version info as global /api/version endpoint
		r.Get("/version", h.handleTenantVersion)
	})
}

// validateTenantMiddleware validates tenant exists and is enabled.
func (h *SystemHandlers) validateTenantMiddleware(next http.Handler) http.Handler {
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
			http.Error(w, `{"error":"Tenant not found"}`, http.StatusNotFound)
			return
		}

		// Check if tenant is enabled
		if !tenant.Enabled {
			h.log.Warn("Tenant is disabled",
				logger.Str("tenant_id", tenantID),
				logger.Str("path", r.URL.Path))
			http.Error(w, `{"error":"Tenant is disabled"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleEvents handles GET /api/tenants/{tenant_id}/events
//
//	@Summary		List events
//	@Description	Returns a paginated list of events for a tenant
//	@Tags			system
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			limit		query		int		false	"Limit results (default 100)"
//	@Param			offset		query		int		false	"Offset for pagination"
//	@Param			source		query		string	false	"Filter by event source, e.g. \"amfa\" or \"keycloak:MyRealm\" (substring match)"
//	@Param			realm		query		string	false	"Filter to one realm, matching every source system it produces"
//	@Param			start_time	query		string	false	"Start time filter"
//	@Param			end_time	query		string	false	"End time filter"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/events [get]
func (h *SystemHandlers) handleEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	// Parse query parameters
	limit := parseIntParam(r, "limit", 100)
	if limit < 1 {
		limit = 1
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	offset := parseIntParam(r, "offset", 0)
	source := r.URL.Query().Get("source")
	realm := r.URL.Query().Get("realm")
	startTime := r.URL.Query().Get("start_time")
	endTime := r.URL.Query().Get("end_time")

	user, ok := h.requireCaller(ctx, w)
	if !ok {
		return
	}

	sources, err := h.tenantEventSources(ctx, user.ID, tenantID, source, realm)
	if err != nil {
		h.log.Error("Failed to resolve tenant event sources",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to query events")
		return
	}
	if len(sources) == 0 {
		httputil.RespondJSON(w, h.log, http.StatusOK, map[string]interface{}{
			"events": []*domain.Event{},
			"count":  0,
			"total":  int64(0),
			"limit":  limit,
			"offset": offset,
		})
		return
	}

	opts := &events.ListOptions{
		Limit:    limit,
		Offset:   offset,
		TenantID: tenantID,
		Sources:  sources,
	}

	if startTime != "" {
		opts.StartTime = &startTime
	}
	if endTime != "" {
		opts.EndTime = &endTime
	}

	// Get events
	eventList, err := h.eventRepo.List(ctx, opts)
	if err != nil {
		h.log.Error("Failed to query events",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to query events")
		return
	}

	// Get total count for pagination
	total, err := h.eventRepo.CountWithFilter(ctx, opts)
	if err != nil {
		h.log.Error("Failed to count events",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		// Continue with 0 count rather than failing the request
		total = 0
	}

	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]interface{}{
		"events": eventList,
		"count":  len(eventList),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// handleStats handles GET /api/tenants/{tenant_id}/stats
//
//	@Summary		Get statistics
//	@Description	Returns event statistics for a tenant
//	@Tags			system
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			source		query		string	false	"Filter by event source, e.g. \"amfa\" or \"keycloak:MyRealm\" (substring match)"
//	@Param			realm		query		string	false	"Filter to one realm, matching every source system it produces"
//	@Param			start_time	query		string	false	"Start time filter"
//	@Param			end_time	query		string	false	"End time filter"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/stats [get]
func (h *SystemHandlers) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	// Parse query parameters for filtering
	source := r.URL.Query().Get("source")
	realm := r.URL.Query().Get("realm")
	startTime := r.URL.Query().Get("start_time")
	endTime := r.URL.Query().Get("end_time")

	user, ok := h.requireCaller(ctx, w)
	if !ok {
		return
	}

	sources, err := h.tenantEventSources(ctx, user.ID, tenantID, source, realm)
	if err != nil {
		h.log.Error("Failed to resolve tenant event sources",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve statistics")
		return
	}
	if len(sources) == 0 {
		httputil.RespondJSON(w, h.log, http.StatusOK, map[string]interface{}{
			"total_events": int64(0),
		})
		return
	}

	opts := &events.ListOptions{
		TenantID: tenantID,
		Sources:  sources,
	}

	if startTime != "" {
		opts.StartTime = &startTime
	}
	if endTime != "" {
		opts.EndTime = &endTime
	}

	// Get total event count
	totalEvents, err := h.eventRepo.CountWithFilter(ctx, opts)
	if err != nil {
		h.log.Error("Failed to count events",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve statistics")
		return
	}

	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]interface{}{
		"total_events": totalEvents,
	})
}

// handleVersion handles GET /api/version (global, unauthenticated endpoint).
// Only the semantic version is returned. Build metadata (git commit, build
// date, Go runtime, platform) is gated behind the authenticated tenant route
// to prevent unauthenticated fingerprinting of the deployment.
//
//	@Summary		Get application version
//	@Description	Returns the application semantic version (public endpoint)
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	version.PublicInfo
//	@Router			/api/version [get]
func (h *SystemHandlers) handleVersion(w http.ResponseWriter, r *http.Request) {
	httputil.RespondJSON(w, h.log, http.StatusOK, version.GetPublic())
}

// handleTenantVersion handles GET /api/tenants/{tenant_id}/version
//
//	@Summary		Get application version (tenant scoped)
//	@Description	Returns the application version information (requires auth)
//	@Tags			system
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Success		200			{object}	version.Info
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/version [get]
func (h *SystemHandlers) handleTenantVersion(w http.ResponseWriter, r *http.Request) {
	// Return same version info as global endpoint
	httputil.RespondJSON(w, h.log, http.StatusOK, version.Get())
}

// CanHandle returns true if this handler can handle system routes (for TenantSubRouter compatibility).
func (h *SystemHandlers) CanHandle(resourcePath string) bool {
	switch resourcePath {
	case "events", "stats", "version":
		return true
	default:
		return false
	}
}

// Ensure SystemHandlers implements RouteRegistrar and TenantSubRouter
var _ RouteRegistrar = (*SystemHandlers)(nil)
var _ TenantSubRouter = (*SystemHandlers)(nil)
