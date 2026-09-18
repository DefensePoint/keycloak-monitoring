// Package users provides user repository interfaces and implementations.
package users

import (
	"context"
	"errors"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
)

// ErrUserDeleted is returned when a login matches an account that has been
// deleted, instead of quietly creating a second one for the same person.
//
// A soft delete leaves the row in place but invisible to every ordinary query,
// so a returning user looks like somebody the platform has never seen. Creating
// an account for them would undo an administrator's decision without recording
// that it had been undone: the new account carries none of the old one's roles,
// but it does carry a session, and the deleted row stays deleted so nothing in
// the interface shows what happened.
//
// Until the indexes on users were narrowed to live rows this could not arise —
// the insert collided with the deleted row's email address and the login failed
// on a constraint violation. That was never a decision anyone made, and it also
// blocked re-adding somebody deliberately. This is the decision that replaces
// it.
var ErrUserDeleted = errors.New("account has been deleted")

// DeletedAccountError carries the id of the deleted account a login matched.
//
// The refusal is logged, and a line saying only that an account was deleted
// leaves an administrator with no way to tell which one — the first thing they
// will want to know. The id answers it without the log carrying the address or
// subject that arrived with the login: those identify the person, the id
// identifies the row, and the row is what an administrator acts on.
type DeletedAccountError struct{ UserID uint }

func (e *DeletedAccountError) Error() string { return ErrUserDeleted.Error() }

// Unwrap keeps errors.Is(err, ErrUserDeleted) working for every caller that
// only cares that the account was deleted.
func (e *DeletedAccountError) Unwrap() error { return ErrUserDeleted }

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
