package chi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/DefensePoint/keycloak-monitoring/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// A sign-in refused because the account is deactivated or blocked is a
// deliberate authorization outcome, not a server fault. Reporting it as 500
// hides a security decision behind a fake error: operators chasing "Internal
// server error" in the logs have no reason to look at the user's status, and
// the blocked person is told the product is broken rather than that they are
// not allowed in.

type callbackStatusServiceStub struct{ findErr error }

func (s *callbackStatusServiceStub) Login(_ context.Context) (string, string, error) {
	return "", "", nil
}
func (s *callbackStatusServiceStub) Callback(_ context.Context, _, _, _ string) (*auth.Session, *domain.User, error) {
	return nil, nil, nil
}
func (s *callbackStatusServiceStub) Logout(_ context.Context, _ string) error { return nil }
func (s *callbackStatusServiceStub) ValidateSession(_ context.Context, _ string) (*auth.UserInfo, error) {
	return nil, nil
}
func (s *callbackStatusServiceStub) RefreshSession(_ context.Context, _ string) (*auth.Session, error) {
	return nil, nil
}
func (s *callbackStatusServiceStub) FindOrCreateUser(_ context.Context, _ *auth.UserInfo) (*domain.User, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	return &domain.User{ID: 1, Subject: "idp-subject", IsActive: true}, nil
}

type callbackStatusProviderStub struct{}

func (p *callbackStatusProviderStub) GetAuthCodeURL(_ string) string { return "https://idp.invalid" }
func (p *callbackStatusProviderStub) Exchange(_ context.Context, _ string) (*oauth2.Token, error) {
	return (&oauth2.Token{AccessToken: "access", Expiry: time.Now().Add(time.Hour)}).
		WithExtra(map[string]interface{}{"id_token": "id-token"}), nil
}
func (p *callbackStatusProviderStub) VerifyIDToken(_ context.Context, _ string) (*auth.UserInfo, error) {
	return &auth.UserInfo{Subject: "idp-subject", Email: "person@example.com"}, nil
}
func (p *callbackStatusProviderStub) RefreshToken(_ context.Context, _ string) (*oauth2.Token, error) {
	return nil, nil
}

// callbackStatusFor drives handleCallback all the way to FindOrCreateUser and
// returns the response it produced.
func callbackStatusFor(t *testing.T, findErr error) *httptest.ResponseRecorder {
	t.Helper()

	states := auth.NewStateStore(time.Minute)
	const state = "test-state-value-long-enough"
	if err := states.Save(state); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	h := &AuthHandlers{
		service:    &callbackStatusServiceStub{findErr: findErr},
		provider:   &callbackStatusProviderStub{},
		stateStore: states,
		log:        logger.NewNoop(),
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=auth-code&state="+state, nil)
	rec := httptest.NewRecorder()
	h.handleCallback(rec, req)
	return rec
}

func TestHandleCallback_RefusedAccountIsUnauthorizedNotServerError(t *testing.T) {
	rec := callbackStatusFor(t, &auth.AuthError{
		Code:    auth.ErrCodeUnauthorized,
		Message: "account is inactive or blocked",
	})

	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("a deactivated or blocked account must not be reported as a server fault, got %d", rec.Code)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected %d for a refused account, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestHandleCallback_GenuineFailureIsStillAServerError(t *testing.T) {
	rec := callbackStatusFor(t, errors.New("database connection refused"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("a real failure must still be %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}
