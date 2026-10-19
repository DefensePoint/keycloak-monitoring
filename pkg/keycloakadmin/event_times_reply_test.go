package keycloakadmin

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func epochQueries(f *fakeKeycloak) (epoch, byDate int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, q := range f.queries {
		if keycloakDateRE.MatchString(q.Get("dateFrom")) {
			byDate++
		} else {
			epoch++
		}
	}
	return epoch, byDate
}

// Keycloak 26.7 does not show its version to the minimal monitoring roles.
// Its reply to an epoch dateFrom decides: accepted, events are fetched by
// time, oldest first and by page, from then on.
func TestGetEventsSince_VersionHiddenAndEpochAcceptedFetchesByTime(t *testing.T) {
	for _, name := range []string{"version hidden", "server info refused"} {
		t.Run(name, func(t *testing.T) {
			f := &fakeKeycloak{hideVersion: name == "version hidden"}
			c := newFakeKeycloakClient(t, f, 2)
			f.events = []EventRepresentation{{Time: 3}, {Time: 2}}
			for i := 0; i < 3; i++ {
				_, full, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1), 0, EventsPoll)
				if err != nil {
					t.Fatalf("GetEventsSince() error = %v", err)
				}
				if !full {
					t.Errorf("call %d: a page of max events is not reported full; paging by time is off", i)
				}
			}
			if epoch, byDate := epochQueries(f); epoch != 3 || byDate != 0 {
				t.Errorf("sent %d epoch and %d date queries, want 3 epoch and none by date", epoch, byDate)
			}
			if q := f.lastQuery(); q.Get("direction") != "asc" {
				t.Errorf("direction = %q, want asc", q.Get("direction"))
			}
		})
	}
}

// Before 26.2 Keycloak refuses an epoch dateFrom with a 400. That first poll
// is served by date in the same call, and later ones go by date directly.
func TestGetEventsSince_VersionHiddenAndEpochRefusedFallsBackToDate(t *testing.T) {
	f := &fakeKeycloak{hideVersion: true, refuseEpoch: true}
	c := newFakeKeycloakClient(t, f, 1000)
	f.events = []EventRepresentation{{Time: 1791996699500}, {Time: 1791996699450}}

	events, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1791996699416), 0, EventsPoll)
	if err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	if len(events) != 2 || events[0].Time != 1791996699450 {
		t.Errorf("events = %v, want both, oldest first", events)
	}
	for i := 0; i < 2; i++ {
		if _, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1791996699416), 0, EventsPoll); err != nil {
			t.Fatalf("GetEventsSince() error = %v", err)
		}
	}
	if epoch, byDate := epochQueries(f); epoch != 1 || byDate != 3 {
		t.Errorf("sent %d epoch and %d date queries, want the one refused epoch query and 3 by date", epoch, byDate)
	}
}

// Any other failure decides nothing: the error is returned and the next poll
// tries by time again.
func TestGetEventsSince_OtherErrorsDoNotDecide(t *testing.T) {
	for _, status := range []int{403, 404} { // not retried; a 5xx is retried by doRequest, a 400 decides
		f := &fakeKeycloak{hideVersion: true, failEvents: []int{status}}
		c := newFakeKeycloakClient(t, f, 1000)
		if _, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1), 0, EventsPoll); err == nil {
			t.Fatalf("status %d: GetEventsSince() error = nil", status)
		}
		if _, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1), 0, EventsPoll); err != nil {
			t.Fatalf("status %d: second GetEventsSince() error = %v", status, err)
		}
		if epoch, byDate := epochQueries(f); epoch != 2 || byDate != 0 {
			t.Errorf("status %d: sent %d epoch and %d date queries, want 2 epoch", status, epoch, byDate)
		}
	}
}

// The decision lasts until the next server info refresh, so an upgrade past
// 26.2 is picked up.
func TestGetEventsSince_ReplyDecisionIsCheckedAgainAfterARefresh(t *testing.T) {
	f := &fakeKeycloak{hideVersion: true, refuseEpoch: true}
	c := newFakeKeycloakClient(t, f, 1000)
	if _, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1), 0, EventsPoll); err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	f.mu.Lock()
	f.refuseEpoch = false // upgraded
	f.mu.Unlock()
	c.now = func() time.Time { return time.Now().Add(eventTimesCheckEvery + time.Second) }
	if _, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1), 0, EventsPoll); err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	if q := f.lastQuery(); keycloakDateRE.MatchString(q.Get("dateFrom")) {
		t.Errorf("after the refresh dateFrom = %q, want epoch milliseconds", q.Get("dateFrom"))
	}
}

// infoLogger counts the precision lines it is given.
type infoLogger struct {
	noopLogger
	mu    sync.Mutex
	lines int
}

func (l *infoLogger) Info(msg string, _ ...any) {
	if msg == "Keycloak event time precision" {
		l.mu.Lock()
		l.lines++
		l.mu.Unlock()
	}
}

// The hourly refresh undecides a reply-based precision; deciding it the same
// way again is not logged again.
func TestGetEventsSince_PrecisionFromTheReplyIsLoggedOncePerChange(t *testing.T) {
	f := &fakeKeycloak{hideVersion: true}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	log := &infoLogger{}
	c, err := NewClient(context.Background(), &ClientConfig{ServerURL: srv.URL, AdminRealm: "master", ClientID: "m", ClientSecret: "s", HTTPClient: srv.Client(), MaxEventsPerPoll: 1000}, log)
	if err != nil {
		t.Fatal(err)
	}
	for hour := 0; hour < 4; hour++ {
		offset := time.Duration(hour) * (eventTimesCheckEvery + time.Second)
		c.now = func() time.Time { return time.Now().Add(offset) }
		if _, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1), 0, EventsPoll); err != nil {
			t.Fatalf("GetEventsSince() error = %v", err)
		}
	}
	if log.lines != 1 {
		t.Errorf("precision logged %d times over 4 refreshes, want once", log.lines)
	}
	f.mu.Lock()
	f.refuseEpoch = true // downgraded
	f.mu.Unlock()
	c.now = func() time.Time { return time.Now().Add(5 * (eventTimesCheckEvery + time.Second)) }
	if _, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1), 0, EventsPoll); err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	if log.lines != 2 {
		t.Errorf("precision logged %d times after it changed, want twice", log.lines)
	}
}

// A proxy that replaces Keycloak's 400 text still gets polls by date, with a
// warning, rather than every poll failing.
func TestGetEventsSince_A400WithoutDateFromStillFallsBackToDate(t *testing.T) {
	f := &fakeKeycloak{hideVersion: true, failEvents: []int{400}}
	log := &recordingLogger{}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := NewClient(context.Background(), &ClientConfig{ServerURL: srv.URL, AdminRealm: "master", ClientID: "m", ClientSecret: "s", HTTPClient: srv.Client(), MaxEventsPerPoll: 1000}, log)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1791996699416), 0, EventsPoll); err != nil {
			t.Fatalf("poll %d: GetEventsSince() error = %v", i, err)
		}
	}
	if epoch, byDate := epochQueries(f); epoch != 1 || byDate != 3 {
		t.Errorf("sent %d epoch and %d date queries, want the one refused epoch query and 3 by date", epoch, byDate)
	}
	if len(log.matching("does not name Date(From)")) != 1 {
		t.Errorf("want one warning about the unnamed 400, got %q", log.lines)
	}
}

// Keycloak's refusal outranks the version (a rollback, or an older node behind
// the same URL): the poll is served by date at once instead of failing until
// the next refresh, and after it the version is tried once more.
func TestGetEventsSince_RefusalOutranksTheVersion(t *testing.T) {
	f := &fakeKeycloak{version: "26.6.3", refuseEpoch: true}
	c := newFakeKeycloakClient(t, f, 1000)
	for i := 0; i < 3; i++ {
		if _, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1791996699416), 0, EventsPoll); err != nil {
			t.Fatalf("poll %d: GetEventsSince() error = %v", i, err)
		}
	}
	if epoch, byDate := epochQueries(f); epoch != 1 || byDate != 3 {
		t.Errorf("sent %d epoch and %d date queries, want 1 refused and 3 by date", epoch, byDate)
	}
	c.now = func() time.Time { return time.Now().Add(eventTimesCheckEvery + time.Second) }
	if _, _, err := c.GetEventsSince(context.Background(), "acme", time.UnixMilli(1791996699416), 0, EventsPoll); err != nil {
		t.Fatalf("after the refresh: GetEventsSince() error = %v", err)
	}
	if epoch, byDate := epochQueries(f); epoch != 2 || byDate != 4 {
		t.Errorf("after the refresh sent %d epoch and %d date queries in all, want one more of each", epoch, byDate)
	}
}
