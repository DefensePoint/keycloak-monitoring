// Package users provides user repository interfaces and implementations.
package users

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
)

// Repository defines the interface for user persistence operations.
type Repository interface {
	// FindOrCreateBySubject finds a user by subject (OAuth2 sub claim) or creates a new one.
	FindOrCreateBySubject(ctx context.Context, user *domain.User) (*domain.User, error)

	// GetBySubject retrieves a user by their subject (OAuth2 sub claim).
	GetBySubject(ctx context.Context, subject string) (*domain.User, error)

	// GetByEmail retrieves a user by their email address.
	GetByEmail(ctx context.Context, email string) (*domain.User, error)

	// GetByUsername retrieves a user by their username (for simple auth).
	GetByUsername(ctx context.Context, username string) (*domain.User, error)

	// GetByID retrieves a user by their ID.
	GetByID(ctx context.Context, userID uint) (*domain.User, error)

	// ListUsers retrieves users with pagination.
	ListUsers(ctx context.Context, opts *httputil.ListOptions) ([]*domain.User, error)

	// CountUsers returns the total number of users.
	CountUsers(ctx context.Context) (int64, error)

	// CreateSimpleAuthUser creates a new user with username/password authentication.
	CreateSimpleAuthUser(ctx context.Context, user *domain.User) error

	// Update updates an existing user.
	Update(ctx context.Context, user *domain.User) error

	// Delete deletes a user (soft delete by marking as inactive).
	Delete(ctx context.Context, userID uint) error

	// UpdateLastAccessed updates the last accessed timestamp for a user.
	UpdateLastAccessed(ctx context.Context, subject string) error

	// UpdatePassword updates a user's password hash.
	UpdatePassword(ctx context.Context, userID uint, passwordHash string) error

	// UpdatePasswordWithFlags updates a user's password hash, timestamp, and must change password flag.
	UpdatePasswordWithFlags(ctx context.Context, userID uint, passwordHash string, passwordChangedAt *time.Time, mustChangePassword bool) error

	// BlockUser blocks a user account.
	BlockUser(ctx context.Context, subject string, reason string) error

	// UnblockUser unblocks a user account.
	UnblockUser(ctx context.Context, subject string) error
}
