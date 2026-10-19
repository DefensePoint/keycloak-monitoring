package auth

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// FindOrCreateBySubject writes the last_login_at it is given, so an SSO
// sign-in that leaves LastLoginAt unset stores NULL. The SSO sign-in must hand
// it the time of this sign-in: the MCP token check and the email_verified
// migration read it.
func TestSSOSignInRecordsTheSignInTime(t *testing.T) {
	stored := &domain.User{ID: 1, Subject: "idp-subject", IsActive: true}

	for name, signIn := range map[string]func(*service) error{
		"FindOrCreateUser": func(svc *service) error {
			_, err := svc.FindOrCreateUser(context.Background(), &UserInfo{Subject: "idp-subject"})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc, _, users := newCallbackService(stored)
			before := time.Now()
			if err := signIn(svc); err != nil {
				t.Fatalf("sign-in: %v", err)
			}
			got := users.received
			if got == nil || got.LastLoginAt == nil {
				t.Fatal("LastLoginAt was not set on the sign-in")
			}
			if got.LastLoginAt.Before(before) {
				t.Errorf("LastLoginAt = %v, want the time of this sign-in (at or after %v)", got.LastLoginAt, before)
			}
		})
	}
}
