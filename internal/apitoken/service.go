package apitoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

const tokenPrefix = "pat_"

// Service defines the interface for API token operations.
type Service interface {
	// Create issues a new token for a user. actorID is the user performing the
	// call, recorded as created_by: an administrator can issue a token that
	// acts as someone else, so the audit line must say who did. tenantIDs is
	// the token's tenant allowlist and must name at least one tenant. The
	// plaintext token is returned exactly once in the result and is never
	// stored or logged.
	Create(ctx context.Context, actorID, userID uint, name string, tenantIDs []string, expiresAt *time.Time) (*CreatedToken, error)

	// List retrieves metadata for all tokens.
	List(ctx context.Context) ([]*Token, error)

	// Revoke marks a token as revoked. actorID is the user performing the
	// call, recorded as revoked_by.
	Revoke(ctx context.Context, actorID, id uint) error

	// Validate resolves a plaintext token to the identity it authenticates.
	Validate(ctx context.Context, plaintext string) (*Identity, error)
}

// service implements the Service interface.
type service struct {
	repo  Repository
	users UserStore
	log   Logger

	// ssoSignInMaxAge is how long an SSO user's tokens keep working after
	// their last SSO sign-in. Zero turns the check off.
	ssoSignInMaxAge time.Duration
}

// Option configures a Service.
type Option func(*service)

// WithSSOSignInMaxAge refuses tokens of SSO users whose last SSO sign-in is
// older than maxAge. Zero, the default, turns the check off.
func WithSSOSignInMaxAge(maxAge time.Duration) Option {
	return func(s *service) { s.ssoSignInMaxAge = maxAge }
}

// NewService creates a new API token service.
func NewService(repo Repository, users UserStore, log Logger, opts ...Option) Service {
	s := &service{repo: repo, users: users, log: log}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Create issues a new token for a user.
func (s *service) Create(ctx context.Context, actorID, userID uint, name string, tenantIDs []string, expiresAt *time.Time) (*CreatedToken, error) {
	if actorID == 0 {
		return nil, ErrNoActor
	}
	if len(tenantIDs) == 0 {
		return nil, ErrNoTenantIDs
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to load user: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	now := time.Now().UTC()
	exp := now.Add(DefaultExpiry)
	if expiresAt != nil {
		if expiresAt.Before(now) {
			return nil, ErrExpiryInPast
		}
		exp = expiresAt.UTC()
		if max := now.Add(MaxExpiry); exp.After(max) {
			exp = max
		}
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	plaintext := tokenPrefix + base64.RawURLEncoding.EncodeToString(raw)

	token := &Token{
		UserID:    userID,
		Name:      name,
		TenantIDs: tenantIDs,
		ExpiresAt: &exp,
	}
	if err := s.repo.Create(ctx, token, digestOf(plaintext)); err != nil {
		return nil, fmt.Errorf("failed to create api token: %w", err)
	}

	s.log.Info("API token created",
		"token_id", token.ID,
		"user_id", userID,
		"created_by", actorID,
		"name", name,
		"tenant_ids", tenantIDs,
		"expires_at", exp)

	return &CreatedToken{Token: *token, Plaintext: plaintext}, nil
}

// List retrieves metadata for all tokens.
func (s *service) List(ctx context.Context) ([]*Token, error) {
	return s.repo.List(ctx)
}

// Revoke marks a token as revoked.
func (s *service) Revoke(ctx context.Context, actorID, id uint) error {
	if actorID == 0 {
		return ErrNoActor
	}
	if err := s.repo.Revoke(ctx, id, time.Now().UTC()); err != nil {
		return err
	}
	s.log.Info("API token revoked", "token_id", id, "revoked_by", actorID)
	return nil
}

// Validate resolves a plaintext token to the identity it authenticates.
func (s *service) Validate(ctx context.Context, plaintext string) (*Identity, error) {
	token, err := s.repo.GetByDigest(ctx, digestOf(plaintext))
	if err != nil {
		return nil, fmt.Errorf("failed to look up api token: %w", err)
	}
	if token == nil {
		return nil, ErrTokenNotFound
	}
	if token.RevokedAt != nil {
		return nil, ErrTokenRevoked
	}
	if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	// Tokens predating the mandatory allowlist are rejected rather than
	// grandfathered: an unscoped token is exactly the thing the allowlist
	// exists to prevent, and its holder can mint a scoped replacement.
	if len(token.TenantIDs) == 0 {
		return nil, ErrTokenUnscoped
	}

	user, err := s.users.GetByID(ctx, token.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to load user: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	if user.IsBlocked {
		return nil, ErrUserBlocked
	}
	if !user.IsActive {
		return nil, ErrUserInactive
	}
	if s.ssoSignInTooOld(user) {
		return nil, ErrSSOSignInTooOld
	}

	return &Identity{TokenID: token.ID, User: user, TenantIDs: token.TenantIDs}, nil
}

// digestOf returns the SHA-256 hex digest of a plaintext token.
// ssoSignInTooOld reports whether user is an SSO user whose last SSO sign-in is
// older than the limit. Nothing here asks the IdP: a user it has disabled
// cannot sign in, so their last sign-in only ages. Local users never sign in
// through SSO and are not affected.
//
// A user with no recorded sign-in is not refused. Right after an upgrade every
// SSO user has none until the API server's startup stamps one, and the MCP
// server can start first; refusing them would cut off every SSO user's MCP
// access for as long as the API server takes, or fails, to come up. That
// startup stamp gives each of them a sign-in time, so they still age out.
func (s *service) ssoSignInTooOld(user *domain.User) bool {
	if s.ssoSignInMaxAge <= 0 || user.AuthMethod != domain.AuthMethodOAuth || user.LastLoginAt == nil {
		return false
	}
	return time.Since(*user.LastLoginAt) > s.ssoSignInMaxAge
}

func digestOf(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
