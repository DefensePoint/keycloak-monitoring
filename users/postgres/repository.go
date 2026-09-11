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
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
	"github.com/DefensePoint/keycloak-monitoring/users"
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
func (r *Repository) FindOrCreateBySubject(ctx context.Context, user *domain.User) (*domain.User, error) {
	dbUser := toDBUser(user)
	var existingUser database.User

	// Try to find existing user by subject
	result := r.db.WithContext(ctx).Where(sqlWhereSubject, dbUser.Subject).First(&existingUser)

	if result.Error == gorm.ErrRecordNotFound {
		// User doesn't exist, create new user
		if err := r.db.WithContext(ctx).Create(dbUser).Error; err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
		return toDomainUser(dbUser), nil
	}

	if result.Error != nil {
		return nil, fmt.Errorf("failed to query user: %w", result.Error)
	}

	// User exists, update their information
	updates := map[string]interface{}{
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
