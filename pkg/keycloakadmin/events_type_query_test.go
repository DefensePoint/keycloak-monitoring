package keycloakadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Keycloak reads each type parameter as one event type: a comma-joined list
// is an unknown type and fails the whole request, so every events poll of a
// tenant with two or more types configured failed.
func TestGetEvents_SendsOneTypeParameterPerEventType(t *testing.T) {
	var mu sync.Mutex
	var query url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 300})
		case strings.HasSuffix(r.URL.Path, "/admin/realms/acme/events"):
			mu.Lock()
			query = r.URL.Query()
			mu.Unlock()
			_, _ = w.Write([]byte("[]"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	newClient := func(t *testing.T, types []string) *Client {
		t.Helper()
		c, err := NewClient(context.Background(), &ClientConfig{
			ServerURL:    srv.URL,
			AdminRealm:   "master",
			ClientID:     "monitoring-service",
			ClientSecret: "s",
			HTTPClient:   srv.Client(),
			EventTypes:   types,
		}, noopLogger{})
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}
		return c
	}
	sentTypes := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return query["type"]
	}

	for _, tc := range []struct {
		name  string
		types []string
	}{
		{"two types", []string{"LOGIN", "LOGIN_ERROR"}},
		{"four types", []string{"LOGIN", "LOGIN_ERROR", "LOGOUT", "REGISTER"}},
		{"one type", []string{"LOGIN"}},
		{"no types", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newClient(t, tc.types)

			if _, _, err := c.GetEventsSince(context.Background(), "acme", time.Now().Add(-time.Minute), 0, EventsPoll); err != nil {
				t.Fatalf("GetEventsSince() error = %v", err)
			}
			if got := sentTypes(); !slices.Equal(got, tc.types) {
				t.Errorf("events poll sent type = %q, want %q", got, tc.types)
			}

			if _, err := c.GetEvents(context.Background(), "acme", &EventQueryOptions{Types: tc.types}); err != nil {
				t.Fatalf("GetEvents() error = %v", err)
			}
			if got := sentTypes(); !slices.Equal(got, tc.types) {
				t.Errorf("GetEvents sent type = %q, want %q", got, tc.types)
			}
		})
	}
}
