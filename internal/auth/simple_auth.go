package auth

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// simpleAuthService handles username/password authentication.
type simpleAuthService struct {
	userRepo             UserRepository
	passwordHasher       *BcryptPasswordHasher
	sessionManager       *CookieSessionManager
	passwordRequirements PasswordRequirements
	logger               *logger.Logger
}

// NewSimpleAuthService creates a new simple authentication service.
func NewSimpleAuthService(
	userRepo UserRepository,
	sessionManager *CookieSessionManager,
	log *logger.Logger,
) SimpleAuthService {
	return &simpleAuthService{
		userRepo:             userRepo,
		passwordHasher:       NewBcryptPasswordHasher(10), // bcrypt cost of 10
		sessionManager:       sessionManager,
		passwordRequirements: DefaultPasswordRequirements(),
		logger:               log.WithComponent("simple_auth"),
	}
}

// Authenticate authenticates a user with username and password.
func (s *simpleAuthService) Authenticate(ctx context.Context, username, password string) (*UserInfo, error) {
	// Retrieve user from database
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		s.logger.Warn("Authentication failed - user not found",
			logger.Str("username", username))
		return nil, &AuthError{
			Code:    ErrCodeUnauthorized,
			Message: "invalid username or password",
		}
	}

	// Check if account is active
	if !user.IsActive || user.IsBlocked {
		s.logger.Warn("Authentication failed - account inactive or blocked",
			logger.Str("username", username),
			logger.Bool("is_active", user.IsActive),
			logger.Bool("is_blocked", user.IsBlocked))
		return nil, &AuthError{
			Code:    ErrCodeUnauthorized,
			Message: "account is inactive or blocked",
		}
	}

	// Verify password
	if !s.passwordHasher.VerifyPassword(user.PasswordHash, password) {
		s.logger.Warn("Authentication failed - invalid password",
			logger.Str("username", username))
		return nil, &AuthError{
			Code:    ErrCodeUnauthorized,
			Message: "invalid username or password",
		}
	}

	// Update last login info (log error but don't fail authentication)
	if err := s.userRepo.UpdateLastAccessed(ctx, user.Subject); err != nil {
		s.logger.Warn("Failed to update last accessed time",
			logger.Str("subject", user.Subject),
			logger.Err(err))
	}

	// Convert to UserInfo
	userInfo := &UserInfo{
		ID:                 user.ID,
		Subject:            user.Subject,
		Email:              user.Email,
		EmailVerified:      user.EmailVerified,
		Name:               user.Name,
		GivenName:          user.GivenName,
		FamilyName:         user.FamilyName,
		PreferredUsername:  user.PreferredUsername,
		Locale:             user.Locale,
		UpdatedAt:          user.UpdatedAt,
		MustChangePassword: user.MustChangePassword,
	}

	s.logger.Info("User authenticated successfully",
		logger.Str("username", username),
		logger.Str("email", user.Email),
		logger.Bool("must_change_password", user.MustChangePassword))

	return userInfo, nil
}

// ChangePassword changes a user's password.
func (s *simpleAuthService) ChangePassword(ctx context.Context, userID uint, currentPassword, newPassword string) error {
	// This implementation needs the username to verify old password
	// For now, we'll return an error indicating this operation isn't fully supported
	// The full implementation would need to look up the user by ID first
	return &AuthError{
		Code:    "NOT_IMPLEMENTED",
		Message: "ChangePassword by userID not yet implemented - use ChangePasswordByUsername",
	}
}

// ChangePasswordByUsername changes a user's password using username.
func (s *simpleAuthService) ChangePasswordByUsername(ctx context.Context, username, oldPassword, newPassword string) error {
	// Authenticate with old password first
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return &AuthError{
			Code:    ErrCodeUnauthorized,
			Message: "user not found",
		}
	}

	if !s.passwordHasher.VerifyPassword(user.PasswordHash, oldPassword) {
		return &AuthError{
			Code:    ErrCodeUnauthorized,
			Message: "invalid old password",
		}
	}

	// Validate new password strength
	if err := ValidatePassword(newPassword, s.passwordRequirements); err != nil {
		s.logger.Warn("New password validation failed",
			logger.Str("username", username),
			logger.Err(err))
		return &AuthError{
			Code:    "INVALID_PASSWORD",
			Message: err.Error(),
		}
	}

	// Hash new password
	hashedPassword, err := s.passwordHasher.HashPassword(newPassword)
	if err != nil {
		return err
	}

	// Update password, timestamp, and clear MustChangePassword flag
	now := time.Now()
	err = s.userRepo.UpdatePasswordWithFlags(ctx, user.ID, hashedPassword, &now, false)
	if err != nil {
		return &AuthError{
			Code:    "UPDATE_FAILED",
			Message: "failed to update password",
			Err:     err,
		}
	}

	s.logger.Info("Password changed successfully",
		logger.Str("username", username))

	return nil
}

// CreateUser creates a new user with username/password authentication.
func (s *simpleAuthService) CreateUser(ctx context.Context, username, password, email, name string) error {
	// Validate password strength
	if err := ValidatePassword(password, s.passwordRequirements); err != nil {
		s.logger.Warn("Password validation failed",
			logger.Str("username", username),
			logger.Err(err))
		return &AuthError{
			Code:    "INVALID_PASSWORD",
			Message: err.Error(),
		}
	}

	// Hash password
	hashedPassword, err := s.passwordHasher.HashPassword(password)
	if err != nil {
		return err
	}

	// Create user
	now := time.Now()
	user := &domain.User{
		Username:          username,
		PasswordHash:      hashedPassword,
		Email:             email,
		Name:              name,
		Subject:           email, // Use email as subject for simple auth users
		AuthMethod:        domain.AuthMethodSimple,
		IsActive:          true,
		IsBlocked:         false,
		PasswordChangedAt: &now,
	}

	err = s.userRepo.CreateSimpleAuthUser(ctx, user)
	if err != nil {
		return &AuthError{
			Code:    "CREATE_FAILED",
			Message: "failed to create user",
			Err:     err,
		}
	}

	s.logger.Info("User created successfully",
		logger.Str("username", username),
		logger.Str("email", email))

	return nil
}

// GetPasswordRequirements returns the current password requirements.
func (s *simpleAuthService) GetPasswordRequirements() PasswordRequirements {
	return s.passwordRequirements
}
