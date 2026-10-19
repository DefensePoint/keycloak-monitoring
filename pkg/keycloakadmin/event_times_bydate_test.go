package keycloakadmin

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newestFirst returns n events from since onwards, one per step, newest
// first as Keycloak before 26.2 lists them, with IDs prefix-0 (oldest) on.
func newestFirst(prefix string, since time.Time, n int, step time.Duration) []EventRepresentation {
	events := make([]EventRepresentation, n)
	for i := range n {
		events[n-1-i] = EventRepresentation{ID: fmt.Sprintf("%s-%d", prefix, i), Time: since.Add(time.Duration(i) * step).UnixMilli()}
	}
	return events
}

func eventIDs(events []*EventRepresentation) map[string]int {
	ids := make(map[string]int, len(events))
	for _, e := range events {
		ids[e.ID]++
	}
	return ids
}

func assertOldestFirst(t *testing.T, events []*EventRepresentation) {
	t.Helper()
	for i := 1; i < len(events); i++ {
		if events[i].Time < events[i-1].Time {
			t.Fatalf("events[%d] at %d is older than events[%d] at %d; want oldest first", i, events[i].Time, i-1, events[i-1].Time)
		}
	}
}

func eventsQueries(f *fakeKeycloak) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var firsts []string
	for _, q := range f.queries {
		firsts = append(firsts, q.Get("first"))
	}
	return firsts
}

// Before 26.2 a poll read only the newest page, so a burst bigger than
// max_events_per_poll lost its oldest events: a brute-force run lost its start.
func TestGetEventsSince_BeforeKeycloak262ReadsABurstBiggerThanAPage(t *testing.T) {
	since := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	f := &fakeKeycloak{version: "26.0.8", events: newestFirst("burst", since, 2500, 10*time.Millisecond)}
	c := newFakeKeycloakClient(t, f, 1000)

	events, full, err := c.GetEventsSince(context.Background(), "acme", since, 0, EventsPoll)
	if err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	if full {
		t.Error("full = true; the walk read to since, so nothing follows")
	}
	ids := eventIDs(events)
	for i := range 2500 {
		if ids[fmt.Sprintf("burst-%d", i)] == 0 {
			t.Fatalf("burst-%d is missing: got %d of 2500 events", i, len(ids))
		}
	}
	assertOldestFirst(t, events)
	if got := fmt.Sprint(eventsQueries(f)); got != "[ 1000 2000]" {
		t.Errorf("events requests at first = %s, want [ 1000 2000]: one page at a time, newest first", got)
	}
}

// The walk ends at the page that reaches events older than since, so a poll
// does not read the rest of the day.
func TestGetEventsSince_BeforeKeycloak262StopsAtThePageThatReachesSince(t *testing.T) {
	since := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	older := newestFirst("older", since.Add(-time.Hour), 3000, 10*time.Millisecond)
	newer := newestFirst("new", since, 1500, 10*time.Millisecond)
	f := &fakeKeycloak{version: "26.0.8", events: append(newer, older...)}
	c := newFakeKeycloakClient(t, f, 1000)

	events, _, err := c.GetEventsSince(context.Background(), "acme", since, 0, EventsPoll)
	if err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	if len(events) != 1500 {
		t.Errorf("got %d events, want the 1500 at or after since", len(events))
	}
	if got := fmt.Sprint(eventsQueries(f)); got != "[ 1000]" {
		t.Errorf("events requests at first = %s, want [ 1000]: the second page reached since", got)
	}
}

// New events push the older ones down the newest-first list while the walk
// pages through it. That can only show an event twice, never skip one.
func TestGetEventsSince_BeforeKeycloak262KeepsEveryEventWhileNewOnesArrive(t *testing.T) {
	since := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	f := &fakeKeycloak{version: "26.0.8", events: newestFirst("burst", since, 2500, 10*time.Millisecond)}
	arrived := 0
	f.beforeEvents = func(f *fakeKeycloak) {
		fresh := newestFirst(fmt.Sprintf("late%d", arrived), since.Add(time.Minute+time.Duration(arrived)*time.Second), 300, time.Millisecond)
		f.events = append(fresh, f.events...)
		arrived++
	}
	c := newFakeKeycloakClient(t, f, 1000)

	events, _, err := c.GetEventsSince(context.Background(), "acme", since, 0, EventsPoll)
	if err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	ids := eventIDs(events)
	for i := range 2500 {
		if ids[fmt.Sprintf("burst-%d", i)] == 0 {
			t.Fatalf("burst-%d is missing after %d arrivals: got %d distinct events", i, arrived, len(ids))
		}
	}
	assertOldestFirst(t, events)
}

// readCutShort reads, as read, a backlog bigger than maxDatePagesPerRead
// pages and returns the events and the warnings logged.
func readCutShort(t *testing.T, read EventsRead) ([]*EventRepresentation, *fakeKeycloak, []string) {
	t.Helper()
	since := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	const size = 10
	f := &fakeKeycloak{version: "26.0.8", events: newestFirst("backlog", since, maxDatePagesPerRead*size+25, time.Millisecond)}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	log := &recordingLogger{}
	c, err := NewClient(context.Background(), &ClientConfig{ServerURL: srv.URL, AdminRealm: "master", ClientID: "m", ClientSecret: "s", HTTPClient: srv.Client(), MaxEventsPerPoll: size}, log)
	if err != nil {
		t.Fatal(err)
	}
	events, _, err := c.GetEventsSince(context.Background(), "acme", since, 0, read)
	if err != nil {
		t.Fatalf("GetEventsSince() error = %v", err)
	}
	if got := len(eventsQueries(f)); got != maxDatePagesPerRead {
		t.Errorf("made %d events requests, want the cap of %d", got, maxDatePagesPerRead)
	}
	if len(events) != maxDatePagesPerRead*size {
		t.Errorf("got %d events, want the newest %d", len(events), maxDatePagesPerRead*size)
	}
	return events, f, log.matching("WARN")
}

// A backlog bigger than maxDatePagesPerRead pages is cut short, so one poll
// stays bounded. The oldest events are then lost, and that must be said.
func TestGetEventsSince_BeforeKeycloak262WarnsWhenThePollIsCutShort(t *testing.T) {
	_, _, warned := readCutShort(t, EventsPoll)
	if len(warned) != 1 || !strings.Contains(warned[0], "acme") || !strings.Contains(warned[0], "were not read") {
		t.Errorf("warnings = %q, want one naming the realm and saying the oldest events were not read", warned)
	}
}

// A sweep reads again what the polls already read, for events committed late.
// Cut short, it misses only those, and must not say the events were not read:
// during an attack that would claim the attack is undercounted when it is not.
func TestGetEventsSince_BeforeKeycloak262WarnsOnlyOfLateCommitsWhenASweepIsCutShort(t *testing.T) {
	_, _, warned := readCutShort(t, EventsSweep)
	if len(warned) != 1 || !strings.Contains(warned[0], "acme") {
		t.Fatalf("warnings = %q, want one naming the realm", warned)
	}
	if strings.Contains(warned[0], "were not read") {
		t.Errorf("warning %q says events were not read; the polls read them", warned[0])
	}
	for _, want := range []string{"already read", "committed late"} {
		if !strings.Contains(warned[0], want) {
			t.Errorf("warning %q does not say %q", warned[0], want)
		}
	}
}
