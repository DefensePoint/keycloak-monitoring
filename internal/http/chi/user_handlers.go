package chi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/http/dto"
	"github.com/DefensePoint/keycloak-monitoring/internal/http/dto/requests"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
)

var _ = dto.ErrorResponse{} // ensure dto import for swagger
var _ = domain.User{}       // ensure domain import for swagger

// UserService defines the interface for user operations needed by HTTP handlers.
type UserService interface {
	ListUsers(ctx context.Context, opts *httputil.ListOptions) ([]*domain.User, error)
	GetByID(ctx context.Context, userID uint) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	CreateUser(ctx context.Context, user *domain.User) error
	UpdateUser(ctx context.Context, user *domain.User) error
	DeleteUser(ctx context.Context, userID uint) error
}

// UserHandlers handles HTTP requests for user management operations.
type UserHandlers struct {
	service        UserService
	rbacService    RBACChecker
	log            *logger.Logger
	authMiddleware func(http.Handler) http.Handler
	rbacMiddleware *RBACMiddleware
}

// NewUserHandlers creates a new UserHandlers.
func NewUserHandlers(
	service UserService,
	rbacService RBACChecker,
	log *logger.Logger,
	authMiddleware func(http.Handler) http.Handler,
	rbacMiddleware *RBACMiddleware,
) *UserHandlers {
	return &UserHandlers{
		service:        service,
		rbacService:    rbacService,
		log:            log,
		authMiddleware: authMiddleware,
		rbacMiddleware: rbacMiddleware,
	}
}

// RegisterRoutes registers all user routes using Chi router.
// Routes:
//   - GET    /api/users        - List users (requires platform_users:read)
//   - POST   /api/users        - Create user (requires platform_users:write)
//   - GET    /api/users/{id}   - Get user (requires platform_users:read)
//   - PUT    /api/users/{id}   - Update user (requires platform_users:write)
//   - DELETE /api/users/{id}   - Delete user (requires platform_users:delete)
//
// User accounts are platform-wide, so each permission must be granted for
// every tenant; a grant for one tenant is not enough. Only an admin can
// change or delete an admin account.
func (h *UserHandlers) RegisterRoutes(r chi.Router) {
	r.Route("/api/users", func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.rbacMiddleware.RequireGlobalPermission(rbac.PermissionPlatformUsersRead))

		r.Get("/", h.listUsers)
		r.With(h.rbacMiddleware.RequireGlobalPermission(rbac.PermissionPlatformUsersWrite)).Post("/", h.createUser)

		r.Route("/{userID}", func(r chi.Router) {
			r.Get("/", h.getUser)
			r.With(h.rbacMiddleware.RequireGlobalPermission(rbac.PermissionPlatformUsersWrite)).Put("/", h.updateUser)
			r.With(h.rbacMiddleware.RequireGlobalPermission(rbac.PermissionPlatformUsersDelete)).Delete("/", h.deleteUser)
		})
	})
}

// allowedToManage refuses a non-admin acting on another user who holds any
// role, and writes the response. platform_users:write can reset a password,
// so without this anyone holding it could sign in as an operator, or an
// admin, and use every permission that user's roles give. Only an admin
// assigns roles, so only an admin manages anyone who has one; others manage
// the users with none, and themselves.
func (h *UserHandlers) allowedToManage(w http.ResponseWriter, r *http.Request, targetID uint) bool {
	ctx := r.Context()

	caller := GetUserFromContext(ctx)
	if caller == nil {
		httputil.RespondUnauthorized(w, h.log, "Unauthorized")
		return false
	}

	callerIsAdmin, err := h.rbacService.IsAdmin(ctx, caller.ID)
	if err != nil {
		h.log.Error("Failed to check caller admin status", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to check permissions")
		return false
	}
	if callerIsAdmin || targetID == caller.ID {
		return true
	}

	targetHoldsRole, err := h.rbacService.HoldsAnyRole(ctx, targetID)
	if err != nil {
		h.log.Error("Failed to check the target's roles", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to check permissions")
		return false
	}
	if targetHoldsRole {
		httputil.RespondForbidden(w, h.log, "Only an administrator can change or delete a user who holds a role")
		return false
	}
	return true
}

// listUsers handles GET /api/users
//
//	@Summary		List all users
//	@Description	Returns a list of all platform users
//	@Tags			users
//	@Produce		json
//	@Success		200	{object}	object{users=[]domain.User}
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		403	{object}	dto.ErrorResponse
//	@Failure		500	{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/users [get]
func (h *UserHandlers) listUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opts := &httputil.ListOptions{
		Limit:  1000,
		Offset: 0,
	}

	users, err := h.service.ListUsers(ctx, opts)
	if err != nil {
		h.log.Error("Failed to list users", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to retrieve users")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"users": users,
	})
}

// createUser handles POST /api/users
//
//	@Summary		Create a new user
//	@Description	Creates a new platform user with the provided information
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			request	body		requests.CreateUser	true	"User creation request"
//	@Success		201		{object}	domain.User
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		409		{object}	dto.ErrorResponse	"User already exists"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/users [post]
func (h *UserHandlers) createUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, ok := httputil.ValidateAndParse[requests.CreateUser](w, r, h.log)
	if !ok {
		return
	}

	passwordHash, err := hashPassword(req.Password)
	if err != nil {
		h.log.Error("Failed to hash password", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to create user")
		return
	}

	newUser := &domain.User{
		Subject:           req.Username,
		Email:             req.Email,
		Name:              req.Name,
		PreferredUsername: req.Username,
		Username:          req.Username,
		PasswordHash:      passwordHash,
		AuthMethod:        domain.AuthMethodSimple,
		IsActive:          true,
		IsBlocked:         false,
		EmailVerified:     true,
	}

	if err := h.service.CreateUser(ctx, newUser); err != nil {
		h.log.Error("Failed to create user", logger.Err(err))
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
			httputil.RespondJSON(w, h.log, http.StatusConflict, map[string]string{"error": "User already exists"})
		} else {
			httputil.RespondInternalError(w, h.log, "Failed to create user")
		}
		return
	}

	httputil.RespondJSON(w, h.log, http.StatusCreated, newUser)
}

// getUser handles GET /api/users/{id}
//
//	@Summary		Get user by ID
//	@Description	Returns a single user by their ID
//	@Tags			users
//	@Produce		json
//	@Param			userID	path		int	true	"User ID"
//	@Success		200		{object}	domain.User
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid user ID"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse	"User not found"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/users/{userID} [get]
func (h *UserHandlers) getUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid user ID")
		return
	}

	user, err := h.service.GetByID(ctx, uint(userID))
	if err != nil {
		h.log.Error("Failed to get user", logger.Uint("user_id", uint(userID)), logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to get user")
		return
	}

	if user == nil {
		httputil.RespondNotFound(w, h.log, "User not found")
		return
	}

	httputil.RespondSuccess(w, h.log, user)
}

// updateUser handles PUT /api/users/{id}
//
//	@Summary		Update user
//	@Description	Updates an existing user. OAuth users can only have their status changed. Deactivating, blocking, reactivating or unblocking a user revokes their live API tokens: they stop working immediately, and a reactivated or unblocked user starts with none and issues new ones.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			userID	path		int					true	"User ID"
//	@Param			request	body		requests.UpdateUser	true	"User update request"
//	@Success		200		{object}	domain.User
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid user ID or request"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse	"OAuth users cannot be modified, or a non-admin targeted another user who holds a role"
//	@Failure		404		{object}	dto.ErrorResponse	"User not found"
//	@Failure		409		{object}	dto.ErrorResponse	"Username already taken"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/users/{userID} [put]
func (h *UserHandlers) updateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid user ID")
		return
	}

	req, ok := httputil.ValidateAndParse[requests.UpdateUser](w, r, h.log)
	if !ok {
		return
	}

	user, err := h.service.GetByID(ctx, uint(userID))
	if err != nil {
		h.log.Error("Failed to get user", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to get user")
		return
	}

	if user == nil {
		httputil.RespondNotFound(w, h.log, "User not found")
		return
	}
	if !h.allowedToManage(w, r, user.ID) {
		return
	}
	wasActive, wasBlocked := user.IsActive, user.IsBlocked

	// OAuth users can only have their status changed
	if user.AuthMethod == domain.AuthMethodOAuth {
		if req.Email != nil || req.Name != nil || req.Username != nil || req.Password != nil {
			httputil.RespondForbidden(w, h.log, "OAuth users' identity fields are managed by the OAuth provider and cannot be modified")
			return
		}
		if req.IsActive != nil {
			user.IsActive = *req.IsActive
		}
		if req.IsBlocked != nil {
			user.IsBlocked = *req.IsBlocked
		}
	} else {
		if req.Email != nil {
			user.Email = *req.Email
		}
		if req.Name != nil {
			user.Name = *req.Name
		}
		if req.Username != nil && *req.Username != "" && *req.Username != user.Username {
			// No account with the name is the answer we hope for, not a fault.
			existingUser, err := h.service.GetByUsername(ctx, *req.Username)
			if err != nil && !errors.Is(err, users.ErrUserNotFound) {
				h.log.Error("Failed to check username availability", logger.Err(err))
				httputil.RespondInternalError(w, h.log, "Failed to check username availability")
				return
			}
			if existingUser != nil && existingUser.ID != uint(userID) {
				httputil.RespondJSON(w, h.log, http.StatusConflict, map[string]string{"error": "Username already taken"})
				return
			}
			user.Username = *req.Username
			user.PreferredUsername = *req.Username
			user.Subject = *req.Username
		}
		if req.Password != nil && *req.Password != "" {
			passwordHash, err := hashPassword(*req.Password)
			if err != nil {
				h.log.Error("Failed to hash password", logger.Err(err))
				httputil.RespondInternalError(w, h.log, "Failed to update password")
				return
			}
			user.PasswordHash = passwordHash
		}
		if req.IsActive != nil {
			user.IsActive = *req.IsActive
		}
		if req.IsBlocked != nil {
			user.IsBlocked = *req.IsBlocked
		}
	}

	if err := h.service.UpdateUser(ctx, user); err != nil {
		h.log.Error("Failed to update user", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to update user")
		return
	}
	if wasActive && !user.IsActive {
		h.logDeactivation(ctx, user.ID)
	}
	if !wasActive && user.IsActive {
		h.logRestore(ctx, user.ID, "User reactivated; any API tokens they still held are revoked", "reactivated_by")
	}
	if !wasBlocked && user.IsBlocked {
		h.logBlock(ctx, user.ID)
	}
	if wasBlocked && !user.IsBlocked {
		h.logRestore(ctx, user.ID, "User unblocked; any API tokens they still held are revoked", "unblocked_by")
	}

	httputil.RespondSuccess(w, h.log, user)
}

// deleteUser handles DELETE /api/users/{id}, which deactivates rather than
// deletes.
//
// The route keeps the DELETE verb because that is the REST spelling of
// "remove this" and changing it would break every existing client. What the
// words say is another matter: the account, its roles and its history are all
// kept, the change is reversible by setting the account active again, and
// nothing is erased. Everything a person reads now says deactivate.
//
// It takes effect at once: deactivating an active account ends its web
// sessions and revokes its API tokens in the same transaction, and neither
// comes back if the account is reactivated (see the users repository's
// endAccess). The auth middleware also refuses any session of an inactive or
// blocked account on its next request.
//
//	@Summary		Deactivate user
//	@Description	Deactivates a user by their ID. The account, its roles and its history are kept, and the change is reversible. It takes effect at once: deactivating an active user ends their web sessions and revokes their API tokens, and reactivation restores neither.
//	@Tags			users
//	@Produce		json
//	@Param			userID	path		int	true	"User ID"
//	@Success		200		{object}	dto.MessageResponse
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid user ID"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse	"A non-admin targeted another user who holds a role"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/users/{userID} [delete]
func (h *UserHandlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid user ID")
		return
	}

	if !h.allowedToManage(w, r, uint(userID)) {
		return
	}

	if err := h.service.DeleteUser(ctx, uint(userID)); err != nil {
		h.log.Error("Failed to delete user", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to delete user")
		return
	}
	h.logDeactivation(ctx, uint(userID))

	httputil.RespondSuccess(w, h.log, map[string]string{"message": "User deleted successfully"})
}

// logDeactivation records who deactivated a user. Deactivating an active user
// revokes their API tokens, so this is also the audit line for those
// revocations, the counterpart of the revoked_by an individual revoke logs.
func (h *UserHandlers) logDeactivation(ctx context.Context, userID uint) {
	h.log.Info("User deactivated; an active user's API tokens are revoked with it",
		logger.Uint("user_id", userID),
		logger.Uint("deactivated_by", actorIDFrom(ctx)))
}

// logBlock records who blocked a user, and so who revoked their API tokens;
// see logDeactivation.
func (h *UserHandlers) logBlock(ctx context.Context, userID uint) {
	h.log.Info("User blocked; their API tokens are revoked with it",
		logger.Uint("user_id", userID),
		logger.Uint("blocked_by", actorIDFrom(ctx)))
}

// logRestore records who reactivated or unblocked a user. Restoring a user
// revokes any API tokens they still hold, so this is the audit line for those.
func (h *UserHandlers) logRestore(ctx context.Context, userID uint, msg, actorField string) {
	h.log.Info(msg,
		logger.Uint("user_id", userID),
		logger.Uint(actorField, actorIDFrom(ctx)))
}

// actorIDFrom returns the ID of the user making the request, or 0 when the
// request carries none.
func actorIDFrom(ctx context.Context) uint {
	if actor := GetUserFromContext(ctx); actor != nil {
		return actor.ID
	}
	return 0
}

// hashPassword hashes a password using bcrypt.
func hashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

// Ensure UserHandlers implements RouteRegistrar
var _ RouteRegistrar = (*UserHandlers)(nil)
