package chi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/http/dto"
	"github.com/DefensePoint/keycloak-monitoring/internal/http/dto/requests"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
)

var _ = dto.ErrorResponse{} // ensure dto import for swagger
var _ = domain.Role{}       // ensure domain import for swagger

// RBACHandlers handles HTTP requests for RBAC operations.
type RBACHandlers struct {
	service        rbac.Service
	log            *logger.Logger
	authMiddleware func(http.Handler) http.Handler
	rbacMiddleware *RBACMiddleware
}

// NewRBACHandlers creates a new RBACHandlers.
func NewRBACHandlers(
	service rbac.Service,
	log *logger.Logger,
	authMiddleware func(http.Handler) http.Handler,
	rbacMiddleware *RBACMiddleware,
) *RBACHandlers {
	return &RBACHandlers{
		service:        service,
		log:            log,
		authMiddleware: authMiddleware,
		rbacMiddleware: rbacMiddleware,
	}
}

// RegisterRoutes registers all RBAC routes using Chi router.
// Routes:
//   - GET  /api/rbac/me                           - Current user info (auth only)
//   - GET  /api/rbac/roles                        - List roles (admin)
//   - POST /api/rbac/roles                        - Create role (admin)
//   - GET  /api/rbac/roles/{id}                   - Get role (admin)
//   - PUT  /api/rbac/roles/{id}                   - Update role (admin)
//   - DELETE /api/rbac/roles/{id}                 - Delete role (admin)
//   - GET  /api/rbac/permissions                  - List permissions (admin)
//   - GET  /api/rbac/users/{user_id}/roles        - User roles (admin)
//   - POST /api/rbac/users/{user_id}/roles        - Assign role (admin)
//   - DELETE /api/rbac/users/{user_id}/roles/{role_id} - Remove role (admin)
//   - GET  /api/rbac/users/{user_id}/policies     - User policies (admin)
//   - POST /api/rbac/users/{user_id}/policies     - Create policy (admin)
//   - DELETE /api/rbac/policies/{id}              - Delete policy (admin)
func (h *RBACHandlers) RegisterRoutes(r chi.Router) {
	r.Route("/api/rbac", func(r chi.Router) {
		r.Use(h.authMiddleware)

		// Current user info - auth only, no admin required
		r.Get("/me", h.handleCurrentUser)

		// Roles - requires admin
		r.Route("/roles", func(r chi.Router) {
			r.Use(h.rbacMiddleware.RequireAdmin())
			r.Get("/", h.listRoles)
			r.Post("/", h.createRole)
			r.Route("/{roleID}", func(r chi.Router) {
				r.Get("/", h.getRole)
				r.Put("/", h.updateRole)
				r.Delete("/", h.deleteRole)
			})
		})

		// Permissions - requires admin
		r.With(h.rbacMiddleware.RequireAdmin()).Get("/permissions", h.handlePermissions)

		// User roles and policies - requires admin
		r.Route("/users/{userID}", func(r chi.Router) {
			r.Use(h.rbacMiddleware.RequireAdmin())

			r.Route("/roles", func(r chi.Router) {
				r.Get("/", h.getUserRoles)
				r.Post("/", h.assignRoleToUser)
				r.Delete("/{roleID}", h.removeRoleFromUser)
			})

			r.Route("/policies", func(r chi.Router) {
				r.Get("/", h.getUserPolicies)
				r.Post("/", h.createUserPolicy)
			})
		})

		// Policy by ID - requires admin
		r.With(h.rbacMiddleware.RequireAdmin()).Delete("/policies/{policyID}", h.deletePolicy)
	})
}

// handleCurrentUser handles GET /api/rbac/me
//
//	@Summary		Get current user RBAC info
//	@Description	Returns the current user's roles, permissions, and tenant policies
//	@Tags			rbac
//	@Produce		json
//	@Param			tenant_id	query		string	false	"Filter by tenant ID"
//	@Success		200			{object}	object{id=uint,subject=string,email=string,email_verified=bool,name=string,preferred_username=string,roles=[]domain.UserRole,permissions=[]string,tenant_policies=[]domain.TenantPolicy}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/me [get]
func (h *RBACHandlers) handleCurrentUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user := GetUserFromContext(ctx)
	if user == nil {
		httputil.RespondUnauthorized(w, h.log, "Not authenticated")
		return
	}

	var tenantID *string
	if tid := r.URL.Query().Get("tenant_id"); tid != "" {
		tenantID = &tid
	}

	userWithRoles, err := h.service.GetUserWithRolesAndPermissions(ctx, user.ID, tenantID)
	if err != nil {
		h.log.Error("Failed to get user RBAC info", logger.Err(err), logger.Uint("user_id", user.ID))
		httputil.RespondInternalError(w, h.log, "Failed to get user info")
		return
	}

	response := map[string]interface{}{
		"id":                 user.ID,
		"subject":            user.Subject,
		"email":              user.Email,
		"email_verified":     user.EmailVerified,
		"name":               user.Name,
		"preferred_username": user.PreferredUsername,
		"roles":              userWithRoles.Roles,
		"permissions":        userWithRoles.Permissions,
		"tenant_policies":    userWithRoles.Policies,
	}

	httputil.RespondSuccess(w, h.log, response)
}

// listRoles handles GET /api/rbac/roles
//
//	@Summary		List all roles
//	@Description	Returns a list of all roles in the system
//	@Tags			rbac
//	@Produce		json
//	@Param			include_inactive	query		bool	false	"Include inactive roles"
//	@Success		200					{object}	object{roles=[]domain.Role}
//	@Failure		401					{object}	dto.ErrorResponse
//	@Failure		403					{object}	dto.ErrorResponse
//	@Failure		500					{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/roles [get]
func (h *RBACHandlers) listRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	includeInactive := r.URL.Query().Get("include_inactive") == "true"

	roles, err := h.service.ListRoles(ctx, includeInactive)
	if err != nil {
		h.log.Error("Failed to list roles", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to list roles")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"roles": roles,
	})
}

// createRole handles POST /api/rbac/roles
//
//	@Summary		Create a new role
//	@Description	Creates a new role with optional permissions
//	@Tags			rbac
//	@Accept			json
//	@Produce		json
//	@Param			request	body		requests.CreateRole	true	"Role creation request"
//	@Success		201		{object}	domain.Role
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/roles [post]
func (h *RBACHandlers) createRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, ok := httputil.ValidateAndParse[requests.CreateRole](w, r, h.log)
	if !ok {
		return
	}

	role, err := h.service.CreateRole(ctx, req.Name, req.DisplayName, req.Description, false)
	if err != nil {
		h.log.Error("Failed to create role", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to create role")
		return
	}

	if len(req.Permissions) > 0 {
		if err := h.service.AssignPermissionsToRole(ctx, role.ID, req.Permissions); err != nil {
			h.log.Error("Failed to assign permissions to role", logger.Err(err))
		}
	}

	httputil.RespondJSON(w, h.log, http.StatusCreated, role)
}

// getRole handles GET /api/rbac/roles/{id}
//
//	@Summary		Get role by ID
//	@Description	Returns a single role with its permissions
//	@Tags			rbac
//	@Produce		json
//	@Param			roleID	path		int	true	"Role ID"
//	@Success		200		{object}	domain.Role
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid role ID"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse	"Role not found"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/roles/{roleID} [get]
func (h *RBACHandlers) getRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	roleIDStr := chi.URLParam(r, "roleID")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid role ID")
		return
	}

	role, err := h.service.GetRoleWithPermissions(ctx, uint(roleID))
	if err != nil {
		h.log.Error("Failed to get role", logger.Uint("role_id", uint(roleID)), logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to get role")
		return
	}

	if role == nil {
		httputil.RespondNotFound(w, h.log, "Role not found")
		return
	}

	httputil.RespondSuccess(w, h.log, role)
}

// updateRole handles PUT /api/rbac/roles/{id}
//
//	@Summary		Update role
//	@Description	Updates an existing role. System roles cannot be modified.
//	@Tags			rbac
//	@Accept			json
//	@Produce		json
//	@Param			roleID	path		int					true	"Role ID"
//	@Param			request	body		requests.UpdateRole	true	"Role update request"
//	@Success		200		{object}	domain.Role
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid role ID or request"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse	"Cannot modify system roles"
//	@Failure		404		{object}	dto.ErrorResponse	"Role not found"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/roles/{roleID} [put]
func (h *RBACHandlers) updateRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	roleIDStr := chi.URLParam(r, "roleID")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid role ID")
		return
	}

	req, ok := httputil.ValidateAndParse[requests.UpdateRole](w, r, h.log)
	if !ok {
		return
	}

	role, err := h.service.GetRoleWithPermissions(ctx, uint(roleID))
	if err != nil {
		h.log.Error("Failed to get role", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to get role")
		return
	}

	if role == nil {
		httputil.RespondNotFound(w, h.log, "Role not found")
		return
	}

	if role.IsSystem {
		httputil.RespondForbidden(w, h.log, "Cannot modify system roles")
		return
	}

	role.DisplayName = req.DisplayName
	role.Description = req.Description

	if err := h.service.UpdateRole(ctx, role); err != nil {
		h.log.Error("Failed to update role", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to update role")
		return
	}

	if req.Permissions != nil {
		if err := h.service.AssignPermissionsToRole(ctx, role.ID, req.Permissions); err != nil {
			h.log.Error("Failed to update role permissions", logger.Err(err))
		}
	}

	httputil.RespondSuccess(w, h.log, role)
}

// deleteRole handles DELETE /api/rbac/roles/{id}
//
//	@Summary		Delete role
//	@Description	Deletes a role. System roles cannot be deleted.
//	@Tags			rbac
//	@Produce		json
//	@Param			roleID	path		int	true	"Role ID"
//	@Success		200		{object}	dto.MessageResponse
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid role ID"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse	"Cannot delete system roles"
//	@Failure		404		{object}	dto.ErrorResponse	"Role not found"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/roles/{roleID} [delete]
func (h *RBACHandlers) deleteRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	roleIDStr := chi.URLParam(r, "roleID")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid role ID")
		return
	}

	role, err := h.service.GetRoleWithPermissions(ctx, uint(roleID))
	if err != nil {
		h.log.Error("Failed to get role", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to get role")
		return
	}

	if role == nil {
		httputil.RespondNotFound(w, h.log, "Role not found")
		return
	}

	if role.IsSystem {
		httputil.RespondForbidden(w, h.log, "Cannot delete system roles")
		return
	}

	if err := h.service.DeleteRole(ctx, uint(roleID)); err != nil {
		h.log.Error("Failed to delete role", logger.Uint("role_id", uint(roleID)), logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to delete role")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]string{"message": "Role deleted successfully"})
}

// handlePermissions handles GET /api/rbac/permissions
//
//	@Summary		List all permissions
//	@Description	Returns a list of all available permissions
//	@Tags			rbac
//	@Produce		json
//	@Param			resource	query		string	false	"Filter by resource name"
//	@Success		200			{object}	object{permissions=[]domain.Permission}
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/permissions [get]
func (h *RBACHandlers) handlePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resource := r.URL.Query().Get("resource")

	permissions, err := h.service.ListPermissions(ctx, resource)
	if err != nil {
		h.log.Error("Failed to list permissions", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to list permissions")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"permissions": permissions,
	})
}

// getUserRoles handles GET /api/rbac/users/{user_id}/roles
//
//	@Summary		Get user roles
//	@Description	Returns all roles assigned to a user
//	@Tags			rbac
//	@Produce		json
//	@Param			userID		path		int		true	"User ID"
//	@Param			tenant_id	query		string	false	"Filter by tenant ID"
//	@Success		200			{object}	object{roles=[]domain.UserRole}
//	@Failure		400			{object}	dto.ErrorResponse	"Invalid user ID"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/users/{userID}/roles [get]
func (h *RBACHandlers) getUserRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid user ID")
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")

	var roles []*domain.UserRole

	if tenantID != "" {
		roles, err = h.service.GetUserRolesForTenant(ctx, uint(userID), tenantID)
	} else {
		roles, err = h.service.GetUserRoles(ctx, uint(userID))
	}

	if err != nil {
		h.log.Error("Failed to get user roles", logger.Err(err), logger.Uint("user_id", uint(userID)))
		httputil.RespondInternalError(w, h.log, "Failed to get user roles")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"roles": roles,
	})
}

// assignRoleToUser handles POST /api/rbac/users/{user_id}/roles
//
//	@Summary		Assign role to user
//	@Description	Assigns a role to a user, optionally scoped to a tenant with expiration
//	@Tags			rbac
//	@Accept			json
//	@Produce		json
//	@Param			userID	path		int					true	"User ID"
//	@Param			request	body		requests.AssignRole	true	"Role assignment request"
//	@Success		200		{object}	dto.MessageResponse
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid user ID or request"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/users/{userID}/roles [post]
func (h *RBACHandlers) assignRoleToUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid user ID")
		return
	}

	req, ok := httputil.ValidateAndParse[requests.AssignRole](w, r, h.log)
	if !ok {
		return
	}

	currentUser := GetUserFromContext(ctx)
	assignedBy := "system"
	if currentUser != nil {
		assignedBy = currentUser.Email
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			httputil.RespondBadRequest(w, h.log, "Invalid expires_at format (use RFC3339)")
			return
		}
		expiresAt = &t
	}

	if err := h.service.AssignRoleToUser(ctx, uint(userID), req.RoleID, req.TenantID, assignedBy, expiresAt); err != nil {
		if errors.Is(err, rbac.ErrAdminRoleTenantScoped) {
			httputil.RespondBadRequest(w, h.log, err.Error())
			return
		}
		h.log.Error("Failed to assign role to user", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to assign role")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]string{"message": "Role assigned successfully"})
}

// removeRoleFromUser handles DELETE /api/rbac/users/{user_id}/roles/{role_id}
//
//	@Summary		Remove role from user
//	@Description	Removes a role assignment from a user
//	@Tags			rbac
//	@Produce		json
//	@Param			userID		path		int		true	"User ID"
//	@Param			roleID		path		int		true	"Role ID"
//	@Param			tenant_id	query		string	false	"Tenant ID (for tenant-scoped roles)"
//	@Success		200			{object}	dto.MessageResponse
//	@Failure		400			{object}	dto.ErrorResponse	"Invalid user or role ID"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/users/{userID}/roles/{roleID} [delete]
func (h *RBACHandlers) removeRoleFromUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid user ID")
		return
	}

	roleIDStr := chi.URLParam(r, "roleID")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid role ID")
		return
	}

	tenantIDStr := r.URL.Query().Get("tenant_id")
	var tenantID *string
	if tenantIDStr != "" {
		tenantID = &tenantIDStr
	}

	if err := h.service.RemoveRoleFromUser(ctx, uint(userID), uint(roleID), tenantID); err != nil {
		h.log.Error("Failed to remove role from user", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to remove role")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]string{"message": "Role removed successfully"})
}

// getUserPolicies handles GET /api/rbac/users/{user_id}/policies
//
//	@Summary		Get user tenant policies
//	@Description	Returns all tenant access policies for a user
//	@Tags			rbac
//	@Produce		json
//	@Param			userID	path		int	true	"User ID"
//	@Success		200		{object}	object{policies=[]domain.TenantPolicy}
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid user ID"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/users/{userID}/policies [get]
func (h *RBACHandlers) getUserPolicies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid user ID")
		return
	}

	policies, err := h.service.GetUserPolicies(ctx, uint(userID))
	if err != nil {
		h.log.Error("Failed to get user policies", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to get user policies")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"policies": policies,
	})
}

// createUserPolicy handles POST /api/rbac/users/{user_id}/policies
//
//	@Summary		Create tenant policy for user
//	@Description	Creates a new tenant access policy for a user
//	@Tags			rbac
//	@Accept			json
//	@Produce		json
//	@Param			userID	path		int							true	"User ID"
//	@Param			request	body		requests.CreateTenantPolicy	true	"Policy creation request"
//	@Success		201		{object}	dto.MessageResponse
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid user ID or request"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/users/{userID}/policies [post]
func (h *RBACHandlers) createUserPolicy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid user ID")
		return
	}

	req, ok := httputil.ValidateAndParse[requests.CreateTenantPolicy](w, r, h.log)
	if !ok {
		return
	}

	currentUser := GetUserFromContext(ctx)
	grantedBy := "system"
	if currentUser != nil {
		grantedBy = currentUser.Email
	}

	if err := h.service.CreateTenantPolicy(ctx, uint(userID), req.TenantID, req.AllowedRealms, grantedBy); err != nil {
		if errors.Is(err, rbac.ErrAllowedRealmBlank) {
			httputil.RespondBadRequest(w, h.log, err.Error())
			return
		}
		h.log.Error("Failed to create tenant policy", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to create policy")
		return
	}

	httputil.RespondJSON(w, h.log, http.StatusCreated, map[string]string{"message": "Policy created successfully"})
}

// deletePolicy handles DELETE /api/rbac/policies/{id}
//
//	@Summary		Delete tenant policy
//	@Description	Deletes a tenant access policy
//	@Tags			rbac
//	@Produce		json
//	@Param			policyID	path		int	true	"Policy ID"
//	@Success		200			{object}	dto.MessageResponse
//	@Failure		400			{object}	dto.ErrorResponse	"Invalid policy ID"
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/rbac/policies/{policyID} [delete]
func (h *RBACHandlers) deletePolicy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	policyIDStr := chi.URLParam(r, "policyID")
	policyID, err := strconv.ParseUint(policyIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid policy ID")
		return
	}

	if err := h.service.DeleteTenantPolicy(ctx, uint(policyID)); err != nil {
		h.log.Error("Failed to delete policy", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to delete policy")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]string{"message": "Policy deleted successfully"})
}

// Ensure RBACHandlers implements RouteRegistrar
var _ RouteRegistrar = (*RBACHandlers)(nil)
