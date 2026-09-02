package auth

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNewMemorySessionStore(t *testing.T) {
	maxAge := 1 * time.Hour
	store := NewMemorySessionStore(maxAge)

	if store == nil {
		t.Fatal("NewMemorySessionStore returned nil")
		return
	}

	if store.maxAge != maxAge {
		t.Errorf("Expected maxAge %v, got %v", maxAge, store.maxAge)
	}

	if store.sessions == nil {
		t.Error("sessions map not initialized")
	}
}

func TestMemorySessionStore_SaveAndGet(t *testing.T) {
	store := NewMemorySessionStore(1 * time.Hour)
	ctx := context.Background()

	session := &Session{
		UserID:         123,
		Email:          "test@example.com",
		Name:           "Test User",
		AccessToken:    "access_token",
		RefreshToken:   "refresh_token",
		IDToken:        "id_token",
		TokenExpiry:    time.Now().Add(1 * time.Hour),
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
	}

	sessionID := "session123"

	t.Run("Save session successfully", func(t *testing.T) {
		err := store.Save(ctx, sessionID, session)
		if err != nil {
			t.Errorf("Save() failed: %v", err)
		}
	})

	t.Run("Get saved session", func(t *testing.T) {
		retrieved, err := store.Get(ctx, sessionID)
		if err != nil {
			t.Fatalf("Get() failed: %v", err)
		}

		if retrieved.UserID != session.UserID {
			t.Errorf("UserID mismatch: expected %d, got %d", session.UserID, retrieved.UserID)
		}
		if retrieved.Email != session.Email {
			t.Errorf("Email mismatch: expected %s, got %s", session.Email, retrieved.Email)
		}
	})

	t.Run("Save nil session returns error", func(t *testing.T) {
		err := store.Save(ctx, "test", nil)
		if err == nil {
			t.Error("Save() with nil session should return error")
		}

		authErr, ok := err.(*AuthError)
		if !ok {
			t.Error("Error should be of type *AuthError")
		} else if authErr.Code != "INVALID_SESSION" {
			t.Errorf("Expected error code INVALID_SESSION, got %s", authErr.Code)
		}
	})
}

func TestMemorySessionStore_GetNonExistent(t *testing.T) {
	store := NewMemorySessionStore(1 * time.Hour)
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Get() should return error for non-existent session")
	}

	authErr, ok := err.(*AuthError)
	if !ok {
		t.Error("Error should be of type *AuthError")
	} else if authErr.Code != ErrCodeSessionNotFound {
		t.Errorf("Expected error code %s, got %s", ErrCodeSessionNotFound, authErr.Code)
	}
}

func TestMemorySessionStore_GetExpired(t *testing.T) {
	store := NewMemorySessionStore(100 * time.Millisecond)
	ctx := context.Background()

	session := &Session{
		UserID:         123,
		CreatedAt:      time.Now().Add(-1 * time.Hour), // Created in the past
		LastAccessedAt: time.Now(),
	}

	sessionID := "expired_session"
	err := store.Save(ctx, sessionID, session)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Wait for session to expire
	time.Sleep(150 * time.Millisecond)

	_, err = store.Get(ctx, sessionID)
	if err == nil {
		t.Error("Get() should return error for expired session")
	}

	authErr, ok := err.(*AuthError)
	if !ok {
		t.Error("Error should be of type *AuthError")
	} else if authErr.Code != ErrCodeSessionNotFound {
		t.Errorf("Expected error code %s, got %s", ErrCodeSessionNotFound, authErr.Code)
	}
}

func TestMemorySessionStore_Delete(t *testing.T) {
	store := NewMemorySessionStore(1 * time.Hour)
	ctx := context.Background()

	session := &Session{
		UserID:    123,
		CreatedAt: time.Now(),
	}

	sessionID := "session_to_delete"
	err := store.Save(ctx, sessionID, session)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify session exists
	_, err = store.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("Get() failed before delete: %v", err)
	}

	// Delete session
	err = store.Delete(ctx, sessionID)
	if err != nil {
		t.Errorf("Delete() failed: %v", err)
	}

	// Verify session no longer exists
	_, err = store.Get(ctx, sessionID)
	if err == nil {
		t.Error("Get() should return error after delete")
	}
}

func TestMemorySessionStore_UpdateLastAccessed(t *testing.T) {
	store := NewMemorySessionStore(1 * time.Hour)
	ctx := context.Background()

	session := &Session{
		UserID:         123,
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now().Add(-1 * time.Hour),
	}

	sessionID := "session123"
	err := store.Save(ctx, sessionID, session)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	oldTime := session.LastAccessedAt
	time.Sleep(10 * time.Millisecond)

	err = store.UpdateLastAccessed(ctx, sessionID)
	if err != nil {
		t.Errorf("UpdateLastAccessed() failed: %v", err)
	}

	retrieved, err := store.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if !retrieved.LastAccessedAt.After(oldTime) {
		t.Error("LastAccessedAt was not updated")
	}
}

func TestMemorySessionStore_UpdateLastAccessed_NonExistent(t *testing.T) {
	store := NewMemorySessionStore(1 * time.Hour)
	ctx := context.Background()

	err := store.UpdateLastAccessed(ctx, "nonexistent")
	if err == nil {
		t.Error("UpdateLastAccessed() should return error for non-existent session")
	}

	authErr, ok := err.(*AuthError)
	if !ok {
		t.Error("Error should be of type *AuthError")
	} else if authErr.Code != ErrCodeSessionNotFound {
		t.Errorf("Expected error code %s, got %s", ErrCodeSessionNotFound, authErr.Code)
	}
}

func TestNewCookieSessionManager(t *testing.T) {
	secret := "test-secret-key-32-characters!!"
	sessionName := "test_session"
	maxAge := 3600
	secure := true
	sameSite := "lax"

	manager := NewCookieSessionManager(secret, sessionName, maxAge, secure, sameSite)

	if manager == nil {
		t.Fatal("NewCookieSessionManager returned nil")
		return
	}

	if manager.name != sessionName {
		t.Errorf("Expected name %s, got %s", sessionName, manager.name)
	}

	if manager.store == nil {
		t.Error("store not initialized")
	}

	if manager.store.Options.MaxAge != maxAge {
		t.Errorf("Expected MaxAge %d, got %d", maxAge, manager.store.Options.MaxAge)
	}

	if manager.store.Options.Secure != secure {
		t.Errorf("Expected Secure %v, got %v", secure, manager.store.Options.Secure)
	}

	if !manager.store.Options.HttpOnly {
		t.Error("HttpOnly should be true")
	}

	if manager.store.Options.Path != "/" {
		t.Errorf("Expected Path /, got %s", manager.store.Options.Path)
	}
}

func TestCookieSessionManager_GetStore(t *testing.T) {
	manager := NewCookieSessionManager("secret", "session", 3600, false, "lax")

	store := manager.GetStore()
	if store == nil {
		t.Error("GetStore() returned nil")
	}

	if store != manager.store {
		t.Error("GetStore() returned different store instance")
	}
}

func TestCookieSessionManager_GetSessionName(t *testing.T) {
	sessionName := "my_session"
	manager := NewCookieSessionManager("secret", sessionName, 3600, false, "lax")

	name := manager.GetSessionName()
	if name != sessionName {
		t.Errorf("Expected session name %s, got %s", sessionName, name)
	}
}

func TestParseSameSite(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected http.SameSite
	}{
		{
			name:     "lax",
			input:    "lax",
			expected: http.SameSiteLaxMode,
		},
		{
			name:     "strict",
			input:    "strict",
			expected: http.SameSiteStrictMode,
		},
		{
			name:     "none",
			input:    "none",
			expected: http.SameSiteNoneMode,
		},
		{
			name:     "default for empty string",
			input:    "",
			expected: http.SameSiteDefaultMode,
		},
		{
			name:     "default for unknown value",
			input:    "invalid",
			expected: http.SameSiteDefaultMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSameSite(tt.input)
			if result != tt.expected {
				t.Errorf("parseSameSite(%s) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewStateStore(t *testing.T) {
	maxAge := 5 * time.Minute
	store := NewStateStore(maxAge)

	if store == nil {
		t.Fatal("NewStateStore returned nil")
		return
	}

	if store.maxAge != maxAge {
		t.Errorf("Expected maxAge %v, got %v", maxAge, store.maxAge)
	}

	if store.states == nil {
		t.Error("states map not initialized")
	}
}

func TestStateStore_SaveAndValidate(t *testing.T) {
	store := NewStateStore(5 * time.Minute)
	state := "test_state_123"

	t.Run("Save state successfully", func(t *testing.T) {
		err := store.Save(state)
		if err != nil {
			t.Errorf("Save() failed: %v", err)
		}
	})

	t.Run("Validate saved state", func(t *testing.T) {
		err := store.Validate(state)
		if err != nil {
			t.Errorf("Validate() failed: %v", err)
		}
	})

	t.Run("State is removed after validation (one-time use)", func(t *testing.T) {
		err := store.Validate(state)
		if err == nil {
			t.Error("Validate() should fail for already-used state")
		}

		authErr, ok := err.(*AuthError)
		if !ok {
			t.Error("Error should be of type *AuthError")
		} else if authErr.Code != ErrCodeInvalidState {
			t.Errorf("Expected error code %s, got %s", ErrCodeInvalidState, authErr.Code)
		}
	})
}

func TestStateStore_ValidateNonExistent(t *testing.T) {
	store := NewStateStore(5 * time.Minute)

	err := store.Validate("nonexistent_state")
	if err == nil {
		t.Error("Validate() should return error for non-existent state")
	}

	authErr, ok := err.(*AuthError)
	if !ok {
		t.Error("Error should be of type *AuthError")
	} else if authErr.Code != ErrCodeInvalidState {
		t.Errorf("Expected error code %s, got %s", ErrCodeInvalidState, authErr.Code)
	}
}

func TestStateStore_ValidateExpired(t *testing.T) {
	store := NewStateStore(100 * time.Millisecond)
	state := "expired_state"

	err := store.Save(state)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Wait for state to expire
	time.Sleep(150 * time.Millisecond)

	err = store.Validate(state)
	if err == nil {
		t.Error("Validate() should return error for expired state")
	}

	authErr, ok := err.(*AuthError)
	if !ok {
		t.Error("Error should be of type *AuthError")
	} else if authErr.Code != ErrCodeInvalidState {
		t.Errorf("Expected error code %s, got %s", ErrCodeInvalidState, authErr.Code)
	}
}

func TestStateStore_MultipleStates(t *testing.T) {
	store := NewStateStore(5 * time.Minute)

	states := []string{"state1", "state2", "state3"}

	// Save multiple states
	for _, state := range states {
		err := store.Save(state)
		if err != nil {
			t.Fatalf("Save(%s) failed: %v", state, err)
		}
	}

	// Validate each state (they should all be valid and then removed)
	for _, state := range states {
		err := store.Validate(state)
		if err != nil {
			t.Errorf("Validate(%s) failed: %v", state, err)
		}

		// Try to validate again (should fail)
		err = store.Validate(state)
		if err == nil {
			t.Errorf("Validate(%s) should fail on second attempt", state)
		}
	}
}

func TestError(t *testing.T) {
	t.Run("Error with underlying error", func(t *testing.T) {
		underlyingErr := &AuthError{Code: "INNER", Message: "inner error"}
		err := &AuthError{
			Code:    "OUTER",
			Message: "outer error",
			Err:     underlyingErr,
		}

		expected := "outer error: inner error"
		if err.Error() != expected {
			t.Errorf("Error() = %s, expected %s", err.Error(), expected)
		}
	})

	t.Run("Error without underlying error", func(t *testing.T) {
		err := &AuthError{
			Code:    "TEST",
			Message: "test error",
		}

		expected := "test error"
		if err.Error() != expected {
			t.Errorf("Error() = %s, expected %s", err.Error(), expected)
		}
	})

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		underlyingErr := &AuthError{Code: "INNER", Message: "inner"}
		err := &AuthError{
			Code:    "OUTER",
			Message: "outer",
			Err:     underlyingErr,
		}

		unwrapped := err.Unwrap()
		if unwrapped != underlyingErr {
			t.Error("Unwrap() did not return the underlying error")
		}
	})

	t.Run("Unwrap returns nil when no underlying error", func(t *testing.T) {
		err := &AuthError{
			Code:    "TEST",
			Message: "test",
		}

		unwrapped := err.Unwrap()
		if unwrapped != nil {
			t.Error("Unwrap() should return nil when there is no underlying error")
		}
	})
}

func TestNewSessionBlacklist(t *testing.T) {
	maxAge := 1 * time.Hour
	bl := NewSessionBlacklist(maxAge)

	if bl == nil {
		t.Fatal("NewSessionBlacklist returned nil")
		return
	}

	if bl.maxAge != maxAge {
		t.Errorf("Expected maxAge %v, got %v", maxAge, bl.maxAge)
	}

	if bl.revoked == nil {
		t.Error("revoked map not initialized")
	}

	if bl.Count() != 0 {
		t.Errorf("Expected empty blacklist, got %d entries", bl.Count())
	}
}

func TestSessionBlacklist_RevokeAndIsRevoked(t *testing.T) {
	bl := NewSessionBlacklist(1 * time.Hour)
	sessionID := "test_session_123"

	t.Run("Session not revoked initially", func(t *testing.T) {
		if bl.IsRevoked(sessionID) {
			t.Error("Session should not be revoked initially")
		}
	})

	t.Run("Revoke session", func(t *testing.T) {
		bl.Revoke(sessionID)

		if !bl.IsRevoked(sessionID) {
			t.Error("Session should be revoked after Revoke()")
		}

		if bl.Count() != 1 {
			t.Errorf("Expected 1 entry in blacklist, got %d", bl.Count())
		}
	})

	t.Run("Other sessions not affected", func(t *testing.T) {
		if bl.IsRevoked("other_session") {
			t.Error("Other sessions should not be revoked")
		}
	})
}

func TestSessionBlacklist_EmptySessionID(t *testing.T) {
	bl := NewSessionBlacklist(1 * time.Hour)

	t.Run("Revoke empty string does nothing", func(t *testing.T) {
		bl.Revoke("")
		if bl.Count() != 0 {
			t.Errorf("Expected 0 entries after revoking empty string, got %d", bl.Count())
		}
	})

	t.Run("IsRevoked empty string returns false", func(t *testing.T) {
		if bl.IsRevoked("") {
			t.Error("IsRevoked('') should return false")
		}
	})
}

func TestSessionBlacklist_MultipleSessions(t *testing.T) {
	bl := NewSessionBlacklist(1 * time.Hour)
	sessions := []string{"session1", "session2", "session3"}

	// Revoke multiple sessions
	for _, sid := range sessions {
		bl.Revoke(sid)
	}

	if bl.Count() != len(sessions) {
		t.Errorf("Expected %d entries, got %d", len(sessions), bl.Count())
	}

	// Verify all are revoked
	for _, sid := range sessions {
		if !bl.IsRevoked(sid) {
			t.Errorf("Session %s should be revoked", sid)
		}
	}
}

func TestSessionBlacklist_RevokeIdempotent(t *testing.T) {
	bl := NewSessionBlacklist(1 * time.Hour)
	sessionID := "test_session"

	// Revoke same session multiple times
	bl.Revoke(sessionID)
	bl.Revoke(sessionID)
	bl.Revoke(sessionID)

	// Should still only have 1 entry (map behavior)
	if bl.Count() != 1 {
		t.Errorf("Expected 1 entry after multiple revocations of same session, got %d", bl.Count())
	}

	if !bl.IsRevoked(sessionID) {
		t.Error("Session should still be revoked")
	}
}
