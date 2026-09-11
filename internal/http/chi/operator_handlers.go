package chi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	_ "github.com/DefensePoint/keycloak-monitoring/internal/http/dto" // swagger
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/operator"
	"github.com/DefensePoint/keycloak-monitoring/rbac"
)

// OperatorHandlers handles HTTP requests for operator metrics operations.
type OperatorHandlers struct {
	service        operator.Service
	rbacService    RBACChecker
	log            *logger.Logger
	authMiddleware func(http.Handler) http.Handler
	rbacMiddleware *RBACMiddleware
}

// NewOperatorHandlers creates a new OperatorHandlers.
func NewOperatorHandlers(
	service operator.Service,
	rbacService RBACChecker,
	log *logger.Logger,
	authMiddleware func(http.Handler) http.Handler,
	rbacMiddleware *RBACMiddleware,
) *OperatorHandlers {
	return &OperatorHandlers{
		service:        service,
		rbacService:    rbacService,
		log:            log,
		authMiddleware: authMiddleware,
		rbacMiddleware: rbacMiddleware,
	}
}

// RegisterRoutes registers all operator routes using Chi router.
// Routes are registered as part of tenant sub-routes.
func (h *OperatorHandlers) RegisterRoutes(r chi.Router) {
	// Routes are registered via RegisterTenantRoutes
}

// RegisterTenantRoutes registers operator metrics routes under a tenant route group.
// Routes:
//   - GET /metrics/operators         - Get all operators metrics
//   - GET /metrics/operators/details - Get specific operator metrics
//   - GET /metrics/actions           - Get operator actions
func (h *OperatorHandlers) RegisterTenantRoutes(r chi.Router) {
	r.Route("/metrics", func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.rbacMiddleware.RequireTenantAccess())
		r.Use(h.rbacMiddleware.RequirePermission(rbac.PermissionMetricsRead))

		r.Get("/operators", h.handleGetAllOperatorsMetrics)
		r.Get("/operators/details", h.handleGetOperatorMetrics)
		r.Get("/actions", h.handleGetOperatorActions)
	})
}

// handleGetAllOperatorsMetrics handles GET /api/tenants/{tenant_id}/metrics/operators
//
//	@Summary		Get all operators metrics
//	@Description	Returns metrics for all operators in a tenant
//	@Tags			operators
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			start_date	query		string	false	"Start date (YYYY-MM-DD)"
//	@Param			end_date	query		string	false	"End date (YYYY-MM-DD)"
//	@Success		200			{array}		domain.OperatorMetricsSummary
//	@Failure		400			{object}	dto.ErrorResponse	"Invalid date range"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/metrics/operators [get]
func (h *OperatorHandlers) handleGetAllOperatorsMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	startDate, endDate, err := h.parseDateRange(r)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid date range: "+err.Error())
		return
	}

	realmNames, hasScope, ok := h.realmScope(ctx, w, tenantID)
	if !ok {
		return
	}
	if !hasScope {
		httputil.RespondSuccess(w, h.log, []*domain.OperatorMetricsSummary{})
		return
	}

	metrics, err := h.service.GetAllOperatorsMetrics(ctx, tenantID, startDate, endDate, realmNames)
	if err != nil {
		h.log.Error("Failed to get all operators metrics",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve operators metrics")
		return
	}

	httputil.RespondSuccess(w, h.log, metrics)
}

// handleGetOperatorMetrics handles GET /api/tenants/{tenant_id}/metrics/operators/details?email=...
//
//	@Summary		Get specific operator metrics
//	@Description	Returns metrics for a specific operator by email
//	@Tags			operators
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			email		query		string	true	"Operator email"
//	@Param			start_date	query		string	false	"Start date (YYYY-MM-DD)"
//	@Param			end_date	query		string	false	"End date (YYYY-MM-DD)"
//	@Success		200			{object}	domain.OperatorMetricsSummary
//	@Failure		400			{object}	dto.ErrorResponse	"Missing email or invalid date range"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/metrics/operators/details [get]
func (h *OperatorHandlers) handleGetOperatorMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	operatorEmail := r.URL.Query().Get("email")
	if operatorEmail == "" {
		httputil.RespondBadRequest(w, h.log, "operator email is required")
		return
	}

	startDate, endDate, err := h.parseDateRange(r)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid date range: "+err.Error())
		return
	}

	realmNames, hasScope, ok := h.realmScope(ctx, w, tenantID)
	if !ok {
		return
	}
	if !hasScope {
		httputil.RespondSuccess(w, h.log, emptyOperatorSummary(tenantID, operatorEmail, startDate, endDate))
		return
	}

	metrics, err := h.service.GetMetrics(ctx, tenantID, operatorEmail, startDate, endDate, realmNames)
	if err != nil {
		h.log.Error("Failed to get operator metrics",
			logger.Str("operator_email", operatorEmail),
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve operator metrics")
		return
	}

	httputil.RespondSuccess(w, h.log, metrics)
}

// emptyOperatorSummary is the summary a caller allowed no realm gets back: the
// period they asked for and no numbers from it.
func emptyOperatorSummary(tenantID, operatorEmail string, startDate, endDate time.Time) *domain.OperatorMetricsSummary {
	return &domain.OperatorMetricsSummary{
		TenantID:      tenantID,
		OperatorEmail: operatorEmail,
		PeriodStart:   startDate,
		PeriodEnd:     endDate,
		AlertsByType:  map[string]int{},
	}
}

// handleGetOperatorActions handles GET /api/tenants/{tenant_id}/metrics/actions?operator_email=...
//
//	@Summary		Get operator actions
//	@Description	Returns a list of operator actions with optional filtering
//	@Tags			operators
//	@Produce		json
//	@Param			tenantID		path		string	true	"Tenant ID"
//	@Param			operator_email	query		string	false	"Filter by operator email"
//	@Param			limit			query		int		false	"Limit results (default 100, max 1000)"
//	@Param			start_date		query		string	false	"Start date (YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"End date (YYYY-MM-DD)"
//	@Success		200				{array}		domain.OperatorAction
//	@Failure		400				{object}	dto.ErrorResponse	"Invalid date range"
//	@Failure		401				{object}	dto.ErrorResponse
//	@Failure		403				{object}	dto.ErrorResponse
//	@Failure		500				{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/metrics/actions [get]
func (h *OperatorHandlers) handleGetOperatorActions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	operatorEmail := r.URL.Query().Get("operator_email")
	limit := parseIntParam(r, "limit", 100)
	if limit < 1 {
		limit = 1
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	startDate, endDate, err := h.parseDateRange(r)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid date range: "+err.Error())
		return
	}

	realmNames, hasScope, ok := h.realmScope(ctx, w, tenantID)
	if !ok {
		return
	}
	if !hasScope {
		httputil.RespondSuccess(w, h.log, []*domain.OperatorAction{})
		return
	}

	actions, err := h.service.GetActions(ctx, tenantID, operatorEmail, startDate, endDate, limit, realmNames)
	if err != nil {
		h.log.Error("Failed to get operator actions",
			logger.Str("operator_email", operatorEmail),
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve operator actions")
		return
	}

	httputil.RespondSuccess(w, h.log, actions)
}

// realmScope resolves the realm filter every operator query runs with,
// responding with the failure itself and returning ok = false. hasScope = false
// means the caller may read no realm and the query must be skipped, since an
// empty realm filter applies no filter at all. A nil realmNames with
// hasScope = true is an unrestricted caller.
func (h *OperatorHandlers) realmScope(ctx context.Context, w http.ResponseWriter, tenantID string) (realmNames []string, hasScope, ok bool) {
	user := GetUserFromContext(ctx)
	if user == nil {
		h.log.Warn("No user info in context for operator metrics scoping")
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

	if all {
		return nil, true, true
	}
	return scope, len(scope) > 0, true
}

// parseDateRange parses start_date and end_date query parameters.
// If not provided, defaults to the last 30 days.
// Returns error if provided dates are invalid.
func (h *OperatorHandlers) parseDateRange(r *http.Request) (time.Time, time.Time, error) {
	startDateStr := r.URL.Query().Get("start_date")
	endDateStr := r.URL.Query().Get("end_date")

	var startDate, endDate time.Time
	var err error

	if startDateStr != "" {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	} else {
		startDate = time.Now().AddDate(0, 0, -30)
	}

	if endDateStr != "" {
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		// Set to end of day
		endDate = endDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	} else {
		endDate = time.Now()
	}

	return startDate, endDate, nil
}

// CanHandle returns true if this handler can handle metrics routes (for TenantSubRouter compatibility).
func (h *OperatorHandlers) CanHandle(resourcePath string) bool {
	return len(resourcePath) >= 7 && resourcePath[:7] == "metrics"
}

// Ensure OperatorHandlers implements TenantSubRouter
var _ TenantSubRouter = (*OperatorHandlers)(nil)
