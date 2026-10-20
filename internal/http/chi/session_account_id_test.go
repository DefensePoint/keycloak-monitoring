package chi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// A simple-auth user's subject is their username. Renaming alice to al
// frees "alice", and a new account can take it: a session cookie from the old
// alice then resolves to the new account by subject. The cookie also carries
// the ID of the account it was issued for, and the middleware refuses it when
// the subject now names another one.
func TestAuthenticateFromSession_RefusesASessionIssuedForAnotherAccount(t *testing.T) {
	mgr := auth.NewCookieSessionManager("0123456789abcdef0123456789abcdef", "kmt", 86400, false, "lax")
	for _, tc := range []struct {
		name    string
		fill    func(map[interface{}]interface{})
		allowed bool
	}{
		{"issued for the account the subject names", func(map[interface{}]interface{}) {}, true},
		{"the subject now names another account", func(v map[interface{}]interface{}) { v["user_id"] = uint(8) }, false},
		{"no account ID, issued before sessions carried one", func(v map[interface{}]interface{}) { delete(v, "user_id") }, false},
		{"account ID zero", func(v map[interface{}]interface{}) { v["user_id"] = uint(0) }, false},
		{"account ID of another type", func(v map[interface{}]interface{}) { v["user_id"] = "7" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewAuthMiddleware(mgr, nil, lookupResult{user: &UserDetails{ID: 7, IsActive: true}}, nil, logger.NewNoop())
			user, err := m.authenticateFromSession(signedSessionRequest(t, mgr, tc.fill))
			if tc.allowed && (err != nil || user == nil || user.ID != 7) {
				t.Fatalf("want accepted, got user=%v err=%v", user, err)
			}
			if !tc.allowed && err == nil {
				t.Fatalf("want refused, got user=%v", user)
			}
		})
	}
}

// fixedAuth signs in one account, of the given ID.
type fixedAuth struct{ id uint }

func (a fixedAuth) Authenticate(_ context.Context, username, _ string) (*auth.UserInfo, error) {
	return &auth.UserInfo{ID: a.id, Subject: username, Email: username + "@example.invalid"}, nil
}
func (fixedAuth) ChangePasswordByUsername(context.Context, string, string, string) error {
	return nil
}
func (fixedAuth) GetPasswordRequirements() auth.PasswordRequirements {
	return auth.PasswordRequirements{}
}

// A simple login stores the account's ID, and the session it opens is
// accepted for that account and refused once its subject names another.
func TestSimpleLogin_TheSessionBelongsToTheAccountThatSignedIn(t *testing.T) {
	mgr := auth.NewCookieSessionManager("0123456789abcdef0123456789abcdef", "kmt", 86400, false, "lax")
	h := NewAuthHandlers(nil, fixedAuth{id: 42}, nil, nil, mgr.GetStore(), "kmt", logger.NewNoop(), true, false, nil, nil)

	w := httptest.NewRecorder()
	h.handleSimpleLogin(w, httptest.NewRequest(http.MethodPost, "/auth/login/simple", strings.NewReader(`{"username":"alice","password":"pw"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	withCookie := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/auth/userinfo", nil)
		for _, c := range w.Result().Cookies() {
			req.AddCookie(c)
		}
		return req
	}
	session, err := mgr.GetStore().Get(withCookie(), "kmt")
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := sessionUserID(session); !ok || id != 42 {
		t.Fatalf("session account ID = %v (%v), want 42", id, ok)
	}

	for _, tc := range []struct {
		holder  uint
		allowed bool
	}{{42, true}, {43, false}} {
		m := NewAuthMiddleware(mgr, nil, lookupResult{user: &UserDetails{ID: tc.holder, IsActive: true}}, nil, logger.NewNoop())
		_, err := m.authenticateFromSession(withCookie())
		if tc.allowed != (err == nil) {
			t.Errorf("subject held by account %d: err=%v, want allowed=%v", tc.holder, err, tc.allowed)
		}
	}
}

// The SSO callback stores the ID of the account it found or created.
func TestHandleCallback_TheSessionCarriesTheAccountID(t *testing.T) {
	rec := callbackStatusFor(t, nil)
	if rec.Code >= 400 {
		t.Fatalf("callback: %d %s", rec.Code, rec.Body)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	session, err := testSessionStore().Get(req, "kmt_test")
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := sessionUserID(session); !ok || id != 1 {
		t.Fatalf("session account ID = %v (%v), want 1, the account FindOrCreateUser returned", id, ok)
	}
}
