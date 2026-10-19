package chi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// An SSO callback used to be accepted from any browser as long as its state was
// in the server-wide store, so a callback link captured from one person's login
// signed in whoever opened it, as that person. The login is now kept in the
// browser that started it, and the callback redeems the code with that login's
// PKCE verifier and checks its nonce. Keeping it in the browser rather than in
// one process's memory also lets any API instance complete the login.

const testSessionSecret = "0123456789abcdef0123456789abcdef"

// testSessionStore is a cookie store configured SameSite=Strict, the setting the
// login cookie must not inherit.
func testSessionStore() SessionStore {
	return NewSessionStoreAdapter(auth.NewCookieSessionManager(
		testSessionSecret, "kmt_test", 3600, false, "strict"))
}

func testLoginCookieWithSecret(t *testing.T, secret string) *auth.LoginCookie {
	t.Helper()
	c, err := auth.NewLoginCookie(secret, "kmt_test", 10*time.Minute, false)
	if err != nil {
		t.Fatalf("login cookie: %v", err)
	}
	return c
}

func testLoginCookie(t *testing.T) *auth.LoginCookie {
	return testLoginCookieWithSecret(t, testSessionSecret)
}

// addLoginCookie gives req the login cookie a browser holds after starting the
// given login.
func addLoginCookie(t *testing.T, h *AuthHandlers, req *http.Request, attempt auth.LoginAttempt) {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := h.loginCookie.Save(rec, httptest.NewRequest(http.MethodGet, "/auth/login", nil), attempt); err != nil {
		t.Fatalf("save login cookie: %v", err)
	}
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
}

// recordingProvider records what the callback hands the provider and, like a
// real provider (RFC 6749 section 4.1.2), accepts each code only once.
type recordingProvider struct {
	callbackStatusProviderStub
	exchanges     int
	usedCodes     map[string]bool
	verifierSent  string
	nonceRequired string
}

func (p *recordingProvider) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	p.exchanges++
	if p.usedCodes == nil {
		p.usedCodes = map[string]bool{}
	}
	if p.usedCodes[code] {
		return nil, errors.New("invalid_grant: code already used")
	}
	p.usedCodes[code] = true
	// An AuthCodeOption only reveals its value by setting it on a request, so
	// render the options the way an authorization URL would.
	rendered := (&oauth2.Config{Endpoint: oauth2.Endpoint{AuthURL: "https://idp.test/auth"}}).AuthCodeURL("s", opts...)
	if u, err := url.Parse(rendered); err == nil {
		p.verifierSent = u.Query().Get("code_verifier")
	}
	return p.callbackStatusProviderStub.Exchange(ctx, code)
}

func (p *recordingProvider) VerifyLoginIDToken(ctx context.Context, token, nonce string) (*auth.UserInfo, error) {
	p.nonceRequired = nonce
	return p.callbackStatusProviderStub.VerifyLoginIDToken(ctx, token, nonce)
}

func bindingHandlersWithCookie(provider auth.Provider, lc *auth.LoginCookie) *AuthHandlers {
	return &AuthHandlers{
		service:       &callbackStatusServiceStub{},
		provider:      provider,
		loginCookie:   lc,
		sessionStore:  testSessionStore(),
		sessionName:   "kmt_test",
		log:           logger.NewNoop(),
		oauth2Enabled: true,
	}
}

func bindingHandlers(t *testing.T, provider auth.Provider) *AuthHandlers {
	return bindingHandlersWithCookie(provider, testLoginCookie(t))
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// sessionCookieSet reports whether the response signed someone in.
func sessionCookieSet(rec *httptest.ResponseRecorder) bool {
	c := cookieNamed(rec, "kmt_test")
	return c != nil && c.MaxAge >= 0
}

func TestOAuth2Login_BindsTheLoginToTheBrowser(t *testing.T) {
	h := bindingHandlers(t, &recordingProvider{})
	h.service = auth.NewService(&recordingProvider{}, nil, nil, logger.NewNoop(), &auth.Config{})

	rec := httptest.NewRecorder()
	h.handleOAuth2Login(rec, httptest.NewRequest(http.MethodGet, "/auth/login", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want a redirect to the provider", rec.Code)
	}
	c := cookieNamed(rec, "kmt_test_login")
	if c == nil {
		t.Fatal("no login cookie set")
	}
	if !c.HttpOnly {
		t.Error("login cookie must be HttpOnly")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("login cookie SameSite = %v, want Lax: the provider's redirect back is cross-site", c.SameSite)
	}
	if c.MaxAge != 600 {
		t.Errorf("login cookie MaxAge = %d, want the login lifetime, 600", c.MaxAge)
	}
}

func TestHandleCallback_RefusesACallbackThisBrowserDidNotStart(t *testing.T) {
	const state = "state-of-someone-elses-login"
	for name, cookieState := range map[string]string{
		"no login cookie":          "",
		"cookie for another login": "state-of-this-browsers-login",
	} {
		t.Run(name, func(t *testing.T) {
			provider := &recordingProvider{}
			h := bindingHandlers(t, provider)

			req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+state, nil)
			if cookieState != "" {
				addLoginCookie(t, h, req, auth.LoginAttempt{State: cookieState, Nonce: "n", CodeVerifier: "v"})
			}
			rec := httptest.NewRecorder()
			h.handleCallback(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if provider.exchanges != 0 {
				t.Errorf("code exchanged %d times, want 0: the code must not be redeemed", provider.exchanges)
			}
		})
	}
}

func TestHandleCallback_RedeemsWithTheVerifierAndChecksTheNonce(t *testing.T) {
	const state = "state-of-this-browsers-login"
	provider := &recordingProvider{}
	h := bindingHandlers(t, provider)

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+state, nil)
	addLoginCookie(t, h, req, auth.LoginAttempt{State: state, Nonce: "nonce-1", CodeVerifier: "verifier-1"})
	cookies := req.Cookies()
	rec := httptest.NewRecorder()
	h.handleCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want the signed-in redirect", rec.Code)
	}
	if provider.verifierSent != "verifier-1" {
		t.Errorf("code_verifier sent = %q, want the login's verifier", provider.verifierSent)
	}
	if provider.nonceRequired != "nonce-1" {
		t.Errorf("nonce required = %q, want the login's nonce", provider.nonceRequired)
	}
	if c := cookieNamed(rec, "kmt_test_login"); c == nil || c.MaxAge >= 0 {
		t.Errorf("login cookie = %+v, want it cleared", c)
	}

	// The same callback again, replayed with a copy of the cookie it came
	// with. The cookie cannot be revoked server-side, so what refuses it is
	// the provider, which accepts each code once: nobody is signed in.
	replay := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+state, nil)
	for _, c := range cookies {
		replay.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	h.handleCallback(rec, replay)
	if rec.Code == http.StatusFound || sessionCookieSet(rec) {
		t.Errorf("replayed callback signed someone in: status %d", rec.Code)
	}
}

// A callback for another login, such as a stale tab or a link someone sent,
// must not cancel the login this browser has in progress.
func TestHandleCallback_AStrangeCallbackLeavesThisBrowsersLoginIntact(t *testing.T) {
	const mine, other = "state-of-this-browsers-login", "state-of-another-login"
	provider := &recordingProvider{}
	h := bindingHandlers(t, provider)

	// The browser holds the cookie for its own login and opens another one's callback.
	strange := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+other, nil)
	addLoginCookie(t, h, strange, auth.LoginAttempt{State: mine, Nonce: "n-" + mine, CodeVerifier: "v-" + mine})
	cookies := strange.Cookies()
	rec := httptest.NewRecorder()
	h.handleCallback(rec, strange)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("strange callback status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if c := cookieNamed(rec, "kmt_test_login"); c != nil {
		t.Errorf("strange callback touched the login cookie: %+v", c)
	}

	// Its own callback, with the cookie it still holds, signs in.
	own := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+mine, nil)
	for _, c := range cookies {
		own.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	h.handleCallback(rec, own)
	if rec.Code != http.StatusFound {
		t.Errorf("own callback status = %d, want the signed-in redirect", rec.Code)
	}
	if provider.verifierSent != "v-"+mine {
		t.Errorf("code_verifier sent = %q, want this browser's login's", provider.verifierSent)
	}
}

// The point of keeping the login in the browser: an instance that did not
// start the login, or one restarted since, completes it, as long as it shares
// the session secret. One with another secret does not.
func TestHandleCallback_AnyInstanceWithTheSecretCompletesTheLogin(t *testing.T) {
	const state = "state-started-on-instance-a"
	attempt := auth.LoginAttempt{State: state, Nonce: "nonce-a", CodeVerifier: "verifier-a"}

	startedOn := bindingHandlers(t, &recordingProvider{})
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state="+state, nil)
	addLoginCookie(t, startedOn, req, attempt)
	cookies := req.Cookies()

	provider := &recordingProvider{}
	other := bindingHandlers(t, provider)
	rec := httptest.NewRecorder()
	other.handleCallback(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("callback on another instance: status = %d, want the signed-in redirect", rec.Code)
	}
	if provider.verifierSent != "verifier-a" || provider.nonceRequired != "nonce-a" {
		t.Errorf("verifier %q nonce %q, want the login's", provider.verifierSent, provider.nonceRequired)
	}

	foreign := bindingHandlersWithCookie(&recordingProvider{}, testLoginCookieWithSecret(t, "another-secret-another-secret-xx"))
	again := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c2&state="+state, nil)
	for _, c := range cookies {
		again.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	foreign.handleCallback(rec, again)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("callback on an instance with another secret: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleCallback_AShortStateIsRefusedNotAPanic(t *testing.T) {
	h := bindingHandlers(t, &recordingProvider{})
	rec := httptest.NewRecorder()
	h.handleCallback(rec, httptest.NewRequest(http.MethodGet, "/auth/callback?code=c&state=x", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
