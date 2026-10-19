package auth

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The session cookie had no Secure flag on deployments served over HTTPS, so
// a browser following an http:// link sent it in the clear before the redirect
// to HTTPS. Over HTTPS the cookies are now Secure whatever auth.session.secure
// says; over plain HTTP the setting decides, so local development still works.

func requestVia(proto string, viaTLS bool) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "http://kmt.example.com/auth/login/simple", nil)
	if proto != "" {
		r.Header.Set("X-Forwarded-Proto", proto)
	}
	if viaTLS {
		r.TLS = &tls.ConnectionState{}
	}
	return r
}

func TestRequestIsHTTPS(t *testing.T) {
	for _, tc := range []struct {
		proto  string
		viaTLS bool
		want   bool
	}{
		{"", false, false},
		{"http", false, false},
		{"https", false, true},
		{"HTTPS", false, true},
		{"https, http", false, true},
		{"http, https", false, false},
		{"", true, true},
	} {
		if got := RequestIsHTTPS(requestVia(tc.proto, tc.viaTLS)); got != tc.want {
			t.Errorf("X-Forwarded-Proto %q, TLS %v: RequestIsHTTPS = %v, want %v", tc.proto, tc.viaTLS, got, tc.want)
		}
	}
}

func sessionCookieFrom(t *testing.T, configured bool, r *http.Request) *http.Cookie {
	t.Helper()
	m := NewCookieSessionManager(loginCookieTestSecret, "kmt_session", 3600, configured, "lax")
	s, err := m.GetStore().New(r, m.GetSessionName())
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	s.Values["authenticated"] = true
	rec := httptest.NewRecorder()
	if err := m.Save(r, rec, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "kmt_session" {
			return c
		}
	}
	t.Fatal("no session cookie written")
	return nil
}

func TestSessionCookie_SecureOverHTTPSWhateverTheSetting(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured bool
		r          *http.Request
		want       bool
	}{
		{"https via proxy, secure: false", false, requestVia("https", false), true},
		{"https direct, secure: false", false, requestVia("", true), true},
		{"plain http, secure: false", false, requestVia("http", false), false},
		{"plain http, secure: true", true, requestVia("", false), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := sessionCookieFrom(t, tc.configured, tc.r)
			if c.Secure != tc.want {
				t.Errorf("Secure = %v, want %v", c.Secure, tc.want)
			}
			if !c.HttpOnly {
				t.Error("the session cookie lost HttpOnly")
			}
		})
	}
}

// Marking one response's cookie Secure must not leak into the store's options
// and so into a later plain HTTP response.
func TestSessionCookie_HTTPSDoesNotChangeTheStore(t *testing.T) {
	m := NewCookieSessionManager(loginCookieTestSecret, "kmt_session", 3600, false, "lax")
	for _, r := range []*http.Request{requestVia("https", false), requestVia("", false)} {
		s, _ := m.GetStore().New(r, m.GetSessionName())
		if err := m.Save(r, httptest.NewRecorder(), s); err != nil {
			t.Fatalf("save: %v", err)
		}
	}
	if m.GetStore().Options.Secure {
		t.Error("an HTTPS response changed the store's Secure option")
	}
	if c := sessionCookieFrom(t, false, requestVia("", false)); c.Secure {
		t.Error("a plain HTTP response after an HTTPS one got a Secure cookie")
	}
}

func TestLoginCookie_SecureOverHTTPSWhateverTheSetting(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured bool
		r          *http.Request
		want       bool
	}{
		{"https via proxy, secure: false", false, requestVia("https", false), true},
		{"plain http, secure: false", false, requestVia("", false), false},
		{"plain http, secure: true", true, requestVia("", false), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewLoginCookie(loginCookieTestSecret, "kmt_test", 10*time.Minute, tc.configured)
			if err != nil {
				t.Fatalf("NewLoginCookie: %v", err)
			}
			rec := httptest.NewRecorder()
			if err := c.Save(rec, tc.r, LoginAttempt{State: "s", Nonce: "n", CodeVerifier: "v"}); err != nil {
				t.Fatalf("Save: %v", err)
			}
			var got *http.Cookie
			for _, ck := range rec.Result().Cookies() {
				if ck.Name == c.Name() {
					got = ck
				}
			}
			if got == nil || got.Secure != tc.want {
				t.Fatalf("login cookie %+v, want Secure %v", got, tc.want)
			}

			// Taking it clears it, with the same flag.
			cb := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
			cb.Header = tc.r.Header.Clone()
			cb.TLS = tc.r.TLS
			cb.AddCookie(got)
			rec = httptest.NewRecorder()
			if _, ok := c.Take(rec, cb, "s"); !ok {
				t.Fatal("Take did not find the login")
			}
			for _, ck := range rec.Result().Cookies() {
				if ck.Name == c.Name() && ck.Secure != tc.want {
					t.Errorf("clearing cookie Secure = %v, want %v", ck.Secure, tc.want)
				}
			}
		})
	}
}
