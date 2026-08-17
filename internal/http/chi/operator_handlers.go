package chi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	_ "repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain" // swagger
	httputil "repos.defensepoint.com/iam/oss-pmt/kmt/internal/http"
	_ "repos.defensepoint.com/iam/oss-pmt/kmt/internal/http/dto" // swagger
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/operator"
	"repos.defensepoint.com/iam/oss-pmt/kmt/rbac"
)

// OperatorHandlers handles HTTP requests for operator metrics operations.
type OperatorHandlers struct {
	service        operator.Service
	log            *logger.Logger
	authMiddleware func(http.Handler) http.Handler
	rbacMiddleware *RBACMiddleware
}

// NewOperatorHandlers creates a new OperatorHandlers.
func NewOperatorHandlers(
	service operator.Service,
	log *logger.Logger,
	authMiddleware func(http.Handler) http.Handler,
	rbacMiddleware *RBACMiddleware,
) *OperatorHandlers {
	return &OperatorHandlers{
		service:        service,
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

	metrics, err := h.service.GetAllOperatorsMetrics(ctx, tenantID, startDate, endDate)
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

	metrics, err := h.service.GetMetrics(ctx, tenantID, operatorEmail, startDate, endDate)
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

	actions, err := h.service.GetActions(ctx, tenantID, operatorEmail, startDate, endDate, limit)
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
