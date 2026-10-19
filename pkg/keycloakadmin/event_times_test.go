package keycloakadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var keycloakDateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// fakeKeycloak answers the events endpoint with events, newest first, and
// server info with version ("" answers server info with a 403) and, when set,
// the event types it lists.
type fakeKeycloak struct {
	mu         sync.Mutex
	version    string
	eventTypes []string
	// hideVersion serves server info without the version, as Keycloak 26.7
	// does to the minimal monitoring roles; refuseEpoch answers an epoch
	// dateFrom with Keycloak's 400 from before 26.2; failEvents answers the
	// next events requests with that status.
	hideVersion bool
	refuseEpoch bool
	failEvents  []int
	events      []EventRepresentation
	queries     []url.Values
	serverInfos int
	// beforeEvents runs, with mu held, before each events request is answered.
	beforeEvents func(f *fakeKeycloak)
}

func (f *fakeKeycloak) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 300})
	case strings.HasSuffix(r.URL.Path, "/admin/serverinfo"):
		f.serverInfos++
		if f.version == "" && !f.hideVersion {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		info := map[string]any{"systemInfo": map[string]any{"version": f.version}}
		if f.hideVersion {
			info["systemInfo"] = map[string]any{"serverTime": "now"}
		}
		if f.eventTypes != nil {
			info["enums"] = map[string]any{"eventType": f.eventTypes, "operationType": []string{"CREATE"}}
		}
		_ = json.NewEncoder(w).Encode(info)
	case strings.HasSuffix(r.URL.Path, "/events"):
		f.queries = append(f.queries, r.URL.Query())
		if len(f.failEvents) > 0 {
			status := f.failEvents[0]
			f.failEvents = f.failEvents[1:]
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"unknown_error"}`))
			return
		}
		if from := r.URL.Query().Get("dateFrom"); f.refuseEpoch && from != "" && !keycloakDateRE.MatchString(from) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Invalid value for 'Date(From)', expected format is yyyy-MM-dd"}`))
			return
		}
		if f.beforeEvents != nil {
			f.beforeEvents(f)
		}
		page := f.events
		if first, _ := strconv.Atoi(r.URL.Query().Get("first")); first > 0 {
			page = page[min(first, len(page)):]
		}
		if max, _ := strconv.Atoi(r.URL.Query().Get("max")); max > 0 && max < len(page) {
			page = page[:max]
		}
		_ = json.NewEncoder(w).Encode(page)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeKeycloak) lastQuery() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries[len(f.queries)-1]
}

func newFakeKeycloakClient(t *testing.T, f *fakeKeycloak, maxEvents int) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := NewClient(context.Background(), &ClientConfig{
		ServerURL:        srv.URL,
		AdminRealm:       "master",
		ClientID:         "monitoring-service",
		ClientSecret:     "s",
		HTTPClient:       srv.Client(),
		MaxEventsPerPoll: maxEvents,
	}, noopLogger{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return c
}

func TestGetEventsSince_SendsMillisecondsFromKeycloak262(t *testing.T) {
	for _, tc := range []struct {
		version string
		exact   bool
	}{
		{"26.2.0", true},
		{"26.2.5", true},
		{"26.6.3", true},
		{"27.0.0", true},
		{"999.0.0-SNAPSHOT", true},
		{"26.1.4", false},
		{"26.0.6.vendor-00001", false},
		{"24.0.5", false},
		{"22.0.13.vendor-00001", false},
		{"unknown", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			f := &fakeKeycloak{version: tc.version}
			c := newFakeKeycloakClient(t, f, 1000)
			if _, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1791996699416), 0, EventsPoll); err != nil {
				t.Fatalf("GetEventsSince() error = %v", err)
			}
			q := f.lastQuery()
			if exact := q.Get("dateFrom") == "1791996699416"; exact != tc.exact {
				t.Errorf("dateFrom = %q, direction = %q; want epoch milliseconds: %v", q.Get("dateFrom"), q.Get("direction"), tc.exact)
			}
			if !tc.exact && (!keycloakDateRE.MatchString(q.Get("dateFrom")) || q.Has("direction")) {
				t.Errorf("dateFrom = %q, direction = %q; want a date and no direction", q.Get("dateFrom"), q.Get("direction"))
			}
		})
	}
}

// Before 26.2 the query is by date, newest first, so the client asks from the
// day before (Keycloak reads the date in its own time zone) and keeps the
// events at or after since, oldest first.
func TestGetEventsSince_BeforeKeycloak262KeepsTheEventsFromSince(t *testing.T) {
	since := time.Date(2026, 10, 15, 0, 0, 10, 0, time.UTC)
	ms := func(d time.Duration) int64 { return since.Add(d).UnixMilli() }
	f := &fakeKeycloak{version: "26.0.6.vendor-00001", events: []EventRepresentation{
		{ID: "e4", Time: ms(20 * time.Second)},
		{ID: "e3", Time: ms(10 * time.Second)},
		{ID: "e2", Time: ms(0)},
		{ID: "e1", Time: ms(-20 * time.Second)},
	}}
	c := newFakeKeycloakClient(t, f, 1000)

	events, full, err := c.GetEventsSince(context.Background(), "acme", since, 0, EventsPoll)
	if err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	var ids []string
	for _, e := range events {
		ids = append(ids, e.ID)
	}
	if strings.Join(ids, ",") != "e2,e3,e4" || full {
		t.Errorf("got %v, full=%v; want e2,e3,e4 oldest first and not full", ids, full)
	}
	if got, want := f.lastQuery().Get("dateFrom"), since.Add(-24*time.Hour).Format("2006-01-02"); got != want {
		t.Errorf("dateFrom = %q, want the day before since, %q", got, want)
	}
	if rest, _, _ := c.GetEventsSince(context.Background(), "acme", since, 2, EventsPoll); len(rest) != 1 || rest[0].ID != "e4" {
		t.Errorf("from offset 2 got %v, want e4", rest)
	}
}

// The version is read once and trusted for an hour, so an upgrade is picked
// up without asking for server info on every poll.
func TestGetEventsSince_ReadsTheVersionAgainAfterAnHour(t *testing.T) {
	f := &fakeKeycloak{version: "26.0.6"}
	c := newFakeKeycloakClient(t, f, 1000)
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	poll := func() string {
		t.Helper()
		if _, _, err := c.GetEventsSince(context.Background(), "acme", now, 0, EventsPoll); err != nil {
			t.Fatalf("GetEventsSince() error = %v", err)
		}
		return f.lastQuery().Get("dateFrom")
	}

	if got := poll(); !keycloakDateRE.MatchString(got) {
		t.Fatalf("26.0 dateFrom = %q, want a date", got)
	}
	f.mu.Lock()
	f.version = "26.2.5"
	f.mu.Unlock()
	now = now.Add(59 * time.Minute)
	if got := poll(); !keycloakDateRE.MatchString(got) {
		t.Errorf("within the hour dateFrom = %q, want still a date", got)
	}
	now = now.Add(2 * time.Minute)
	if got := poll(); keycloakDateRE.MatchString(got) {
		t.Errorf("after the hour dateFrom = %q, want epoch milliseconds", got)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.serverInfos != 2 {
		t.Errorf("server info read %d times, want 2", f.serverInfos)
	}
}
