package chi

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	_ "github.com/DefensePoint/keycloak-monitoring/internal/http/dto" // swagger
	"github.com/DefensePoint/keycloak-monitoring/internal/http/dto/requests"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
)

// AlertsOperatorService defines the interface for operator metrics.
type AlertsOperatorService interface {
	RecordAction(ctx context.Context, action *domain.OperatorAction) error
	GetLastActionForAlert(ctx context.Context, tenantID string, alertID uint) (*domain.OperatorAction, error)
}

// AlertsNotificationService defines the interface for notifications.
type AlertsNotificationService interface {
	NotifyAlertResolution(ctx context.Context, alert *domain.Alert) error
}

// AlertsHandlers handles HTTP requests for alert operations.
type AlertsHandlers struct {
	service             alerts.Service
	rbacService         RBACChecker
	operatorService     AlertsOperatorService
	notificationService AlertsNotificationService
	log                 *logger.Logger
	authMiddleware      func(http.Handler) http.Handler
	rbacMiddleware      *RBACMiddleware
}

// NewAlertsHandlers creates a new AlertsHandlers.
func NewAlertsHandlers(
	service alerts.Service,
	rbacService RBACChecker,
	operatorService AlertsOperatorService,
	notificationService AlertsNotificationService,
	log *logger.Logger,
	authMiddleware func(http.Handler) http.Handler,
	rbacMiddleware *RBACMiddleware,
) *AlertsHandlers {
	return &AlertsHandlers{
		service:             service,
		rbacService:         rbacService,
		operatorService:     operatorService,
		notificationService: notificationService,
		log:                 log,
		authMiddleware:      authMiddleware,
		rbacMiddleware:      rbacMiddleware,
	}
}

// recordOperatorAction records an operator action for metrics tracking with transition times.
func (h *AlertsHandlers) recordOperatorAction(ctx context.Context, alert *domain.Alert, actionType string, operatorEmail string, operatorID uint, comment string) {
	if h.operatorService == nil {
		return // Metrics tracking not enabled
	}

	now := time.Now()

	// Calculate time from detection to this action
	timeFromDetection := int(now.Sub(alert.FirstDetected).Seconds())

	// Get the previous action for this alert to calculate transition time
	lastAction, err := h.operatorService.GetLastActionForAlert(ctx, alert.TenantID, alert.ID)
	var previousStatus string
	var previousActionTime *time.Time
	var timeFromPreviousAction int

	if err == nil && lastAction != nil {
		previousActionTime = &lastAction.ActionTime
		timeFromPreviousAction = int(now.Sub(lastAction.ActionTime).Seconds())

		// Determine previous status based on last action type
		switch lastAction.ActionType {
		case "acknowledged":
			previousStatus = "acknowledged"
		case "resolved":
			previousStatus = "resolved"
		case "ignored":
			previousStatus = "ignored"
		default:
			previousStatus = "active"
		}
	} else {
		// No previous action, alert was in active state
		previousStatus = "active"
		timeFromPreviousAction = timeFromDetection
	}

	action := &domain.OperatorAction{
		TenantID:                      alert.TenantID,
		AlertID:                       alert.ID,
		OperatorID:                    operatorID,
		OperatorEmail:                 operatorEmail,
		ActionType:                    actionType,
		ActionTime:                    now,
		PreviousStatus:                previousStatus,
		PreviousActionTime:            previousActionTime,
		AlertSeverity:                 string(alert.Severity),
		AlertType:                     string(alert.Type),
		RealmName:                     alert.RealmName,
		TimeFromDetectionSeconds:      timeFromDetection,
		TimeFromPreviousActionSeconds: timeFromPreviousAction,
		Comment:                       comment,

		// Legacy fields for backwards compatibility
		ResponseTimeSeconds:   timeFromDetection,
		ResolutionTimeSeconds: timeFromDetection,
	}

	// Record asynchronously to not block the main request
	go func() {
		if err := h.operatorService.RecordAction(context.Background(), action); err != nil {
			h.log.Error("Failed to record operator action for metrics",
				logger.Err(err),
				logger.Uint("alert_id", alert.ID),
				logger.Str("operator_email", operatorEmail),
				logger.Str("action_type", actionType))
		}
	}()
}

// RegisterRoutes registers all alerts routes using Chi router.
// Routes are registered as part of tenant sub-routes.
func (h *AlertsHandlers) RegisterRoutes(r chi.Router) {
	// Routes are registered via RegisterTenantRoutes
}

// RegisterTenantRoutes registers alerts routes under a tenant route group.
// Routes:
//   - GET    /alerts                  - List alerts
//   - GET    /alerts/stats            - Alert statistics
//   - GET    /alerts/by-realm         - Alerts by realm
//   - GET    /alerts/get              - Get alert (query param)
//   - GET    /alerts/{alertID}        - Get alert
//   - PUT    /alerts/{alertID}/status - Update alert status
//   - POST   /alerts/{alertID}/resolve - Resolve alert
//   - POST   /alerts/update-status    - Update status (query param)
//   - POST   /alerts/resolve          - Resolve (query param)
//   - DELETE /alerts/delete           - Delete alert (query param)
func (h *AlertsHandlers) RegisterTenantRoutes(r chi.Router) {
	r.Route("/alerts", func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.rbacMiddleware.RequireTenantAccess())

		r.Get("/", h.handleListAlerts)
		r.Get("/stats", h.handleAlertStats)
		r.Get("/by-realm", h.handleListAlertsByRealm)
		r.Get("/get", h.handleGetAlert)
		r.Post("/update-status", h.handleUpdateAlertStatus)
		r.Post("/resolve", h.handleResolveAlert)
		r.Delete("/delete", h.handleDeleteAlert)

		r.Route("/{alertID}", func(r chi.Router) {
			r.Get("/", h.handleGetAlertByID)
			r.Put("/status", h.handleUpdateAlertStatusByID)
			r.Post("/resolve", h.handleResolveAlertByID)
		})
	})
}

// handleListAlerts handles GET /api/tenants/{tenant_id}/alerts
//
//	@Summary		List alerts
//	@Description	Returns a list of alerts for a tenant with optional filtering
//	@Tags			alerts
//	@Produce		json
//	@Param			tenantID		path		string	true	"Tenant ID"
//	@Param			limit			query		int		false	"Limit results (default 100)"
//	@Param			offset			query		int		false	"Offset for pagination (default 0)"
//	@Param			severity		query		string	false	"Filter by severity (critical, high, medium, low)"
//	@Param			status			query		string	false	"Filter by status (active, resolved, acknowledged, ignored)"
//	@Param			type			query		string	false	"Filter by alert type"
//	@Param			realm			query		string	false	"Filter by realm name"
//	@Param			resource_type	query		string	false	"Filter by resource type"
//	@Success		200				{object}	object{alerts=[]domain.Alert,count=int,total=int,limit=int,offset=int}
//	@Failure		401				{object}	dto.ErrorResponse
//	@Failure		403				{object}	dto.ErrorResponse
//	@Failure		500				{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/alerts [get]
func (h *AlertsHandlers) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	if !h.checkAlertPermission(w, r, tenantID, rbac.PermissionAlertsRead) {
		return
	}

	limit := parseIntParam(r, "limit", 100)
	if limit < 1 {
		limit = 1
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	offset := parseIntParam(r, "offset", 0)
	severity := r.URL.Query().Get("severity")
	status := r.URL.Query().Get("status")
	alertType := r.URL.Query().Get("type")
	realmName := r.URL.Query().Get("realm")
	resourceType := r.URL.Query().Get("resource_type")

	if limit > MaxLimit {
		limit = MaxLimit
	}

	opts := &alerts.ListOptions{
		Limit:        limit,
		Offset:       offset,
		Severity:     domain.AlertSeverity(severity),
		Status:       domain.AlertStatus(status),
		Type:         domain.AlertType(alertType),
		RealmName:    realmName,
		ResourceType: resourceType,
	}

	caller, scope, all, isAdmin, ok := h.callerRealmScope(ctx, w, tenantID)
	if !ok {
		return
	}
	if !all {
		switch {
		case realmName != "":
			if !slices.Contains(scope, realmName) {
				h.denyRealm(w, caller, tenantID, realmName)
				return
			}
		case len(scope) == 0:
			httputil.RespondSuccess(w, h.log, map[string]interface{}{
				"alerts": []*domain.Alert{},
				"count":  0,
				"total":  int64(0),
				"limit":  limit,
				"offset": offset,
			})
			return
		default:
			opts.RealmNames = scope
		}
	}

	alertList, totalCount, err := h.service.ListAlerts(ctx, tenantID, opts)
	if err != nil {
		h.log.Error("Failed to list alerts", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve alerts")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"alerts": h.sanitizeAlerts(alertList, isAdmin),
		"count":  len(alertList),
		"total":  totalCount,
		"limit":  limit,
		"offset": offset,
	})
}

// handleGetAlert handles GET /api/tenants/{tenant_id}/alerts/get?id={id}
//
//	@Summary		Get alert by query param
//	@Description	Returns a single alert by its ID (passed as query parameter)
//	@Tags			alerts
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			id			query		string	true	"Alert ID"
//	@Success		200			{object}	domain.Alert
//	@Failure		400			{object}	dto.ErrorResponse	"Missing alert_id"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Alert not found"
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/alerts/get [get]
func (h *AlertsHandlers) handleGetAlert(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")

	alertID := r.URL.Query().Get("id")
	if alertID == "" {
		httputil.RespondBadRequest(w, h.log, "alert_id is required")
		return
	}

	h.getAlertByID(w, r, tenantID, alertID)
}

// handleGetAlertByID handles GET /api/tenants/{tenant_id}/alerts/{alertId}
//
//	@Summary		Get alert by ID
//	@Description	Returns a single alert by its ID
//	@Tags			alerts
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			alertID		path		string	true	"Alert ID"
//	@Success		200			{object}	domain.Alert
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Alert not found"
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/alerts/{alertID} [get]
func (h *AlertsHandlers) handleGetAlertByID(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	alertID := chi.URLParam(r, "alertID")

	h.getAlertByID(w, r, tenantID, alertID)
}

// getAlertByID is the shared implementation for getting an alert by ID
func (h *AlertsHandlers) getAlertByID(w http.ResponseWriter, r *http.Request, tenantID, alertID string) {
	if !h.checkAlertPermission(w, r, tenantID, rbac.PermissionAlertsRead) {
		return
	}

	alert, isAdmin, ok := h.alertInScope(w, r, tenantID, alertID)
	if !ok {
		return
	}

	httputil.RespondSuccess(w, h.log, h.sanitizeAlert(alert, isAdmin))
}

// alertInScope loads an alert and refuses it when its realm falls outside the
// caller's, responding itself and returning ok = false. An out-of-scope alert
// answers exactly as a missing one, so the route is not an oracle for the alert
// IDs in realms the caller cannot see.
func (h *AlertsHandlers) alertInScope(w http.ResponseWriter, r *http.Request, tenantID, alertID string) (alert *domain.Alert, isAdmin, ok bool) {
	ctx := r.Context()

	caller, scope, all, isAdmin, ok := h.callerRealmScope(ctx, w, tenantID)
	if !ok {
		return nil, false, false
	}

	alert, err := h.service.GetAlert(ctx, tenantID, alertID)
	if err != nil || alert == nil {
		h.log.Error("Failed to get alert", logger.Str("alert_id", alertID), logger.Err(err))
		httputil.RespondNotFound(w, h.log, "Alert not found")
		return nil, false, false
	}

	if !all && !slices.Contains(scope, alert.RealmName) {
		h.log.Warn("Alert outside the caller's realms",
			logger.Uint("user_id", caller.ID),
			logger.Str("email", caller.Email),
			logger.Str("tenant_id", tenantID),
			logger.Str("alert_id", alertID),
			logger.Str("realm", alert.RealmName))
		httputil.RespondNotFound(w, h.log, "Alert not found")
		return nil, false, false
	}

	return alert, isAdmin, true
}

// handleUpdateAlertStatus handles POST /api/tenants/{tenant_id}/alerts/update-status?id={id}
//
//	@Summary		Update alert status by query param
//	@Description	Updates the status of an alert (passed as query parameter)
//	@Tags			alerts
//	@Accept			json
//	@Produce		json
//	@Param			tenantID	path		string						true	"Tenant ID"
//	@Param			id			query		string						true	"Alert ID"
//	@Param			request		body		requests.UpdateAlertStatus	true	"Status update request"
//	@Success		200			{object}	domain.Alert
//	@Failure		400			{object}	dto.ErrorResponse	"Missing alert_id or invalid status"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/alerts/update-status [post]
func (h *AlertsHandlers) handleUpdateAlertStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")

	alertID := r.URL.Query().Get("id")
	if alertID == "" {
		httputil.RespondBadRequest(w, h.log, "alert_id is required")
		return
	}

	h.updateAlertStatusByID(w, r, tenantID, alertID)
}

// handleUpdateAlertStatusByID handles PUT /api/tenants/{tenant_id}/alerts/{alertId}/status
//
//	@Summary		Update alert status by ID
//	@Description	Updates the status of an alert by its ID
//	@Tags			alerts
//	@Accept			json
//	@Produce		json
//	@Param			tenantID	path		string						true	"Tenant ID"
//	@Param			alertID		path		string						true	"Alert ID"
//	@Param			request		body		requests.UpdateAlertStatus	true	"Status update request"
//	@Success		200			{object}	domain.Alert
//	@Failure		400			{object}	dto.ErrorResponse	"Invalid status"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/alerts/{alertID}/status [put]
func (h *AlertsHandlers) handleUpdateAlertStatusByID(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	alertID := chi.URLParam(r, "alertID")

	h.updateAlertStatusByID(w, r, tenantID, alertID)
}

// updateAlertStatusByID is the shared implementation for updating alert status
func (h *AlertsHandlers) updateAlertStatusByID(w http.ResponseWriter, r *http.Request, tenantID, alertID string) {
	if !h.checkAlertPermission(w, r, tenantID, rbac.PermissionAlertsAcknowledge) {
		return
	}

	ctx := r.Context()

	_, isAdmin, ok := h.alertInScope(w, r, tenantID, alertID)
	if !ok {
		return
	}

	req, ok := httputil.ValidateAndParse[requests.UpdateAlertStatus](w, r, h.log)
	if !ok {
		return
	}

	status := domain.AlertStatus(req.Status)

	updatedAlert, err := h.service.UpdateStatus(ctx, tenantID, alertID, status, req.AcknowledgedBy)
	if err != nil {
		h.log.Error("Failed to update alert status",
			logger.Str("alert_id", alertID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to update alert status")
		return
	}

	// Record operator action for metrics
	user := GetUserFromContext(ctx)
	if user != nil && updatedAlert != nil {
		actionType := "status_changed"
		switch status {
		case domain.AlertStatusAcknowled:
			actionType = "acknowledged"
		case domain.AlertStatusIgnored:
			actionType = "ignored"
		}
		h.recordOperatorAction(ctx, updatedAlert, actionType, user.Email, user.ID, "")
	}

	httputil.RespondSuccess(w, h.log, h.sanitizeAlert(updatedAlert, isAdmin))
}

// handleResolveAlert handles POST /api/tenants/{tenant_id}/alerts/resolve?id={id}
//
//	@Summary		Resolve alert by query param
//	@Description	Resolves an alert (passed as query parameter)
//	@Tags			alerts
//	@Produce		json
//	@Param			tenantID	path		string				true	"Tenant ID"
//	@Param			id			query		string				true	"Alert ID"
//	@Success		200			{object}	domain.Alert		"Resolved alert"
//	@Failure		400			{object}	dto.ErrorResponse	"Missing alert_id"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Alert not found"
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/alerts/resolve [post]
func (h *AlertsHandlers) handleResolveAlert(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")

	alertID := r.URL.Query().Get("id")
	if alertID == "" {
		httputil.RespondBadRequest(w, h.log, "alert_id is required")
		return
	}

	h.resolveAlertByID(w, r, tenantID, alertID)
}

// handleResolveAlertByID handles POST /api/tenants/{tenant_id}/alerts/{alertId}/resolve
//
//	@Summary		Resolve alert by ID
//	@Description	Resolves an alert by its ID
//	@Tags			alerts
//	@Produce		json
//	@Param			tenantID	path		string			true	"Tenant ID"
//	@Param			alertID		path		string			true	"Alert ID"
//	@Success		200			{object}	domain.Alert	"Resolved alert"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Alert not found"
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/alerts/{alertID}/resolve [post]
func (h *AlertsHandlers) handleResolveAlertByID(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	alertID := chi.URLParam(r, "alertID")

	h.resolveAlertByID(w, r, tenantID, alertID)
}

// resolveAlertByID is the shared implementation for resolving an alert
func (h *AlertsHandlers) resolveAlertByID(w http.ResponseWriter, r *http.Request, tenantID, alertID string) {
	if !h.checkAlertPermission(w, r, tenantID, rbac.PermissionAlertsResolve) {
		return
	}

	ctx := r.Context()

	// Get the alert before resolving to send notifications
	alertBefore, isAdmin, ok := h.alertInScope(w, r, tenantID, alertID)
	if !ok {
		return
	}

	resolvedAlert, err := h.service.ResolveAlert(ctx, tenantID, alertID)
	if err != nil {
		h.log.Error("Failed to resolve alert",
			logger.Str("alert_id", alertID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to resolve alert")
		return
	}

	// Record operator action for metrics
	user := GetUserFromContext(ctx)
	if user != nil && resolvedAlert != nil {
		h.recordOperatorAction(ctx, resolvedAlert, "resolved", user.Email, user.ID, "")
	}

	// Send resolution notifications asynchronously
	if h.notificationService != nil && alertBefore.Status != domain.AlertStatusResolved {
		go func() {
			notifAlert := &domain.Alert{
				AlertID:        resolvedAlert.AlertID,
				TenantID:       resolvedAlert.TenantID,
				Type:           resolvedAlert.Type,
				Severity:       resolvedAlert.Severity,
				Status:         resolvedAlert.Status,
				Title:          resolvedAlert.Title,
				Description:    resolvedAlert.Description,
				Source:         resolvedAlert.Source,
				ResourceType:   resolvedAlert.ResourceType,
				ResourceID:     resolvedAlert.ResourceID,
				ResourceName:   resolvedAlert.ResourceName,
				Recommendation: resolvedAlert.Recommendation,
				RealmName:      resolvedAlert.RealmName,
				FirstDetected:  resolvedAlert.FirstDetected,
				LastSeen:       resolvedAlert.LastSeen,
				ResolvedAt:     resolvedAlert.ResolvedAt,
			}

			if err := h.notificationService.NotifyAlertResolution(context.Background(), notifAlert); err != nil {
				h.log.Error("Failed to send resolution notifications",
					logger.Str("alert_id", alertID),
					logger.Err(err))
			}
		}()
	}

	httputil.RespondSuccess(w, h.log, h.sanitizeAlert(resolvedAlert, isAdmin))
}

// handleDeleteAlert handles DELETE /api/tenants/{tenant_id}/alerts/delete?id={id}
//
//	@Summary		Delete alert
//	@Description	Deletes an alert by its ID (passed as query parameter)
//	@Tags			alerts
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			id			query		string	true	"Alert ID"
//	@Success		200			{object}	object{status=string}
//	@Failure		400			{object}	dto.ErrorResponse	"Missing alert_id"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/alerts/delete [delete]
func (h *AlertsHandlers) handleDeleteAlert(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")

	if !h.checkAlertPermission(w, r, tenantID, rbac.PermissionAlertsDelete) {
		return
	}

	alertID := r.URL.Query().Get("id")
	if alertID == "" {
		httputil.RespondBadRequest(w, h.log, "alert_id is required")
		return
	}

	ctx := r.Context()
	if _, _, ok := h.alertInScope(w, r, tenantID, alertID); !ok {
		return
	}

	if err := h.service.DeleteAlert(ctx, tenantID, alertID); err != nil {
		h.log.Error("Failed to delete alert",
			logger.Str("alert_id", alertID),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to delete alert")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]string{"status": "deleted"})
}

// handleAlertStats handles GET /api/tenants/{tenant_id}/alerts/stats
//
//	@Summary		Get alert statistics
//	@Description	Returns alert statistics for a tenant, optionally filtered by realm
//	@Tags			alerts
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realm_name	query		string	false	"Filter by realm name"
//	@Success		200			{object}	alerts.Statistics
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/alerts/stats [get]
func (h *AlertsHandlers) handleAlertStats(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")

	if !h.checkAlertPermission(w, r, tenantID, rbac.PermissionAlertsRead) {
		return
	}

	ctx := r.Context()
	realmName := r.URL.Query().Get("realm_name")

	caller, scope, all, _, ok := h.callerRealmScope(ctx, w, tenantID)
	if !ok {
		return
	}

	var realms []string
	switch {
	case realmName != "":
		if !all && !slices.Contains(scope, realmName) {
			h.denyRealm(w, caller, tenantID, realmName)
			return
		}
		realms = []string{realmName}
	case !all:
		// An empty scope must not reach the service: no realm argument counts
		// every realm of the tenant.
		if len(scope) == 0 {
			httputil.RespondSuccess(w, h.log, &alerts.Statistics{
				BySeverity: map[string]int{},
				ByType:     map[string]int{},
			})
			return
		}
		realms = scope
	}

	stats, err := h.service.GetStatistics(ctx, tenantID, realms...)
	if err != nil {
		h.log.Error("Failed to get alert statistics", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve alert statistics")
		return
	}

	httputil.RespondSuccess(w, h.log, stats)
}

// handleListAlertsByRealm handles GET /api/tenants/{tenant_id}/alerts/by-realm
//
//	@Summary		List alerts by realm
//	@Description	Returns alerts filtered by realm name
//	@Tags			alerts
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Param			realm		query		string	true	"Realm name"
//	@Param			limit		query		int		false	"Limit results (default 100)"
//	@Param			offset		query		int		false	"Offset for pagination (default 0)"
//	@Success		200			{object}	object{realm=string,alerts=[]domain.Alert,count=int}
//	@Failure		400			{object}	dto.ErrorResponse	"Missing realm"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/alerts/by-realm [get]
func (h *AlertsHandlers) handleListAlertsByRealm(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")

	if !h.checkAlertPermission(w, r, tenantID, rbac.PermissionAlertsRead) {
		return
	}

	realmName := r.URL.Query().Get("realm")
	if realmName == "" {
		httputil.RespondBadRequest(w, h.log, "realm is required")
		return
	}

	ctx := r.Context()

	caller, scope, all, isAdmin, ok := h.callerRealmScope(ctx, w, tenantID)
	if !ok {
		return
	}
	if !all && !slices.Contains(scope, realmName) {
		h.denyRealm(w, caller, tenantID, realmName)
		return
	}

	limit := parseIntParam(r, "limit", 100)
	offset := parseIntParam(r, "offset", 0)

	if limit < 1 {
		limit = 1
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	opts := &alerts.ListOptions{
		Limit:     limit,
		Offset:    offset,
		RealmName: realmName,
		Status:    domain.AlertStatusActive,
	}

	alertList, err := h.service.ListAlertsByRealm(ctx, tenantID, realmName, opts)
	if err != nil {
		h.log.Error("Failed to list realm alerts",
			logger.Str("realm", realmName),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve realm alerts")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"realm":  realmName,
		"alerts": h.sanitizeAlerts(alertList, isAdmin),
		"count":  len(alertList),
	})
}

// callerRealmScope resolves the realms the caller may read within a tenant,
// responding with the failure itself and returning ok = false. all = true means
// unrestricted, and scope is then empty rather than every realm. isAdmin is
// narrower than all: an unrestricted policy is not administrator rights.
func (h *AlertsHandlers) callerRealmScope(ctx context.Context, w http.ResponseWriter, tenantID string) (user *UserDetails, scope []string, all, isAdmin, ok bool) {
	user = GetUserFromContext(ctx)
	if user == nil {
		h.log.Warn("No user context found, denying access")
		httputil.RespondUnauthorized(w, h.log, "Authentication required")
		return nil, nil, false, false, false
	}

	scope, all, isAdmin, err := allowedRealmsWithAdmin(ctx, h.rbacService, user.ID, tenantID)
	if err != nil {
		h.log.Error("Failed to check realm access",
			logger.Err(err),
			logger.Uint("user_id", user.ID),
			logger.Str("tenant_id", tenantID))
		httputil.RespondInternalError(w, h.log, "Failed to check realm access")
		return nil, nil, false, false, false
	}

	return user, scope, all, isAdmin, true
}

func (h *AlertsHandlers) denyRealm(w http.ResponseWriter, user *UserDetails, tenantID, realmName string) {
	h.log.Warn("Realm access denied",
		logger.Uint("user_id", user.ID),
		logger.Str("email", user.Email),
		logger.Str("tenant_id", tenantID),
		logger.Str("realm", realmName))
	httputil.RespondForbidden(w, h.log, "Access to realm denied")
}

// sanitizeAlert removes sensitive internal fields from an alert for non-admin users.
func (h *AlertsHandlers) sanitizeAlert(alert *domain.Alert, isAdmin bool) *domain.Alert {
	if isAdmin {
		return alert
	}
	return alerts.SanitizeAlert(alert)
}

// sanitizeAlerts removes sensitive internal fields from alerts for non-admin users.
func (h *AlertsHandlers) sanitizeAlerts(alertList []*domain.Alert, isAdmin bool) []*domain.Alert {
	if isAdmin {
		return alertList
	}
	return alerts.SanitizeAlerts(alertList)
}

// checkAlertPermission checks if the user has the specified alert permission.
// Implements fail-closed security: denies access if RBAC or user context is unavailable.
func (h *AlertsHandlers) checkAlertPermission(w http.ResponseWriter, r *http.Request, tenantID, permission string) bool {
	if h.rbacService == nil {
		h.log.Warn("RBAC service not configured, denying access for security")
		httputil.RespondForbidden(w, h.log, "Authorization service not available")
		return false
	}

	user := GetUserFromContext(r.Context())
	if user == nil {
		h.log.Warn("No user context found, denying access")
		httputil.RespondUnauthorized(w, h.log, "Authentication required")
		return false
	}

	hasPermission, err := h.rbacService.HasPermission(r.Context(), user.ID, permission, &tenantID)
	if err != nil {
		h.log.Error("Failed to check alert permission",
			logger.Str("permission", permission),
			logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to check permissions")
		return false
	}

	if !hasPermission {
		h.log.Warn("Alert permission denied",
			logger.Uint("user_id", user.ID),
			logger.Str("permission", permission),
			logger.Str("tenant_id", tenantID))
		httputil.RespondForbidden(w, h.log, "Permission denied: "+permission+" required")
		return false
	}

	return true
}

// CanHandle returns true if this handler can handle alerts routes (for TenantSubRouter compatibility).
func (h *AlertsHandlers) CanHandle(resourcePath string) bool {
	return len(resourcePath) >= 6 && resourcePath[:6] == "alerts"
}

// Ensure AlertsHandlers implements TenantSubRouter
var _ TenantSubRouter = (*AlertsHandlers)(nil)
