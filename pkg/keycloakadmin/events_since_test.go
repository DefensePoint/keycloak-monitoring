package keycloakadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// GetEventsSince must send dateFrom as epoch milliseconds, oldest first. The
// yyyy-MM-dd form made Keycloak return the whole day on every poll.
func TestGetEventsSince_QueriesFromTheExactMillisecondOldestFirst(t *testing.T) {
	var mu sync.Mutex
	var query url.Values
	pageLen := 2
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 300})
		case strings.HasSuffix(r.URL.Path, "/admin/serverinfo"):
			_ = json.NewEncoder(w).Encode(map[string]any{"systemInfo": map[string]any{"version": "26.2.5"}})
		case strings.HasSuffix(r.URL.Path, "/admin/realms/acme/events"):
			mu.Lock()
			query = r.URL.Query()
			n := pageLen
			mu.Unlock()
			events := make([]EventRepresentation, n)
			for i := range events {
				events[i] = EventRepresentation{ID: "e", Time: int64(i)}
			}
			_ = json.NewEncoder(w).Encode(events)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := NewClient(context.Background(), &ClientConfig{
		ServerURL:        srv.URL,
		AdminRealm:       "master",
		ClientID:         "monitoring-service",
		ClientSecret:     "s",
		HTTPClient:       srv.Client(),
		EventTypes:       []string{"LOGIN"},
		MaxEventsPerPoll: 2,
	}, noopLogger{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	since := time.UnixMilli(1791996699416)
	events, full, err := c.GetEventsSince(context.Background(), "acme", since, 4, EventsPoll)
	if err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	if len(events) != 2 || !full {
		t.Errorf("got %d events, full=%v; want 2 and a full page", len(events), full)
	}
	mu.Lock()
	for key, want := range map[string]string{
		"dateFrom":  "1791996699416",
		"direction": "asc",
		"first":     "4",
		"max":       "2",
		"type":      "LOGIN",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("query %s = %q, want %q", key, got, want)
		}
	}
	if query.Has("dateTo") {
		t.Errorf("query has dateTo = %q; the fetch must be open-ended", query.Get("dateTo"))
	}
	pageLen = 1
	mu.Unlock()

	if _, full, err := c.GetEventsSince(context.Background(), "acme", since, 0, EventsPoll); err != nil || full {
		t.Errorf("short page: full=%v err=%v, want not full", full, err)
	}
}
