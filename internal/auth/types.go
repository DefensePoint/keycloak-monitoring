// Package auth provides authentication and authorization functionality.
package auth

import (
	"time"
)

// UserInfo represents authenticated user information from the identity provider.
// This is a local DTO used for OAuth/OIDC flows.
type UserInfo struct {
	ID                 uint      `json:"id"`
	Subject            string    `json:"subject"`
	Email              string    `json:"email"`
	EmailVerified      bool      `json:"email_verified"`
	Name               string    `json:"name"`
	GivenName          string    `json:"given_name"`
	FamilyName         string    `json:"family_name"`
	PreferredUsername  string    `json:"preferred_username"`
	Locale             string    `json:"locale"`
	UpdatedAt          time.Time `json:"updated_at"`
	MustChangePassword bool      `json:"must_change_password"`
}

// Session represents an authenticated user session.
// This is a local DTO for session management.
type Session struct {
	UserID         uint      `json:"user_id"` // Numeric user ID from database
	Subject        string    `json:"subject"` // OAuth2 subject (sub claim)
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	AccessToken    string    `json:"access_token"`
	RefreshToken   string    `json:"refresh_token"`
	IDToken        string    `json:"id_token"`
	TokenExpiry    time.Time `json:"token_expiry"`
	CreatedAt      time.Time `json:"created_at"`
	LastAccessedAt time.Time `json:"last_accessed_at"`
}

// Config represents OAuth2/OIDC configuration.
type Config struct {
	ProviderURL     string
	ClientID        string
	ClientSecret    string
	RedirectURL     string
	Scopes          []string
	SessionSecret   string
	SessionMaxAge   time.Duration
	SessionSecure   bool
	SessionSameSite string
	SkipIssuerCheck bool
	SkipExpiryCheck bool
}

// AuthError represents authentication-related errors.
type AuthError struct {
	Code    string
	Message string
	Err     error
}

func (e *AuthError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *AuthError) Unwrap() error {
	return e.Err
}

// Common error codes.
const (
	ErrCodeInvalidToken    = "INVALID_TOKEN"
	ErrCodeTokenExpired    = "TOKEN_EXPIRED"
	ErrCodeUnauthorized    = "UNAUTHORIZED"
	ErrCodeSessionNotFound = "SESSION_NOT_FOUND"
	ErrCodeProviderError   = "PROVIDER_ERROR"
	ErrCodeInvalidState    = "INVALID_STATE"
	ErrCodeExchangeFailed  = "EXCHANGE_FAILED"
)
