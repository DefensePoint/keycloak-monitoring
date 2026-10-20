package chi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// raceRevoker keeps each subject's session epoch.
type raceRevoker struct {
	mu    sync.Mutex
	epoch map[string]int64
}

func (r *raceRevoker) CurrentSessionEpoch(_ context.Context, subject string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// As the real one: an unknown subject reads as 0, not as an error.
	return r.epoch[subject], nil
}

func (r *raceRevoker) RevokeSessions(context.Context, string, int64) (bool, error) { return false, nil }

func (r *raceRevoker) bump(subject string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.epoch[subject]++
}

// changingAuth reads the account's row, epoch with the password hash, and
// while it checks the password an administrator changes it: the stored epoch
// moves past the one read. withoutEpoch is an authenticator that does not
// return the epoch.
type changingAuth struct {
	revoker      *raceRevoker
	subject      string
	withoutEpoch bool
}

func (a changingAuth) Authenticate(ctx context.Context, username, _ string) (*auth.UserInfo, error) {
	read, _ := a.revoker.CurrentSessionEpoch(ctx, a.subject)
	a.revoker.bump(a.subject)
	info := &auth.UserInfo{Subject: a.subject, Email: username + "@example.invalid"}
	if !a.withoutEpoch {
		info.SessionEpoch = &read
	}
	return info, nil
}
func (changingAuth) ChangePasswordByUsername(context.Context, string, string, string) error {
	return nil
}
func (changingAuth) GetPasswordRequirements() auth.PasswordRequirements {
	return auth.PasswordRequirements{}
}

func loginEpoch(t *testing.T, subject string, withoutEpoch bool) (cookieEpoch, current int64) {
	t.Helper()
	mgr := auth.NewCookieSessionManager("0123456789abcdef0123456789abcdef", "kmt", 86400, false, "lax")
	revoker := &raceRevoker{epoch: map[string]int64{subject: 4}}
	h := NewAuthHandlers(nil, changingAuth{revoker: revoker, subject: subject, withoutEpoch: withoutEpoch}, nil, nil, mgr.GetStore(), "kmt", logger.NewNoop(), true, false, nil, nil)
	h.SetSessionRevoker(revoker)

	w := httptest.NewRecorder()
	h.handleSimpleLogin(w, httptest.NewRequest(http.MethodPost, "/auth/login/simple", strings.NewReader(`{"username":"bob","password":"old-password"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/userinfo", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}
	session, err := mgr.GetStore().Get(req, "kmt")
	if err != nil {
		t.Fatal(err)
	}
	current, _ = revoker.CurrentSessionEpoch(context.Background(), subject)
	return sessionEpoch(session), current
}

// A password changed while the login checks the old one: the session gets
// the epoch read with that password, so the middleware refuses it. Whatever
// the subject: the bootstrap admin's is its email, not its username.
func TestSimpleLogin_APasswordChangedDuringTheLoginLeavesTheSessionRefused(t *testing.T) {
	for _, subject := range []string{"bob", "admin@example.com"} {
		cookie, current := loginEpoch(t, subject, false)
		if cookie >= current {
			t.Errorf("subject %s: session epoch %d, account epoch %d; want the session older, so refused", subject, cookie, current)
		}
	}
}

// An authenticator that does not return the epoch gets the current one read
// after the check, as before.
func TestSimpleLogin_WithoutAnEpochFromTheAuthenticatorReadsTheCurrentOne(t *testing.T) {
	cookie, current := loginEpoch(t, "bob", true)
	if cookie != current {
		t.Errorf("session epoch %d, account epoch %d; want the current one", cookie, current)
	}
}

// lookupResult answers GetUserBySubject with a fixed result.
type lookupResult struct {
	user *UserDetails
	err  error
}

func (l lookupResult) GetUserBySubject(context.Context, string) (*UserDetails, error) {
	return l.user, l.err
}

// A session whose account no longer has its subject (an admin renamed a
// simple-auth user) is refused, not accepted with no checks. A failure to
// look the account up still falls back to the cookie.
func TestAuthenticateFromSession_RefusesASessionWhoseAccountIsGone(t *testing.T) {
	mgr := auth.NewCookieSessionManager("0123456789abcdef0123456789abcdef", "kmt", 86400, false, "lax")
	fill := func(v map[interface{}]interface{}) { v["session_epoch"] = int64(0) }
	for _, tc := range []struct {
		name    string
		lookup  lookupResult
		allowed bool
	}{
		{"no account has the subject", lookupResult{err: ErrUnknownAccount}, false},
		{"no account and no error", lookupResult{}, false},
		{"the lookup failed", lookupResult{err: errors.New("connection refused")}, true},
		{"the account exists and is active", lookupResult{user: &UserDetails{ID: 7, IsActive: true}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewAuthMiddleware(mgr, nil, tc.lookup, nil, logger.NewNoop())
			user, err := m.authenticateFromSession(signedSessionRequest(t, mgr, fill))
			if tc.allowed && err != nil {
				t.Fatalf("want accepted, got %v", err)
			}
			if !tc.allowed && err == nil {
				t.Fatalf("want refused, got user=%v", user)
			}
		})
	}
}
