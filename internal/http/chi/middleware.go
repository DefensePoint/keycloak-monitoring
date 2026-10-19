package chi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/sessions"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
)

// Context keys for request context.
type contextKey string

const (
	// TenantIDKey is the context key for tenant ID.
	TenantIDKey contextKey = "tenant_id"
	// UserInfoKey is the context key for authenticated user info.
	UserInfoKey contextKey = "user_info"
)

func ClientIPMiddleware(trusted []string) (func(http.Handler) http.Handler, error) {
	cidrs := make([]string, 0, len(trusted))
	for i, c := range trusted {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, err := netip.ParsePrefix(c); err != nil {
			hint := ""
			if ip, ipErr := netip.ParseAddr(c); ipErr == nil {
				bits := 32
				if ip.Is6() {
					bits = 128
				}
				hint = fmt.Sprintf(" (did you mean %s/%d?)", ip, bits)
			}
			return nil, fmt.Errorf("trusted_proxies[%d] %q is not a CIDR%s", i, c, hint)
		}
		cidrs = append(cidrs, c)
	}
	if len(cidrs) == 0 {
		return chimw.ClientIPFromRemoteAddr, nil
	}
	return chimw.ClientIPFromXFF(cidrs...), nil
}

func clientIP(r *http.Request) string {
	if ip := chimw.GetClientIP(r.Context()); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

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
				logger.Str("remote_addr", clientIP(r)),
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

// CrossOriginMiddleware refuses state-changing browser requests from any
// origin other than the API's own or one listed in allowedOrigins, and
// grants credentialed CORS only to allowedOrigins.
func CrossOriginMiddleware(allowedOrigins []string, log *logger.Logger) (func(http.Handler) http.Handler, error) {
	protection := http.NewCrossOriginProtection()
	allowed := make(map[string]bool, len(allowedOrigins))
	var origins []string
	for _, entry := range allowedOrigins {
		origins = append(origins, strings.FieldsFunc(entry, func(r rune) bool {
			return r == ',' || unicode.IsSpace(r)
		})...)
	}
	for _, entry := range origins {
		if strings.Contains(entry, "*") {
			return nil, fmt.Errorf("http.cors.allowed_origins: %q: wildcards are not allowed, list each origin", entry)
		}
		origin, err := normalizeOrigin(entry)
		if err != nil {
			return nil, fmt.Errorf("http.cors.allowed_origins: %w", err)
		}
		if err := protection.AddTrustedOrigin(origin); err != nil {
			return nil, fmt.Errorf("http.cors.allowed_origins: %w", err)
		}
		if origin != entry {
			log.Info("Allowed origin normalized to the form browsers send",
				logger.Str("configured", entry), logger.Str("origin", origin))
		}
		allowed[origin] = true
	}
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Warn("Cross-origin request refused",
			logger.Str("method", r.Method),
			logger.Str("path", r.URL.Path),
			logger.Str("origin", r.Header.Get("Origin")),
			logger.Str("sec_fetch_site", r.Header.Get("Sec-Fetch-Site")))
		httputil.RespondForbidden(w, log, "Cross-origin request refused")
	}))

	return func(next http.Handler) http.Handler {
		protected := protection.Handler(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Origin")
			origin := r.Header.Get("Origin")
			if !allowed[origin] {
				protected.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			protected.ServeHTTP(w, r)
		})
	}, nil
}

// normalizeOrigin returns origin in the form a browser's Origin header takes,
// which both allowlists compare as an exact string: lowercase scheme and host,
// no default port. It rejects what a browser never sends: a scheme other than
// http or https, user info, or a non-ASCII host (browsers send punycode).
func normalizeOrigin(origin string) (string, error) {
	u, err := url.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("invalid origin %q: %w", origin, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("invalid origin %q: scheme must be http or https", origin)
	}
	if u.User != nil {
		return "", fmt.Errorf("invalid origin %q: user info is not allowed", origin)
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("invalid origin %q: path, query, and fragment are not allowed", origin)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("invalid origin %q: host is required", origin)
	}
	for _, r := range host {
		if r > unicode.MaxASCII {
			return "", fmt.Errorf("invalid origin %q: use the punycode (xn--) form of the host", origin)
		}
	}
	port := u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return scheme + "://" + host, nil
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
	// SessionEpoch counts the user's logouts; a session cookie carrying an
	// older value is rejected.
	SessionEpoch int64 `json:"-"`
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
			// The epoch lives in the database, so a logout made through any
			// server instance, or before a restart, is honored here.
			if sessionEpoch(session) < fullDetails.SessionEpoch {
				return nil, &authError{message: "session has been revoked"}
			}
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
	HasGlobalPermission(ctx context.Context, userID uint, permission string) (bool, error)
	GetUserPermissions(ctx context.Context, userID uint, tenantID *string) ([]string, error)
	IsAdmin(ctx context.Context, userID uint) (bool, error)
	GetEffectiveUserRoles(ctx context.Context, userID uint) ([]*UserRoleInfo, error)
	// HoldsAnyRole reports whether the user has any role assignment at all,
	// expired ones and those of a deactivated role included: an
	// administrator can make those grant again.
	HoldsAnyRole(ctx context.Context, userID uint) (bool, error)
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

// RequireGlobalPermission is RequirePermission for a platform-wide resource,
// such as the platform user accounts. Only a grant that applies to every
// tenant counts: a route without a tenant checks with a nil tenant, and
// HasPermission then accepts a grant made for any single tenant.
func (m *RBACMiddleware) RequireGlobalPermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			user := GetUserFromContext(ctx)
			if user == nil {
				m.log.Warn("No user info in context for permission check")
				httputil.RespondUnauthorized(w, m.log, "Unauthorized")
				return
			}

			isAdmin, adminErr := m.service.IsAdmin(ctx, user.ID)
			if adminErr == nil && isAdmin {
				next.ServeHTTP(w, r)
				return
			}

			hasPermission, err := m.service.HasGlobalPermission(ctx, user.ID, permission)
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

// sessionEpoch returns the session epoch the login stored in the cookie.
// Cookies from before logout was recorded per user carry none and count as 0,
// which stays valid until the user's first logout.
func sessionEpoch(session *sessions.Session) int64 {
	epoch, _ := session.Values["session_epoch"].(int64)
	return epoch
}
