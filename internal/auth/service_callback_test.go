package auth

import (
	"context"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// The OAuth2 callback must refuse an account the platform has deactivated or
// blocked, exactly as simple auth does. The account flags live on the row the
// repository returns, not on the profile built from the IdP claims: the claims
// describe who the IdP says you are, the row describes whether this platform
// still lets you in.

type callbackProviderStub struct{ userInfo *UserInfo }

func (p *callbackProviderStub) GetAuthCodeURL(state string) string { return "https://idp.invalid/auth" }

func (p *callbackProviderStub) Exchange(_ context.Context, _ string) (*oauth2.Token, error) {
	return (&oauth2.Token{AccessToken: "access", Expiry: time.Now().Add(time.Hour)}).
		WithExtra(map[string]interface{}{"id_token": "id-token"}), nil
}

func (p *callbackProviderStub) VerifyIDToken(_ context.Context, _ string) (*UserInfo, error) {
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
}

func (r *callbackUserRepoStub) FindOrCreateBySubject(_ context.Context, user *domain.User) (*domain.User, error) {
	r.received = user
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

func TestCallback_ActiveUserGetsASession(t *testing.T) {
	svc, sessions, _ := newCallbackService(&domain.User{
		ID: 1, Subject: "idp-subject", Email: "person@example.com",
		IsActive: true, IsBlocked: false,
	})

	session, user, err := svc.Callback(context.Background(), "code", "state", "state")
	if err != nil {
		t.Fatalf("active user should sign in, got error: %v", err)
	}
	if session == nil || user == nil {
		t.Fatalf("active user should get a session and a user, got session=%v user=%v", session, user)
	}
	if sessions.saved != 1 {
		t.Errorf("expected exactly one session saved, got %d", sessions.saved)
	}
}

func TestCallback_DeactivatedUserIsRefused(t *testing.T) {
	svc, sessions, users := newCallbackService(&domain.User{
		ID: 1, Subject: "idp-subject", Email: "person@example.com",
		IsActive: false, IsBlocked: false,
	})

	session, user, err := svc.Callback(context.Background(), "code", "state", "state")
	if err == nil {
		t.Fatal("a deactivated account must not be able to sign in via SSO")
	}
	if session != nil || user != nil {
		t.Errorf("no session or user may be returned for a deactivated account, got session=%v user=%v", session, user)
	}
	if sessions.saved != 0 {
		t.Errorf("no session may be persisted for a deactivated account, saved %d", sessions.saved)
	}
	// Pins the reason this bug was invisible: the profile built from IdP claims
	// always says active, so only the persisted row can refuse the login.
	if users.received == nil || !users.received.IsActive {
		t.Errorf("expected the service to pass an active profile in, so the refusal must come from the stored row")
	}
}

func TestCallback_BlockedUserIsRefused(t *testing.T) {
	svc, sessions, _ := newCallbackService(&domain.User{
		ID: 1, Subject: "idp-subject", Email: "person@example.com",
		IsActive: true, IsBlocked: true,
	})

	session, user, err := svc.Callback(context.Background(), "code", "state", "state")
	if err == nil {
		t.Fatal("a blocked account must not be able to sign in via SSO")
	}
	if session != nil || user != nil {
		t.Errorf("no session or user may be returned for a blocked account, got session=%v user=%v", session, user)
	}
	if sessions.saved != 0 {
		t.Errorf("no session may be persisted for a blocked account, saved %d", sessions.saved)
	}
}

// FindOrCreateUser is the method the live OAuth2 callback actually reaches:
// handleCallback (internal/http/chi/auth_handlers.go) calls it directly and
// then mints a session, never going through service.Callback. Guarding only
// Callback would leave the real login path open.

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
	svc, _, _ := newCallbackService(&domain.User{
		ID: 1, Subject: "idp-subject", IsActive: false, IsBlocked: false,
	})

	user, err := svc.FindOrCreateUser(context.Background(), &UserInfo{Subject: "idp-subject"})
	if err == nil {
		t.Fatal("a deactivated account must be refused on the live SSO path")
	}
	if user != nil {
		t.Errorf("no user may be handed back for a deactivated account, got %v", user)
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

func TestCallback_RefusalMatchesSimpleAuth(t *testing.T) {
	svc, _, _ := newCallbackService(&domain.User{
		ID: 1, Subject: "idp-subject", IsActive: false,
	})

	_, _, err := svc.Callback(context.Background(), "code", "state", "state")

	authErr, ok := err.(*AuthError)
	if !ok {
		t.Fatalf("expected *AuthError so both login paths answer alike, got %T: %v", err, err)
	}
	if authErr.Code != ErrCodeUnauthorized {
		t.Errorf("expected %q to match simple auth, got %q", ErrCodeUnauthorized, authErr.Code)
	}
}
