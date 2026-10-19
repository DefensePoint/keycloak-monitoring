package apitoken

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// A user disabled in the IdP cannot sign in, so their last SSO sign-in only
// ages. These cases pin which tokens the sign-in limit refuses.
func TestValidateSSOSignInLimit(t *testing.T) {
	const plaintext = "pat_c3NvLXNpZ25pbi1saW1pdC10ZXN0LXRva2VuLWFh"
	const limit = 14 * 24 * time.Hour
	expiry := time.Now().Add(time.Hour)
	ago := func(d time.Duration) *time.Time { t := time.Now().Add(-d); return &t }

	tests := []struct {
		name      string
		method    domain.AuthMethod
		lastLogin *time.Time
		maxAge    time.Duration
		wantErr   error
	}{
		{"SSO user signed in within the limit", domain.AuthMethodOAuth, ago(13 * 24 * time.Hour), limit, nil},
		{"SSO user whose sign-in is older than the limit", domain.AuthMethodOAuth, ago(15 * 24 * time.Hour), limit, ErrSSOSignInTooOld},
		// Right after an upgrade, before the API server stamps a sign-in time.
		{"SSO user with no recorded sign-in yet", domain.AuthMethodOAuth, nil, limit, nil},
		{"SSO user past the limit with the check off", domain.AuthMethodOAuth, ago(400 * 24 * time.Hour), 0, nil},
		{"local user past the limit", domain.AuthMethodSimple, ago(400 * 24 * time.Hour), limit, nil},
		{"local user with no recorded sign-in", domain.AuthMethodSimple, nil, limit, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{getByDigestFn: func(context.Context, string) (*Token, error) {
				return &Token{ID: 1, UserID: 42, TenantIDs: []string{"tenant-1"}, ExpiresAt: &expiry}, nil
			}}
			users := &mockUserStore{getByIDFn: func(context.Context, uint) (*domain.User, error) {
				u := activeUser(42)
				u.AuthMethod = tt.method
				u.LastLoginAt = tt.lastLogin
				return u, nil
			}}
			svc := NewService(repo, users, noopLogger{}, WithSSOSignInMaxAge(tt.maxAge))

			identity, err := svc.Validate(context.Background(), plaintext)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && identity == nil {
				t.Fatal("Validate() returned no identity for an accepted token")
			}
		})
	}
}

// Signing in again restores a refused token: the check reads the latest
// sign-in on every use and revokes nothing.
func TestValidateSSOSignInAgainRestoresTheToken(t *testing.T) {
	const plaintext = "pat_c3NvLXNpZ25pbi1hZ2Fpbi10ZXN0LXRva2VuLWJi"
	expiry := time.Now().Add(time.Hour)
	stale := time.Now().Add(-30 * 24 * time.Hour)
	user := activeUser(42)
	user.AuthMethod = domain.AuthMethodOAuth
	user.LastLoginAt = &stale

	repo := &mockRepository{getByDigestFn: func(context.Context, string) (*Token, error) {
		return &Token{ID: 1, UserID: 42, TenantIDs: []string{"tenant-1"}, ExpiresAt: &expiry}, nil
	}}
	users := &mockUserStore{getByIDFn: func(context.Context, uint) (*domain.User, error) { return user, nil }}
	svc := NewService(repo, users, noopLogger{}, WithSSOSignInMaxAge(14*24*time.Hour))

	if _, err := svc.Validate(context.Background(), plaintext); !errors.Is(err, ErrSSOSignInTooOld) {
		t.Fatalf("before signing in again: error = %v, want %v", err, ErrSSOSignInTooOld)
	}
	now := time.Now()
	user.LastLoginAt = &now
	if _, err := svc.Validate(context.Background(), plaintext); err != nil {
		t.Fatalf("after signing in again: error = %v, want the token accepted", err)
	}
}
