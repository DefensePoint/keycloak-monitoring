package chi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"

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
	loginCookie      *auth.LoginCookie
	authMiddleware   func(http.Handler) http.Handler
	sessionBlacklist *auth.SessionBlacklist
	sessionRevoker   SessionRevoker
}

// SessionRevoker records logouts so that every server instance, and a
// restarted one, rejects the sessions a logout ended. A login stores the
// user's current epoch in the cookie; a logout moves it on.
type SessionRevoker interface {
	CurrentSessionEpoch(ctx context.Context, subject string) (int64, error)
	RevokeSessions(ctx context.Context, subject string, sessionEpoch int64) (bool, error)
}

// SetSessionRevoker sets where logouts are recorded.
func (h *AuthHandlers) SetSessionRevoker(r SessionRevoker) {
	h.sessionRevoker = r
}

// currentSessionEpoch is the value a new session cookie carries. Failing the
// login when it cannot be read is deliberate: a guessed value could either be
// rejected at once or outlive a logout.
func (h *AuthHandlers) currentSessionEpoch(ctx context.Context, subject string) (int64, error) {
	if h.sessionRevoker == nil {
		return 0, nil
	}
	return h.sessionRevoker.CurrentSessionEpoch(ctx, subject)
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
	loginCookie *auth.LoginCookie,
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
		loginCookie:      loginCookie,
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
//   - POST /auth/logout              - Logout (public; GET answers 405)
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
		r.Post("/logout", h.handleLogout)
		r.Get("/logout", h.handleLogoutGet)
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
		epoch, err := h.currentSessionEpoch(r.Context(), userInfo.Subject)
		if err != nil {
			h.log.Error("Failed to read the session epoch", logger.Err(err))
			httputil.RespondInternalError(w, h.log, "Failed to create session")
			return
		}
		session.Values["session_epoch"] = epoch

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

	url, attempt, err := h.service.Login(r.Context())
	if err != nil {
		h.log.Error("Failed to initiate OAuth2 login", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to initiate login")
		return
	}

	// Keep the attempt for the callback in the browser that started it: its
	// state for CSRF protection, its nonce and PKCE verifier to check what the
	// provider returns. Holding it in the browser rather than in this process
	// ties the callback to that browser, so a callback link captured from
	// someone else's login cannot sign in whoever opens it, and lets any API
	// instance complete the login.
	if h.loginCookie == nil {
		httputil.RespondInternalError(w, h.log, "OAuth2 login cookie not configured")
		return
	}
	if err := h.loginCookie.Save(w, r, attempt); err != nil {
		h.log.Error("Failed to save login cookie", logger.Err(err))
		httputil.RespondInternalError(w, h.log, "Failed to initiate login")
		return
	}

	h.log.Info("Redirecting to OAuth2 login",
		logger.Str("state", attempt.State[:8]+"..."))

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

	// The callback must come from the browser that started the login, and
	// that browser's cookie holds the login's nonce and PKCE verifier.
	if h.loginCookie == nil {
		httputil.RespondInternalError(w, h.log, "OAuth2 login cookie not configured")
		return
	}
	attempt, ok := h.loginCookie.Take(w, r, state)
	if !ok {
		h.log.Warn("OAuth2 callback was not started in this browser, or its login expired",
			logger.Str("state", logPrefix(state)))
		httputil.RespondBadRequest(w, h.log, "Invalid state parameter")
		return
	}

	if h.service == nil {
		httputil.RespondInternalError(w, h.log, "OAuth2 service not configured")
		return
	}

	// Exchange authorization code for tokens using provider directly. The PKCE
	// verifier proves this is the client that asked for the code.
	token, err := h.provider.Exchange(r.Context(), code, oauth2.VerifierOption(attempt.CodeVerifier))
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

	// Verify ID token, including the nonce this login sent, and extract user info
	userInfo, err := h.provider.VerifyLoginIDToken(r.Context(), rawIDToken, attempt.Nonce)
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
		epoch, err := h.currentSessionEpoch(r.Context(), userInfo.Subject)
		if err != nil {
			h.log.Error("Failed to read the session epoch", logger.Err(err))
			httputil.RespondInternalError(w, h.log, "Session error")
			return
		}
		cookieSession.Values["session_epoch"] = epoch

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

// handleLogoutGet refuses a GET logout. Logout used to be a GET, so any page
// could end the session just by linking to it, an image tag being enough;
// it is a POST now, which a cross-site page cannot send with the cookie.
//
//	@Summary		Logout with GET (refused)
//	@Description	Logout is POST /auth/logout. GET answers 405 and leaves the session alone.
//	@Tags			Auth
//	@Failure		405	{object}	dto.ErrorResponse
//	@Router			/auth/logout [get]
func (h *AuthHandlers) handleLogoutGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", http.MethodPost)
	httputil.RespondJSON(w, h.log, http.StatusMethodNotAllowed, map[string]string{
		"error": "logout is POST /auth/logout",
	})
}

// crossSiteRequest reports whether the request came from a page on another
// origin. Sec-Fetch-Site decides when the browser sends it: only
// "same-origin" and "none" (typed or bookmarked) pass. Chrome sends it only to
// secure origins, HTTPS or localhost, so over plain http the Origin header
// decides instead: its host and port must be the one the browser addressed,
// which behind the web server's proxy is X-Forwarded-Host, since the proxy
// changes Host. A page cannot set either header on a cross-site form post or
// no-cors fetch. A request with neither header, from a script or an old
// browser, is not refused; the session cookie's SameSite setting still
// covers old browsers sending it cross-site.
func crossSiteRequest(r *http.Request) bool {
	switch site := r.Header.Get("Sec-Fetch-Site"); site {
	case "same-origin", "none":
		return false
	case "":
	default:
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		// Includes "null", sent by sandboxed frames and some redirects.
		return true
	}
	addressed := r.Header.Get("X-Forwarded-Host")
	if addressed == "" {
		addressed = r.Host
	}
	// A proxy chain may list several hosts; the first is the client's.
	if i := strings.IndexByte(addressed, ','); i >= 0 {
		addressed = addressed[:i]
	}
	addressed = strings.TrimSpace(addressed)
	// Without a port to compare, compare the host names alone; with one,
	// compare both. A port-less host comes from a legacy nginx setup that
	// forwards $host, which drops the port; the deployment guide now
	// forwards $http_host, which keeps it.
	if _, _, err := net.SplitHostPort(addressed); err != nil {
		host := (&url.URL{Host: addressed}).Hostname()
		return !strings.EqualFold(u.Hostname(), host)
	}
	return !strings.EqualFold(u.Host, addressed)
}

// handleLogout logs out the user.
//
//	@Summary		Logout
//	@Description	Clears the user session and logs out. Refused when the request comes from a page on another origin (Sec-Fetch-Site, or Origin when the browser sends no Sec-Fetch-Site).
//	@Tags			Auth
//	@Success		200	{object}	map[string]bool	"Logged out"
//	@Failure		403	{object}	dto.ErrorResponse	"Request from another site"
//	@Failure		500	{object}	dto.ErrorResponse
//	@Router			/auth/logout [post]
func (h *AuthHandlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	if crossSiteRequest(r) {
		h.log.Warn("Refused a logout from another site",
			logger.Str("sec_fetch_site", r.Header.Get("Sec-Fetch-Site")),
			logger.Str("origin", r.Header.Get("Origin")))
		httputil.RespondJSON(w, h.log, http.StatusForbidden, map[string]string{
			"error": "logout from another site refused",
		})
		return
	}

	// Get session
	session, err := h.sessionStore.Get(r, h.sessionName)
	if err != nil {
		// No session to end: already logged out
		httputil.RespondJSON(w, h.log, http.StatusOK, map[string]bool{"success": true})
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

	// Record the logout in the database, so every instance and a restarted
	// one reject this user's sessions from before it. The in-memory
	// blacklist above still covers this instance if the write fails.
	if h.sessionRevoker != nil {
		if subject, ok := session.Values["subject"].(string); ok && subject != "" {
			moved, err := h.sessionRevoker.RevokeSessions(r.Context(), subject, sessionEpoch(session))
			switch {
			case err != nil:
				h.log.Error("Failed to record logout; other server instances keep accepting this session until it expires",
					logger.Str("session_id", sessionRef), logger.Err(err))
			case !moved:
				h.log.Info("Logout from a session already revoked; the user's newer sessions are left alone",
					logger.Str("session_id", sessionRef))
			}
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

	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]bool{"success": true})
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
	return a.manager.Save(r, w, s)
}

// Ensure AuthHandlers implements RouteRegistrar
var _ RouteRegistrar = (*AuthHandlers)(nil)
