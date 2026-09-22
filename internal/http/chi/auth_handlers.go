package chi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	_ "github.com/DefensePoint/keycloak-monitoring/internal/http/dto" // swagger
	"github.com/DefensePoint/keycloak-monitoring/internal/http/dto/requests"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// OAuth2AdminService defines the interface for automatic admin role assignment.
type OAuth2AdminService interface {
	EnsureAdminRole(ctx context.Context, user *domain.User) error
}

// SessionStore provides session storage for authentication.
type SessionStore interface {
	Get(r *http.Request, name string) (*sessions.Session, error)
	Save(r *http.Request, w http.ResponseWriter, s *sessions.Session) error
}

// SimpleAuthService defines the interface for simple authentication.
type SimpleAuthService interface {
	Authenticate(ctx context.Context, username, password string) (*auth.UserInfo, error)
	ChangePasswordByUsername(ctx context.Context, username, oldPassword, newPassword string) error
	GetPasswordRequirements() auth.PasswordRequirements
}

// AuthHandlers handles HTTP requests for authentication operations.
type AuthHandlers struct {
	service          auth.Service
	simpleAuth       SimpleAuthService
	oauth2AdminSvc   OAuth2AdminService
	provider         auth.Provider
	sessionStore     SessionStore
	sessionName      string
	log              *logger.Logger
	simpleEnabled    bool
	oauth2Enabled    bool
	stateStore       *auth.StateStore
	authMiddleware   func(http.Handler) http.Handler
	sessionBlacklist *auth.SessionBlacklist
}

// NewAuthHandlers creates a new AuthHandlers.
func NewAuthHandlers(
	service auth.Service,
	simpleAuth SimpleAuthService,
	oauth2AdminSvc OAuth2AdminService,
	provider auth.Provider,
	sessionStore SessionStore,
	sessionName string,
	log *logger.Logger,
	simpleEnabled bool,
	oauth2Enabled bool,
	stateStore *auth.StateStore,
	authMiddleware func(http.Handler) http.Handler,
) *AuthHandlers {
	return &AuthHandlers{
		service:          service,
		simpleAuth:       simpleAuth,
		oauth2AdminSvc:   oauth2AdminSvc,
		provider:         provider,
		sessionStore:     sessionStore,
		sessionName:      sessionName,
		log:              log,
		simpleEnabled:    simpleEnabled,
		oauth2Enabled:    oauth2Enabled,
		stateStore:       stateStore,
		authMiddleware:   authMiddleware,
		sessionBlacklist: nil, // Set via SetSessionBlacklist
	}
}

// SetSessionBlacklist sets the session blacklist for logout revocation.
func (h *AuthHandlers) SetSessionBlacklist(bl *auth.SessionBlacklist) {
	h.sessionBlacklist = bl
}

// generateSessionID generates a cryptographically secure random session ID.
func generateSessionID() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(bytes)
}

// RegisterRoutes registers all auth routes using Chi router.
// Routes:
//   - GET  /auth/config              - Get auth config (public)
//   - POST /auth/login/simple        - Simple login (public)
//   - GET  /auth/login/oauth2        - OAuth2 login (public)
//   - GET  /auth/callback            - OAuth2 callback (public)
//   - GET  /auth/logout              - Logout (public)
//   - GET  /auth/password/requirements - Password requirements (public)
//   - GET  /auth/userinfo            - Current user info (auth required)
//   - POST /auth/refresh             - Refresh token (auth required)
//   - POST /auth/password/change     - Change password (auth required)
func (h *AuthHandlers) RegisterRoutes(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		// Public routes (no auth required)
		r.Get("/config", h.handleAuthConfig)
		r.Post("/login/simple", h.handleSimpleLogin)
		r.Get("/login/oauth2", h.handleOAuth2Login)
		r.Get("/callback", h.handleCallback)
		r.Get("/logout", h.handleLogout)
		r.Get("/password/requirements", h.handlePasswordRequirements)

		// Protected routes (auth required)
		r.Group(func(r chi.Router) {
			r.Use(h.authMiddleware)
			r.Get("/userinfo", h.handleUserInfo)
			r.Post("/refresh", h.handleRefresh)
			r.Post("/password/change", h.handleChangePassword)
		})
	})
}

// handleAuthConfig returns which authentication methods are enabled.
//
//	@Summary		Get authentication configuration
//	@Description	Returns which authentication methods are enabled (simple, OAuth2)
//	@Tags			Auth
//	@Produce		json
//	@Success		200	{object}	object{simple_enabled=bool,oauth2_enabled=bool,auth_required=bool}
//	@Router			/auth/config [get]
func (h *AuthHandlers) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	config := map[string]interface{}{
		"simple_enabled": h.simpleEnabled,
		"oauth2_enabled": h.oauth2Enabled,
		"auth_required":  true,
	}

	httputil.RespondSuccess(w, h.log, config)
}

// handleSimpleLogin handles simple username/password login.
//
//	@Summary		Login with username and password
//	@Description	Authenticate using simple username/password credentials
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		requests.SimpleLogin	true	"Login credentials"
//	@Success		200		{object}	object{success=bool,user=auth.UserInfo}
//	@Failure		401		{object}	object{success=bool,message=string}
//	@Failure		403		{object}	dto.ErrorResponse	"Simple auth disabled"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Router			/auth/login/simple [post]
func (h *AuthHandlers) handleSimpleLogin(w http.ResponseWriter, r *http.Request) {
	if !h.simpleEnabled {
		httputil.RespondForbidden(w, h.log, "Simple authentication is disabled")
		return
	}

	if h.simpleAuth == nil {
		httputil.RespondInternalError(w, h.log, "Simple auth service not configured")
		return
	}

	req, ok := httputil.ValidateAndParse[requests.SimpleLogin](w, r, h.log)
	if !ok {
		return
	}

	userInfo, err := h.simpleAuth.Authenticate(r.Context(), req.Username, req.Password)
	if err != nil {
		h.log.Warn("Authentication failed",
			logger.Str("username", req.Username),
			logger.Err(err))
		httputil.RespondJSON(w, h.log, http.StatusUnauthorized, map[string]interface{}{
			"success": false,
			"message": "Invalid username or password",
		})
		return
	}

	// Save session to cookie
	if h.sessionStore != nil {
		session, err := h.sessionStore.Get(r, h.sessionName)
		if err != nil {
			h.log.Warn("Failed to get session, creating new one", logger.Err(err))
		}

		// Generate unique session ID for blacklist tracking
		sessionID := generateSessionID()

		session.Values["authenticated"] = true
		session.Values["auth_method"] = "simple"
		session.Values["session_id"] = sessionID
		session.Values["subject"] = userInfo.Subject
		session.Values["email"] = userInfo.Email
		session.Values["name"] = userInfo.Name
		session.Values["created_at"] = time.Now().Unix()

		if err := h.sessionStore.Save(r, w, session); err != nil {
			h.log.Error("Failed to save session", logger.Err(err))
			httputil.RespondInternalError(w, h.log, "Failed to create session")
			return
		}
	}

	h.log.Info("User authenticated successfully via simple auth",
		logger.Str("username", req.Username),
		logger.Str("email", userInfo.Email))

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"success": true,
		"user":    userInfo,
	})
}

// handleOAuth2Login initiates the OAuth2 authorization code flow.
//
//	@Summary		Initiate OAuth2 login
//	@Description	Redirects to OAuth2 provider for authentication
//	@Tags			Auth
//	@Success		302	"Redirect to OAuth2 provider"
//	@Failure		403	{object}	dto.ErrorResponse	"OAuth2 disabled"
//	@Failure		500	{object}	dto.ErrorResponse
//	@Router			/auth/login/oauth2 [get]
func (h *AuthHandlers) handleOAuth2Login(w http.ResponseWriter, r *http.Request) {
	if !h.oauth2Enabled {
		httputil.RespondForbidden(w, h.log, "OAuth2 authentication is disabled")
		return
	}

	if h.service == nil {
		httputil.RespondInternalError(w, h.log, "OAuth2 service not configured")
		return
	}

	url, state, err := h.service.Login(r.Context())
	if err != nil {
		h.log.Error("Failed to initiate OAuth2 login", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to initiate login")
		return
	}

	// Store state for CSRF protection
	if err := h.stateStore.Save(state); err != nil {
		h.log.Error("Failed to save state", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to initiate login")
		return
	}

	h.log.Info("Redirecting to OAuth2 login",
		logger.Str("state", state[:8]+"..."))

	http.Redirect(w, r, url, http.StatusFound)
}

// handleCallback handles the OAuth2 callback from the identity provider.
//
//	@Summary		OAuth2 callback
//	@Description	Handles callback from OAuth2 provider after authentication
//	@Tags			Auth
//	@Param			code	query	string	true	"Authorization code"
//	@Param			state	query	string	true	"CSRF state parameter"
//	@Success		302		"Redirect to home page"
//	@Failure		400		{object}	dto.ErrorResponse	"Invalid parameters"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Router			/auth/callback [get]
func (h *AuthHandlers) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.log.Warn("Missing code or state parameter")
		httputil.RespondBadRequest(w, h.log, "Invalid callback parameters")
		return
	}

	// Validate state to prevent CSRF
	if err := h.stateStore.Validate(state); err != nil {
		h.log.Warn("Invalid state parameter",
			logger.Err(err),
			logger.Str("state", state[:8]+"..."))
		httputil.RespondBadRequest(w, h.log, "Invalid state parameter")
		return
	}

	if h.service == nil {
		httputil.RespondInternalError(w, h.log, "OAuth2 service not configured")
		return
	}

	// Exchange authorization code for tokens using provider directly
	token, err := h.provider.Exchange(r.Context(), code)
	if err != nil {
		h.log.Error("Failed to exchange code for token", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Authentication failed")
		return
	}

	// Extract and verify ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		h.log.Error("No id_token in token response")
		httputil.RespondInternalError(w, h.log, "Authentication failed")
		return
	}

	// Verify ID token and extract user info
	userInfo, err := h.provider.VerifyIDToken(r.Context(), rawIDToken)
	if err != nil {
		h.log.Error("Failed to verify ID token", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Authentication failed")
		return
	}

	// Create/update user in database via service
	user, err := h.service.FindOrCreateUser(r.Context(), userInfo)
	if err != nil {
		// A deactivated or blocked account is refused on purpose. Reporting it
		// as a server fault would hide a security decision behind a fake error:
		// nobody chasing "Internal server error" would think to check the
		// user's status. errors.As rather than a type assertion so a future
		// wrap upstream cannot silently turn this back into a 500.
		var authErr *auth.AuthError
		if errors.As(err, &authErr) && authErr.Code == auth.ErrCodeUnauthorized {
			h.log.Warn("OAuth2 sign-in refused",
				logger.Str("subject", userInfo.Subject),
				logger.Err(err))
			httputil.RespondUnauthorized(w, h.log, authErr.Message)
			return
		}

		h.log.Error("Failed to create user in database", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Internal server error")
		return
	}

	// Ensure admin role for configured users
	if h.oauth2AdminSvc != nil && user != nil {
		if adminErr := h.oauth2AdminSvc.EnsureAdminRole(r.Context(), user); adminErr != nil {
			// Log error but don't fail authentication
			h.log.Error("Failed to assign admin role to OAuth2 user",
				logger.Uint("user_id", user.ID),
				logger.Str("email", user.Email),
				logger.Err(adminErr))
		}
	}

	// Create session using gorilla/sessions (same as simple auth)
	if h.sessionStore != nil {
		cookieSession, err := h.sessionStore.Get(r, h.sessionName)
		if err != nil {
			h.log.Warn("Failed to get session, creating new one", logger.Err(err))
		}

		// Generate unique session ID for blacklist tracking
		sessionID := generateSessionID()

		// Store only essential user info in session (same as old implementation)
		cookieSession.Values["authenticated"] = true
		cookieSession.Values["auth_method"] = "oauth2"
		cookieSession.Values["session_id"] = sessionID
		cookieSession.Values["subject"] = userInfo.Subject
		cookieSession.Values["email"] = userInfo.Email
		cookieSession.Values["name"] = userInfo.Name
		cookieSession.Values["created_at"] = time.Now().Unix()

		if err := h.sessionStore.Save(r, w, cookieSession); err != nil {
			h.log.Error("Failed to save session", logger.Err(err))
			httputil.RespondInternalError(w, h.log, "Session error")
			return
		}
	}

	h.log.Info("User authenticated successfully via OAuth2",
		logger.Str("subject", userInfo.Subject),
		logger.Str("email", userInfo.Email))

	http.Redirect(w, r, "/", http.StatusFound)
}

// handleLogout logs out the user.
//
//	@Summary		Logout
//	@Description	Clears the user session and logs out
//	@Tags			Auth
//	@Success		302	"Redirect to home page"
//	@Failure		500	{object}	dto.ErrorResponse
//	@Router			/auth/logout [get]
func (h *AuthHandlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Get session
	session, err := h.sessionStore.Get(r, h.sessionName)
	if err != nil {
		// Session doesn't exist, just redirect
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Get user info for logging
	var email string
	if e, ok := session.Values["email"].(string); ok {
		email = e
	}

	// A logout is worth being able to find again, so the line below names the
	// session it ended. The session carries no user id — only the subject,
	// address and name, which identify the person rather than the account — so
	// the truncated session id is what ties this to the sign-in that created it
	// and to the revocation just below.
	var sessionRef string
	if sessionID, ok := session.Values["session_id"].(string); ok && len(sessionID) >= 8 {
		sessionRef = sessionID[:8] + "..."
	}

	// Add session ID to blacklist to prevent reuse (server-side revocation)
	if h.sessionBlacklist != nil {
		if sessionID, ok := session.Values["session_id"].(string); ok && sessionID != "" {
			h.sessionBlacklist.Revoke(sessionID)
			h.log.Debug("Session revoked and added to blacklist",
				logger.Str("session_id", sessionRef),
				logger.Str("email", email))
		}
	}

	// Clear session cookie
	session.Options.MaxAge = -1 // Delete cookie
	if err := h.sessionStore.Save(r, w, session); err != nil {
		h.log.Error("Failed to clear session", logger.Err(err))
		http.Error(w, "Logout failed", http.StatusInternalServerError)
		return
	}

	h.log.Info("User logged out",
		logger.Str("session_id", sessionRef),
		logger.Str("email", email))

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleUserInfo returns the current user's information.
//
//	@Summary		Get current user info
//	@Description	Returns information about the currently authenticated user
//	@Tags			Auth
//	@Security		BearerAuth
//	@Produce		json
//	@Success		200	{object}	chi.UserDetails
//	@Failure		401	{object}	dto.ErrorResponse
//	@Router			/auth/userinfo [get]
func (h *AuthHandlers) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		httputil.RespondUnauthorized(w, h.log, "Not authenticated")
		return
	}

	httputil.RespondSuccess(w, h.log, user)
}

// handleRefresh refreshes an expired access token.
//
//	@Summary		Refresh access token
//	@Description	Refreshes an expired access token using the refresh token
//	@Tags			Auth
//	@Security		BearerAuth
//	@Produce		json
//	@Success		200	{object}	object{status=string,message=string,expiry=string}
//	@Failure		400	{object}	dto.ErrorResponse	"Token refresh not available"
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		500	{object}	dto.ErrorResponse
//	@Router			/auth/refresh [post]
func (h *AuthHandlers) handleRefresh(w http.ResponseWriter, r *http.Request) {
	// Get session using gorilla/sessions
	session, err := h.sessionStore.Get(r, h.sessionName)
	if err != nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get refresh token from session
	refreshToken := getSessionString(session, "refresh_token")
	if refreshToken == "" {
		h.log.Warn("Token refresh not available - tokens not stored in session to avoid cookie size limit")
		http.Error(w, "Token refresh not available. Tokens are not stored in session to avoid exceeding cookie size limit. Please re-authenticate.", http.StatusBadRequest)
		return
	}

	// Check if provider is configured
	if h.provider == nil {
		http.Error(w, "OAuth2 provider not configured", http.StatusInternalServerError)
		return
	}

	// Refresh the token using the provider
	newToken, err := h.provider.RefreshToken(r.Context(), refreshToken)
	if err != nil {
		h.log.Error("Failed to refresh token", logger.Err(err))
		http.Error(w, "Token refresh failed", http.StatusInternalServerError)
		return
	}

	// Update session with new tokens
	session.Values["access_token"] = newToken.AccessToken
	if newToken.RefreshToken != "" {
		session.Values["refresh_token"] = newToken.RefreshToken
	}
	session.Values["token_expiry"] = newToken.Expiry.Unix()

	// Update ID token if present
	if rawIDToken, ok := newToken.Extra("id_token").(string); ok {
		session.Values["id_token"] = rawIDToken
	}

	// Save session
	if err := h.sessionStore.Save(r, w, session); err != nil {
		h.log.Error("Failed to save session", logger.Err(err))
		http.Error(w, "Session save error", http.StatusInternalServerError)
		return
	}

	h.log.Info("Token refreshed successfully",
		logger.Str("user_id", getSessionString(session, "subject")))

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"status":  "success",
		"message": "Token refreshed successfully",
		"expiry":  newToken.Expiry,
	})
}

// handleChangePassword handles password change requests.
//
//	@Summary		Change password
//	@Description	Change the password for a user account
//	@Tags			Auth
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		requests.ChangePassword	true	"Password change request"
//	@Success		200		{object}	object{success=bool,message=string}
//	@Failure		400		{object}	object{success=bool,message=string}
//	@Failure		401		{object}	object{success=bool,message=string}
//	@Failure		403		{object}	dto.ErrorResponse	"Simple auth disabled"
//	@Failure		500		{object}	dto.ErrorResponse
//	@Router			/auth/password/change [post]
func (h *AuthHandlers) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if !h.simpleEnabled {
		httputil.RespondForbidden(w, h.log, "Simple authentication is disabled")
		return
	}

	if h.simpleAuth == nil {
		httputil.RespondInternalError(w, h.log, "Simple auth service not configured")
		return
	}

	req, ok := httputil.ValidateAndParse[requests.ChangePassword](w, r, h.log)
	if !ok {
		return
	}

	err := h.simpleAuth.ChangePasswordByUsername(r.Context(), req.Username, req.OldPassword, req.NewPassword)
	if err != nil {
		h.log.Warn("Password change failed",
			logger.Str("username", req.Username),
			logger.Err(err))

		// Determine status code based on error type
		statusCode := http.StatusBadRequest
		if authErr, ok := err.(*auth.AuthError); ok {
			if authErr.Code == auth.ErrCodeUnauthorized {
				statusCode = http.StatusUnauthorized
			}
		}

		httputil.RespondJSON(w, h.log, statusCode, map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	h.log.Info("Password changed successfully",
		logger.Str("username", req.Username))

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"success": true,
		"message": "Password changed successfully",
	})
}

// handlePasswordRequirements returns password requirements.
//
//	@Summary		Get password requirements
//	@Description	Returns the password policy requirements
//	@Tags			Auth
//	@Produce		json
//	@Success		200	{object}	object{description=string,min_length=int}
//	@Router			/auth/password/requirements [get]
func (h *AuthHandlers) handlePasswordRequirements(w http.ResponseWriter, r *http.Request) {
	var requirements auth.PasswordRequirements
	if h.simpleAuth != nil {
		requirements = h.simpleAuth.GetPasswordRequirements()
	} else {
		requirements = auth.DefaultPasswordRequirements()
	}

	description := auth.GetPasswordRequirementsDescription(requirements)

	httputil.RespondSuccess(w, h.log, map[string]interface{}{
		"description": description,
		"min_length":  requirements.MinLength,
	})
}

// sessionStoreAdapter adapts auth.CookieSessionManager to SessionStore interface.
type sessionStoreAdapter struct {
	manager *auth.CookieSessionManager
}

// NewSessionStoreAdapter creates a new adapter for CookieSessionManager.
func NewSessionStoreAdapter(manager *auth.CookieSessionManager) SessionStore {
	return &sessionStoreAdapter{manager: manager}
}

// Get retrieves a session by name.
func (a *sessionStoreAdapter) Get(r *http.Request, name string) (*sessions.Session, error) {
	return a.manager.GetStore().Get(r, name)
}

// Save saves the session.
func (a *sessionStoreAdapter) Save(r *http.Request, w http.ResponseWriter, s *sessions.Session) error {
	return s.Save(r, w)
}

// Ensure AuthHandlers implements RouteRegistrar
var _ RouteRegistrar = (*AuthHandlers)(nil)
