package fx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
)

const (
	kmtHost       = "kmt.example.com"
	kmtOrigin     = "https://kmt.example.com"
	siblingOrigin = "https://other.example.com"
	trustedOrigin = "https://trusted.example.org"
)

type probeRoutes struct {
	auth  func(http.Handler) http.Handler
	calls int
}

func (p *probeRoutes) RegisterRoutes(r chi.Router) {
	r.With(p.auth).HandleFunc("/api/probe", func(w http.ResponseWriter, _ *http.Request) {
		p.calls++
		w.WriteHeader(http.StatusOK)
	})
}

func newCrossOriginServer(t *testing.T, allowedOrigins []string) (http.Handler, *probeRoutes, *http.Cookie) {
	t.Helper()

	log := logger.NewNoop()
	sessions := auth.NewCookieSessionManager(strings.Repeat("s", 32), "kmt_session", 3600, true, "lax")
	authMW := chihttp.NewAuthMiddleware(sessions, nil, nil, nil, log)
	probe := &probeRoutes{auth: authMW.Authenticate}

	cfg := &config.AppConfig{}
	cfg.HTTP.CORS.AllowedOrigins = allowedOrigins
	srv, err := provideHTTPServer(HTTPServerParams{
		Config:         cfg,
		Router:         chihttp.NewRouter(log, authMW.Authenticate, nil, probe),
		AuthMiddleware: authMW,
		Logger:         log,
		Metrics:        metrics.New(),
	})
	if err != nil {
		t.Fatalf("provideHTTPServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	session, err := sessions.GetStore().Get(req, sessions.GetSessionName())
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	session.Values["authenticated"] = true
	session.Values["subject"] = "probe-user"
	rec := httptest.NewRecorder()
	if err := session.Save(req, rec); err != nil {
		t.Fatalf("save session: %v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("session cookies = %d, want 1", len(cookies))
	}
	return srv.Handler, probe, cookies[0]
}

type originCase struct {
	name         string
	origin       string
	secFetchSite string
	allowUnsafe  bool
}

func TestCrossOriginProtection(t *testing.T) {
	cases := []originCase{
		{name: "same origin, browser", origin: kmtOrigin, secFetchSite: "same-origin", allowUnsafe: true},
		{name: "same origin, no Sec-Fetch-Site", origin: kmtOrigin, allowUnsafe: true},
		{name: "no Origin, API client", allowUnsafe: true},
		{name: "sibling subdomain", origin: siblingOrigin, secFetchSite: "same-site"},
		{name: "sibling subdomain, no Sec-Fetch-Site", origin: siblingOrigin},
		{name: "foreign site", origin: "https://evil.example.net", secFetchSite: "cross-site"},
		{name: "null origin", origin: "null", secFetchSite: "cross-site"},
	}
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}

	for _, tc := range cases {
		for _, method := range methods {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				handler, probe, cookie := newCrossOriginServer(t, nil)
				rec := serveProbe(handler, method, tc.origin, tc.secFetchSite, cookie)

				wantCalled := method == http.MethodGet || tc.allowUnsafe
				if wantCalled && rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200", rec.Code)
				}
				if !wantCalled && rec.Code != http.StatusForbidden {
					t.Fatalf("status = %d, want 403", rec.Code)
				}
				if got := probe.calls == 1; got != wantCalled {
					t.Fatalf("handler called = %v, want %v", got, wantCalled)
				}
				if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
					t.Fatalf("Access-Control-Allow-Origin = %q, want none", got)
				}
				if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
					t.Fatalf("Access-Control-Allow-Credentials = %q, want none", got)
				}
			})
		}
	}
}

func TestCrossOriginAllowedOrigin(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			handler, probe, cookie := newCrossOriginServer(t, []string{trustedOrigin})
			rec := serveProbe(handler, method, trustedOrigin, "cross-site", cookie)

			if rec.Code != http.StatusOK || probe.calls != 1 {
				t.Fatalf("status = %d, calls = %d, want 200 and 1", rec.Code, probe.calls)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != trustedOrigin {
				t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, trustedOrigin)
			}
			if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
				t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
			}
		})
	}

	t.Run("environment form", func(t *testing.T) {
		handler, probe, cookie := newCrossOriginServer(t, []string{"https://a.example.org " + trustedOrigin + ",https://b.example.org"})
		rec := serveProbe(handler, http.MethodDelete, trustedOrigin, "cross-site", cookie)
		if rec.Code != http.StatusOK || probe.calls != 1 {
			t.Fatalf("status = %d, calls = %d, want 200 and 1", rec.Code, probe.calls)
		}
	})

	t.Run("other origins stay refused", func(t *testing.T) {
		handler, probe, cookie := newCrossOriginServer(t, []string{trustedOrigin})
		rec := serveProbe(handler, http.MethodDelete, siblingOrigin, "same-site", cookie)
		if rec.Code != http.StatusForbidden || probe.calls != 0 {
			t.Fatalf("status = %d, calls = %d, want 403 and 0", rec.Code, probe.calls)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("Access-Control-Allow-Origin = %q, want none", got)
		}
	})
}

func TestCrossOriginPreflight(t *testing.T) {
	handler, probe, _ := newCrossOriginServer(t, []string{trustedOrigin})

	for _, tc := range []struct {
		origin    string
		wantAllow string
	}{
		{origin: trustedOrigin, wantAllow: trustedOrigin},
		{origin: siblingOrigin},
		{origin: "https://evil.example.net"},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/api/probe", nil)
			req.Host = kmtHost
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Access-Control-Request-Method", http.MethodDelete)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.wantAllow {
				t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, tc.wantAllow)
			}
			if tc.wantAllow != "" && rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", rec.Code)
			}
			if tc.wantAllow == "" && rec.Header().Get("Access-Control-Allow-Methods") != "" {
				t.Fatal("refused preflight carries Access-Control-Allow-Methods")
			}
			if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
				t.Fatalf("Vary = %q, want Origin", rec.Header().Get("Vary"))
			}
		})
	}
	if probe.calls != 0 {
		t.Fatalf("handler called %d times by preflight, want 0", probe.calls)
	}
}

func TestCrossOriginRejectsBadAllowedOrigins(t *testing.T) {
	for _, origin := range []string{"*", "https://kmt.example.com/", "https://kmt.example.com/app", "kmt.example.com", "https://*.example.com",
		"https://user@kmt.example.com", "ftp://kmt.example.com", "chrome-extension://abcdef", "https://bücher.example"} {
		t.Run(origin, func(t *testing.T) {
			cfg := &config.AppConfig{}
			cfg.HTTP.CORS.AllowedOrigins = []string{origin}
			log := logger.NewNoop()
			_, err := provideHTTPServer(HTTPServerParams{
				Config:  cfg,
				Router:  chihttp.NewRouter(log, nil, nil),
				Logger:  log,
				Metrics: metrics.New(),
			})
			if err == nil {
				t.Fatalf("allowed origin %q accepted, want a startup error", origin)
			}
		})
	}
}

func serveProbe(handler http.Handler, method, origin, secFetchSite string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/probe", nil)
	req.Host = kmtHost
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if secFetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", secFetchSite)
	}
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// Browsers send Origin lowercase and without a default port, and both
// allowlists compare exact strings, so a configured origin is normalized.
func TestCrossOriginAllowedOriginIsNormalized(t *testing.T) {
	for _, tc := range []struct {
		configured, browserSends string
	}{
		{configured: "https://Trusted.Example.ORG", browserSends: "https://trusted.example.org"},
		{configured: "HTTPS://trusted.example.org:443", browserSends: "https://trusted.example.org"},
		{configured: "http://LOCALHOST:18999", browserSends: "http://localhost:18999"},
		{configured: "http://app.example.org:80", browserSends: "http://app.example.org"},
		{configured: "https://[::1]:443", browserSends: "https://[::1]"},
		{configured: "https://trusted.example.org:8443", browserSends: "https://trusted.example.org:8443"},
	} {
		t.Run(tc.configured, func(t *testing.T) {
			handler, probe, cookie := newCrossOriginServer(t, []string{tc.configured})
			rec := serveProbe(handler, http.MethodDelete, tc.browserSends, "cross-site", cookie)
			if rec.Code != http.StatusOK || probe.calls != 1 {
				t.Fatalf("status = %d, calls = %d, want 200 and 1", rec.Code, probe.calls)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.browserSends {
				t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, tc.browserSends)
			}
		})
	}

	t.Run("a non-default port is still a different origin", func(t *testing.T) {
		handler, probe, cookie := newCrossOriginServer(t, []string{"https://trusted.example.org:8443"})
		rec := serveProbe(handler, http.MethodDelete, "https://trusted.example.org", "cross-site", cookie)
		if rec.Code != http.StatusForbidden || probe.calls != 0 {
			t.Fatalf("status = %d, calls = %d, want 403 and 0", rec.Code, probe.calls)
		}
	})
}

// Over plain HTTP browsers send no Sec-Fetch-Site, so Origin is compared with
// Host, port included. nginx's $http_host keeps the port; $host drops it, and
// the UI's own write is then refused, which docs/deployment.md warns about.
func TestCrossOriginNonStandardPortNeedsThePortInHost(t *testing.T) {
	for _, tc := range []struct {
		name, host string
		want       int
	}{
		{name: "Host with the port ($http_host)", host: "kmt.example.com:8080", want: http.StatusOK},
		{name: "Host without the port ($host)", host: "kmt.example.com", want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, _, cookie := newCrossOriginServer(t, nil)
			req := httptest.NewRequest(http.MethodPost, "/api/probe", nil)
			req.Host = tc.host
			req.Header.Set("Origin", "http://kmt.example.com:8080")
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
