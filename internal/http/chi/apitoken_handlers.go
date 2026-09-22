package chi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/apitoken"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/http/dto"
	"github.com/DefensePoint/keycloak-monitoring/internal/http/dto/requests"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

var _ = dto.MessageResponse{} // ensure dto import for swagger

// APITokenHandlers handles HTTP requests for API token management.
type APITokenHandlers struct {
	service        apitoken.Service
	log            *logger.Logger
	authMiddleware func(http.Handler) http.Handler
	rbacMiddleware *RBACMiddleware
}

// NewAPITokenHandlers creates a new APITokenHandlers.
func NewAPITokenHandlers(
	service apitoken.Service,
	log *logger.Logger,
	authMiddleware func(http.Handler) http.Handler,
	rbacMiddleware *RBACMiddleware,
) *APITokenHandlers {
	return &APITokenHandlers{
		service:        service,
		log:            log,
		authMiddleware: authMiddleware,
		rbacMiddleware: rbacMiddleware,
	}
}

// RegisterRoutes registers all API token routes using Chi router.
// Routes:
//   - POST   /api/tokens           - Create token (admin)
//   - GET    /api/tokens           - List tokens (admin)
//   - DELETE /api/tokens/{tokenID} - Revoke token (admin)
func (h *APITokenHandlers) RegisterRoutes(r chi.Router) {
	r.Route("/api/tokens", func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.rbacMiddleware.RequireAdmin())

		r.Get("/", h.listTokens)
		r.Post("/", h.createToken)
		r.Delete("/{tokenID}", h.revokeToken)
	})
}

// createToken handles POST /api/tokens
//
//	@Summary		Create an API token
//	@Description	Creates a personal access token for a user. tenant_ids must name at least one tenant; the token can never reach a tenant outside it. The plaintext token is returned only once in this response and cannot be retrieved again. When expires_at is omitted the token expires after 90 days; a requested expiry beyond 365 days is capped at 365 days.
//	@Tags			tokens
//	@Accept			json
//	@Produce		json
//	@Param			request	body		requests.CreateAPIToken	true	"Token creation request"
//	@Success		201		{object}	apitoken.CreatedToken
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid request or expiry"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse	"User not found"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tokens [post]
func (h *APITokenHandlers) createToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	req, ok := httputil.ValidateAndParse[requests.CreateAPIToken](w, r, h.log)
	if !ok {
		return
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

	created, err := h.service.Create(ctx, req.UserID, req.Name, req.TenantIDs, expiresAt)
	if err != nil {
		switch {
		case errors.Is(err, apitoken.ErrUserNotFound):
			httputil.RespondNotFound(w, h.log, "User not found")
		case errors.Is(err, apitoken.ErrNoTenantIDs):
			httputil.RespondBadRequest(w, h.log, "tenant_ids must name at least one tenant")
		case errors.Is(err, apitoken.ErrExpiryInPast):
			httputil.RespondBadRequest(w, h.log, "Expiry must be in the future")
		default:
			h.log.Error("Failed to create API token", logger.Err(err))
			httputil.RespondInternalError(w, h.log, "Failed to create token")
		}
		return
	}

	httputil.RespondJSON(w, h.log, http.StatusCreated, created)
}

// listTokens handles GET /api/tokens
//
//	@Summary		List API tokens
//	@Description	Returns metadata for all API tokens. Token digests and plaintext values are never included.
//	@Tags			tokens
//	@Produce		json
//	@Success		200	{object}	object{tokens=[]apitoken.Token}
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		403	{object}	dto.ErrorResponse
//	@Failure		500	{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tokens [get]
func (h *APITokenHandlers) listTokens(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tokens, err := h.service.List(ctx)
	if err != nil {
		h.log.Error("Failed to list API tokens", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to list tokens")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"tokens": tokens,
	})
}

// revokeToken handles DELETE /api/tokens/{tokenID}
//
//	@Summary		Revoke an API token
//	@Description	Revokes an API token so it can no longer authenticate requests
//	@Tags			tokens
//	@Produce		json
//	@Param			tokenID	path		int	true	"Token ID"
//	@Success		200		{object}	dto.MessageResponse
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid token ID"
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse	"Token not found"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/tokens/{tokenID} [delete]
func (h *APITokenHandlers) revokeToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tokenIDStr := chi.URLParam(r, "tokenID")
	tokenID, err := strconv.ParseUint(tokenIDStr, 10, 64)
	if err != nil {
		httputil.RespondBadRequest(w, h.log, "Invalid token ID")
		return
	}

	if err := h.service.Revoke(ctx, uint(tokenID)); err != nil {
		if errors.Is(err, apitoken.ErrTokenNotFound) {
			httputil.RespondNotFound(w, h.log, "Token not found")
			return
		}
		h.log.Error("Failed to revoke API token", logger.Uint("token_id", uint(tokenID)), logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to revoke token")
		return
	}

	httputil.RespondSuccess(w, h.log, map[string]string{"message": "Token revoked successfully"})
}

// Ensure APITokenHandlers implements RouteRegistrar
var _ RouteRegistrar = (*APITokenHandlers)(nil)
