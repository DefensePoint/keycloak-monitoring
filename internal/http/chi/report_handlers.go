package chi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	_ "github.com/DefensePoint/keycloak-monitoring/internal/http/dto" // swagger
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/reports"
)

// ReportTenantService provides tenant information for reports.
type ReportTenantService interface {
	GetTenantName(ctx context.Context, tenantID string) (string, error)
}

// ReportHandlers handles HTTP requests for report generation.
type ReportHandlers struct {
	service        reports.Service
	tenantService  ReportTenantService
	rbacService    RBACChecker
	log            *logger.Logger
	authMiddleware func(http.Handler) http.Handler
	rbacMiddleware *RBACMiddleware
}

// NewReportHandlers creates a new ReportHandlers.
func NewReportHandlers(
	service reports.Service,
	tenantService ReportTenantService,
	rbacService RBACChecker,
	log *logger.Logger,
	authMiddleware func(http.Handler) http.Handler,
	rbacMiddleware *RBACMiddleware,
) *ReportHandlers {
	return &ReportHandlers{
		service:        service,
		tenantService:  tenantService,
		rbacService:    rbacService,
		log:            log,
		authMiddleware: authMiddleware,
		rbacMiddleware: rbacMiddleware,
	}
}

// RegisterRoutes registers all report routes using Chi router.
// Routes are registered as part of tenant sub-routes.
func (h *ReportHandlers) RegisterRoutes(r chi.Router) {
	// Routes are registered via RegisterTenantRoutes
}

// RegisterTenantRoutes registers report routes under a tenant route group.
// Routes:
//   - GET /reports/generate - Generate PDF report
func (h *ReportHandlers) RegisterTenantRoutes(r chi.Router) {
	r.Route("/reports", func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.rbacMiddleware.RequireTenantAccess())

		r.Get("/generate", h.handleGenerateReport)
	})
}

// handleGenerateReport generates a PDF report for a tenant.
// GET /api/tenants/{tenant_id}/reports/generate?start_date=2024-01-01&end_date=2024-01-31
//
//	@Summary		Generate PDF report
//	@Description	Generates a PDF report for a tenant within a date range
//	@Tags			reports
//	@Produce		application/pdf
//	@Param			tenantID	path		string				true	"Tenant ID"
//	@Param			start_date	query		string				true	"Start date (YYYY-MM-DD)"
//	@Param			end_date	query		string				true	"End date (YYYY-MM-DD)"
//	@Success		200			{file}		binary				"PDF file"
//	@Failure		400			{object}	dto.ErrorResponse	"Missing or invalid dates"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Tenant not found"
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/reports/generate [get]
func (h *ReportHandlers) handleGenerateReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	// Get tenant name
	var tenantName string
	if h.tenantService != nil {
		name, err := h.tenantService.GetTenantName(ctx, tenantID)
		if err != nil {
			h.log.Error("Failed to get tenant", logger.Str("tenant_id", tenantID), logger.Err(err))
			httputil.RespondNotFound(w, h.log, "Tenant not found")
			return
		}
		tenantName = name
	} else {
		tenantName = tenantID
	}

	// Parse date parameters
	startDateStr := r.URL.Query().Get("start_date")
	endDateStr := r.URL.Query().Get("end_date")

	if startDateStr == "" || endDateStr == "" {
		httputil.RespondBadRequest(w, h.log, "start_date and end_date are required")
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid start_date format. Use YYYY-MM-DD")
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid end_date format. Use YYYY-MM-DD")
		return
	}

	// Validate date range
	if endDate.Before(startDate) {
		httputil.RespondBadRequest(w, h.log, "end_date must be after start_date")
		return
	}

	// Set end date to end of day
	endDate = endDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	h.log.Info("Generating report",
		logger.Str("tenant_id", tenantID),
		logger.Str("tenant_name", tenantName),
		logger.Str("start_date", startDateStr),
		logger.Str("end_date", endDateStr))

	user := GetUserFromContext(ctx)
	if user == nil {
		h.log.Warn("No user info in context for report scoping")
		httputil.RespondUnauthorized(w, h.log, "Authentication required")
		return
	}

	scope, all, err := allowedRealms(ctx, h.rbacService, user.ID, tenantID)
	if err != nil {
		h.log.Error("Failed to check realm access",
			logger.Err(err),
			logger.Uint("user_id", user.ID),
			logger.Str("tenant_id", tenantID))
		httputil.RespondInternalError(w, h.log, "Failed to check realm access")
		return
	}

	// Create report request
	req := &reports.GenerateRequest{
		TenantID:   tenantID,
		TenantName: tenantName,
		StartDate:  startDate,
		EndDate:    endDate,
		Scope:      reports.RealmScope{Realms: scope, All: all},
	}

	// Aggregate data
	data, err := h.service.AggregateData(ctx, req)
	if err != nil {
		h.log.Error("Failed to aggregate report data",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to aggregate report data")
		return
	}

	// Generate PDF
	pdfBytes, err := h.service.GeneratePDF(data)
	if err != nil {
		h.log.Error("Failed to generate PDF",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to generate PDF report")
		return
	}

	// Set response headers
	filename := "kmt-report-" + tenantName + "-" + startDateStr + "-to-" + endDateStr + ".pdf"
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")

	// Write PDF to response
	_, err = w.Write(pdfBytes)
	if err != nil {
		h.log.Error("Failed to write PDF response",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		return
	}

	h.log.Info("Report generated successfully",
		logger.Str("tenant_id", tenantID),
		logger.Str("tenant_name", tenantName),
		logger.Int("pdf_size_bytes", len(pdfBytes)))
}

// CanHandle returns true if this handler can handle reports routes (for TenantSubRouter compatibility).
func (h *ReportHandlers) CanHandle(resourcePath string) bool {
	return len(resourcePath) >= 7 && resourcePath[:7] == "reports"
}

// Ensure ReportHandlers implements TenantSubRouter
var _ TenantSubRouter = (*ReportHandlers)(nil)
