package chi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"

	"github.com/DefensePoint/keycloak-monitoring/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/rbac"
)

// Context keys for request context.
type contextKey string

const (
	// TenantIDKey is the context key for tenant ID.
	TenantIDKey contextKey = "tenant_id"
	// UserInfoKey is the context key for authenticated user info.
	UserInfoKey contextKey = "user_info"
)

// LoggingMiddleware logs HTTP requests with timing information.
func LoggingMiddleware(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create a response writer wrapper to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			log.Info("HTTP request",
				logger.Str("method", r.Method),
				logger.Str("path", r.URL.Path),
				logger.Int("status", wrapped.statusCode),
				logger.Dur("duration", duration),
				logger.Str("remote_addr", r.RemoteAddr),
			)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// RecoveryMiddleware recovers from panics and returns a 500 error.
func RecoveryMiddleware(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error("Panic recovered",
						logger.Any("panic", err),
						logger.Str("path", r.URL.Path),
						logger.Str("method", r.Method),
					)
					httputil.RespondInternalError(w, log, "Internal server error")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders defines the response security headers applied by
// SecurityHeadersMiddleware. Empty values skip the corresponding header.
type SecurityHeaders struct {
	HSTS                  string
	ContentSecurityPolicy string
	XContentTypeOptions   string
	XFrameOptions         string
	ReferrerPolicy        string
}

// SecurityHeadersMiddleware adds browser-enforced security headers to every
// response (HSTS, CSP, anti-MIME-sniffing, anti-clickjacking, referrer policy).
// Headers are only set when their value is non-empty so deployments can
// selectively enable/disable each one via configuration.
func SecurityHeadersMiddleware(headers SecurityHeaders) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			if headers.HSTS != "" {
				h.Set("Strict-Transport-Security", headers.HSTS)
			}
			if headers.ContentSecurityPolicy != "" {
				h.Set("Content-Security-Policy", headers.ContentSecurityPolicy)
			}
			if headers.XContentTypeOptions != "" {
				h.Set("X-Content-Type-Options", headers.XContentTypeOptions)
			}
			if headers.XFrameOptions != "" {
				h.Set("X-Frame-Options", headers.XFrameOptions)
			}
			if headers.ReferrerPolicy != "" {
				h.Set("Referrer-Policy", headers.ReferrerPolicy)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware adds CORS headers to responses.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get origin from request
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		// Allow credentials requires specific origin (not wildcard)
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// UserLookupService provides user lookup functionality.
type UserLookupService interface {
	GetUserBySubject(ctx context.Context, subject string) (*UserDetails, error)
}

// TenantLookupService provides tenant lookup functionality.
type TenantLookupService interface {
	GetDefaultTenantID(ctx context.Context) (string, error)
}

// UserDetails represents authenticated user details for middleware.
type UserDetails struct {
	ID                 uint      `json:"ID"`
	Subject            string    `json:"Subject"`
	Email              string    `json:"Email"`
	EmailVerified      bool      `json:"EmailVerified"`
	Name               string    `json:"Name"`
	GivenName          string    `json:"GivenName"`
	FamilyName         string    `json:"FamilyName"`
	PreferredUsername  string    `json:"PreferredUsername"`
	Locale             string    `json:"Locale"`
	UpdatedAt          time.Time `json:"UpdatedAt"`
	MustChangePassword bool      `json:"MustChangePassword"`
}

// AuthMiddleware provides authentication middleware for HTTP handlers.
// Supports both session-based (cookie) and token-based (Bearer) authentication.
type AuthMiddleware struct {
	sessionManager   *auth.CookieSessionManager
	authService      auth.Service
	userLookup       UserLookupService
	tenantService    TenantLookupService
	sessionBlacklist *auth.SessionBlacklist
	log              *logger.Logger
	publicPaths      map[string]bool
}

// NewAuthMiddleware creates a new authentication middleware.
func NewAuthMiddleware(
	sessionManager *auth.CookieSessionManager,
	authService auth.Service,
	userLookup UserLookupService,
	tenantService TenantLookupService,
	log *logger.Logger,
) *AuthMiddleware {
	return &AuthMiddleware{
		sessionManager:   sessionManager,
		authService:      authService,
		userLookup:       userLookup,
		tenantService:    tenantService,
		sessionBlacklist: nil, // Set via SetSessionBlacklist
		log:              log,
		publicPaths: map[string]bool{
			"/health":                     true,
			"/ready":                      true,
			"/auth/config":                true,
			"/auth/login":                 true,
			"/auth/login/simple":          true,
			"/auth/login/oauth2":          true,
			"/auth/callback":              true,
			"/auth/logout":                true,
			"/auth/password/requirements": true,
		},
	}
}

// SetSessionBlacklist sets the session blacklist for revoked session checking.
func (m *AuthMiddleware) SetSessionBlacklist(bl *auth.SessionBlacklist) {
	m.sessionBlacklist = bl
}

// GetSessionBlacklist returns the session blacklist.
func (m *AuthMiddleware) GetSessionBlacklist() *auth.SessionBlacklist {
	return m.sessionBlacklist
}

// Authenticate returns a middleware that validates authentication.
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if path is public
		if m.publicPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()

		// Try session-based auth first (cookie)
		userDetails, err := m.authenticateFromSession(r)
		if err == nil && userDetails != nil {
			ctx = context.WithValue(ctx, UserInfoKey, userDetails)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Try token-based auth (Bearer token) if OAuth2 service is available
		if m.authService != nil {
			userDetails, err = m.authenticateFromToken(r)
			if err == nil && userDetails != nil {
				ctx = context.WithValue(ctx, UserInfoKey, userDetails)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Authentication failed
		m.log.Debug("Authentication failed",
			logger.Str("path", r.URL.Path),
			logger.Str("method", r.Method))
		httputil.RespondUnauthorized(w, m.log, "Authentication required")
	})
}

// authenticateFromSession authenticates a user from an HTTP session cookie.
func (m *AuthMiddleware) authenticateFromSession(r *http.Request) (*UserDetails, error) {
	if m.sessionManager == nil {
		return nil, &authError{message: "session manager not configured"}
	}

	session, err := m.sessionManager.GetStore().Get(r, m.sessionManager.GetSessionName())
	if err != nil {
		return nil, &authError{message: "invalid session", err: err}
	}

	// Check if user is authenticated
	authenticated, ok := session.Values["authenticated"].(bool)
	if !ok || !authenticated {
		return nil, &authError{message: "not authenticated"}
	}

	// Check if session has been revoked (blacklisted on logout)
	sessionID := getSessionString(session, "session_id")
	if sessionID != "" && m.sessionBlacklist != nil && m.sessionBlacklist.IsRevoked(sessionID) {
		return nil, &authError{message: "session has been revoked"}
	}

	// Check if session has expired (server-side validation)
	createdAt, ok := session.Values["created_at"].(int64)
	if ok {
		maxAge := int64(m.sessionManager.GetMaxAge())
		if maxAge > 0 && time.Now().Unix()-createdAt > maxAge {
			return nil, &authError{message: "session expired"}
		}
	}

	// Extract essential user info from session
	subject := getSessionString(session, "subject")
	email := getSessionString(session, "email")

	userDetails := &UserDetails{
		Subject: subject,
		Email:   email,
		Name:    getSessionString(session, "name"),
	}

	// Fetch full user details from database (including ID for RBAC)
	if m.userLookup != nil && subject != "" {
		fullDetails, err := m.userLookup.GetUserBySubject(r.Context(), subject)
		if err == nil && fullDetails != nil {
			userDetails = fullDetails
		}
	}

	return userDetails, nil
}

// authenticateFromToken authenticates a user from a Bearer token.
func (m *AuthMiddleware) authenticateFromToken(r *http.Request) (*UserDetails, error) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, &authError{message: "no bearer token"}
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return nil, &authError{message: "empty token"}
	}

	// Validate token using auth service
	userInfo, err := m.authService.ValidateSession(r.Context(), token)
	if err != nil {
		return nil, &authError{message: "invalid token", err: err}
	}

	userDetails := &UserDetails{
		ID:                 userInfo.ID,
		Subject:            userInfo.Subject,
		Email:              userInfo.Email,
		EmailVerified:      userInfo.EmailVerified,
		Name:               userInfo.Name,
		GivenName:          userInfo.GivenName,
		FamilyName:         userInfo.FamilyName,
		PreferredUsername:  userInfo.PreferredUsername,
		Locale:             userInfo.Locale,
		UpdatedAt:          userInfo.UpdatedAt,
		MustChangePassword: userInfo.MustChangePassword,
	}

	// Try to get full user details from database (may have more up-to-date info)
	if m.userLookup != nil {
		fullDetails, err := m.userLookup.GetUserBySubject(r.Context(), userInfo.Subject)
		if err == nil && fullDetails != nil {
			userDetails = fullDetails
		}
	}

	return userDetails, nil
}

// getSessionString safely extracts a string value from session.
func getSessionString(session *sessions.Session, key string) string {
	if val, ok := session.Values[key].(string); ok {
		return val
	}
	return ""
}

// authError is a simple error type for authentication errors.
type authError struct {
	message string
	err     error
}

func (e *authError) Error() string {
	if e.err != nil {
		return e.message + ": " + e.err.Error()
	}
	return e.message
}

// RequireTenant returns a middleware that ensures a tenant ID is present.
func (m *AuthMiddleware) RequireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Try to get tenant from URL param (Chi style)
		tenantID := chi.URLParam(r, "tenantID")

		// If not in URL, try query parameter
		if tenantID == "" {
			tenantID = r.URL.Query().Get("tenant_id")
		}

		// If not in query, try to get from header
		if tenantID == "" {
			tenantID = r.Header.Get("X-Tenant-ID")
		}

		// If still empty, try to get default tenant
		if tenantID == "" && m.tenantService != nil {
			defaultID, err := m.tenantService.GetDefaultTenantID(ctx)
			if err == nil && defaultID != "" {
				tenantID = defaultID
			}
		}

		if tenantID == "" {
			httputil.RespondBadRequest(w, m.log, "tenant_id is required")
			return
		}

		// Add tenant ID to context
		ctx = context.WithValue(ctx, TenantIDKey, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserFromContext extracts user details from context.
func GetUserFromContext(ctx context.Context) *UserDetails {
	if user, ok := ctx.Value(UserInfoKey).(*UserDetails); ok {
		return user
	}
	return nil
}

// GetTenantFromContext extracts tenant ID from context.
func GetTenantFromContext(ctx context.Context) string {
	if tenantID, ok := ctx.Value(TenantIDKey).(string); ok {
		return tenantID
	}
	return ""
}

// RBACChecker provides RBAC verification for HTTP handlers.
// This is the single interface all handlers use for permission checks.
type RBACChecker interface {
	HasPermission(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error)
	GetUserPermissions(ctx context.Context, userID uint, tenantID *string) ([]string, error)
	IsAdmin(ctx context.Context, userID uint) (bool, error)
	GetUserRoles(ctx context.Context, userID uint) ([]*UserRoleInfo, error)
	HasAccessToTenant(ctx context.Context, userID uint, tenantID string) (bool, error)
	HasAccessToRealm(ctx context.Context, userID uint, tenantID, realmName string) (bool, error)
	GetUserPolicies(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error)
}

// RBACService is an alias for RBACChecker for backward compatibility.
// Deprecated: Use RBACChecker instead.
type RBACService = RBACChecker

// UserRoleInfo represents minimal user role information needed by HTTP layer.
type UserRoleInfo struct {
	RoleID   uint
	RoleName string
	TenantID *string
}

// allowedRealms resolves the realms a caller may read within a tenant. A
// platform administrator, and a caller with no policy row for the tenant, are
// unrestricted and get all = true. Callers must treat all = false with an empty
// slice as "no realm", never as "no restriction".
func allowedRealms(ctx context.Context, svc RBACChecker, userID uint, tenantID string) (realms []string, all bool, err error) {
	realms, all, _, err = allowedRealmsWithAdmin(ctx, svc, userID, tenantID)
	return realms, all, err
}

// allowedRealmsWithAdmin is allowedRealms plus the administrator check it
// already ran. isAdmin is not the same answer as all: an unrestricted policy is
// not administrator rights.
func allowedRealmsWithAdmin(ctx context.Context, svc RBACChecker, userID uint, tenantID string) (realms []string, all, isAdmin bool, err error) {
	if svc == nil {
		return nil, false, false, errors.New("rbac service not configured")
	}

	isAdmin, err = svc.IsAdmin(ctx, userID)
	if err != nil {
		return nil, false, false, err
	}
	if isAdmin {
		return nil, true, true, nil
	}

	policies, err := svc.GetUserPolicies(ctx, userID)
	if err != nil {
		return nil, false, false, err
	}

	// Shared with the MCP authorizer, which reads the same rows over a
	// different transport and must reach the same answer.
	realms, all = rbac.ResolveRealmScope(policies, tenantID)
	if all {
		return nil, true, false, nil
	}

	return realms, false, false, nil
}

// RBACMiddleware provides HTTP middleware for RBAC authorization.
type RBACMiddleware struct {
	service RBACChecker
	log     *logger.Logger
}

// NewRBACMiddleware creates a new RBAC middleware.
func NewRBACMiddleware(service RBACChecker, log *logger.Logger) *RBACMiddleware {
	return &RBACMiddleware{
		service: service,
		log:     log,
	}
}

// RequirePermission returns a middleware that checks if the user has the required permission.
func (m *RBACMiddleware) RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			user := GetUserFromContext(ctx)
			if user == nil {
				m.log.Warn("No user info in context for permission check")
				httputil.RespondUnauthorized(w, m.log, "Unauthorized")
				return
			}

			var tenantID *string
			if tid := GetTenantFromContext(ctx); tid != "" {
				tenantID = &tid
			}

			// ADMIN BYPASS: Admins can do anything
			isAdmin, adminErr := m.service.IsAdmin(ctx, user.ID)
			if adminErr == nil && isAdmin {
				m.log.Debug("Admin bypass - granting all permissions",
					logger.Uint("user_id", user.ID),
					logger.Str("email", user.Email),
					logger.Str("permission", permission))
				next.ServeHTTP(w, r)
				return
			}

			// Check permission
			hasPermission, err := m.service.HasPermission(ctx, user.ID, permission, tenantID)
			if err != nil {
				m.log.Error("Failed to check permission",
					logger.Err(err),
					logger.Uint("user_id", user.ID),
					logger.Str("permission", permission))
				httputil.RespondInternalError(w, m.log, "Failed to check permissions")
				return
			}

			if !hasPermission {
				m.log.Warn("Permission denied",
					logger.Uint("user_id", user.ID),
					logger.Str("email", user.Email),
					logger.Str("permission", permission))
				http.Error(w, `{"error":"`+httputil.ErrPermissionDenied+`"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission returns a middleware that checks if the user has any of the required permissions.
func (m *RBACMiddleware) RequireAnyPermission(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			user := GetUserFromContext(ctx)
			if user == nil {
				m.log.Warn("No user info in context for permission check")
				httputil.RespondUnauthorized(w, m.log, "Unauthorized")
				return
			}

			var tenantID *string
			if tid := GetTenantFromContext(ctx); tid != "" {
				tenantID = &tid
			}

			// ADMIN BYPASS
			isAdmin, adminErr := m.service.IsAdmin(ctx, user.ID)
			if adminErr == nil && isAdmin {
				next.ServeHTTP(w, r)
				return
			}

			// Check if user has any of the required permissions
			for _, permission := range permissions {
				hasPermission, err := m.service.HasPermission(ctx, user.ID, permission, tenantID)
				if err != nil {
					m.log.Error("Failed to check permission",
						logger.Err(err),
						logger.Uint("user_id", user.ID),
						logger.Str("permission", permission))
					continue
				}

				if hasPermission {
					next.ServeHTTP(w, r)
					return
				}
			}

			m.log.Warn("Permission denied - no matching permissions",
				logger.Uint("user_id", user.ID),
				logger.Str("email", user.Email))
			http.Error(w, `{"error":"`+httputil.ErrPermissionDenied+`"}`, http.StatusForbidden)
		})
	}
}

// RequireAdmin returns a middleware that requires admin role.
func (m *RBACMiddleware) RequireAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			user := GetUserFromContext(ctx)
			if user == nil {
				m.log.Warn("No user info in context for admin check")
				httputil.RespondUnauthorized(w, m.log, "Unauthorized")
				return
			}

			isAdmin, err := m.service.IsAdmin(ctx, user.ID)
			if err != nil {
				m.log.Error("Failed to check admin status",
					logger.Err(err),
					logger.Uint("user_id", user.ID))
				httputil.RespondInternalError(w, m.log, "Failed to verify admin status")
				return
			}

			if !isAdmin {
				m.log.Warn("Admin access denied",
					logger.Uint("user_id", user.ID),
					logger.Str("email", user.Email))
				httputil.RespondForbidden(w, m.log, "Admin access required")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireTenantAccess returns a middleware that requires access to the current tenant.
func (m *RBACMiddleware) RequireTenantAccess() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			user := GetUserFromContext(ctx)
			if user == nil {
				httputil.RespondUnauthorized(w, m.log, "Unauthorized")
				return
			}

			tenantID := GetTenantFromContext(ctx)
			if tenantID == "" {
				httputil.RespondBadRequest(w, m.log, "Tenant ID is required")
				return
			}

			// Check tenants:read permission (matching legacy behavior)
			hasPermission, err := m.service.HasPermission(ctx, user.ID, rbac.PermissionTenantsRead, &tenantID)
			if err != nil {
				m.log.Error("Failed to check tenant permission",
					logger.Err(err),
					logger.Uint("user_id", user.ID),
					logger.Str("tenant_id", tenantID))
				httputil.RespondInternalError(w, m.log, "Failed to verify tenant access")
				return
			}

			if !hasPermission {
				m.log.Warn("Tenant access denied",
					logger.Uint("user_id", user.ID),
					logger.Str("tenant_id", tenantID))
				http.Error(w, `{"error":"Access to tenant denied"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TenantIDMiddleware extracts tenant ID from URL and adds to context.
func TenantIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		if tenantID != "" {
			ctx := context.WithValue(r.Context(), TenantIDKey, tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		next.ServeHTTP(w, r)
	})
}
