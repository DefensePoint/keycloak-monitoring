package chi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/http/dto"
	"github.com/DefensePoint/keycloak-monitoring/internal/http/dto/requests"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

var _ = dto.ErrorResponse{}     // ensure dto import for swagger
var _ = domain.KeycloakTenant{} // ensure domain import for swagger

// KeycloakConnectionTester defines the interface for testing Keycloak connections.
type KeycloakConnectionTester interface {
	TestConnection(ctx context.Context, serverURL, adminRealm, clientID, clientSecret string) error

	// ValidateServerURL applies the SSRF policy to a server URL without
	// attempting authentication. Returns nil if the URL is acceptable;
	// callers should treat any non-nil error as a generic validation
	// failure and avoid echoing the underlying reason to clients.
	ValidateServerURL(ctx context.Context, serverURL string) error
}

// ClientCreationError represents an error during Keycloak client creation.
// This error type signals that the client could not be created (e.g., invalid URL, config).
type ClientCreationError interface {
	error
	IsClientCreationError() bool
}

// TenantHandlers handles tenant management HTTP requests.
type TenantHandlers struct {
	service          tenant.Service
	rbacService      RBACChecker
	connectionTester KeycloakConnectionTester
	log              *logger.Logger
	authMiddleware   func(http.Handler) http.Handler
	rbacMiddleware   *RBACMiddleware
	subRouters       []TenantSubRouter
}

// NewTenantHandlers creates new tenant handlers.
func NewTenantHandlers(
	service tenant.Service,
	rbacService RBACChecker,
	connectionTester KeycloakConnectionTester,
	log *logger.Logger,
	authMiddleware func(http.Handler) http.Handler,
	rbacMiddleware *RBACMiddleware,
	subRouters ...TenantSubRouter,
) *TenantHandlers {
	return &TenantHandlers{
		service:          service,
		rbacService:      rbacService,
		connectionTester: connectionTester,
		log:              log,
		authMiddleware:   authMiddleware,
		rbacMiddleware:   rbacMiddleware,
		subRouters:       subRouters,
	}
}

// RegisterRoutes registers all tenant routes using Chi router.
// Routes:
//   - GET    /api/tenants                      - List tenants
//   - POST   /api/tenants                      - Create tenant
//   - POST   /api/tenants/test-connection      - Test Keycloak connection
//   - GET    /api/tenants/{tenantID}           - Get tenant
//   - PUT    /api/tenants/{tenantID}           - Update tenant
//   - DELETE /api/tenants/{tenantID}           - Delete tenant
//   - GET    /api/tenants/{tenantID}/health    - Get tenant health
//
// Sub-routes under /api/tenants/{tenantID} are registered by TenantSubRouter handlers:
//   - /keycloak/*  - Keycloak monitoring routes
//   - /alerts/*    - Alert management routes
//   - /reports/*   - Report generation routes
//   - /operator/*  - Operator metrics routes
//   - /events      - Event listing
//   - /stats       - Statistics
func (h *TenantHandlers) RegisterRoutes(r chi.Router) {
	r.Route("/api/tenants", func(r chi.Router) {
		// Apply auth middleware to all tenant routes
		r.Use(h.authMiddleware)

		// List and create tenants
		r.With(h.rbacMiddleware.RequirePermission(rbac.PermissionTenantsRead)).Get("/", h.listTenants)
		r.With(h.rbacMiddleware.RequirePermission(rbac.PermissionTenantsWrite)).Post("/", h.createTenant)

		// Test connection requires write permission
		r.With(h.rbacMiddleware.RequirePermission(rbac.PermissionTenantsWrite)).Post("/test-connection", h.testConnection)

		// Tenant by ID routes
		r.Route("/{tenantID}", func(r chi.Router) {
			r.Use(TenantIDMiddleware)
			r.Use(h.rbacMiddleware.RequireTenantAccess())

			r.With(h.rbacMiddleware.RequirePermission(rbac.PermissionTenantsRead)).Get("/", h.getTenant)
			r.With(h.rbacMiddleware.RequirePermission(rbac.PermissionTenantsWrite)).Put("/", h.updateTenant)
			r.With(h.rbacMiddleware.RequirePermission(rbac.PermissionTenantsDelete)).Delete("/", h.deleteTenant)
			r.With(h.rbacMiddleware.RequirePermission(rbac.PermissionTenantsRead)).Get("/health", h.getTenantHealth)

			// Register all tenant sub-routes (keycloak, alerts, reports, etc.)
			for _, sub := range h.subRouters {
				sub.RegisterTenantRoutes(r)
			}
		})
	})
}

// listTenants handles GET /api/tenants
//
//	@Summary		List all tenants
//	@Description	Returns a list of all Keycloak tenants the user has access to
//	@Tags			tenants
//	@Produce		json
//	@Success		200	{object}	object{tenants=[]domain.KeycloakTenant,count=int}
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		403	{object}	dto.ErrorResponse
//	@Failure		500	{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants [get]
func (h *TenantHandlers) listTenants(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenants, err := h.service.ListTenants(ctx)
	if err != nil {
		h.log.Error("Failed to list tenants", logger.Err(err))
		httputil.RespondJSON(w, h.log, http.StatusInternalServerError, map[string]string{
			"error": "Failed to list tenants",
		})
		return
	}

	// Filter tenants based on user roles
	filteredTenants := h.filterTenantsByUserRoles(ctx, r, tenants)

	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]interface{}{
		"tenants": filteredTenants,
		"count":   len(filteredTenants),
	})
}

// filterTenantsByUserRoles filters tenants based on user's role assignments.
// Implements fail-closed security: returns empty list if RBAC or user context is unavailable.
func (h *TenantHandlers) filterTenantsByUserRoles(ctx context.Context, r *http.Request, tenants []*domain.KeycloakTenant) []*domain.KeycloakTenant {
	if h.rbacService == nil {
		h.log.Warn("RBAC service not configured, returning empty tenant list for security")
		return []*domain.KeycloakTenant{}
	}

	user := GetUserFromContext(r.Context())
	if user == nil {
		h.log.Warn("No user context found, returning empty tenant list")
		return []*domain.KeycloakTenant{}
	}

	return rbac.FilterTenantsByUserRoles(ctx, tenantVisibilityAdapter{h.rbacService}, user.ID, tenants, h.log)
}

// tenantVisibilityAdapter presents an RBACChecker as the checker
// rbac.FilterTenantsByUserRoles expects.
type tenantVisibilityAdapter struct {
	svc RBACChecker
}

func (a tenantVisibilityAdapter) IsAdmin(ctx context.Context, userID uint) (bool, error) {
	return a.svc.IsAdmin(ctx, userID)
}

func (a tenantVisibilityAdapter) GetUserRoles(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
	roles, err := a.svc.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	converted := make([]*domain.UserRole, len(roles))
	for i, role := range roles {
		converted[i] = &domain.UserRole{RoleID: role.RoleID, TenantID: role.TenantID}
	}
	return converted, nil
}

// createTenant handles POST /api/tenants
//
//	@Summary		Create a new tenant
//	@Description	Creates a new Keycloak tenant with the provided configuration
//	@Tags			tenants
//	@Accept			json
//	@Produce		json
//	@Param			request	body		requests.CreateTenant	true	"Tenant creation request"
//	@Success		201		{object}	object{tenant=domain.KeycloakTenant}
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		409		{object}	dto.ErrorResponse	"Another tenant is already default"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants [post]
func (h *TenantHandlers) createTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	dto, ok := httputil.ValidateAndParse[requests.CreateTenant](w, r, h.log)
	if !ok {
		return
	}

	// SSRF guard: reject server URLs that would dial loopback / cloud
	// metadata / other reserved ranges. The detailed reason is logged
	// server-side; the client gets a generic validation error so the
	// response cannot be used to map internal network topology.
	if h.connectionTester != nil {
		if err := h.connectionTester.ValidateServerURL(ctx, dto.ServerURL); err != nil {
			h.log.Warn("Rejected server_url on create tenant",
				logger.Str("tenant_id", dto.TenantID),
				logger.Str("server_url", dto.ServerURL),
				logger.Err(err))
			httputil.RespondJSON(w, h.log, http.StatusBadRequest, map[string]string{
				"error": "Invalid server_url",
			})
			return
		}
	}

	req := tenant.CreateRequest{
		TenantID:      dto.TenantID,
		Name:          dto.Name,
		Description:   dto.Description,
		ServerURL:     dto.ServerURL,
		AdminRealm:    dto.AdminRealm,
		ClientID:      dto.ClientID,
		ClientSecret:  dto.ClientSecret,
		Configuration: dto.Configuration,
		DefaultRealm:  dto.DefaultRealm,
		Enabled:       dto.Enabled,
		IsDefault:     dto.IsDefault,
		Tags:          dto.Tags,
		Owner:         dto.Owner,
		Amfa:          toTenantAmfa(dto.Amfa),
	}

	if req.IsDefault {
		existing, err := h.service.GetDefault(ctx)
		if err == nil && existing != nil {
			h.log.Warn("Cannot set tenant as default: another tenant is already marked as default",
				logger.Str("existing_default", existing.TenantID),
				logger.Str("requested_tenant", req.TenantID))
			httputil.RespondJSON(w, h.log, http.StatusConflict, map[string]string{
				"error":            "Another tenant is already marked as default",
				"existing_default": existing.TenantID,
				"existing_name":    existing.Name,
			})
			return
		}
	}

	newTenant, err := h.service.CreateTenant(ctx, &req)
	if err != nil {
		h.log.Error("Failed to create tenant",
			logger.Str("tenant_id", req.TenantID),
			logger.Err(err))
		httputil.RespondJSON(w, h.log, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	h.log.Info("Tenant created successfully", logger.Str("tenant_id", req.TenantID))
	httputil.RespondJSON(w, h.log, http.StatusCreated, map[string]interface{}{
		"tenant": newTenant,
	})
}

// getTenant handles GET /api/tenants/{tenantID}
//
//	@Summary		Get tenant by ID
//	@Description	Returns a single tenant by its ID
//	@Tags			tenants
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Success		200			{object}	object{tenant=domain.KeycloakTenant}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Tenant not found"
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID} [get]
func (h *TenantHandlers) getTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	t, err := h.service.GetTenant(ctx, tenantID)
	if err != nil {
		h.log.Warn("Tenant not found", logger.Str("tenant_id", tenantID))
		httputil.RespondJSON(w, h.log, http.StatusNotFound, map[string]string{
			"error": "Tenant not found",
		})
		return
	}

	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]interface{}{
		"tenant": t,
	})
}

// updateTenant handles PUT /api/tenants/{tenantID}
//
//	@Summary		Update tenant
//	@Description	Updates an existing tenant's configuration
//	@Tags			tenants
//	@Accept			json
//	@Produce		json
//	@Param			tenantID	path		string					true	"Tenant ID"
//	@Param			request		body		requests.UpdateTenant	true	"Tenant update request"
//	@Success		200			{object}	object{tenant=domain.KeycloakTenant}
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Tenant not found"
//	@Failure		409			{object}	dto.ErrorResponse	"Another tenant is already default"
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID} [put]
func (h *TenantHandlers) updateTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	dto, ok := httputil.ValidateAndParse[requests.UpdateTenant](w, r, h.log)
	if !ok {
		return
	}

	// SSRF guard: only kicks in if the caller is changing server_url.
	// Tenants with a previously-stored URL are unaffected by this check;
	// the persistent client (pkg/keycloakadmin) revalidates at dial time
	// for those.
	if dto.ServerURL != nil && h.connectionTester != nil {
		if err := h.connectionTester.ValidateServerURL(ctx, *dto.ServerURL); err != nil {
			h.log.Warn("Rejected server_url on update tenant",
				logger.Str("tenant_id", tenantID),
				logger.Str("server_url", *dto.ServerURL),
				logger.Err(err))
			httputil.RespondJSON(w, h.log, http.StatusBadRequest, map[string]string{
				"error": "Invalid server_url",
			})
			return
		}
	}

	req := tenant.UpdateRequest{
		Name:          dto.Name,
		Description:   dto.Description,
		ServerURL:     dto.ServerURL,
		AdminRealm:    dto.AdminRealm,
		ClientID:      dto.ClientID,
		ClientSecret:  dto.ClientSecret,
		Configuration: dto.Configuration,
		DefaultRealm:  dto.DefaultRealm,
		Enabled:       dto.Enabled,
		IsDefault:     dto.IsDefault,
		Tags:          dto.Tags,
		Owner:         dto.Owner,
		Amfa:          toTenantAmfa(dto.Amfa),
	}

	if req.IsDefault != nil && *req.IsDefault {
		existing, err := h.service.GetDefault(ctx)
		if err == nil && existing != nil && existing.TenantID != tenantID {
			h.log.Warn("Cannot set tenant as default: another tenant is already marked as default",
				logger.Str("existing_default", existing.TenantID),
				logger.Str("requested_tenant", tenantID))
			httputil.RespondJSON(w, h.log, http.StatusConflict, map[string]string{
				"error":            "Another tenant is already marked as default",
				"existing_default": existing.TenantID,
				"existing_name":    existing.Name,
			})
			return
		}
	}

	updated, err := h.service.UpdateTenant(ctx, tenantID, &req)
	if err != nil {
		if errors.Is(err, tenant.ErrConfigDefinedTenantReadOnly) {
			h.log.Warn("Rejected update of config-defined tenant",
				logger.Str("tenant_id", tenantID))
			httputil.RespondJSON(w, h.log, http.StatusForbidden, map[string]string{
				"error": err.Error(),
			})
			return
		}
		h.log.Error("Failed to update tenant",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondJSON(w, h.log, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	h.log.Info("Tenant updated successfully", logger.Str("tenant_id", tenantID))
	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]interface{}{
		"tenant": updated,
	})
}

// deleteTenant handles DELETE /api/tenants/{tenantID}
//
//	@Summary		Delete tenant
//	@Description	Deletes a tenant by its ID
//	@Tags			tenants
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Success		200			{object}	dto.MessageResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Tenant not found"
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID} [delete]
func (h *TenantHandlers) deleteTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	if err := h.service.DeleteTenant(ctx, tenantID); err != nil {
		if errors.Is(err, tenant.ErrConfigDefinedTenantReadOnly) {
			h.log.Warn("Rejected delete of config-defined tenant",
				logger.Str("tenant_id", tenantID))
			httputil.RespondJSON(w, h.log, http.StatusForbidden, map[string]string{
				"error": err.Error(),
			})
			return
		}
		h.log.Error("Failed to delete tenant",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		httputil.RespondJSON(w, h.log, http.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
		return
	}

	h.log.Info("Tenant deleted successfully", logger.Str("tenant_id", tenantID))
	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]string{
		"message": "Tenant deleted successfully",
	})
}

// getTenantHealth handles GET /api/tenants/{tenantID}/health
//
//	@Summary		Get tenant health status
//	@Description	Returns the health status of a tenant's Keycloak connection
//	@Tags			tenants
//	@Produce		json
//	@Param			tenantID	path		string	true	"Tenant ID"
//	@Success		200			{object}	tenant.HealthStatus
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"Tenant not found"
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tenants/{tenantID}/health [get]
func (h *TenantHandlers) getTenantHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := chi.URLParam(r, "tenantID")

	health, err := h.service.GetHealth(ctx, tenantID)
	if err != nil {
		h.log.Warn("Tenant not found", logger.Str("tenant_id", tenantID))
		httputil.RespondJSON(w, h.log, http.StatusNotFound, map[string]string{
			"error": "Tenant not found",
		})
		return
	}

	httputil.RespondJSON(w, h.log, http.StatusOK, health)
}

// testConnection handles POST /api/tenants/test-connection
//
//	@Summary		Test Keycloak connection
//	@Description	Tests connection to a Keycloak server with the provided credentials
//	@Tags			tenants
//	@Accept			json
//	@Produce		json
//	@Param			request	body		requests.TestTenantConnection	true	"Connection test request"
//	@Success		200		{object}	object{status=string,message=string}
//	@Failure		400		{object}	object{status=string,message=string,error=string}	"Client creation failed"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		503		{object}	object{status=string,message=string}	"Connection testing unavailable"
//	@Security		BearerAuth
//	@Router			/api/tenants/test-connection [post]
func (h *TenantHandlers) testConnection(w http.ResponseWriter, r *http.Request) {
	dto, ok := httputil.ValidateAndParse[requests.TestTenantConnection](w, r, h.log)
	if !ok {
		return
	}

	// Apply defaults for optional fields
	adminRealm := dto.AdminRealm
	if adminRealm == "" {
		adminRealm = "master"
	}
	clientID := dto.ClientID
	if clientID == "" {
		clientID = "admin-cli"
	}

	if h.connectionTester == nil {
		h.log.Warn("Connection tester not configured")
		httputil.RespondJSON(w, h.log, http.StatusServiceUnavailable, map[string]interface{}{
			"status":  "error",
			"message": "Connection testing is not available",
		})
		return
	}

	ctx := r.Context()
	err := h.connectionTester.TestConnection(
		ctx,
		dto.ServerURL,
		adminRealm,
		clientID,
		dto.ClientSecret,
	)

	if err != nil {
		// Check if this is a client creation error (should return 400)
		if clientErr, ok := err.(ClientCreationError); ok && clientErr.IsClientCreationError() {
			h.log.Warn("Failed to create test Keycloak client",
				logger.Str("server_url", dto.ServerURL),
				logger.Err(err))
			httputil.RespondJSON(w, h.log, http.StatusBadRequest, map[string]interface{}{
				"status":  "error",
				"message": "Failed to create Keycloak client",
				"error":   err.Error(),
			})
			return
		}

		// Authentication failure (return 200 with error status)
		h.log.Warn("Keycloak connection test failed",
			logger.Str("server_url", dto.ServerURL))
		httputil.RespondJSON(w, h.log, http.StatusOK, map[string]interface{}{
			"status":  "error",
			"message": "Failed to authenticate with Keycloak. Please check your credentials and server URL.",
		})
		return
	}

	h.log.Info("Keycloak connection test successful",
		logger.Str("server_url", dto.ServerURL),
		logger.Str("admin_realm", adminRealm))

	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Successfully connected and authenticated with Keycloak",
	})
}

// Ensure TenantHandlers implements RouteRegistrar
var _ RouteRegistrar = (*TenantHandlers)(nil)

// toTenantAmfa converts the request block to the service shape. A nil block
// stays nil so an absent block leaves a tenant's AMFA settings untouched rather
// than clearing them.
func toTenantAmfa(a *requests.TenantAmfa) *tenant.AmfaRequest {
	if a == nil {
		return nil
	}
	return &tenant.AmfaRequest{
		Enabled:            a.Enabled,
		APIBaseURL:         a.APIBaseURL,
		EventsLookbackDays: a.EventsLookbackDays,
		APITimeoutSeconds:  a.APITimeoutSeconds,
	}
}
