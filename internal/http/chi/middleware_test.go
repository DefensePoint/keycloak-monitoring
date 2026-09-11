package chi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

func TestAllowedRealms(t *testing.T) {
	tests := []struct {
		name       string
		rbac       *mockRBACChecker
		wantRealms []string
		wantAll    bool
		wantErr    bool
	}{
		{
			name:    "admin is unrestricted",
			rbac:    &mockRBACChecker{isAdmin: true, policies: policyFor("tenant-a", "realmB")},
			wantAll: true,
		},
		{
			name:    "caller with no policy row is unrestricted",
			rbac:    &mockRBACChecker{},
			wantAll: true,
		},
		{
			name:       "policy row restricts to exactly its realms",
			rbac:       &mockRBACChecker{policies: policyFor("tenant-a", "realmB")},
			wantRealms: []string{"realmB"},
		},
		{
			// A stored empty list denies every realm, which is what
			// rbac.Service.HasAccessToRealm enforces for the same row.
			name: "policy row with an empty realm list allows no realm",
			rbac: &mockRBACChecker{policies: policyFor("tenant-a")},
		},
		{
			name: "every row for the tenant contributes, whatever the order",
			rbac: &mockRBACChecker{policies: []*domain.TenantPolicy{
				{TenantID: "tenant-a", AllowedRealms: []string{"realmA"}},
				{TenantID: "tenant-a", AllowedRealms: []string{"realmB"}},
			}},
			wantRealms: []string{"realmA", "realmB"},
		},
		{
			name: "the same rows in the opposite order give the same union",
			rbac: &mockRBACChecker{policies: []*domain.TenantPolicy{
				{TenantID: "tenant-a", AllowedRealms: []string{"realmB"}},
				{TenantID: "tenant-a", AllowedRealms: []string{"realmA"}},
			}},
			wantRealms: []string{"realmB", "realmA"},
		},
		{
			name: "an empty row alongside a listing row does not shrink the union",
			rbac: &mockRBACChecker{policies: []*domain.TenantPolicy{
				{TenantID: "tenant-a"},
				{TenantID: "tenant-a", AllowedRealms: []string{"realmB"}},
			}},
			wantRealms: []string{"realmB"},
		},
		{
			name:    "row whose realms column is SQL NULL stays unrestricted",
			rbac:    &mockRBACChecker{policies: []*domain.TenantPolicy{{TenantID: "tenant-a", RealmsUnrestricted: true}}},
			wantAll: true,
		},
		{
			name:    "policy row for another tenant does not restrict this one",
			rbac:    &mockRBACChecker{policies: policyFor("tenant-other", "realmB")},
			wantAll: true,
		},
		{
			name:    "policy lookup failure denies",
			rbac:    &mockRBACChecker{policiesErr: errors.New("policy lookup failed")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realms, all, err := allowedRealms(context.Background(), tt.rbac, 1, "tenant-a")

			if tt.wantErr {
				if err == nil {
					t.Fatalf("allowedRealms() error = nil, want an error")
				}
				if all {
					t.Errorf("allowedRealms() all = true on error, want false")
				}
				return
			}
			if err != nil {
				t.Fatalf("allowedRealms() error = %v", err)
			}
			if all != tt.wantAll {
				t.Fatalf("allowedRealms() all = %v, want %v", all, tt.wantAll)
			}
			if all {
				return
			}
			if len(realms) == 0 && len(tt.wantRealms) == 0 {
				return
			}
			if !reflect.DeepEqual(realms, tt.wantRealms) {
				t.Errorf("allowedRealms() realms = %v, want %v", realms, tt.wantRealms)
			}
		})
	}
}

func TestAllowedRealms_NilService(t *testing.T) {
	realms, all, err := allowedRealms(context.Background(), nil, 1, "tenant-a")

	if err == nil {
		t.Fatalf("allowedRealms() error = nil, want an error")
	}
	if all || len(realms) != 0 {
		t.Errorf("allowedRealms() = %v, all = %v, want no realms and all = false", realms, all)
	}
}

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
