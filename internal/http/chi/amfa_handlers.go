// Package chi — AMFA (Adaptive MFA) HTTP handlers.
//
// Routes are registered under /api/tenants/{tenantID}/amfa/* via
// RegisterTenantRoutes. AMFA has no global (untenanted) routes, so
// RegisterRoutes is a no-op kept to satisfy the RouteRegistrar interface.
package chi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfacheck"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// AmfaHandlers handles HTTP requests for AMFA event/stats/geo endpoints.
type AmfaHandlers struct {
	service        amfa.Service
	tenantService  tenant.Service
	rbacService    RBACService
	log            *logger.Logger
	authMiddleware func(http.Handler) http.Handler
	rbacMiddleware *RBACMiddleware
	checkerRunner  *amfacheck.Runner
}

// NewAmfaHandlers creates a new AmfaHandlers.
func NewAmfaHandlers(
	svc amfa.Service,
	tenantSvc tenant.Service,
	rbacSvc RBACService,
	log *logger.Logger,
	authMW func(http.Handler) http.Handler,
	rbacMW *RBACMiddleware,
	checkerRunner *amfacheck.Runner,
) *AmfaHandlers {
	return &AmfaHandlers{
		service:        svc,
		tenantService:  tenantSvc,
		rbacService:    rbacSvc,
		log:            log,
		authMiddleware: authMW,
		rbacMiddleware: rbacMW,
		checkerRunner:  checkerRunner,
	}
}

// RegisterRoutes is a no-op for AMFA — it has no global (non-tenant) routes.
func (h *AmfaHandlers) RegisterRoutes(_ chi.Router) {}

// RegisterTenantRoutes registers AMFA routes under tenant scope:
//
//	GET /amfa/events
//	GET /amfa/stats
//	GET /amfa/geo
//
// All three require auth, tenant access, and the amfa:read permission.
func (h *AmfaHandlers) RegisterTenantRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		if h.authMiddleware != nil {
			r.Use(h.authMiddleware)
		}
		if h.rbacMiddleware != nil {
			r.Use(h.rbacMiddleware.RequireTenantAccess())
			r.Use(h.rbacMiddleware.RequirePermission("amfa:read"))
		}

		r.Get("/amfa/events", h.handleAmfaEvents)
		r.Get("/amfa/stats", h.handleAmfaStats)
		r.Get("/amfa/geo", h.handleAmfaGeo)
		r.Get("/amfa-checker", h.handleAmfaCheckerStatus)
		r.Post("/amfa-checker/run", h.handleAmfaCheckerRun)
	})
}

// handleAmfaEvents handles GET /api/tenants/{tenantID}/amfa/events.
//
// Required query params:
//   - realm_id
//
// Optional query params:
//   - limit (default 25, capped at MaxLimit)
//   - offset (default 0)
//   - start_time / end_time (RFC3339)
//   - risk_level (integer; passed as ">=" filter by the repository)
//   - event_type (exact match)
func (h *AmfaHandlers) handleAmfaEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")
	q := r.URL.Query()

	realm := q.Get("realm_id")
	if realm == "" {
		httputil.RespondBadRequest(w, h.log, "realm_id query param is required")
		return
	}

	if !h.checkRealmAccess(ctx, w, tenantID, realm) {
		return
	}

	limit := parseIntParam(r, "limit", 25)
	if limit < 1 {
		limit = 1
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	offset := parseIntParam(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	// Cap offset to prevent a trivial DoS via deep-page scans against AMFA's
	// auth_event/auth_process JOIN. 50_000 rows is 250 pages at the maximum
	// page size (200) — well past any realistic user navigation. For deeper
	// access, time-range filtering should be used instead.
	const maxOffset = 50_000
	if offset > maxOffset {
		httputil.RespondBadRequest(w, h.log,
			fmt.Sprintf("offset must be <= %d; use a tighter time range to access older events", maxOffset))
		return
	}

	start, end, err := parseTimeRange(q.Get("start_time"), q.Get("end_time"))
	if err != nil {
		httputil.RespondBadRequest(w, h.log, err.Error())
		return
	}

	var riskPtr *int
	if rl := q.Get("risk_level"); rl != "" {
		if n, err := strconv.Atoi(rl); err == nil {
			riskPtr = &n
		}
	}

	opts := amfa.ListEventsOptions{
		RealmID:   realm,
		StartTime: start,
		EndTime:   end,
		RiskLevel: riskPtr,
		EventType: q.Get("event_type"),
		Limit:     limit,
		Offset:    offset,
	}

	events, total, err := h.service.ListEvents(ctx, tenantID, opts)
	if errors.Is(err, amfa.ErrOffsetTooLarge) {
		httputil.RespondBadRequest(w, h.log,
			"offset is too large for this tenant's AMFA transport; use a tighter time range to access older events")
		return
	}
	if errors.Is(err, amfa.ErrAmfaNotConfigured) {
		httputil.RespondJSON(w, h.log, http.StatusNotFound, map[string]interface{}{
			"error":   "amfa_not_configured",
			"message": "AMFA Events is not configured for this tenant",
		})
		return
	}
	if errors.Is(err, amfa.ErrAmfaUnavailable) {
		httputil.RespondJSON(w, h.log, http.StatusServiceUnavailable, map[string]interface{}{
			"error": "amfa_unavailable",
		})
		return
	}
	if err != nil {
		h.log.Error("AMFA list events failed",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "AMFA list events failed")
		return
	}

	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]interface{}{
		"items":  events,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// handleAmfaStats handles GET /api/tenants/{tenantID}/amfa/stats.
//
// Optional query params:
//   - realm_id (omit for "all realms" - aggregates across every realm in the
//     tenant's AMFA DB)
//   - start_time / end_time (RFC3339)
func (h *AmfaHandlers) handleAmfaStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")
	q := r.URL.Query()

	// realm_id is optional here: an empty value means "all realms" and the KPI
	// query drops the realm filter (unlike /amfa/events and /amfa/geo, which
	// stay per-realm). This backs the "All Realms" selection on the Events page.
	realm, narrowed, ok := h.statsRealm(ctx, w, tenantID, q.Get("realm_id"))
	if !ok {
		return
	}

	start, end, err := parseTimeRange(q.Get("start_time"), q.Get("end_time"))
	if err != nil {
		httputil.RespondBadRequest(w, h.log, err.Error())
		return
	}

	stats, err := h.service.GetStats(ctx, tenantID, amfa.StatsOptions{
		RealmID:   realm,
		StartTime: start,
		EndTime:   end,
	})
	if errors.Is(err, amfa.ErrAllRealmsUnsupported) {
		respondAllRealmsUnsupported(w, h.log,
			"This tenant's AMFA integration doesn't support aggregating stats across all realms; select a single realm instead")
		return
	}
	if errors.Is(err, amfa.ErrAmfaNotConfigured) {
		httputil.RespondJSON(w, h.log, http.StatusNotFound, map[string]interface{}{
			"error":   "amfa_not_configured",
			"message": "AMFA Events is not configured for this tenant",
		})
		return
	}
	if errors.Is(err, amfa.ErrAmfaUnavailable) {
		httputil.RespondJSON(w, h.log, http.StatusServiceUnavailable, map[string]interface{}{
			"error": "amfa_unavailable",
		})
		return
	}
	if err != nil {
		h.log.Error("AMFA stats failed",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "AMFA stats failed")
		return
	}

	applied := ""
	if narrowed {
		applied = realm
	}
	httputil.RespondJSON(w, h.log, http.StatusOK, amfaStatsResponse{Stats: stats, AppliedRealmID: applied})
}

// amfaStatsResponse is the KPI payload. AppliedRealmID names the realm the
// query was answered for when that is narrower than the request, and is omitted
// otherwise.
type amfaStatsResponse struct {
	amfa.Stats
	AppliedRealmID string `json:"applied_realm_id,omitempty"`
}

// handleAmfaGeo handles GET /api/tenants/{tenantID}/amfa/geo.
//
// Required query params:
//   - realm_id
//
// Optional query params:
//   - start_time / end_time (RFC3339)
//
// Always returns a JSON array (never null) — the slice is initialised to an
// empty (non-nil) slice when the service returns no rows so that the JSON
// encoding is `[]` rather than `null`.
func (h *AmfaHandlers) handleAmfaGeo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")
	q := r.URL.Query()

	realm := q.Get("realm_id")
	if realm == "" {
		httputil.RespondBadRequest(w, h.log, "realm_id query param is required")
		return
	}

	if !h.checkRealmAccess(ctx, w, tenantID, realm) {
		return
	}

	start, end, err := parseTimeRange(q.Get("start_time"), q.Get("end_time"))
	if err != nil {
		httputil.RespondBadRequest(w, h.log, err.Error())
		return
	}

	buckets, err := h.service.GetGeoBuckets(ctx, tenantID, amfa.GeoOptions{
		RealmID:   realm,
		StartTime: start,
		EndTime:   end,
	})
	if errors.Is(err, amfa.ErrAmfaNotConfigured) {
		httputil.RespondJSON(w, h.log, http.StatusNotFound, map[string]interface{}{
			"error":   "amfa_not_configured",
			"message": "AMFA Events is not configured for this tenant",
		})
		return
	}
	if errors.Is(err, amfa.ErrAmfaUnavailable) {
		httputil.RespondJSON(w, h.log, http.StatusServiceUnavailable, map[string]interface{}{
			"error": "amfa_unavailable",
		})
		return
	}
	if err != nil {
		h.log.Error("AMFA geo failed",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "AMFA geo failed")
		return
	}

	if buckets == nil {
		buckets = []amfa.GeoBucket{}
	}
	httputil.RespondJSON(w, h.log, http.StatusOK, buckets)
}

// handleAmfaCheckerStatus reports whether the AMFA risk-alert checker is running
// for this tenant, so the UI can show/hide the "Reload AMFA alerts" button.
func (h *AmfaHandlers) handleAmfaCheckerStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	enabled := h.checkerRunner != nil && h.checkerRunner.Enabled(tenantID)
	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]any{"enabled": enabled})
}

// handleAmfaCheckerRun triggers an immediate AMFA checker run for this tenant.
// It runs the same checks the background ticker runs, synchronously, bounded by
// a timeout so a slow/unreachable AMFA DB cannot hang the request.
func (h *AmfaHandlers) handleAmfaCheckerRun(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if h.checkerRunner == nil {
		httputil.RespondError(w, h.log, http.StatusConflict, "amfa_checker_not_enabled")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := h.checkerRunner.RunNow(ctx, tenantID); err != nil {
		// RunNow today only returns ErrCheckerNotEnabled (RunCheckNow is
		// best-effort and logs per-check errors, like the ticker). The 503/500
		// arms are defensive: they map cleanly if RunCheckNow ever surfaces a
		// timeout or hard error in the future.
		switch {
		case errors.Is(err, amfacheck.ErrCheckerNotEnabled):
			httputil.RespondError(w, h.log, http.StatusConflict, "amfa_checker_not_enabled")
		case errors.Is(err, amfa.ErrAmfaUnavailable),
			errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			// AMFA DB unreachable (or the run timed out): report 503 so the UI
			// shows an error instead of a silent success. Any alerts that did
			// save before the failure remain and surface on the next refresh.
			httputil.RespondError(w, h.log, http.StatusServiceUnavailable, "amfa_unavailable")
		default:
			httputil.RespondError(w, h.log, http.StatusInternalServerError, "amfa_checker_run_failed")
		}
		return
	}
	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]any{"status": "ok"})
}

// checkRealmAccess verifies that the caller may read AMFA data for a realm of
// this tenant, responding with the denial itself when they may not. An empty
// realm asks for every realm, which only an unrestricted caller may do.
func (h *AmfaHandlers) checkRealmAccess(ctx context.Context, w http.ResponseWriter, tenantID, realm string) bool {
	user, scope, all, ok := h.callerRealmScope(ctx, w, tenantID, realm)
	if !ok {
		return false
	}
	if all {
		return true
	}

	if realm == "" || !slices.Contains(scope, realm) {
		h.denyRealm(w, user, tenantID, realm)
		return false
	}

	return true
}

// statsRealm resolves the realm the KPI query runs against, responding with the
// refusal itself and returning ok = false. An omitted realm asks for every realm
// at once: a caller restricted to exactly one realm is narrowed to it and
// narrowed reports that, while any other restricted scope is refused.
func (h *AmfaHandlers) statsRealm(ctx context.Context, w http.ResponseWriter, tenantID, realm string) (resolved string, narrowed, ok bool) {
	user, scope, all, ok := h.callerRealmScope(ctx, w, tenantID, realm)
	if !ok {
		return "", false, false
	}
	if all {
		return realm, false, true
	}

	if realm != "" {
		if !slices.Contains(scope, realm) {
			h.denyRealm(w, user, tenantID, realm)
			return "", false, false
		}
		return realm, false, true
	}

	if len(scope) == 1 {
		return scope[0], true, true
	}

	h.log.Warn("All-realms stats refused by realm scope",
		logger.Uint("user_id", user.ID),
		logger.Str("tenant_id", tenantID),
		logger.Int("allowed_realms", len(scope)))
	respondRealmScopeRequiresRealm(w, h.log)
	return "", false, false
}

// callerRealmScope resolves the realms the caller may read within a tenant,
// responding with the failure itself and returning ok = false. all = true means
// unrestricted, and scope is then empty rather than every realm.
func (h *AmfaHandlers) callerRealmScope(ctx context.Context, w http.ResponseWriter, tenantID, realm string) (user *UserDetails, scope []string, all, ok bool) {
	user = GetUserFromContext(ctx)
	if user == nil {
		h.log.Warn("No user info in context for realm access check")
		httputil.RespondUnauthorized(w, h.log, "Authentication required")
		return nil, nil, false, false
	}

	scope, all, err := allowedRealms(ctx, h.rbacService, user.ID, tenantID)
	if err != nil {
		h.log.Error("Failed to check realm access",
			logger.Err(err),
			logger.Uint("user_id", user.ID),
			logger.Str("tenant_id", tenantID),
			logger.Str("realm", realm))
		httputil.RespondInternalError(w, h.log, "Failed to check realm access")
		return nil, nil, false, false
	}

	return user, scope, all, true
}

func (h *AmfaHandlers) denyRealm(w http.ResponseWriter, user *UserDetails, tenantID, realm string) {
	h.log.Warn("Realm access denied",
		logger.Uint("user_id", user.ID),
		logger.Str("email", user.Email),
		logger.Str("tenant_id", tenantID),
		logger.Str("realm", realm))
	httputil.RespondForbidden(w, h.log, "Access to realm denied")
}

// respondAllRealmsUnsupported reports that this tenant's AMFA transport cannot
// aggregate across realms. A property of the integration, not of the caller's
// permissions, so it must not answer a permission refusal.
func respondAllRealmsUnsupported(w http.ResponseWriter, log *logger.Logger, message string) {
	httputil.RespondJSON(w, log, http.StatusNotImplemented, map[string]interface{}{
		"error":   "amfa_all_realms_unsupported",
		"message": message,
	})
}

// respondRealmScopeRequiresRealm refuses an all-realms request whose aggregate
// would cross the caller's realm scope. A property of the caller's permissions,
// not of the integration, so it stays distinct from amfa_all_realms_unsupported.
func respondRealmScopeRequiresRealm(w http.ResponseWriter, log *logger.Logger) {
	httputil.RespondJSON(w, log, http.StatusForbidden, map[string]interface{}{
		"error":   "amfa_realm_scope_requires_realm",
		"message": "Your access is limited to specific realms, so these metrics cannot be shown across all of them. Select one of your realms.",
	})
}

// parseTimeRange parses optional RFC3339 start/end query values into pointers.
// An empty string yields nil (meaning "param not supplied" — the repository
// applies its default window). A non-empty string that fails to parse returns
// an error so the handler can respond with 400 BadRequest rather than silently
// substituting the default and returning a different window than the user asked
// for. Also validates that start <= end when both are supplied.
func parseTimeRange(startStr, endStr string) (*time.Time, *time.Time, error) {
	var start, end *time.Time
	if startStr != "" {
		t, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid start_time: must be RFC3339 (e.g. 2026-05-01T00:00:00Z): %w", err)
		}
		start = &t
	}
	if endStr != "" {
		t, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid end_time: must be RFC3339 (e.g. 2026-05-01T00:00:00Z): %w", err)
		}
		end = &t
	}
	if start != nil && end != nil && start.After(*end) {
		return nil, nil, fmt.Errorf("invalid time range: start_time must be <= end_time")
	}
	return start, end, nil
}

// Ensure AmfaHandlers satisfies the router interfaces.
var (
	_ RouteRegistrar  = (*AmfaHandlers)(nil)
	_ TenantSubRouter = (*AmfaHandlers)(nil)
)
