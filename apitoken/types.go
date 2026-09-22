// Package apitoken provides personal access token issuance and validation.
package apitoken

import (
	"errors"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

const (
	// DefaultExpiry applies when no expiry is requested at creation.
	DefaultExpiry = 90 * 24 * time.Hour

	// MaxExpiry caps a requested expiry.
	MaxExpiry = 365 * 24 * time.Hour
)

var (
	ErrTokenNotFound = errors.New("api token not found")
	ErrTokenRevoked  = errors.New("api token revoked")
	ErrTokenExpired  = errors.New("api token expired")
	ErrTokenUnscoped = errors.New("api token has no tenant allowlist")
	ErrNoTenantIDs   = errors.New("api token requires at least one tenant id")
	ErrUserNotFound  = errors.New("api token user not found")
	ErrUserInactive  = errors.New("api token user is inactive")
	ErrUserBlocked   = errors.New("api token user is blocked")
	ErrExpiryInPast  = errors.New("api token expiry is in the past")
)

// Token represents API token metadata. It never carries the token digest;
// the digest stays inside the repository layer.
type Token struct {
	ID        uint       `json:"id"`
	UserID    uint       `json:"user_id"`
	Name      string     `json:"name"`
	TenantIDs []string   `json:"tenant_ids,omitempty"` // Non-empty allowlist; a token always names its tenants
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreatedToken is the result of creating a token. Plaintext is available
// only here, exactly once at creation time.
type CreatedToken struct {
	Token
	Plaintext string `json:"token"`
}

// Identity is the authenticated principal a valid token resolves to.
type Identity struct {
	// TokenID is the ID of the exact token that authenticated, distinct from
	// the user it resolves to. Consumers bind per-token artifacts, such as
	// MCP pagination cursors, to it.
	TokenID   uint
	User      *domain.User
	TenantIDs []string
}
