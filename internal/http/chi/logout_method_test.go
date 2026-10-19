package chi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// Logout used to be a GET, so any page could end the user's session by
// linking to it. It is a POST now, refused when the browser says the request
// came from another site.

func logoutRouter(t *testing.T) (http.Handler, *auth.CookieSessionManager) {
	t.Helper()
	mgr := auth.NewCookieSessionManager("0123456789abcdef0123456789abcdef", "kmt", 86400, false, "lax")
	h := &AuthHandlers{
		sessionStore:   mgr.GetStore(),
		sessionName:    mgr.GetSessionName(),
		log:            logger.NewNoop(),
		authMiddleware: func(next http.Handler) http.Handler { return next },
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r, mgr
}

func sessionCookie(t *testing.T, mgr *auth.CookieSessionManager) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s, _ := mgr.GetStore().New(req, mgr.GetSessionName())
	s.Values["authenticated"] = true
	s.Values["subject"] = "alice"
	s.Values["session_id"] = "session-0001"
	rec := httptest.NewRecorder()
	if err := s.Save(req, rec); err != nil {
		t.Fatal(err)
	}
	return rec.Result().Cookies()[0]
}

// clearsSession reports whether the response deletes the session cookie.
func clearsSession(rec *httptest.ResponseRecorder, name string) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

func TestLogout_PostFromThisSiteEndsTheSession(t *testing.T) {
	r, mgr := logoutRouter(t)
	for _, site := range []string{"", "same-origin", "none"} {
		req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		req.AddCookie(sessionCookie(t, mgr))
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("Sec-Fetch-Site %q: status %d, want 200", site, rec.Code)
		}
		if !clearsSession(rec, "kmt") {
			t.Errorf("Sec-Fetch-Site %q: the session cookie was not cleared", site)
		}
	}
}

func TestLogout_PostFromAnotherSiteIsRefused(t *testing.T) {
	r, mgr := logoutRouter(t)
	for _, site := range []string{"cross-site", "same-site"} {
		req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		req.AddCookie(sessionCookie(t, mgr))
		req.Header.Set("Sec-Fetch-Site", site)
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("Sec-Fetch-Site %q: status %d, want 403", site, rec.Code)
		}
		if clearsSession(rec, "kmt") {
			t.Errorf("Sec-Fetch-Site %q: the session cookie was cleared", site)
		}
	}
}

func TestLogout_GetIsRefused(t *testing.T) {
	r, mgr := logoutRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	req.AddCookie(sessionCookie(t, mgr))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want POST", got)
	}
	if clearsSession(rec, "kmt") {
		t.Fatal("a GET cleared the session cookie")
	}
}

func TestLogout_PostWithoutASessionSucceeds(t *testing.T) {
	r, _ := logoutRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
}

// Chrome sends Sec-Fetch-Site only to HTTPS or localhost, so over plain http
// the Origin header decides: it must name the host the browser addressed.
func TestLogout_OriginDecidesWithoutSecFetchSite(t *testing.T) {
	r, mgr := logoutRouter(t)
	for _, tc := range []struct {
		name      string
		host      string
		forwarded string
		origin    string
		site      string
		want      int
	}{
		{"same host, direct", "app.example:7888", "", "http://app.example:7888", "", http.StatusOK},
		{"same host, through the web proxy", "api-server:7888", "app.example:7880", "http://app.example:7880", "", http.StatusOK},
		{"host in other case", "api-server:7888", "App.Example:7880", "http://app.example:7880", "", http.StatusOK},
		{"sibling subdomain", "api-server:7888", "app.example:7880", "http://evil.example:7880", "", http.StatusForbidden},
		{"same host, other port", "api-server:7888", "app.example:7880", "http://app.example:9999", "", http.StatusForbidden},
		{"other site", "api-server:7888", "app.example:7880", "https://evil.test", "", http.StatusForbidden},
		{"opaque origin", "app.example:7888", "", "null", "", http.StatusForbidden},
		{"Sec-Fetch-Site wins over a matching Origin", "app.example:7888", "", "http://app.example:7888", "same-site", http.StatusForbidden},
		{"no browser headers at all", "app.example:7888", "", "", "", http.StatusOK},
		{"proxy forwarded no port (nginx $host), same host", "app.example", "", "http://app.example:8080", "", http.StatusOK},
		{"proxy forwarded no port, sibling subdomain", "app.example", "", "http://evil.example:8080", "", http.StatusForbidden},
		{"proxy forwarded no port, standard port", "app.example", "", "https://app.example", "", http.StatusOK},
		{"IPv6 host with port", "[::1]:7888", "", "http://[::1]:7888", "", http.StatusOK},
		{"IPv6 host, other port", "[::1]:7888", "", "http://[::1]:9999", "", http.StatusForbidden},
		{"IPv6 host without port", "[::1]", "", "http://[::1]:8080", "", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
			req.Host = tc.host
			req.AddCookie(sessionCookie(t, mgr))
			if tc.forwarded != "" {
				req.Header.Set("X-Forwarded-Host", tc.forwarded)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.site != "" {
				req.Header.Set("Sec-Fetch-Site", tc.site)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d", rec.Code, tc.want)
			}
			if cleared := clearsSession(rec, "kmt"); cleared != (tc.want == http.StatusOK) {
				t.Fatalf("session cleared = %v with status %d", cleared, rec.Code)
			}
		})
	}
}
