package chi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

type epochLookup struct{ epoch int64 }

func (l epochLookup) GetUserBySubject(ctx context.Context, subject string) (*UserDetails, error) {
	return &UserDetails{ID: 7, Subject: subject, SessionEpoch: l.epoch}, nil
}

// signedSessionRequest returns a request carrying a session cookie signed the
// way a login signs it, with values set by fill.
func signedSessionRequest(t *testing.T, mgr *auth.CookieSessionManager, fill func(map[interface{}]interface{})) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s, err := mgr.GetStore().New(req, mgr.GetSessionName())
	if err != nil {
		t.Fatal(err)
	}
	s.Values["authenticated"] = true
	s.Values["subject"] = "alice"
	fill(s.Values)
	rec := httptest.NewRecorder()
	if err := s.Save(req, rec); err != nil {
		t.Fatal(err)
	}
	out := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	for _, c := range rec.Result().Cookies() {
		out.AddCookie(c)
	}
	return out
}

func TestAuthenticateFromSession_HonorsTheSessionEpoch(t *testing.T) {
	mgr := auth.NewCookieSessionManager("0123456789abcdef0123456789abcdef", "kmt", 86400, false, "lax")
	epoch := func(e int64) func(map[interface{}]interface{}) {
		return func(v map[interface{}]interface{}) { v["session_epoch"] = e }
	}
	none := func(map[interface{}]interface{}) {}

	for _, tc := range []struct {
		name      string
		userEpoch int64
		fill      func(map[interface{}]interface{})
		allowed   bool
	}{
		{"never logged out, cookie from before this change", 0, none, true},
		{"never logged out, current cookie", 0, epoch(0), true},
		{"logged out since, cookie from before this change", 1, none, false},
		{"logged out since this cookie was issued", 1, epoch(0), false},
		{"cookie issued after the last logout", 1, epoch(1), true},
		{"several logouts since", 3, epoch(1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewAuthMiddleware(mgr, nil, epochLookup{epoch: tc.userEpoch}, nil, logger.NewNoop())
			user, err := m.authenticateFromSession(signedSessionRequest(t, mgr, tc.fill))
			if tc.allowed && (err != nil || user == nil || user.ID != 7) {
				t.Fatalf("want accepted, got user=%v err=%v", user, err)
			}
			if !tc.allowed && err == nil {
				t.Fatalf("want rejected, got user=%v", user)
			}
		})
	}
}
