// Package postgres provides PostgreSQL implementation of users repository interface.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

const (
	sqlWhereSubject  = "subject = ?"
	errUserNotFound  = "user not found: %s"
	errFailedGetUser = "failed to get user: %w"
)

// Repository implements users.Repository using GORM.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new users Repository.
func NewRepository(db *gorm.DB) users.Repository {
	return &Repository{db: db}
}

// FindOrCreateBySubject finds a user by subject (OAuth2 sub claim) or creates a new one.
// refuseIfDeleted reports users.ErrUserDeleted when this login belongs to an
// account that was deleted.
//
// Matched on subject and on email, because the two identify a returning person
// in different situations: the subject is stable while the identity provider is,
// and the email survives a realm being rebuilt, which is the case that hands a
// familiar person a brand new subject.
//
// Blank values match nobody. Every one of these columns is legitimately empty
// for some kind of account — Keycloak users often have no email — so matching on
// one would refuse every future login of that shape on behalf of a single
// deleted row.
//
// Unscoped, since the whole point is to see what ordinary queries hide.
func (r *Repository) refuseIfDeleted(ctx context.Context, candidate *database.User) error {
	q := r.db.WithContext(ctx).Unscoped().Model(&database.User{}).
		Where("deleted_at IS NOT NULL")

	switch {
	case candidate.Subject != "" && candidate.Email != "":
		q = q.Where("subject = ? OR email = ?", candidate.Subject, candidate.Email)
	case candidate.Subject != "":
		q = q.Where("subject = ?", candidate.Subject)
	case candidate.Email != "":
		q = q.Where("email = ?", candidate.Email)
	default:
		// Nothing identifying to match on; the create path takes over.
		return nil
	}

	var deleted database.User
	err := q.Select("id").First(&deleted).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to check for a deleted account: %w", err)
	}
	return &users.DeletedAccountError{UserID: deleted.ID}
}

func (r *Repository) FindOrCreateBySubject(ctx context.Context, user *domain.User) (*domain.User, error) {
	dbUser := toDBUser(user)
	var existingUser database.User

	// Try to find existing user by subject
	result := r.db.WithContext(ctx).Where(sqlWhereSubject, dbUser.Subject).First(&existingUser)

	switch {
	case result.Error == nil:
		// Matched on subject: the ordinary case.

	case errors.Is(result.Error, gorm.ErrRecordNotFound):
		// Nobody live matches, but a deleted account might. Refuse rather than
		// create a second one for the same person: see users.ErrUserDeleted.
		if err := r.refuseIfDeleted(ctx, dbUser); err != nil {
			return nil, err
		}

		// An unfamiliar subject can still be a familiar person.
		adopted, err := r.adoptByVerifiedEmail(ctx, dbUser, &existingUser)
		if err != nil {
			return nil, err
		}
		if !adopted {
			if err := r.db.WithContext(ctx).Create(dbUser).Error; err != nil {
				return nil, fmt.Errorf("failed to create user: %w", err)
			}
			return toDomainUser(dbUser), nil
		}
		// existingUser now holds the account this login belongs to, so the
		// update below runs for it exactly as for a subject match: a relinked
		// login refreshes the profile like any other.

	default:
		return nil, fmt.Errorf("failed to query user: %w", result.Error)
	}

	// User exists, update their information
	updates := map[string]interface{}{
		// Carried because adoption above may have changed it. For a subject
		// match this writes back the value it just matched on.
		"subject":            dbUser.Subject,
		"email":              dbUser.Email,
		"email_verified":     dbUser.EmailVerified,
		"name":               dbUser.Name,
		"given_name":         dbUser.GivenName,
		"family_name":        dbUser.FamilyName,
		"preferred_username": dbUser.PreferredUsername,
		"locale":             dbUser.Locale,
		"auth_method":        dbUser.AuthMethod,
		"last_login_at":      dbUser.LastLoginAt,
		"last_login_ip":      dbUser.LastLoginIP,
		"login_count":        gorm.Expr("login_count + 1"),
		"last_accessed_at":   dbUser.LastAccessedAt,
	}

	if err := r.db.WithContext(ctx).Model(&existingUser).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	// Reload user to get updated values
	if err := r.db.WithContext(ctx).Where(sqlWhereSubject, dbUser.Subject).First(&existingUser).Error; err != nil {
		return nil, fmt.Errorf("failed to reload user: %w", err)
	}

	return toDomainUser(&existingUser), nil
}

// adoptByVerifiedEmail finds the account this login belongs to when the subject
// is new but the person is not, and loads it into existing.
//
// An identity provider's subject is stable only while the provider is. Rebuild
// a realm, migrate to a new one, or re-provision a user, and the same person
// arrives with a new sub claim. Without this they become a second account:
// their roles, tenant policies and history stay with a row nobody can reach any
// more, and an administrator has to notice and merge them by hand.
//
// The address is what identifies them across that change, so this matches on
// email — but only on an address the identity provider says it has verified.
// That condition is the whole safety of it. An unverified address is a claim
// the person made about themselves, and honouring it would let anyone who can
// register an address in the realm inherit whatever account already holds it.
// Until recently every account read as verified whether or not anyone had
// checked, which is why this could not be built before that was fixed.
//
// Restricted further to accounts that hold no local password. Adopting a
// username-and-password account would let an SSO login take over local
// credentials, which is a different decision from the one this implements and a
// wider trust boundary than a realm rebuild needs. Those credentials also carry
// the way back in when the identity provider is unreachable, which is exactly
// when nobody can afford to have lost them.
//
// Tested on the password rather than on auth_method, which looks like the
// natural column and is not trustworthy here: role_sync builds its profile
// without an AuthMethod, and the update below writes that blank through, so
// every account Keycloak has synced sits at "" until the next restart repairs
// it. Keying on auth_method meant adoption silently skipped exactly the
// accounts a Keycloak deployment has most of. The password is the thing that
// would actually be taken over, so it is the thing to ask about.
//
// At most one account can match: the unique index on email covers live rows
// carrying an address, so there is no ambiguity to resolve.
func (r *Repository) adoptByVerifiedEmail(ctx context.Context, candidate *database.User, existing *database.User) (bool, error) {
	if !candidate.EmailVerified || candidate.Email == "" || candidate.Subject == "" {
		return false, nil
	}

	err := r.db.WithContext(ctx).
		Where("email = ? AND (password_hash IS NULL OR password_hash = '')", candidate.Email).
		First(existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to look for an account with this email: %w", err)
	}

	return true, nil
}

// GetBySubject retrieves a user by their subject (OAuth2 sub claim).
func (r *Repository) GetBySubject(ctx context.Context, subject string) (*domain.User, error) {
	var user database.User
	result := r.db.WithContext(ctx).Where(sqlWhereSubject, subject).First(&user)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf(errUserNotFound, subject)
	}
	if result.Error != nil {
		return nil, fmt.Errorf(errFailedGetUser, result.Error)
	}

	return toDomainUser(&user), nil
}

// GetByEmail retrieves a user by their email address.
func (r *Repository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user database.User
	result := r.db.WithContext(ctx).Where("email = ?", email).First(&user)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf(errUserNotFound, email)
	}
	if result.Error != nil {
		return nil, fmt.Errorf(errFailedGetUser, result.Error)
	}

	return toDomainUser(&user), nil
}

// GetByUsername retrieves a user by their username (for simple auth).
func (r *Repository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user database.User
	result := r.db.WithContext(ctx).Where("username = ?", username).First(&user)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get user: %w", result.Error)
	}

	return toDomainUser(&user), nil
}

// GetByID retrieves a user by their ID.
func (r *Repository) GetByID(ctx context.Context, userID uint) (*domain.User, error) {
	var dbUser database.User
	result := r.db.WithContext(ctx).First(&dbUser, userID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", result.Error)
	}

	return toDomainUser(&dbUser), nil
}

// ListUsers retrieves users with pagination.
func (r *Repository) ListUsers(ctx context.Context, opts *httputil.ListOptions) ([]*domain.User, error) {
	if opts == nil {
		opts = &httputil.ListOptions{Limit: 100, Offset: 0}
	}

	var users []*database.User
	result := r.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(opts.Limit).
		Offset(opts.Offset).
		Find(&users)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to query users: %w", result.Error)
	}

	// Convert database users to domain users
	domainUsers := make([]*domain.User, len(users))
	for i, user := range users {
		domainUsers[i] = toDomainUser(user)
	}

	return domainUsers, nil
}

// CountUsers returns the total number of users.
func (r *Repository) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).Model(&database.User{}).Count(&count)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to count users: %w", result.Error)
	}

	return count, nil
}

// CreateSimpleAuthUser creates a new user with username/password authentication.
func (r *Repository) CreateSimpleAuthUser(ctx context.Context, user *domain.User) error {
	dbUser := toDBUser(user)
	result := r.db.WithContext(ctx).Create(dbUser)
	if result.Error != nil {
		return fmt.Errorf("failed to create user: %w", result.Error)
	}
	// Update the domain user with the created ID
	user.ID = dbUser.ID
	return nil
}

// Update updates an existing user.
func (r *Repository) Update(ctx context.Context, user *domain.User) error {
	dbUser := toDBUser(user)

	result := r.db.WithContext(ctx).Model(&database.User{}).Where("id = ?", user.ID).Updates(dbUser)
	if result.Error != nil {
		return fmt.Errorf("failed to update user: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// Delete deletes a user (soft delete by marking as inactive).
// Delete deactivates the account rather than deleting anything.
//
// The name is the interface's, kept so this stays a drop-in implementation,
// but it writes is_active and nothing else: no row is removed, deleted_at is
// never set, and the roles, policies and history all stay. Nothing anywhere in
// the platform sets deleted_at on a user, so an account is only ever
// deactivated, and reactivating it restores exactly what was there.
//
// It does not end a session that already exists. The middleware re-reads the
// account on every request but only to recover the ID that RBAC needs, and the
// struct it fills in carries no status field, so is_active is never consulted
// once somebody holds a session: access continues until that session expires,
// up to session max_age, 24 hours by default. API tokens do re-check, and stop
// working immediately.
func (r *Repository) Delete(ctx context.Context, userID uint) error {
	result := r.db.WithContext(ctx).Model(&database.User{}).Where("id = ?", userID).Update("is_active", false)
	if result.Error != nil {
		return fmt.Errorf("failed to delete user: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateLastAccessed updates the last accessed timestamp for a user.
func (r *Repository) UpdateLastAccessed(ctx context.Context, subject string) error {
	result := r.db.WithContext(ctx).
		Model(&database.User{}).
		Where(sqlWhereSubject, subject).
		Update("last_accessed_at", gorm.Expr("NOW()"))

	if result.Error != nil {
		return fmt.Errorf("failed to update last accessed: %w", result.Error)
	}

	return nil
}

// UpdatePassword updates a user's password hash.
func (r *Repository) UpdatePassword(ctx context.Context, userID uint, passwordHash string) error {
	result := r.db.WithContext(ctx).
		Model(&database.User{}).
		Where("id = ?", userID).
		Update("password_hash", passwordHash)

	if result.Error != nil {
		return fmt.Errorf("failed to update password: %w", result.Error)
	}

	return nil
}

// UpdatePasswordWithFlags updates a user's password hash, timestamp, and must change password flag.
func (r *Repository) UpdatePasswordWithFlags(ctx context.Context, userID uint, passwordHash string, passwordChangedAt *time.Time, mustChangePassword bool) error {
	updates := map[string]interface{}{
		"password_hash":        passwordHash,
		"password_changed_at":  passwordChangedAt,
		"must_change_password": mustChangePassword,
	}

	result := r.db.WithContext(ctx).
		Model(&database.User{}).
		Where("id = ?", userID).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update password: %w", result.Error)
	}

	return nil
}

// BlockUser blocks a user account.
func (r *Repository) BlockUser(ctx context.Context, subject string, reason string) error {
	updates := map[string]interface{}{
		"is_blocked":     true,
		"blocked_at":     gorm.Expr("NOW()"),
		"blocked_reason": reason,
	}

	result := r.db.WithContext(ctx).
		Model(&database.User{}).
		Where(sqlWhereSubject, subject).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to block user: %w", result.Error)
	}

	return nil
}

// UnblockUser unblocks a user account.
func (r *Repository) UnblockUser(ctx context.Context, subject string) error {
	updates := map[string]interface{}{
		"is_blocked":     false,
		"blocked_at":     nil,
		"blocked_reason": "",
	}

	result := r.db.WithContext(ctx).
		Model(&database.User{}).
		Where(sqlWhereSubject, subject).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to unblock user: %w", result.Error)
	}

	return nil
}

// toDomainUser converts a database User to a domain User.
func toDomainUser(user *database.User) *domain.User {
	if user == nil {
		return nil
	}

	// Convert LastAccessedAt from time.Time to *time.Time
	var lastAccessedAt *time.Time
	if !user.LastAccessedAt.IsZero() {
		lastAccessedAt = &user.LastAccessedAt
	}

	return &domain.User{
		ID:                 user.ID,
		Subject:            user.Subject,
		Email:              user.Email,
		EmailVerified:      user.EmailVerified,
		Name:               user.Name,
		GivenName:          user.GivenName,
		FamilyName:         user.FamilyName,
		PreferredUsername:  user.PreferredUsername,
		Locale:             user.Locale,
		Username:           user.Username,
		PasswordHash:       user.PasswordHash,
		AuthMethod:         domain.AuthMethod(user.AuthMethod),
		MustChangePassword: user.MustChangePassword,
		PasswordChangedAt:  user.PasswordChangedAt,
		IsActive:           user.IsActive,
		IsBlocked:          user.IsBlocked,
		BlockedReason:      user.BlockedReason,
		BlockedAt:          user.BlockedAt,
		LastLoginAt:        user.LastLoginAt,
		LastLoginIP:        user.LastLoginIP,
		LastAccessedAt:     lastAccessedAt,
		LoginCount:         user.LoginCount,
		CreatedAt:          user.CreatedAt,
		UpdatedAt:          user.UpdatedAt,
	}
}

// toDBUser converts a domain User to a database User.
func toDBUser(user *domain.User) *database.User {
	if user == nil {
		return nil
	}

	// Convert LastAccessedAt from *time.Time to time.Time
	var lastAccessedAt time.Time
	if user.LastAccessedAt != nil {
		lastAccessedAt = *user.LastAccessedAt
	}

	return &database.User{
		ID:                user.ID,
		Subject:           user.Subject,
		Email:             user.Email,
		EmailVerified:     user.EmailVerified,
		Name:              user.Name,
		GivenName:         user.GivenName,
		FamilyName:        user.FamilyName,
		PreferredUsername: user.PreferredUsername,
		Locale:            user.Locale,
		Username:          user.Username,
		PasswordHash:      user.PasswordHash,
		AuthMethod:        string(user.AuthMethod),
		IsActive:          user.IsActive,
		IsBlocked:         user.IsBlocked,
		BlockedReason:     user.BlockedReason,
		BlockedAt:         user.BlockedAt,
		LastLoginAt:       user.LastLoginAt,
		LastLoginIP:       user.LastLoginIP,
		LastAccessedAt:    lastAccessedAt,
		LoginCount:        user.LoginCount,
		CreatedAt:         user.CreatedAt,
		UpdatedAt:         user.UpdatedAt,
	}
}
