package auth

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/sessions"
	"gorm.io/gorm"

	dbmodels "github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// Ensure Session can be serialized with gob (for gorilla/sessions).
func init() {
	gob.Register(&Session{})
	gob.Register(time.Time{})
}

// MemorySessionStore is an in-memory implementation of SessionStore.
type MemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	maxAge   time.Duration
}

// NewMemorySessionStore creates a new in-memory session store.
func NewMemorySessionStore(maxAge time.Duration) *MemorySessionStore {
	store := &MemorySessionStore{
		sessions: make(map[string]*Session),
		maxAge:   maxAge,
	}

	// Start cleanup goroutine
	go store.cleanupExpiredSessions()

	return store
}

// Save stores a session in memory.
func (s *MemorySessionStore) Save(ctx context.Context, sessionID string, session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session == nil {
		return &AuthError{
			Code:    "INVALID_SESSION",
			Message: "session cannot be nil",
		}
	}

	s.sessions[sessionID] = session
	return nil
}

// Get retrieves a session from memory.
func (s *MemorySessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, &AuthError{
			Code:    ErrCodeSessionNotFound,
			Message: "session not found",
		}
	}

	// Check if session has expired
	if time.Since(session.CreatedAt) > s.maxAge {
		return nil, &AuthError{
			Code:    ErrCodeSessionNotFound,
			Message: "session has expired",
		}
	}

	return session, nil
}

// Delete removes a session from memory.
func (s *MemorySessionStore) Delete(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionID)
	return nil
}

// UpdateLastAccessed updates the last accessed timestamp for a session.
func (s *MemorySessionStore) UpdateLastAccessed(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return &AuthError{
			Code:    ErrCodeSessionNotFound,
			Message: "session not found",
		}
	}

	session.LastAccessedAt = time.Now()
	return nil
}

// cleanupExpiredSessions periodically removes expired sessions.
func (s *MemorySessionStore) cleanupExpiredSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, session := range s.sessions {
			if now.Sub(session.CreatedAt) > s.maxAge {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

// publishedSessionSecrets are the session secrets this repository has printed
// in its example config and docs. The session cookie is signed with the
// secret, so anyone who has read one of these can sign a cookie for any user,
// an administrator included, on an install that kept it.
var publishedSessionSecrets = []string{
	"change-this-to-a-random-secret-key-at-least-32-chars",
	"your-32-chars-key",
	"dev-secret-key-minimum-32-characters",
	"dev-secret-key-min-32-characters-long",
	"docker-secret-key-minimum-32-characters",
	"must-be-at-least-32-characters-long",
	"same-secret-every-time-32-chars-minimum",
}

// ValidateSessionSecret refuses a session secret that is empty, one of the
// values this repository has published, or a ${NAME} placeholder the config
// loader left unexpanded. The server must not start with any of them: an empty
// secret fails every login, and the others are text anyone can read, so they
// let anyone forge a session.
func ValidateSessionSecret(secret string) error {
	const howTo = "Set MONITORING_AUTH_SESSION_SECRET (or auth.session.secret) to a random value, " +
		"for example the output of: openssl rand -base64 32"
	s := strings.TrimSpace(secret)
	if s == "" {
		return fmt.Errorf("SECURITY ERROR: auth.session.secret is empty. %s", howTo)
	}
	// The loader keeps ${NAME} as literal text when NAME is unset, which would
	// make the placeholder itself the key.
	if strings.Contains(s, "${") {
		return fmt.Errorf("SECURITY ERROR: auth.session.secret is %q, a placeholder whose environment "+
			"variable is not set, so the placeholder text itself would sign sessions. Set that variable. %s", s, howTo)
	}
	for _, published := range publishedSessionSecrets {
		if strings.EqualFold(s, published) {
			return fmt.Errorf("SECURITY ERROR: auth.session.secret is an example value published in this "+
				"project's config and docs, so anyone could forge a session cookie, an administrator's "+
				"included. %s", howTo)
		}
	}
	return nil
}

// CookieSessionManager manages HTTP sessions using cookies.
// This wraps gorilla/sessions for a cleaner interface.
type CookieSessionManager struct {
	store *sessions.CookieStore
	name  string
}

// NewCookieSessionManager creates a new cookie-based session manager.
//
// HttpOnly is intentionally hardcoded to true and not exposed via configuration:
// the session cookie carries authentication state, so allowing client-side
// JavaScript to read it would defeat one of the main mitigations against
// session hijacking via XSS. Do not change this without a security review.
func NewCookieSessionManager(secret string, sessionName string, maxAge int, secure bool, sameSite string) *CookieSessionManager {
	store := sessions.NewCookieStore([]byte(secret))

	// Configure session options
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true, // see function doc — must remain true
		Secure:   secure,
		SameSite: parseSameSite(sameSite),
	}

	return &CookieSessionManager{
		store: store,
		name:  sessionName,
	}
}

// Save writes the session cookie. It is Secure when auth.session.secure is
// set, and also whenever the request came over HTTPS.
func (m *CookieSessionManager) Save(r *http.Request, w http.ResponseWriter, s *sessions.Session) error {
	secureOverHTTPS(r, s)
	return s.Save(r, w)
}

// GetStore returns the underlying cookie store.
func (m *CookieSessionManager) GetStore() *sessions.CookieStore {
	return m.store
}

// GetSessionName returns the session name.
func (m *CookieSessionManager) GetSessionName() string {
	return m.name
}

// GetMaxAge returns the session max age in seconds.
func (m *CookieSessionManager) GetMaxAge() int {
	return m.store.Options.MaxAge
}

// parseSameSite converts a string to http.SameSite value.
func parseSameSite(sameSite string) http.SameSite {
	switch sameSite {
	case "lax":
		return http.SameSiteLaxMode
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteDefaultMode
	}
}

// SessionBlacklist manages revoked session IDs to prevent reuse after logout.
// This addresses the security finding where tokens could be reused after logout.
type SessionBlacklist struct {
	mu         sync.RWMutex
	revoked    map[string]time.Time
	maxAge     time.Duration
	cleanupInt time.Duration
}

// NewSessionBlacklist creates a new session blacklist with automatic cleanup.
// maxAge determines how long revoked sessions are kept (should match session maxAge).
func NewSessionBlacklist(maxAge time.Duration) *SessionBlacklist {
	bl := &SessionBlacklist{
		revoked:    make(map[string]time.Time),
		maxAge:     maxAge,
		cleanupInt: 15 * time.Minute,
	}

	go bl.cleanup()

	return bl
}

// Revoke adds a session ID to the blacklist.
// Called on logout to prevent the session from being reused.
func (bl *SessionBlacklist) Revoke(sessionID string) {
	if sessionID == "" {
		return
	}

	bl.mu.Lock()
	defer bl.mu.Unlock()

	bl.revoked[sessionID] = time.Now()
}

// IsRevoked checks if a session ID has been revoked.
// Returns true if the session is blacklisted and should be rejected.
func (bl *SessionBlacklist) IsRevoked(sessionID string) bool {
	if sessionID == "" {
		return false
	}

	bl.mu.RLock()
	defer bl.mu.RUnlock()

	_, exists := bl.revoked[sessionID]
	return exists
}

// cleanup periodically removes expired entries from the blacklist.
// Entries older than maxAge are removed since those sessions would be
// expired anyway and no longer need to be tracked.
func (bl *SessionBlacklist) cleanup() {
	ticker := time.NewTicker(bl.cleanupInt)
	defer ticker.Stop()

	for range ticker.C {
		bl.mu.Lock()
		now := time.Now()
		for sessionID, revokedAt := range bl.revoked {
			if now.Sub(revokedAt) > bl.maxAge {
				delete(bl.revoked, sessionID)
			}
		}
		bl.mu.Unlock()
	}
}

// Count returns the number of revoked sessions in the blacklist.
// Useful for monitoring and debugging.
func (bl *SessionBlacklist) Count() int {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	return len(bl.revoked)
}

// DatabaseSessionStore implements SessionStore using the database.
type DatabaseSessionStore struct {
	db     *gorm.DB
	maxAge time.Duration
}

// NewDatabaseSessionStore creates a new database-backed session store.
func NewDatabaseSessionStore(db *gorm.DB, maxAge time.Duration) *DatabaseSessionStore {
	store := &DatabaseSessionStore{
		db:     db,
		maxAge: maxAge,
	}

	// Start cleanup goroutine
	go store.cleanupExpiredSessions()

	return store
}

// Save stores a session in the database.
func (s *DatabaseSessionStore) Save(ctx context.Context, sessionID string, session *Session) error {
	if session == nil {
		return &AuthError{
			Code:    "INVALID_SESSION",
			Message: "session cannot be nil",
		}
	}

	// Serialize session data to JSON
	sessionData, err := json.Marshal(session)
	if err != nil {
		return &AuthError{
			Code:    "SESSION_SERIALIZATION_ERROR",
			Message: fmt.Sprintf("failed to serialize session: %v", err),
			Err:     err,
		}
	}

	// Calculate expiration time
	expiresAt := session.CreatedAt.Add(s.maxAge)

	// Create or update session in database
	// Note: UserID in the database model is *uint (foreign key to users table)
	// while session.UserID is a uint (database user ID).
	// The session data contains the user identifier; the DB UserID field can be null.
	dbSession := &dbmodels.Session{
		SessionID:      sessionID,
		Data:           sessionData,
		ExpiresAt:      expiresAt,
		LastAccessedAt: session.LastAccessedAt,
		UserID:         nil, // Can be set later by looking up user in users table
	}

	// Upsert: update if exists, insert if not
	result := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Assign(map[string]interface{}{
			"data":             sessionData,
			"expires_at":       expiresAt,
			"last_accessed_at": session.LastAccessedAt,
		}).
		FirstOrCreate(dbSession)

	if result.Error != nil {
		return &AuthError{
			Code:    "DATABASE_ERROR",
			Message: fmt.Sprintf("failed to save session: %v", result.Error),
			Err:     result.Error,
		}
	}

	return nil
}

// Get retrieves a session from the database.
func (s *DatabaseSessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	var dbSession dbmodels.Session

	result := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&dbSession)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, &AuthError{
				Code:    ErrCodeSessionNotFound,
				Message: "session not found",
			}
		}
		return nil, &AuthError{
			Code:    "DATABASE_ERROR",
			Message: fmt.Sprintf("failed to retrieve session: %v", result.Error),
			Err:     result.Error,
		}
	}

	// Check if session has expired
	if time.Now().After(dbSession.ExpiresAt) {
		// Delete expired session
		_ = s.Delete(ctx, sessionID)
		return nil, &AuthError{
			Code:    ErrCodeSessionNotFound,
			Message: "session has expired",
		}
	}

	// Deserialize session data
	var session Session
	if err := json.Unmarshal(dbSession.Data, &session); err != nil {
		return nil, &AuthError{
			Code:    "SESSION_DESERIALIZATION_ERROR",
			Message: fmt.Sprintf("failed to deserialize session: %v", err),
			Err:     err,
		}
	}

	return &session, nil
}

// Delete removes a session from the database.
func (s *DatabaseSessionStore) Delete(ctx context.Context, sessionID string) error {
	result := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&dbmodels.Session{})

	if result.Error != nil {
		return &AuthError{
			Code:    "DATABASE_ERROR",
			Message: fmt.Sprintf("failed to delete session: %v", result.Error),
			Err:     result.Error,
		}
	}

	return nil
}

// UpdateLastAccessed updates the last accessed timestamp in the database.
func (s *DatabaseSessionStore) UpdateLastAccessed(ctx context.Context, sessionID string) error {
	result := s.db.WithContext(ctx).
		Model(&dbmodels.Session{}).
		Where("session_id = ?", sessionID).
		Update("last_accessed_at", time.Now())

	if result.Error != nil {
		return &AuthError{
			Code:    "DATABASE_ERROR",
			Message: fmt.Sprintf("failed to update session: %v", result.Error),
			Err:     result.Error,
		}
	}

	if result.RowsAffected == 0 {
		return &AuthError{
			Code:    ErrCodeSessionNotFound,
			Message: "session not found",
		}
	}

	return nil
}

// cleanupExpiredSessions periodically removes expired sessions from the database.
func (s *DatabaseSessionStore) cleanupExpiredSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		// Delete all expired sessions
		result := s.db.
			Where("expires_at < ?", time.Now()).
			Delete(&dbmodels.Session{})

		// Silently ignore cleanup errors - this is a background task
		_ = result.Error
	}
}
