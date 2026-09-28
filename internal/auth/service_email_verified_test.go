package auth

import (
	"context"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// email_verified is the IdP's statement about an address, not a property of
// having used OAuth. Both login paths once stored true unconditionally, so an
// address nobody had confirmed still read as confirmed. These tests pin the
// claim to what the provider actually said, in both directions — asserting only
// the true case would pass just as well against the hardcoded value.

// newEmailVerifiedService builds the callback service with a provider that
// reports the given claim, and an active stored user so the login completes.
func newEmailVerifiedService(claim bool) (*service, *callbackUserRepoStub) {
	users := &callbackUserRepoStub{storedUser: &domain.User{
		ID: 1, Subject: "idp-subject", Email: "person@example.com",
		IsActive: true, IsBlocked: false,
	}}
	provider := &callbackProviderStub{userInfo: &UserInfo{
		Subject:       "idp-subject",
		Email:         "person@example.com",
		EmailVerified: claim,
		Name:          "Person",
	}}
	svc := NewService(provider, &callbackSessionStoreSpy{}, users, logger.NewNoop(), &Config{}).(*service)
	return svc, users
}

func TestCallback_EmailVerifiedFollowsTheIdP(t *testing.T) {
	for _, claim := range []bool{true, false} {
		svc, users := newEmailVerifiedService(claim)

		if _, _, err := svc.Callback(context.Background(), "code", "state", "state"); err != nil {
			t.Fatalf("claim=%v: active user should sign in, got error: %v", claim, err)
		}
		if users.received == nil {
			t.Fatalf("claim=%v: expected a profile to reach the repository", claim)
		}
		if users.received.EmailVerified != claim {
			t.Errorf("claim=%v: stored EmailVerified = %v, want %v — the IdP's claim must be "+
				"recorded, not assumed", claim, users.received.EmailVerified, claim)
		}
	}
}

func TestFindOrCreateUser_EmailVerifiedFollowsTheIdP(t *testing.T) {
	for _, claim := range []bool{true, false} {
		svc, users := newEmailVerifiedService(claim)

		_, err := svc.FindOrCreateUser(context.Background(), &UserInfo{
			Subject:       "idp-subject",
			Email:         "person@example.com",
			EmailVerified: claim,
		})
		if err != nil {
			t.Fatalf("claim=%v: active user should be returned, got error: %v", claim, err)
		}
		if users.received == nil {
			t.Fatalf("claim=%v: expected a profile to reach the repository", claim)
		}
		if users.received.EmailVerified != claim {
			t.Errorf("claim=%v: stored EmailVerified = %v, want %v", claim,
				users.received.EmailVerified, claim)
		}
	}
}
