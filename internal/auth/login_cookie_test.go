package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const loginCookieTestSecret = "0123456789abcdef0123456789abcdef"

func newTestLoginCookie(t *testing.T, maxAge time.Duration) *LoginCookie {
	t.Helper()
	c, err := NewLoginCookie(loginCookieTestSecret, "kmt_test", maxAge, true)
	if err != nil {
		t.Fatalf("NewLoginCookie: %v", err)
	}
	return c
}

// savedCookie returns the cookie a browser holds after Save.
func savedCookie(t *testing.T, c *LoginCookie, attempt LoginAttempt) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := c.Save(rec, httptest.NewRequest(http.MethodGet, "/auth/login", nil), attempt); err != nil {
		t.Fatalf("Save: %v", err)
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == c.Name() {
			return ck
		}
	}
	t.Fatal("Save set no login cookie")
	return nil
}

func callbackWith(ck *http.Cookie) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	if ck != nil {
		r.AddCookie(ck)
	}
	return r
}

var testAttempt = LoginAttempt{
	State:        "state-value-0123456789",
	Nonce:        "nonce-value-0123456789",
	CodeVerifier: "verifier-value-0123456789-abcdefghijklmnop",
}

func TestLoginCookie_TakeReturnsTheSavedLoginAndClearsIt(t *testing.T) {
	c := newTestLoginCookie(t, 10*time.Minute)
	ck := savedCookie(t, c, testAttempt)

	rec := httptest.NewRecorder()
	got, ok := c.Take(rec, callbackWith(ck), testAttempt.State)
	if !ok || got != testAttempt {
		t.Fatalf("Take = %+v, %v; want the saved attempt", got, ok)
	}
	cleared := rec.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != c.Name() || cleared[0].MaxAge >= 0 {
		t.Errorf("Take did not clear the login cookie: %+v", cleared)
	}
}

func TestLoginCookie_SetsTheAttributesTheCallbackNeeds(t *testing.T) {
	ck := savedCookie(t, newTestLoginCookie(t, 10*time.Minute), testAttempt)
	if ck.Name != "kmt_test_login" || !ck.HttpOnly || !ck.Secure || ck.SameSite != http.SameSiteLaxMode || ck.MaxAge != 600 || ck.Path != "/" {
		t.Errorf("cookie = %+v", ck)
	}
}

// The verifier and nonce must not be readable from the cookie, not even by the
// browser holding it.
func TestLoginCookie_IsEncrypted(t *testing.T) {
	ck := savedCookie(t, newTestLoginCookie(t, 10*time.Minute), testAttempt)
	decoded, err := base64.URLEncoding.DecodeString(ck.Value)
	if err != nil {
		t.Fatalf("cookie value is not base64: %v", err)
	}
	for _, secret := range []string{testAttempt.CodeVerifier, testAttempt.Nonce, testAttempt.State} {
		if strings.Contains(ck.Value, secret) || strings.Contains(string(decoded), secret) {
			t.Errorf("cookie exposes %q", secret)
		}
		// gorilla base64-encodes the payload inside the signed envelope too.
		for _, part := range strings.Split(string(decoded), "|") {
			if inner, err := base64.URLEncoding.DecodeString(part); err == nil && strings.Contains(string(inner), secret) {
				t.Errorf("cookie exposes %q once its payload is decoded", secret)
			}
		}
	}
}

// The library enforces expiry from the signed timestamp with the store's max
// age, so a login cookie the browser kept past its lifetime is refused.
func TestLoginCookie_ExpiredIsRefused(t *testing.T) {
	c := newTestLoginCookie(t, time.Second)
	ck := savedCookie(t, c, testAttempt)
	time.Sleep(2100 * time.Millisecond)
	if _, ok := c.Take(httptest.NewRecorder(), callbackWith(ck), testAttempt.State); ok {
		t.Fatal("an expired login cookie was accepted")
	}
}

func TestLoginCookie_TamperedIsRefused(t *testing.T) {
	c := newTestLoginCookie(t, 10*time.Minute)
	ck := savedCookie(t, c, testAttempt)
	b := []byte(ck.Value)
	i := len(b) / 2
	if b[i] == 'A' {
		b[i] = 'B'
	} else {
		b[i] = 'A'
	}
	ck.Value = string(b)
	if _, ok := c.Take(httptest.NewRecorder(), callbackWith(ck), testAttempt.State); ok {
		t.Fatal("a tampered login cookie was accepted")
	}
}

func TestLoginCookie_AnotherSecretCannotReadIt(t *testing.T) {
	ck := savedCookie(t, newTestLoginCookie(t, 10*time.Minute), testAttempt)
	other, err := NewLoginCookie("another-secret-another-secret-xx", "kmt_test", 10*time.Minute, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := other.Take(httptest.NewRecorder(), callbackWith(ck), testAttempt.State); ok {
		t.Fatal("a login cookie was accepted under another secret")
	}
}

// The session cookie is signed with the secret itself, the login cookie with
// keys derived from it, so neither can stand in for the other.
func TestLoginCookie_ASessionCookieCannotStandInForIt(t *testing.T) {
	c := newTestLoginCookie(t, 10*time.Minute)
	sessions := NewCookieSessionManager(loginCookieTestSecret, "kmt_test_login", 600, true, "lax")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s, _ := sessions.GetStore().Get(req, "kmt_test_login")
	s.Values[loginStateKey] = testAttempt.State
	if err := sessions.GetStore().Save(req, rec, s); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Take(httptest.NewRecorder(), callbackWith(rec.Result().Cookies()[0]), testAttempt.State); ok {
		t.Fatal("a cookie signed with the session key was accepted as a login cookie")
	}
}

func TestLoginCookie_AnotherStateLeavesTheCookieAlone(t *testing.T) {
	c := newTestLoginCookie(t, 10*time.Minute)
	ck := savedCookie(t, c, testAttempt)
	rec := httptest.NewRecorder()
	if _, ok := c.Take(rec, callbackWith(ck), "state-of-another-login"); ok {
		t.Fatal("a callback for another state was accepted")
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("a callback for another state changed the cookie: %+v", rec.Result().Cookies())
	}
}

func TestLoginCookie_NoCookieIsRefused(t *testing.T) {
	c := newTestLoginCookie(t, 10*time.Minute)
	if _, ok := c.Take(httptest.NewRecorder(), callbackWith(nil), testAttempt.State); ok {
		t.Fatal("a callback without a login cookie was accepted")
	}
}

func TestLoginCookie_SaveRefusesAnAttemptWithoutState(t *testing.T) {
	c := newTestLoginCookie(t, 10*time.Minute)
	if err := c.Save(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), LoginAttempt{Nonce: "n"}); err == nil {
		t.Fatal("Save accepted an attempt without state")
	}
}
