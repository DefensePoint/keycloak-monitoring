package auth

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
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

// StateStore manages OAuth2 state tokens for CSRF protection.
type StateStore struct {
	mu     sync.RWMutex
	states map[string]time.Time
	maxAge time.Duration
}

// NewStateStore creates a new state store for CSRF protection.
func NewStateStore(maxAge time.Duration) *StateStore {
	store := &StateStore{
		states: make(map[string]time.Time),
		maxAge: maxAge,
	}

	// Start cleanup goroutine
	go store.cleanup()

	return store
}

// Save stores a state token with its creation time.
func (s *StateStore) Save(state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.states[state] = time.Now()
	return nil
}

// Validate checks if a state token is valid and removes it (one-time use).
func (s *StateStore) Validate(state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	createdAt, exists := s.states[state]
	if !exists {
		return &AuthError{
			Code:    ErrCodeInvalidState,
			Message: "invalid state parameter",
		}
	}

	// Check expiration
	if time.Since(createdAt) > s.maxAge {
		delete(s.states, state)
		return &AuthError{
			Code:    ErrCodeInvalidState,
			Message: "state parameter has expired",
		}
	}

	// Remove state after validation (one-time use)
	delete(s.states, state)
	return nil
}

// cleanup periodically removes expired state tokens.
func (s *StateStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for state, createdAt := range s.states {
			if now.Sub(createdAt) > s.maxAge {
				delete(s.states, state)
			}
		}
		s.mu.Unlock()
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
