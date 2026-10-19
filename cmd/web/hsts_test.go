package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

// The page the browser loads first comes from the web server, which sent no
// HSTS, so a typed or linked http:// address was never upgraded before the
// session cookie went out. The API's own responses carry HSTS too, and the
// proxy must not send it twice.

var hstsOn = config.SecurityHeadersConfig{Enabled: true, HSTS: "max-age=31536000"}

func TestHSTS_OnTheApplicationPage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := withHSTS(hstsOn, createSPAHandler(dir))

	for _, path := range []string{"/", "/demo1/alerts"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Values("Strict-Transport-Security"); len(got) != 1 || got[0] != "max-age=31536000" {
			t.Errorf("%s: Strict-Transport-Security = %q, want one max-age=31536000", path, got)
		}
	}
}

func TestHSTS_OnceOnAProxiedResponse(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		w.Header().Add("Set-Cookie", "a=1")
		w.Header().Add("Set-Cookie", "b=2")
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()
	h := withHSTS(hstsOn, createProxyHandler(api.URL))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rbac/me", nil))
	if got := rec.Header().Values("Strict-Transport-Security"); len(got) != 1 {
		t.Errorf("Strict-Transport-Security sent %d times: %q", len(got), got)
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 2 {
		t.Errorf("a repeated backend header lost values: Set-Cookie %q", got)
	}
}

func TestHSTS_OffWhenDisabled(t *testing.T) {
	for _, headers := range []config.SecurityHeadersConfig{
		{Enabled: true, HSTS: ""},
		{Enabled: false, HSTS: "max-age=31536000"},
	} {
		rec := httptest.NewRecorder()
		withHSTS(headers, http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("%+v: Strict-Transport-Security = %q, want none", headers, got)
		}
	}
}
