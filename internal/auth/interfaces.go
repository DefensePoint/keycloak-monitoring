package auth

import (
	"context"
	"time"

	"golang.org/x/oauth2"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Provider defines the interface for OAuth2/OIDC authentication providers.
type Provider interface {
	// GetAuthCodeURL returns the URL for initiating the OAuth2 flow.
	GetAuthCodeURL(state string) string

	// Exchange exchanges an authorization code for tokens.
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)

	// VerifyIDToken verifies and extracts claims from an ID token.
	VerifyIDToken(ctx context.Context, token string) (*UserInfo, error)

	// RefreshToken refreshes an expired access token.
	RefreshToken(ctx context.Context, refreshToken string) (*oauth2.Token, error)
}

// SessionStore defines the interface for session management.
type SessionStore interface {
	// Save stores a session.
	Save(ctx context.Context, sessionID string, session *Session) error

	// Get retrieves a session by ID.
	Get(ctx context.Context, sessionID string) (*Session, error)

	// Delete removes a session.
	Delete(ctx context.Context, sessionID string) error

	// UpdateLastAccessed updates the last accessed timestamp.
	UpdateLastAccessed(ctx context.Context, sessionID string) error
}

// TokenValidator defines the interface for token validation.
type TokenValidator interface {
	// ValidateToken validates an access token and returns user info.
	ValidateToken(ctx context.Context, token string) (*UserInfo, error)
}

// UserRepository defines the interface for user persistence.
type UserRepository interface {
	// GetBySubject retrieves a user by their subject (unique identifier).
	GetBySubject(ctx context.Context, subject string) (*domain.User, error)

	// GetByEmail retrieves a user by email.
	GetByEmail(ctx context.Context, email string) (*domain.User, error)

	// GetByUsername retrieves a user by username.
	GetByUsername(ctx context.Context, username string) (*domain.User, error)

	// FindOrCreateBySubject finds an existing user or creates a new one.
	FindOrCreateBySubject(ctx context.Context, user *domain.User) (*domain.User, error)

	// UpdateLastAccessed updates the last accessed timestamp.
	UpdateLastAccessed(ctx context.Context, subject string) error

	// CreateSimpleAuthUser creates a user for simple authentication.
	CreateSimpleAuthUser(ctx context.Context, user *domain.User) error

	// UpdatePasswordWithFlags updates password and related flags.
	UpdatePasswordWithFlags(ctx context.Context, userID uint, passwordHash string, passwordChangedAt *time.Time, mustChangePassword bool) error
}

// PasswordHasher defines the interface for password hashing.
type PasswordHasher interface {
	// Hash generates a hash for the given password.
	Hash(password string) (string, error)

	// Compare compares a password with a hash.
	Compare(password, hash string) error
}

// PasswordValidator defines the interface for password validation.
type PasswordValidator interface {
	// Validate validates a password against the policy.
	Validate(password string) error
}
