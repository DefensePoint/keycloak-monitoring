package keycloakadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// identityProviderServer serves n identity providers. With paging, it answers
// first/max as Keycloak does, max defaulting to 100; without, it returns all
// of them whatever was asked, as a server that ignores the parameters would.
func identityProviderServer(t *testing.T, n int, paging bool) (*Client, *int) {
	t.Helper()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 300})
		case strings.HasSuffix(r.URL.Path, "/admin/serverinfo"):
			_ = json.NewEncoder(w).Encode(map[string]any{"systemInfo": map[string]any{"version": "26.6.3"}})
		case strings.HasSuffix(r.URL.Path, "/admin/realms/acme/identity-provider/instances"):
			requests++
			first, max := 0, n
			if paging {
				max = 100
				if v := r.URL.Query().Get("first"); v != "" {
					first, _ = strconv.Atoi(v)
				}
				if v := r.URL.Query().Get("max"); v != "" {
					max, _ = strconv.Atoi(v)
				}
			}
			idps := []IdentityProviderRepresentation{}
			for i := first; i < n && i < first+max; i++ {
				idps = append(idps, IdentityProviderRepresentation{Alias: fmt.Sprintf("idp-%03d", i)})
			}
			_ = json.NewEncoder(w).Encode(idps)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(context.Background(), &ClientConfig{
		ServerURL:    srv.URL,
		AdminRealm:   "master",
		ClientID:     "monitoring-service",
		ClientSecret: "s",
		HTTPClient:   srv.Client(),
	}, noopLogger{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return c, &requests
}

// Keycloak returns at most 100 identity providers unless asked for more, so a
// realm with more must be read page by page.
func TestGetIdentityProviders_ReadsEveryPage(t *testing.T) {
	for _, n := range []int{0, 1, 100, 101, 250} {
		c, _ := identityProviderServer(t, n, true)
		idps, err := c.GetIdentityProviders(context.Background(), "acme")
		if err != nil {
			t.Fatalf("%d providers: %v", n, err)
		}
		if len(idps) != n {
			t.Fatalf("%d providers: got %d", n, len(idps))
		}
		seen := map[string]bool{}
		for _, idp := range idps {
			if seen[idp.Alias] {
				t.Fatalf("%d providers: %s listed twice", n, idp.Alias)
			}
			seen[idp.Alias] = true
		}
	}
}

// A server that ignores first and max returns everything on every request:
// the list must stop rather than repeat it or loop.
func TestGetIdentityProviders_ServerWithoutPaging(t *testing.T) {
	for _, n := range []int{3, 100, 250} {
		c, requests := identityProviderServer(t, n, false)
		idps, err := c.GetIdentityProviders(context.Background(), "acme")
		if err != nil {
			t.Fatalf("%d providers: %v", n, err)
		}
		if len(idps) != n {
			t.Fatalf("%d providers: got %d", n, len(idps))
		}
		if *requests > 2 {
			t.Fatalf("%d providers: %d requests", n, *requests)
		}
	}
}
