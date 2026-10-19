package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

func TestProxyKeepsOriginCheckWorking(t *testing.T) {
	crossOrigin, err := chihttp.CrossOriginMiddleware(nil, logger.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	var gotHost, gotForwardedHost string
	api := httptest.NewServer(crossOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotForwardedHost = r.Header.Get("X-Forwarded-Host")
		w.WriteHeader(http.StatusOK)
	})))
	defer api.Close()
	proxy := createProxyHandler(api.URL)

	for _, tc := range []struct {
		name         string
		origin       string
		host         string
		secFetchSite string
		want         int
	}{
		{name: "same origin, browser", origin: "https://kmt.example.com", secFetchSite: "same-origin", want: http.StatusOK},
		{name: "same origin, no Sec-Fetch-Site", origin: "https://kmt.example.com", want: http.StatusOK},
		{name: "non-standard port over http", origin: "http://kmt.example.com:8080", host: "kmt.example.com:8080", want: http.StatusOK},
		{name: "sibling subdomain", origin: "https://other.example.com", secFetchSite: "same-site", want: http.StatusForbidden},
		{name: "sibling subdomain, no Sec-Fetch-Site", origin: "https://other.example.com", want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotHost = ""
			req := httptest.NewRequest(http.MethodDelete, "https://kmt.example.com/api/tenants/t1", nil)
			wantHost := "kmt.example.com"
			if tc.host != "" {
				req.Host, wantHost = tc.host, tc.host
			}
			req.Header.Set("Origin", tc.origin)
			if tc.secFetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tc.secFetchSite)
			}
			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == http.StatusOK && (gotHost != wantHost || gotForwardedHost != wantHost) {
				t.Fatalf("API saw Host %q, X-Forwarded-Host %q; want %q for both", gotHost, gotForwardedHost, wantHost)
			}
		})
	}
}
