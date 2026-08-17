package chi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersMiddleware_AppliesConfiguredHeaders(t *testing.T) {
	headers := SecurityHeaders{
		HSTS:                  "max-age=31536000; includeSubDomains",
		ContentSecurityPolicy: "default-src 'self'",
		XContentTypeOptions:   "nosniff",
		XFrameOptions:         "DENY",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
	}

	handler := SecurityHeadersMiddleware(headers)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	cases := map[string]string{
		"Strict-Transport-Security": headers.HSTS,
		"Content-Security-Policy":   headers.ContentSecurityPolicy,
		"X-Content-Type-Options":    headers.XContentTypeOptions,
		"X-Frame-Options":           headers.XFrameOptions,
		"Referrer-Policy":           headers.ReferrerPolicy,
	}
	for header, want := range cases {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestSecurityHeadersMiddleware_SkipsEmptyValues(t *testing.T) {
	// HSTS deliberately left empty — common during HTTP-only deployments.
	handler := SecurityHeadersMiddleware(SecurityHeaders{
		ContentSecurityPolicy: "default-src 'self'",
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS unexpectedly set to %q when config was empty", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'self'" {
		t.Errorf("CSP = %q, want %q", got, "default-src 'self'")
	}
}
