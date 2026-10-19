package keycloak

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// fakeEventStream answers GetEventsSince from a fixed list the way Keycloak
// does: events at or after since, oldest first, pageSize at a time.
type fakeEventStream struct {
	mu       sync.Mutex
	events   []*keycloakadmin.EventRepresentation
	pageSize int
	requests []fakeEventRequest
	// reverseTies flips the order of same-millisecond events on every other
	// request, as a sort on time alone may.
	reverseTies bool
}

type fakeEventRequest struct {
	since time.Time
	first int
}

func (f *fakeEventStream) add(events ...*keycloakadmin.EventRepresentation) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, events...)
}

func (f *fakeEventStream) get(_ context.Context, _ string, since time.Time, first int) ([]*keycloakadmin.EventRepresentation, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, fakeEventRequest{since: since, first: first})
	var matched []*keycloakadmin.EventRepresentation
	for _, e := range f.events {
		if e.Time >= since.UnixMilli() {
			matched = append(matched, e)
		}
	}
	if f.reverseTies && len(f.requests)%2 == 0 {
		sort.SliceStable(matched, func(i, j int) bool {
			if matched[i].Time != matched[j].Time {
				return matched[i].Time < matched[j].Time
			}
			return matched[i].ID > matched[j].ID
		})
	}
	if first >= len(matched) {
		return nil, false, nil
	}
	end := first + f.pageSize
	if end > len(matched) {
		end = len(matched)
	}
	page := matched[first:end]
	return page, len(page) == f.pageSize, nil
}

func (f *fakeEventStream) firstRequest(t *testing.T) fakeEventRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("GetEventsSince was never called")
	}
	return f.requests[0]
}

func (f *fakeEventStream) lastRequest(t *testing.T) fakeEventRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("GetEventsSince was never called")
	}
	return f.requests[len(f.requests)-1]
}

func streamEvent(id string, at time.Time) *keycloakadmin.EventRepresentation {
	return &keycloakadmin.EventRepresentation{ID: id, Time: at.UnixMilli(), Type: "LOGIN", RealmID: "realm-1", UserID: "user-1"}
}

// savedIDsEventRepo records the event ids each SaveEvents call was given.
// legacyIDPattern matches the legacy ids, as LatestIDlessEventTime does.
var legacyIDPattern = regexp.MustCompile(`-[0-9]{12,}-[A-Z0-9_]+$`)

type savedIDsEventRepo struct {
	*mockEventRepository
	mu    sync.Mutex
	saved map[string]int
	at    map[string]time.Time
	fail  bool
}

func newSavedIDsEventRepo() *savedIDsEventRepo {
	r := &savedIDsEventRepo{mockEventRepository: newMockEventRepository(), saved: make(map[string]int), at: make(map[string]time.Time)}
	// Answers like the database: the newest event saved under an id the
	// poller made, at or after since.
	r.IDlessFunc = func(_ context.Context, _ string, since time.Time) (legacy, hashed time.Time, err error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		for id, at := range r.at {
			if at.Before(since) {
				continue
			}
			if strings.HasPrefix(id, syntheticEventIDPrefix) && at.After(hashed) {
				hashed = at
			}
			if legacyIDPattern.MatchString(id) && at.After(legacy) {
				legacy = at
			}
		}
		return legacy, hashed, nil
	}
	// Answers like the database: ids saved so far are stored.
	r.StoredEventIDsFunc = func(_ context.Context, _ string, ids []string, from, to time.Time) (map[string]bool, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		stored := make(map[string]bool)
		for _, id := range ids {
			if at := r.at[id]; r.saved[id] > 0 && !at.Before(from) && !at.After(to) {
				stored[id] = true
			}
		}
		return stored, nil
	}
	r.SaveEventsFunc = func(_ context.Context, events []*domain.KeycloakEvent) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.fail {
			return errors.New("database unavailable")
		}
		for _, e := range events {
			r.saved[e.EventID]++
			r.at[e.EventID] = e.Time
		}
		return nil
	}
	return r
}

func (r *savedIDsEventRepo) setFail(fail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fail = fail
}

func (r *savedIDsEventRepo) count(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saved[id]
}

func newCursorTestMonitor(stream *fakeEventStream, repo *savedIDsEventRepo, general *mockGeneralEventRepository) *Monitor {
	client := newMockAdminAPI()
	client.GetEventsSinceFunc = stream.get
	return NewMonitor("tenant-1", "Test Tenant", client, newTestConfig(), newTestLogger(),
		repo, general, newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(), nil, nil)
}

// The defect this replaced: every poll fetched and processed the whole day
// again. A second poll with nothing new must process nothing.
func TestCollectEvents_SecondPollProcessesOnlyNewEvents(t *testing.T) {
	now := time.Now()
	stream := &fakeEventStream{pageSize: 100}
	stream.add(streamEvent("a", now.Add(-2*time.Minute)), streamEvent("b", now.Add(-time.Minute)))
	repo := newSavedIDsEventRepo()
	general := newMockGeneralEventRepository()
	m := newCursorTestMonitor(stream, repo, general)

	m.collectEvents(context.Background())
	m.collectEvents(context.Background())
	if repo.count("a") != 1 || repo.count("b") != 1 {
		t.Fatalf("saved a=%d b=%d after a repeat poll, want each once", repo.count("a"), repo.count("b"))
	}

	stream.add(streamEvent("c", now))
	m.collectEvents(context.Background())
	if repo.count("a") != 1 || repo.count("b") != 1 || repo.count("c") != 1 {
		t.Errorf("saved a=%d b=%d c=%d, want each once", repo.count("a"), repo.count("b"), repo.count("c"))
	}
	if got := len(general.getSavedEvents()); got != 3 {
		t.Errorf("merged %d events into the events table, want 3", got)
	}
}

func TestCollectEvents_FirstPollStartsAtTheLookbackFloor(t *testing.T) {
	stream := &fakeEventStream{pageSize: 100}
	m := newCursorTestMonitor(stream, newSavedIDsEventRepo(), newMockGeneralEventRepository())
	m.instanceConfig.Events.LookbackDuration = time.Hour

	before := time.Now()
	m.collectEvents(context.Background())

	want := eventLookbackFloor(before, time.Hour)
	if got := stream.firstRequest(t).since; !got.Equal(want) {
		t.Errorf("first poll since = %v, want %v (start of the day an hour ago)", got, want)
	}
	// New events are fetched from now on, beside the backfill.
	if got := stream.lastRequest(t).since; got.Before(before.Add(-eventPollOverlap - time.Second)) {
		t.Errorf("new events fetched from %v, want about now minus the overlap", got)
	}
}

func TestCollectEvents_CursorTrailsNewestEventByTheOverlap(t *testing.T) {
	start := time.Now().Truncate(time.Millisecond)
	stream := &fakeEventStream{pageSize: 100}
	m := newCursorTestMonitor(stream, newSavedIDsEventRepo(), newMockGeneralEventRepository())
	clock := start
	m.now = func() time.Time { return clock }
	m.collectEvents(context.Background())

	newest := start.Add(2 * time.Minute)
	stream.add(streamEvent("b", newest))
	clock = start.Add(3 * time.Minute)
	m.collectEvents(context.Background())
	m.collectEvents(context.Background())

	if got, want := stream.lastRequest(t).since, newest.Add(-eventPollOverlap); !got.Equal(want) {
		t.Errorf("poll since = %v, want %v", got, want)
	}
}

// An event Keycloak stamps inside the overlap but commits after a poll is
// still collected by the next normal poll.
func TestCollectEvents_LateEventInsideTheOverlapIsCollected(t *testing.T) {
	start := time.Now().Truncate(time.Millisecond)
	stream := &fakeEventStream{pageSize: 100}
	repo := newSavedIDsEventRepo()
	m := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository())
	clock := start
	m.now = func() time.Time { return clock }
	m.collectEvents(context.Background())

	newest := start.Add(time.Minute)
	stream.add(streamEvent("a", newest))
	clock = newest.Add(5 * time.Second)
	m.collectEvents(context.Background())
	stream.add(streamEvent("late", newest.Add(-10*time.Second)))
	clock = clock.Add(5 * time.Second)
	m.collectEvents(context.Background())

	if repo.count("late") != 1 || repo.count("a") != 1 {
		t.Errorf("saved late=%d a=%d, want each once", repo.count("late"), repo.count("a"))
	}
}

// After a start, the day so far is backfilled a sweep's pages per cycle,
// while new events keep being fetched.
func TestCollectEvents_BackfillsTheDayBesideNewEvents(t *testing.T) {
	start := time.Now().Truncate(time.Millisecond)
	stream := &fakeEventStream{pageSize: 2}
	total := 2*maxEventPagesPerSweepCycle + 7
	for i := 0; i < total; i++ {
		stream.add(streamEvent(fmt.Sprintf("old-%03d", i), start.Add(-time.Hour+time.Duration(i)*time.Second)))
	}
	repo := newSavedIDsEventRepo()
	m := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository())
	clock := start
	m.now = func() time.Time { return clock }

	stream.add(streamEvent("new", start.Add(-time.Second)))
	m.collectEvents(context.Background())
	if repo.count("new") != 1 {
		t.Fatalf("a new event waited behind the backfill")
	}
	if got := len(stream.requests); got != maxEventPagesPerSweepCycle+1 {
		t.Fatalf("first cycle made %d requests, want the %d-page sweep share and one for new events", got, maxEventPagesPerSweepCycle)
	}
	for i := 0; i < 4; i++ {
		clock = clock.Add(30 * time.Second)
		m.collectEvents(context.Background())
	}
	for i := 0; i < total; i++ {
		if c := repo.count(fmt.Sprintf("old-%03d", i)); c != 1 {
			t.Errorf("backfilled event %d saved %d times, want 1", i, c)
		}
	}
	if m.eventCursors["test-realm"].sweeping {
		t.Error("the backfill never finished")
	}
}

func TestCollectEvents_PagesThroughABacklogAcrossCycles(t *testing.T) {
	start := time.Now().Truncate(time.Millisecond)
	stream := &fakeEventStream{pageSize: 2}
	repo := newSavedIDsEventRepo()
	m := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository())
	clock := start
	m.now = func() time.Time { return clock }
	m.collectEvents(context.Background())

	total := 2*maxEventPagesPerCycle + 3
	for i := 0; i < total; i++ {
		stream.add(streamEvent(fmt.Sprintf("e%02d", i), start.Add(time.Duration(i)*time.Second)))
	}
	clock = start.Add(time.Minute)
	before := len(stream.requests)
	m.collectEvents(context.Background())
	if got := len(stream.requests) - before; got != maxEventPagesPerCycle {
		t.Fatalf("cycle made %d requests, want the %d-page cap", got, maxEventPagesPerCycle)
	}
	// Each page fetches its newest event again, so a 2-event page brings one
	// new event and the backlog takes a few cycles to drain.
	for i := 0; i < 3; i++ {
		m.collectEvents(context.Background())
	}

	for i := 0; i < total; i++ {
		if c := repo.count(fmt.Sprintf("e%02d", i)); c != 1 {
			t.Errorf("event %d saved %d times, want 1", i, c)
		}
	}
}

// Keycloak orders events by time alone, so events in the same millisecond can
// come back in a different order on each request. A tie at a page boundary
// must not be skipped.
func TestCollectEvents_TieAtAPageBoundaryIsNotSkipped(t *testing.T) {
	base := time.Now().Add(-time.Hour)
	stream := &fakeEventStream{pageSize: 2, reverseTies: true}
	stream.add(streamEvent("a", base), streamEvent("b", base.Add(time.Second)), streamEvent("c", base.Add(time.Second)),
		streamEvent("d", base.Add(2*time.Second)), streamEvent("e", base.Add(2*time.Second)), streamEvent("f", base.Add(3*time.Second)))
	repo := newSavedIDsEventRepo()
	m := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository())

	m.collectEvents(context.Background())

	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		if c := repo.count(id); c != 1 {
			t.Errorf("event %s saved %d times in one cycle, want 1", id, c)
		}
	}
}

// A page entirely inside one millisecond still moves on, by offset.
func TestCollectEvents_FullPageInOneMillisecondMovesOn(t *testing.T) {
	at := time.Now().Add(-time.Hour)
	stream := &fakeEventStream{pageSize: 2}
	stream.add(streamEvent("a", at), streamEvent("b", at), streamEvent("c", at), streamEvent("d", at.Add(time.Second)))
	repo := newSavedIDsEventRepo()
	m := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository())
	m.eventCursors["test-realm"] = newRealmEventCursor(at, time.Now())

	m.collectEvents(context.Background())

	for _, id := range []string{"a", "b", "c", "d"} {
		if c := repo.count(id); c != 1 {
			t.Errorf("event %s saved %d times, want 1", id, c)
		}
	}
}

// Keycloak stores a successful event when its request commits, up to its
// 300s transaction timeout after the event's time. A normal poll, reaching back
// only the overlap, misses such an event; the next sweep collects it, once,
// without processing anything else again.
func TestCollectEvents_SweepCollectsAnEventCommittedLate(t *testing.T) {
	now := time.Now()
	stream := &fakeEventStream{pageSize: 100}
	stream.add(streamEvent("a", now.Add(-time.Second)))
	repo := newSavedIDsEventRepo()
	m := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository())
	m.collectEvents(context.Background())

	// Stamped 290s ago, visible only now.
	stream.add(streamEvent("late", now.Add(-290*time.Second)))
	m.collectEvents(context.Background())
	if c := repo.count("late"); c != 0 {
		t.Fatalf("a normal poll saved the late event %d times; the test needs it outside the overlap", c)
	}

	m.eventCursors["test-realm"].lastSweepStart = time.Now().Add(-eventSweepInterval)
	m.collectEvents(context.Background())
	m.collectEvents(context.Background())

	if repo.count("late") != 1 || repo.count("a") != 1 {
		t.Errorf("saved late=%d a=%d, want each once", repo.count("late"), repo.count("a"))
	}
	if req := stream.lastRequest(t); req.since.Before(now.Add(-eventPollOverlap - 5*time.Second)) {
		t.Errorf("the poll after the sweep reached back to %v; only a sweep should", req.since)
	}
}

// A sweep reaches back to the previous sweep's start minus the late commit
// window, so it covers an event that became visible at any point since then,
// even when sweeps start late.
func TestRealmEventCursor_SweepStartsBeforeThePreviousSweepByTheWindow(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	c := newRealmEventCursor(start.Add(-2*time.Hour), start)
	c.finishSweep() // the backfill
	c.since = start

	if started, _ := c.startSweepIfDue(start.Add(eventSweepInterval-time.Second), 7*time.Minute); started {
		t.Fatal("a sweep started before eventSweepInterval passed")
	}
	late := start.Add(3 * eventSweepInterval) // the next sweep is long overdue
	if started, _ := c.startSweepIfDue(late, 7*time.Minute); !started {
		t.Fatal("no sweep started once due")
	}
	if want := start.Add(-7*time.Minute - eventClockMargin); !c.sweepFrom.Equal(want) {
		t.Errorf("sweep from %v, want the previous start minus the window and margin, %v", c.sweepFrom, want)
	}
	if _, behind := c.startSweepIfDue(late.Add(eventSweepInterval), 7*time.Minute); !behind {
		t.Error("a sweep still running when the next is due was not reported")
	}
	if _, behind := c.startSweepIfDue(late.Add(eventSweepInterval+time.Second), 7*time.Minute); behind {
		t.Error("falling behind was reported twice for one sweep")
	}
}

func TestRealmEventCursor_VisibilityDelay(t *testing.T) {
	base := time.Now().Truncate(time.Second)
	c := newRealmEventCursor(base, base)
	at := base.Add(10 * time.Second)
	c.cover(at, base.UnixMilli(), base.Add(4*time.Second).UnixMilli())
	c.cover(base.Add(40*time.Second), base.Add(5*time.Second).UnixMilli(), openEnded)

	if d, ok := c.visibilityDelay(base.Add(2 * time.Second).UnixMilli()); !ok || d != 8*time.Second {
		t.Errorf("delay = %v, %v; want 8s from the fetch whose range held it", d, ok)
	}
	if d, ok := c.visibilityDelay(base.Add(6 * time.Second).UnixMilli()); !ok || d != 34*time.Second {
		t.Errorf("delay = %v, %v; want 34s from the open-ended fetch", d, ok)
	}
	if _, ok := c.visibilityDelay(base.Add(-time.Second).UnixMilli()); ok {
		t.Error("an event no fetch read before has a known delay")
	}
}

// A Keycloak node whose clock runs ahead must not carry the cursor past the
// events other nodes write with the right time.
func TestCollectEvents_AnEventFromTheFutureDoesNotHideCurrentOnes(t *testing.T) {
	now := time.Now()
	stream := &fakeEventStream{pageSize: 100}
	stream.add(streamEvent("ahead", now.Add(time.Hour)))
	repo := newSavedIDsEventRepo()
	m := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository())
	m.collectEvents(context.Background())

	stream.add(streamEvent("current", time.Now()))
	m.collectEvents(context.Background())

	if repo.count("current") != 1 || repo.count("ahead") != 1 {
		t.Errorf("saved current=%d ahead=%d, want each once", repo.count("current"), repo.count("ahead"))
	}
}

// Before the cursor a failed write was retried because the next poll fetched
// the whole day again. It must still be retried.
func TestCollectEvents_FailedSaveIsRetriedNextCycle(t *testing.T) {
	stream := &fakeEventStream{pageSize: 100}
	stream.add(streamEvent("a", time.Now().Add(-time.Minute)))
	repo := newSavedIDsEventRepo()
	m := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository())

	repo.setFail(true)
	m.collectEvents(context.Background())
	repo.setFail(false)
	m.collectEvents(context.Background())
	m.collectEvents(context.Background())

	if c := repo.count("a"); c != 1 {
		t.Errorf("event saved %d times after the database recovered, want 1", c)
	}
}

func TestCollectEvents_OnlyTheEventWhoseMergeFailedIsRetried(t *testing.T) {
	stream := &fakeEventStream{pageSize: 100}
	stream.add(streamEvent("ok", time.Now().Add(-2*time.Minute)), streamEvent("bad", time.Now().Add(-time.Minute)))
	repo := newSavedIDsEventRepo()
	general := newMockGeneralEventRepository()
	failBad := true
	general.SaveFunc = func(_ context.Context, e *domain.Event) error {
		if e.EventID == "bad" && failBad {
			return errors.New("merge failed")
		}
		return nil
	}
	m := newCursorTestMonitor(stream, repo, general)

	m.collectEvents(context.Background())
	failBad = false
	m.collectEvents(context.Background())

	if repo.count("ok") != 1 || repo.count("bad") != 2 {
		t.Errorf("saved ok=%d bad=%d, want ok once and bad retried once", repo.count("ok"), repo.count("bad"))
	}
}

func TestRealmEventCursor_RetriesOlderThanTheFloorExpire(t *testing.T) {
	floor := time.Now().Add(-time.Hour)
	c := newRealmEventCursor(floor, time.Now())
	c.addRetries([]*keycloakadmin.EventRepresentation{
		streamEvent("old", floor.Add(-time.Second)),
		streamEvent("due", floor.Add(time.Second)),
	})

	due, expired := c.takeRetries(floor)
	if expired != 1 || len(due) != 1 || due[0].ID != "due" {
		t.Errorf("takeRetries = %d due, %d expired; want only \"due\" kept and 1 expired", len(due), expired)
	}
	if again, _ := c.takeRetries(floor); len(again) != 0 {
		t.Errorf("queue not emptied: %d left", len(again))
	}
}

func TestRealmEventCursor_RetryQueueDropsTheOldestPastTheCap(t *testing.T) {
	c := newRealmEventCursor(time.Time{}, time.Now())
	base := time.Now()
	failed := make([]*keycloakadmin.EventRepresentation, maxEventRetries+5)
	for i := range failed {
		failed[i] = streamEvent("e", base.Add(time.Duration(i)*time.Millisecond))
	}
	if dropped := c.addRetries(failed); dropped != 5 {
		t.Errorf("dropped %d, want 5", dropped)
	}
	if len(c.retry) != maxEventRetries || c.retry[0] != failed[5] {
		t.Errorf("queue kept %d events starting at the wrong one; want the newest %d", len(c.retry), maxEventRetries)
	}
}

func TestEventLookbackFloor(t *testing.T) {
	loc := time.FixedZone("CEST", 2*3600)
	now := time.Date(2026, 10, 14, 0, 30, 0, 0, loc)
	if got, want := eventLookbackFloor(now, time.Hour), time.Date(2026, 10, 13, 0, 0, 0, 0, loc); !got.Equal(want) {
		t.Errorf("an hour before 00:30: floor = %v, want %v", got, want)
	}
	if got, want := eventLookbackFloor(now, time.Minute), time.Date(2026, 10, 14, 0, 0, 0, 0, loc); !got.Equal(want) {
		t.Errorf("a minute before 00:30: floor = %v, want %v", got, want)
	}
}

// delayRecorder records the visibility delays the monitor observes.
type delayRecorder struct {
	metrics.NopRecorder
	mu     sync.Mutex
	delays []time.Duration
}

func (r *delayRecorder) ObserveEventVisibilityDelay(_, _ string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delays = append(r.delays, d)
}

// An event that becomes visible long after its time is measured, and warned
// about once the delay passes half the late commit window; so is an event
// stamped ahead of this server's clock.
func TestCollectEvents_ReportsLateAndFutureEvents(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now().Truncate(time.Millisecond)
	stream := &fakeEventStream{pageSize: 100}
	stream.add(streamEvent("a", now.Add(-time.Second)))
	rec := &delayRecorder{}
	client := newMockAdminAPI()
	client.GetEventsSinceFunc = stream.get
	m := NewMonitor("tenant-1", "Test Tenant", client, newTestConfig(), logger.New(zerolog.New(&buf)),
		newSavedIDsEventRepo(), newMockGeneralEventRepository(), newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(), nil, nil)
	m.SetMetrics(rec)
	clock := now
	m.now = func() time.Time { return clock }
	m.collectEvents(context.Background())

	// Stamped 3 minutes before the first poll, which did not return it.
	stream.add(streamEvent("late", now.Add(-3*time.Minute)), streamEvent("ahead", now.Add(5*time.Minute)))
	clock = now.Add(30 * time.Second)
	m.eventCursors["test-realm"].lastSweepStart = clock.Add(-eventSweepInterval)
	m.collectEvents(context.Background())

	rec.mu.Lock()
	delays := append([]time.Duration(nil), rec.delays...)
	rec.mu.Unlock()
	if len(delays) != 1 || delays[0] != 3*time.Minute {
		t.Errorf("observed delays %v, want one of 3m (the first poll read its time and did not return it)", delays)
	}
	out := buf.String()
	for _, want := range []string{"became visible long after their time", `"max_delay":"3m0s"`, "stamped ahead of this server's clock", `"max_ahead":"4m30s"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q", want)
		}
	}
}

func TestMonitor_LateCommitWindowDefaultsToKeycloaks(t *testing.T) {
	m := newCursorTestMonitor(&fakeEventStream{pageSize: 1}, newSavedIDsEventRepo(), newMockGeneralEventRepository())
	if got := m.lateCommitWindow(); got != 5*time.Minute {
		t.Errorf("unset: %v, want 5m", got)
	}
	m.instanceConfig.Events.LateCommitWindow = 15 * time.Minute
	if got := m.lateCommitWindow(); got != 15*time.Minute {
		t.Errorf("set: %v, want 15m", got)
	}
}

// Late events warn once per eventTimingWarnEvery per realm, and the next
// warning, due once that has passed, counts the ones held back.
func TestCollectEvents_LateEventWarningsAreRateLimited(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now().Truncate(time.Millisecond)
	stream := &fakeEventStream{pageSize: 100}
	client := newMockAdminAPI()
	client.GetEventsSinceFunc = stream.get
	m := NewMonitor("tenant-1", "Test Tenant", client, newTestConfig(), logger.New(zerolog.New(&buf)),
		newSavedIDsEventRepo(), newMockGeneralEventRepository(), newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(), nil, nil)
	clock := now
	m.now = func() time.Time { return clock }
	m.collectEvents(context.Background())

	for i := 0; i < 4; i++ {
		clock = clock.Add(30 * time.Second)
		stream.add(streamEvent(fmt.Sprintf("late-%d", i), now.Add(-4*time.Minute)))
		m.eventCursors["test-realm"].lastSweepStart = clock.Add(-eventSweepInterval)
		m.collectEvents(context.Background())
	}
	if got := strings.Count(buf.String(), "became visible long after"); got != 1 {
		t.Fatalf("%d late warnings in 2 minutes, want 1", got)
	}

	clock = clock.Add(eventTimingWarnEvery)
	m.collectEvents(context.Background())
	if got := strings.Count(buf.String(), "became visible long after"); got != 2 {
		t.Fatalf("%d late warnings after %v, want 2", got, eventTimingWarnEvery)
	}
	if !strings.Contains(buf.String(), `"late_events":3`) {
		t.Errorf("the second warning does not count the 3 late events held back:\n%s", buf.String())
	}
}

func TestRealmEventCursor_FetchHistoryDropsTheOldestPastTheCap(t *testing.T) {
	c := newRealmEventCursor(time.Time{}, time.Now())
	at := time.Now()
	for i := range maxEventCoverage + 5 {
		c.cover(at.Add(time.Duration(i)*time.Second), int64(i), int64(i))
	}
	if len(c.covered) != maxEventCoverage || c.covered[0].from != 5 {
		t.Errorf("kept %d fetches starting at %d; want the newest %d", len(c.covered), c.covered[0].from, maxEventCoverage)
	}
}

// A by-date read cut short is reported differently for a sweep, which reads
// again what the polls already read, so the poller must say which reads are
// sweeps. The startup backfill is not: it reads the day for the first time.
func TestCollectEvents_TellsTheClientWhichReadsAreSweeps(t *testing.T) {
	start := time.Now().Truncate(time.Millisecond)
	stream := &fakeEventStream{pageSize: 100}
	stream.add(streamEvent("a", start.Add(-2*time.Minute)))
	client := newMockAdminAPI()
	client.GetEventsSinceFunc = stream.get
	m := NewMonitor("tenant-1", "Test Tenant", client, newTestConfig(), newTestLogger(),
		newSavedIDsEventRepo(), newMockGeneralEventRepository(), newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(), nil, nil)
	clock := start
	m.now = func() time.Time { return clock }
	reads := func() []keycloakadmin.EventsRead {
		client.mu.Lock()
		defer client.mu.Unlock()
		got := client.eventReads
		client.eventReads = nil
		return got
	}

	m.collectEvents(context.Background())
	first := reads()
	if len(first) < 2 {
		t.Fatalf("first cycle reads = %v, want the backfill and the poll", first)
	}
	for i, r := range first {
		if r != keycloakadmin.EventsPoll {
			t.Errorf("first cycle read %d = %v, want EventsPoll: the backfill reads the day for the first time", i, r)
		}
	}

	clock = start.Add(eventSweepInterval + time.Second)
	m.collectEvents(context.Background())
	second := reads()
	if len(second) < 2 {
		t.Fatalf("second cycle reads = %v, want a sweep and the poll", second)
	}
	if second[0] != keycloakadmin.EventsSweep {
		t.Errorf("sweep read = %v, want EventsSweep", second[0])
	}
	if last := second[len(second)-1]; last != keycloakadmin.EventsPoll {
		t.Errorf("poll read = %v, want EventsPoll", last)
	}
}
