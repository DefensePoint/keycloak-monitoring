package chi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// statusLookup answers with an account in the given state, at the epoch the
// session cookie carries.
type statusLookup struct {
	active, blocked bool
	calls           int
}

func (l *statusLookup) GetUserBySubject(_ context.Context, subject string) (*UserDetails, error) {
	l.calls++
	return &UserDetails{ID: 7, Subject: subject, IsActive: l.active, IsBlocked: l.blocked}, nil
}

// A session of an account that is inactive or blocked is refused on its next
// request, even with a current epoch: the account was cut off before
// deactivating and blocking moved the epoch, or by a path that does not.
func TestAuthenticateFromSession_RefusesAnInactiveOrBlockedAccount(t *testing.T) {
	mgr := auth.NewCookieSessionManager("0123456789abcdef0123456789abcdef", "kmt", 86400, false, "lax")
	current := func(v map[interface{}]interface{}) { v["session_epoch"] = int64(0) }

	for _, tc := range []struct {
		name            string
		active, blocked bool
		allowed         bool
	}{
		{"active", true, false, true},
		{"deactivated", false, false, false},
		{"blocked", true, true, false},
		{"deactivated and blocked", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &statusLookup{active: tc.active, blocked: tc.blocked}
			m := NewAuthMiddleware(mgr, nil, lookup, nil, logger.NewNoop())
			user, err := m.authenticateFromSession(signedSessionRequest(t, mgr, current))
			if lookup.calls != 1 {
				t.Fatalf("the account was read %d times, want once", lookup.calls)
			}
			if tc.allowed && (err != nil || user == nil || user.ID != 7) {
				t.Fatalf("want accepted, got user=%v err=%v", user, err)
			}
			if !tc.allowed && err == nil {
				t.Fatalf("want refused, got user=%v", user)
			}
		})
	}
}

// The status travels only inside the server: UserDetails is also the body of
// /auth/userinfo, and the account's flags are not part of it.
func TestUserDetails_KeepsTheAccountStatusOutOfJSON(t *testing.T) {
	b, err := json.Marshal(UserDetails{ID: 7, IsActive: true, IsBlocked: true, SessionEpoch: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"IsActive", "IsBlocked", "SessionEpoch"} {
		if strings.Contains(string(b), field) {
			t.Errorf("%s is in the JSON: %s", field, b)
		}
	}
}
