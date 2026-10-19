package keycloakadmin

import (
	"context"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNormalizeEventTypes(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want []string
	}{
		{nil, nil},
		{[]string{"LOGIN", "LOGIN_ERROR"}, []string{"LOGIN", "LOGIN_ERROR"}},
		{[]string{"LOGIN", " LOGIN_ERROR"}, []string{"LOGIN", "LOGIN_ERROR"}}, // a comma-separated setting split without trimming
		{[]string{"LOGIN, LOGIN_ERROR"}, []string{"LOGIN", "LOGIN_ERROR"}},    // one entry holding a list
		{[]string{"login", "Login_Error"}, []string{"LOGIN", "LOGIN_ERROR"}},
		{[]string{"LOGIN", "login", " LOGIN "}, []string{"LOGIN"}},
		{[]string{"", " ", ",", "LOGIN,,"}, []string{"LOGIN"}},
	} {
		if got := normalizeEventTypes(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("normalizeEventTypes(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// recordingLogger keeps the warnings and errors it is given.
type recordingLogger struct {
	noopLogger
	mu    sync.Mutex
	lines []string
}

func (l *recordingLogger) Warn(msg string, fields ...any)  { l.add("WARN", msg, fields) }
func (l *recordingLogger) Error(msg string, fields ...any) { l.add("ERROR", msg, fields) }
func (l *recordingLogger) add(level, msg string, fields []any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, level+" "+msg+" "+fmt.Sprint(fields...))
}
func (l *recordingLogger) matching(s string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, line := range l.lines {
		if strings.Contains(line, s) {
			out = append(out, line)
		}
	}
	return out
}

func typesClient(t *testing.T, f *fakeKeycloak, types []string, log Logger) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := NewClient(context.Background(), &ClientConfig{
		ServerURL: srv.URL, AdminRealm: "master", ClientID: "monitoring-service", ClientSecret: "s",
		HTTPClient: srv.Client(), MaxEventsPerPoll: 1000, EventTypes: types,
	}, log)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return c
}

var keycloakEventTypes = []string{"LOGIN", "LOGIN_ERROR", "CLIENT_LOGIN", "LOGOUT"}

// Names are trimmed and upper-cased, and the ones Keycloak does not list are
// left out with one warning per server info refresh, on both query paths.
func TestGetEventsSince_SkipsEventTypesKeycloakDoesNotKnow(t *testing.T) {
	for _, version := range []string{"26.6.3", "26.0.8"} {
		t.Run(version, func(t *testing.T) {
			f := &fakeKeycloak{version: version, eventTypes: keycloakEventTypes}
			log := &recordingLogger{}
			c := typesClient(t, f, []string{" login", "LOGIN_ERROR", "LOGINN", "login"}, log)

			for i := 0; i < 3; i++ {
				if _, _, err := c.GetEventsSince(context.Background(), "acme", time.Now(), 0, EventsPoll); err != nil {
					t.Fatalf("GetEventsSince() error = %v", err)
				}
				if got := f.lastQuery()["type"]; !slices.Equal(got, []string{"LOGIN", "LOGIN_ERROR"}) {
					t.Fatalf("sent type = %q, want LOGIN and LOGIN_ERROR", got)
				}
			}
			warns := log.matching("does not know")
			if len(warns) != 1 || !strings.Contains(warns[0], "LOGINN") {
				t.Fatalf("warnings = %q, want one naming LOGINN", warns)
			}

			c.now = func() time.Time { return time.Now().Add(eventTimesCheckEvery + time.Second) }
			if _, _, err := c.GetEventsSince(context.Background(), "acme", time.Now(), 0, EventsPoll); err != nil {
				t.Fatalf("GetEventsSince() error = %v", err)
			}
			if got := len(log.matching("does not know")); got != 2 {
				t.Errorf("%d warnings after %v, want the warning repeated once", got, eventTimesCheckEvery)
			}
		})
	}
}

// With no configured name Keycloak knows, nothing is fetched: an empty list
// would ask for every type.
func TestGetEventsSince_FetchesNothingWhenNoConfiguredTypeIsKnown(t *testing.T) {
	for _, version := range []string{"26.6.3", "26.0.8"} {
		t.Run(version, func(t *testing.T) {
			f := &fakeKeycloak{version: version, eventTypes: keycloakEventTypes}
			log := &recordingLogger{}
			c := typesClient(t, f, []string{"LOGINN", "LOGIN_EROR"}, log)

			_, _, err := c.GetEventsSince(context.Background(), "acme", time.Now(), 0, EventsPoll)
			if err == nil || !strings.Contains(err.Error(), "LOGINN") {
				t.Fatalf("GetEventsSince() error = %v, want one naming the unknown types", err)
			}
			if n := len(f.queries); n != 0 {
				t.Errorf("%d events requests sent, want none", n)
			}
			if len(log.matching("No configured event type")) != 1 {
				t.Errorf("want one error logged, got %q", log.lines)
			}
		})
	}
}

// When server info cannot be read, the normalized names are sent: Keycloak
// then decides, as before.
func TestGetEventsSince_SendsNormalizedTypesWhenKeycloakListsNone(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    *fakeKeycloak
	}{
		{"server info refused", &fakeKeycloak{}},
		{"no event types in server info", &fakeKeycloak{version: "26.6.3"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := typesClient(t, tc.f, []string{"login", " LOGIN_ERROR"}, noopLogger{})
			if _, _, err := c.GetEventsSince(context.Background(), "acme", time.Now(), 0, EventsPoll); err != nil {
				t.Fatalf("GetEventsSince() error = %v", err)
			}
			if got := tc.f.lastQuery()["type"]; !slices.Equal(got, []string{"LOGIN", "LOGIN_ERROR"}) {
				t.Errorf("sent type = %q, want LOGIN and LOGIN_ERROR", got)
			}
		})
	}
}

// No configured types still asks for all of them.
func TestGetEventsSince_NoTypesSendsNoTypeParameter(t *testing.T) {
	f := &fakeKeycloak{version: "26.6.3", eventTypes: keycloakEventTypes}
	c := typesClient(t, f, nil, noopLogger{})
	if _, _, err := c.GetEventsSince(context.Background(), "acme", time.Now(), 0, EventsPoll); err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	if q := f.lastQuery(); q.Has("type") {
		t.Errorf("sent type = %q, want none", q["type"])
	}
}

// Server info unreadable at first: the names are sent as configured; once a
// refresh reads Keycloak's list, the unknown ones are left out.
func TestGetEventsSince_ChecksTypesOnceServerInfoBecomesReadable(t *testing.T) {
	f := &fakeKeycloak{}
	log := &recordingLogger{}
	c := typesClient(t, f, []string{"LOGIN", "LOGINN"}, log)

	if _, _, err := c.GetEventsSince(context.Background(), "acme", time.Now(), 0, EventsPoll); err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	if got := f.lastQuery()["type"]; !slices.Equal(got, []string{"LOGIN", "LOGINN"}) {
		t.Fatalf("before server info: sent type = %q, want both names as configured", got)
	}

	f.mu.Lock()
	f.version, f.eventTypes = "26.6.3", keycloakEventTypes
	f.mu.Unlock()
	c.now = func() time.Time { return time.Now().Add(eventTimesCheckEvery + time.Second) }
	if _, _, err := c.GetEventsSince(context.Background(), "acme", time.Now(), 0, EventsPoll); err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	if got := f.lastQuery()["type"]; !slices.Equal(got, []string{"LOGIN"}) {
		t.Errorf("after server info: sent type = %q, want only LOGIN", got)
	}
	if len(log.matching("does not know")) != 1 {
		t.Errorf("want one warning once the list is read, got %q", log.lines)
	}
}
