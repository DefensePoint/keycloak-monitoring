package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/users"
)

// Service defines the interface for authentication operations.
type Service interface {
	// Login initiates the OAuth2 login flow and returns the redirect URL.
	Login(ctx context.Context) (string, string, error)

	// Callback handles the OAuth2 callback and creates a session.
	// Returns both the session and the complete user object.
	Callback(ctx context.Context, code, state, expectedState string) (*Session, *domain.User, error)

	// Logout terminates a user session.
	Logout(ctx context.Context, sessionID string) error

	// ValidateSession validates a session and returns user info.
	ValidateSession(ctx context.Context, sessionID string) (*UserInfo, error)

	// RefreshSession refreshes an expired session.
	RefreshSession(ctx context.Context, sessionID string) (*Session, error)

	// FindOrCreateUser finds or creates a user from OAuth2 user info.
	FindOrCreateUser(ctx context.Context, userInfo *UserInfo) (*domain.User, error)
}

// SimpleAuthService defines the interface for simple (username/password) authentication.
type SimpleAuthService interface {
	// Authenticate validates credentials and returns user info.
	Authenticate(ctx context.Context, username, password string) (*UserInfo, error)

	// ChangePassword changes a user's password.
	ChangePassword(ctx context.Context, userID uint, currentPassword, newPassword string) error

	// ChangePasswordByUsername changes a user's password using username.
	ChangePasswordByUsername(ctx context.Context, username, oldPassword, newPassword string) error

	// CreateUser creates a new user with simple authentication.
	CreateUser(ctx context.Context, username, password, email, name string) error

	// GetPasswordRequirements returns the current password requirements.
	GetPasswordRequirements() PasswordRequirements
}

// service implements the Service interface.
type service struct {
	provider       Provider
	sessionStore   SessionStore
	userRepository UserRepository
	logger         *logger.Logger
	config         *Config
}

// NewService creates a new authentication service.
func NewService(
	provider Provider,
	sessionStore SessionStore,
	userRepository UserRepository,
	log *logger.Logger,
	config *Config,
) Service {
	return &service{
		provider:       provider,
		sessionStore:   sessionStore,
		userRepository: userRepository,
		logger:         log.WithComponent("auth_service"),
		config:         config,
	}
}

// Login initiates the OAuth2 login flow.
func (s *service) Login(ctx context.Context) (string, string, error) {
	state := generateState()
	url := s.provider.GetAuthCodeURL(state)
	return url, state, nil
}

// Callback handles the OAuth2 callback.
func (s *service) Callback(ctx context.Context, code, state, expectedState string) (*Session, *domain.User, error) {
	if state != expectedState {
		return nil, nil, &AuthError{
			Code:    ErrCodeInvalidState,
			Message: "invalid state parameter",
		}
	}

	token, err := s.provider.Exchange(ctx, code)
	if err != nil {
		return nil, nil, &AuthError{
			Code:    ErrCodeExchangeFailed,
			Message: "failed to exchange authorization code",
			Err:     err,
		}
	}

	idToken, _ := token.Extra("id_token").(string)
	userInfo, err := s.provider.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, nil, &AuthError{
			Code:    ErrCodeInvalidToken,
			Message: "failed to verify ID token",
			Err:     err,
		}
	}

	// Find or create user.
	//
	// EmailVerified carries whatever the IdP asserted, never an assumption. An
	// OIDC provider is free to issue a token for an address nobody has
	// confirmed, and Keycloak does exactly that for a user created by an admin.
	// Storing true regardless made every account look confirmed, which is the
	// one claim account-linking by email would have to rely on.
	user := &domain.User{
		Subject:           userInfo.Subject,
		Email:             userInfo.Email,
		EmailVerified:     userInfo.EmailVerified,
		Name:              userInfo.Name,
		GivenName:         userInfo.GivenName,
		FamilyName:        userInfo.FamilyName,
		PreferredUsername: userInfo.PreferredUsername,
		Locale:            userInfo.Locale,
		AuthMethod:        domain.AuthMethodOAuth,
		IsActive:          true,
	}

	dbUser, err := s.userRepository.FindOrCreateBySubject(ctx, user)
	if err != nil {
		if authErr := s.refuseDeletedAccount(err, userInfo); authErr != nil {
			return nil, nil, authErr
		}
		return nil, nil, fmt.Errorf("failed to find or create user: %w", err)
	}

	if err := s.refuseUnusableAccount(dbUser); err != nil {
		return nil, nil, err
	}

	// Create session
	sessionID := generateSessionID()
	session := &Session{
		UserID:       dbUser.ID,
		Subject:      dbUser.Subject,
		Email:        dbUser.Email,
		Name:         dbUser.Name,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		IDToken:      idToken,
		TokenExpiry:  token.Expiry,
	}

	if err := s.sessionStore.Save(ctx, sessionID, session); err != nil {
		return nil, nil, fmt.Errorf("failed to save session: %w", err)
	}

	return session, dbUser, nil
}

// Logout terminates a user session.
func (s *service) Logout(ctx context.Context, sessionID string) error {
	return s.sessionStore.Delete(ctx, sessionID)
}

// ValidateSession validates a session.
func (s *service) ValidateSession(ctx context.Context, sessionID string) (*UserInfo, error) {
	session, err := s.sessionStore.Get(ctx, sessionID)
	if err != nil {
		return nil, &AuthError{
			Code:    ErrCodeSessionNotFound,
			Message: "session not found",
			Err:     err,
		}
	}

	// Update last accessed
	_ = s.sessionStore.UpdateLastAccessed(ctx, sessionID)

	return &UserInfo{
		Subject: session.Subject,
		Email:   session.Email,
		Name:    session.Name,
	}, nil
}

// RefreshSession refreshes an expired session.
func (s *service) RefreshSession(ctx context.Context, sessionID string) (*Session, error) {
	session, err := s.sessionStore.Get(ctx, sessionID)
	if err != nil {
		return nil, &AuthError{
			Code:    ErrCodeSessionNotFound,
			Message: "session not found",
			Err:     err,
		}
	}

	token, err := s.provider.RefreshToken(ctx, session.RefreshToken)
	if err != nil {
		return nil, &AuthError{
			Code:    ErrCodeTokenExpired,
			Message: "failed to refresh token",
			Err:     err,
		}
	}

	session.AccessToken = token.AccessToken
	session.RefreshToken = token.RefreshToken
	session.TokenExpiry = token.Expiry
	if idToken, ok := token.Extra("id_token").(string); ok && idToken != "" {
		session.IDToken = idToken
	}

	if err := s.sessionStore.Save(ctx, sessionID, session); err != nil {
		return nil, fmt.Errorf("failed to save refreshed session: %w", err)
	}

	return session, nil
}

// refuseUnusableAccount rejects a sign-in for an account this platform has
// deactivated or blocked, matching what simple auth enforces in
// simpleAuthService.Authenticate.
//
// The check has to read the persisted row rather than the profile assembled
// from the IdP claims: that profile always carries IsActive true, because the
// same struct doubles as the create payload for a first-time SSO user. The
// IdP answers "who is this", only our row answers "may they still come in".
// refuseDeletedAccount turns the repository's "this person was deleted" into a
// refusal the caller can act on, and returns nil for every other error so the
// generic path still wraps those.
//
// Kept distinct from refuseUnusableAccount below, which answers a different
// question. That one reads flags on a row that exists; this one is about a row
// that deliberately does not, so there is no account to inspect and nothing the
// user can do about it themselves. Both end as ErrCodeUnauthorized, because to
// the person signing in the difference is not actionable, but the log lines
// differ and that is where an administrator looks.
func (s *service) refuseDeletedAccount(err error, userInfo *UserInfo) *AuthError {
	if !errors.Is(err, users.ErrUserDeleted) {
		return nil
	}

	s.logger.Warn("Authentication failed - the account was deleted",
		logger.Str("subject", userInfo.Subject),
		logger.Str("email", userInfo.Email))

	return &AuthError{
		Code:    ErrCodeUnauthorized,
		Message: "account has been deleted",
		Err:     err,
	}
}

func (s *service) refuseUnusableAccount(user *domain.User) error {
	if user == nil || (user.IsActive && !user.IsBlocked) {
		return nil
	}

	s.logger.Warn("Authentication failed - account inactive or blocked",
		logger.Uint("user_id", user.ID),
		logger.Str("subject", user.Subject),
		logger.Bool("is_active", user.IsActive),
		logger.Bool("is_blocked", user.IsBlocked))

	return &AuthError{
		Code:    ErrCodeUnauthorized,
		Message: "account is inactive or blocked",
	}
}

// FindOrCreateUser finds or creates a user from OAuth2 user info.
func (s *service) FindOrCreateUser(ctx context.Context, userInfo *UserInfo) (*domain.User, error) {
	user := &domain.User{
		Subject:           userInfo.Subject,
		Email:             userInfo.Email,
		EmailVerified:     userInfo.EmailVerified, // See Callback.
		Name:              userInfo.Name,
		GivenName:         userInfo.GivenName,
		FamilyName:        userInfo.FamilyName,
		PreferredUsername: userInfo.PreferredUsername,
		Locale:            userInfo.Locale,
		AuthMethod:        domain.AuthMethodOAuth,
		IsActive:          true,
	}

	dbUser, err := s.userRepository.FindOrCreateBySubject(ctx, user)
	if err != nil {
		if authErr := s.refuseDeletedAccount(err, userInfo); authErr != nil {
			return nil, authErr
		}
		return nil, fmt.Errorf("failed to find or create user: %w", err)
	}

	if err := s.refuseUnusableAccount(dbUser); err != nil {
		return nil, err
	}

	return dbUser, nil
}

// generateState generates a random state for OAuth2 flow.
func generateState() string {
	state, err := GenerateRandomState()
	if err != nil {
		// Fallback to a less secure but functional state
		return fmt.Sprintf("state_%d", time.Now().UnixNano())
	}
	return state
}

// generateSessionID generates a random session ID.
func generateSessionID() string {
	sessionID, err := GenerateSessionID()
	if err != nil {
		// Fallback to a less secure but functional ID
		return fmt.Sprintf("session_%d", time.Now().UnixNano())
	}
	return sessionID
}
