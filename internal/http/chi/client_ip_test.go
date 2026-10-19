package chi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
)

func mustClientIP(t *testing.T, trusted []string) func(http.Handler) http.Handler {
	t.Helper()
	h, err := ClientIPMiddleware(trusted)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestUntrustedXFFDoesNotChangeClientIP(t *testing.T) {
	var got string
	h := mustClientIP(t, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = clientIP(r)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:54321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "203.0.113.10" {
		t.Fatalf("logged client IP = %q, want TCP peer (spoofed XFF must be ignored)", got)
	}
}

func TestTrustedProxyXFFIsUsed(t *testing.T) {
	var got string
	h := mustClientIP(t, []string{"10.0.0.0/8"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = chimw.GetClientIP(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.5")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "198.51.100.7" {
		t.Fatalf("GetClientIP = %q, want 198.51.100.7", got)
	}
}

func TestClientIPMiddlewareRejectsMalformedCIDR(t *testing.T) {
	cases := []struct {
		in      string
		wantSub string
	}{
		{in: "10.0.0.1", wantSub: `trusted_proxies[0] "10.0.0.1" is not a CIDR (did you mean 10.0.0.1/32?)`},
		{in: "not-an-ip", wantSub: `trusted_proxies[0] "not-an-ip" is not a CIDR`},
		{in: "10.0.0.0/8x", wantSub: `trusted_proxies[0] "10.0.0.0/8x" is not a CIDR`},
		{in: "192.168.0.0/33", wantSub: `trusted_proxies[0] "192.168.0.0/33" is not a CIDR`},
	}
	for _, tc := range cases {
		_, err := ClientIPMiddleware([]string{tc.in})
		if err == nil {
			t.Fatalf("%q: expected error", tc.in)
		}
		if !strings.Contains(err.Error(), tc.wantSub) {
			t.Fatalf("%q: error %q, want substring %q", tc.in, err, tc.wantSub)
		}
	}
}
