package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
)

// The OAuth2 callback must refuse an account the platform has deactivated or
// blocked, exactly as simple auth does. The account flags live on the row the
// repository returns, not on the profile built from the IdP claims: the claims
// describe who the IdP says you are, the row describes whether this platform
// still lets you in.

type callbackProviderStub struct{ userInfo *UserInfo }

func (p *callbackProviderStub) GetAuthCodeURL(_ string, _ ...oauth2.AuthCodeOption) string {
	return "https://idp.invalid/auth"
}

func (p *callbackProviderStub) Exchange(_ context.Context, _ string, _ ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return (&oauth2.Token{AccessToken: "access", Expiry: time.Now().Add(time.Hour)}).
		WithExtra(map[string]interface{}{"id_token": "id-token"}), nil
}

func (p *callbackProviderStub) VerifyIDToken(_ context.Context, _ string) (*UserInfo, error) {
	return p.userInfo, nil
}

func (p *callbackProviderStub) VerifyLoginIDToken(_ context.Context, _, _ string) (*UserInfo, error) {
	return p.userInfo, nil
}

func (p *callbackProviderStub) RefreshToken(_ context.Context, _ string) (*oauth2.Token, error) {
	return nil, nil
}

type callbackSessionStoreSpy struct{ saved int }

func (s *callbackSessionStoreSpy) Save(_ context.Context, _ string, _ *Session) error {
	s.saved++
	return nil
}
func (s *callbackSessionStoreSpy) Get(_ context.Context, _ string) (*Session, error) {
	return nil, nil
}
func (s *callbackSessionStoreSpy) Delete(_ context.Context, _ string) error             { return nil }
func (s *callbackSessionStoreSpy) UpdateLastAccessed(_ context.Context, _ string) error { return nil }

// callbackUserRepoStub returns storedUser regardless of what is passed in, and
// records the profile the service handed it. That mirrors production: the
// service always passes IsActive true, and the persisted row is authoritative.
type callbackUserRepoStub struct {
	storedUser *domain.User
	received   *domain.User
	err        error
}

func (r *callbackUserRepoStub) FindOrCreateBySubject(_ context.Context, user *domain.User) (*domain.User, error) {
	r.received = user
	if r.err != nil {
		return nil, r.err
	}
	return r.storedUser, nil
}
func (r *callbackUserRepoStub) GetBySubject(_ context.Context, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *callbackUserRepoStub) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *callbackUserRepoStub) GetByUsername(_ context.Context, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *callbackUserRepoStub) UpdateLastAccessed(_ context.Context, _ string) error { return nil }
func (r *callbackUserRepoStub) CreateSimpleAuthUser(_ context.Context, _ *domain.User) error {
	return nil
}
func (r *callbackUserRepoStub) UpdatePasswordWithFlags(_ context.Context, _ uint, _ string, _ *time.Time, _ bool) error {
	return nil
}

func newCallbackService(stored *domain.User) (*service, *callbackSessionStoreSpy, *callbackUserRepoStub) {
	sessions := &callbackSessionStoreSpy{}
	users := &callbackUserRepoStub{storedUser: stored}
	provider := &callbackProviderStub{userInfo: &UserInfo{
		Subject: "idp-subject",
		Email:   "person@example.com",
		Name:    "Person",
	}}
	svc := NewService(provider, sessions, users, logger.NewNoop(), &Config{}).(*service)
	return svc, sessions, users
}

// FindOrCreateUser is the method the OAuth2 callback reaches: handleCallback
// (internal/http/chi/auth_handlers.go) calls it and then mints a session.

func TestFindOrCreateUser_ActiveUserIsReturned(t *testing.T) {
	svc, _, _ := newCallbackService(&domain.User{
		ID: 1, Subject: "idp-subject", IsActive: true, IsBlocked: false,
	})

	user, err := svc.FindOrCreateUser(context.Background(), &UserInfo{Subject: "idp-subject"})
	if err != nil {
		t.Fatalf("active user should be returned, got error: %v", err)
	}
	if user == nil {
		t.Fatal("expected a user for an active account")
	}
}

func TestFindOrCreateUser_DeactivatedUserIsRefused(t *testing.T) {
	svc, _, users := newCallbackService(&domain.User{
		ID: 1, Subject: "idp-subject", IsActive: false, IsBlocked: false,
	})

	user, err := svc.FindOrCreateUser(context.Background(), &UserInfo{Subject: "idp-subject"})
	if err == nil {
		t.Fatal("a deactivated account must be refused on the live SSO path")
	}
	if user != nil {
		t.Errorf("no user may be handed back for a deactivated account, got %v", user)
	}
	// Pins the reason this bug was invisible: the profile built from IdP claims
	// always says active, so only the persisted row can refuse the login.
	if users.received == nil || !users.received.IsActive {
		t.Errorf("expected the service to pass an active profile in, so the refusal must come from the stored row")
	}
}

func TestFindOrCreateUser_BlockedUserIsRefused(t *testing.T) {
	svc, _, _ := newCallbackService(&domain.User{
		ID: 1, Subject: "idp-subject", IsActive: true, IsBlocked: true,
	})

	user, err := svc.FindOrCreateUser(context.Background(), &UserInfo{Subject: "idp-subject"})
	if err == nil {
		t.Fatal("a blocked account must be refused on the live SSO path")
	}
	if user != nil {
		t.Errorf("no user may be handed back for a blocked account, got %v", user)
	}
}

func TestFindOrCreateUser_RefusalMatchesSimpleAuth(t *testing.T) {
	svc, _, _ := newCallbackService(&domain.User{
		ID: 1, Subject: "idp-subject", IsActive: false,
	})

	_, err := svc.FindOrCreateUser(context.Background(), &UserInfo{Subject: "idp-subject"})

	authErr, ok := err.(*AuthError)
	if !ok {
		t.Fatalf("expected *AuthError so both login paths answer alike, got %T: %v", err, err)
	}
	if authErr.Code != ErrCodeUnauthorized {
		t.Errorf("expected %q to match simple auth, got %q", ErrCodeUnauthorized, authErr.Code)
	}
}

// A login whose address belongs to another account is a refusal, not a server
// fault: the callback answers 401 for ErrCodeUnauthorized and 500 otherwise,
// and a 500 would send whoever investigates looking for a bug instead of at the
// account the log names.
func TestFindOrCreateUser_AnEmailHeldByAnotherAccountIsRefused(t *testing.T) {
	svc, _, repo := newCallbackService(nil)
	repo.err = fmt.Errorf("wrapped: %w", &users.EmailInUseError{UserID: 7})

	user, err := svc.FindOrCreateUser(context.Background(), &UserInfo{
		Subject: "new-subject", Email: "person@example.com", EmailVerified: true,
	})
	if user != nil {
		t.Errorf("no user may be handed back, got %v", user)
	}
	var authErr *AuthError
	if !errors.As(err, &authErr) || authErr.Code != ErrCodeUnauthorized {
		t.Fatalf("want an unauthorized AuthError, got %v", err)
	}
	if !errors.Is(err, users.ErrEmailInUse) {
		t.Errorf("the refusal lost the cause: %v", err)
	}
}
