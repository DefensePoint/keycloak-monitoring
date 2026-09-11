package apitoken

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Repository defines the interface for API token persistence.
type Repository interface {
	// Create persists a new token with its digest and fills in ID and CreatedAt.
	Create(ctx context.Context, token *Token, digest string) error

	// List retrieves all tokens (metadata only).
	List(ctx context.Context) ([]*Token, error)

	// GetByDigest retrieves a token by its SHA-256 hex digest.
	// Returns nil when no token matches.
	GetByDigest(ctx context.Context, digest string) (*Token, error)

	// Revoke marks a token as revoked at the given time.
	Revoke(ctx context.Context, id uint, revokedAt time.Time) error
}

// UserStore is the slice of users.Repository this service needs.
type UserStore interface {
	// GetByID retrieves a user by their ID. Returns nil when no user matches.
	GetByID(ctx context.Context, userID uint) (*domain.User, error)
}

// Logger is the local interface for logging.
// This keeps the domain decoupled from the concrete logger implementation.
type Logger interface {
	Info(msg string, fields ...any)
	Error(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Debug(msg string, fields ...any)
}
