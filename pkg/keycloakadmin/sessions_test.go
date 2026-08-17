package keycloakadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestFlexInt_AcceptsStringAndNumber verifies the client-session-stats counts
// parse from both quoted strings ("1", Keycloak's current form) and bare JSON
// numbers (1), and that empty/null decode to 0.
func TestFlexInt_AcceptsStringAndNumber(t *testing.T) {
	tests := []struct {
		name string
		json string
		want clientSessionStat
	}{
		{"strings", `{"clientId":"a","active":"3","offline":"1"}`, clientSessionStat{ClientID: "a", Active: 3, Offline: 1}},
		{"numbers", `{"clientId":"a","active":2,"offline":4}`, clientSessionStat{ClientID: "a", Active: 2, Offline: 4}},
		{"empty string", `{"clientId":"a","active":"","offline":"0"}`, clientSessionStat{ClientID: "a", Active: 0, Offline: 0}},
		{"null", `{"clientId":"a","active":null,"offline":null}`, clientSessionStat{ClientID: "a", Active: 0, Offline: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got clientSessionStat
			if err := json.Unmarshal([]byte(tt.json), &got); err != nil {
				t.Fatalf("Unmarshal(%s) error = %v", tt.json, err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestGetSessionsCount_UsesClientSessionStats verifies session counting uses the
// single realm-level client-session-stats endpoint (summing active/offline,
// which Keycloak returns as strings) and never the per-client session-count or
// the full-object list endpoints.
func TestGetSessionsCount_UsesClientSessionStats(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	record := func(k string) { mu.Lock(); hits[k]++; mu.Unlock() }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(p, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 300})
		case strings.HasSuffix(p, "/client-session-stats"):
			record("stats")
			// Only clients with sessions are included. Mix string (Keycloak's
			// current form) and numeric counts to exercise flexInt end-to-end.
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "c1", "clientId": "app1", "active": "3", "offline": "1"},
				{"id": "c2", "clientId": "app2", "active": 2, "offline": 4},
			})
		case strings.HasSuffix(p, "session-count"),
			strings.HasSuffix(p, "user-sessions"),
			strings.HasSuffix(p, "offline-sessions"),
			strings.HasSuffix(p, "/clients"):
			record("OTHER")
			http.Error(w, "per-client / list / clients endpoints must not be called", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := &ClientConfig{
		ServerURL:    srv.URL,
		AdminRealm:   "master",
		ClientID:     "monitoring-service",
		ClientSecret: "s",
		HTTPClient:   srv.Client(),
	}
	c, err := NewClient(context.Background(), cfg, noopLogger{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	active, offline, err := c.GetSessionsCount(context.Background(), "master")
	if err != nil {
		t.Fatalf("GetSessionsCount() error = %v", err)
	}
	if active != 5 {
		t.Errorf("activeSessions = %d, want 5 (3+2)", active)
	}
	if offline != 5 {
		t.Errorf("offlineSessions = %d, want 5 (1+4)", offline)
	}

	mu.Lock()
	defer mu.Unlock()
	if hits["OTHER"] != 0 {
		t.Errorf("per-client/list/clients endpoints were called %d times; expected 0", hits["OTHER"])
	}
	if hits["stats"] != 1 {
		t.Errorf("client-session-stats called %d times, want 1 (single realm-level call)", hits["stats"])
	}
}
