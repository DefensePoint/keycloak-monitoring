package keycloak

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// simEvent is an event Keycloak stamps at Time (the stamping node's clock)
// and makes visible at visibleAt (true time), when its transaction commits.
type simEvent struct {
	ev        *keycloakadmin.EventRepresentation
	visibleAt time.Time
}

// simKeycloak serves GetEventsSince over simEvents as of a simulated clock:
// visible events at or after since, oldest first, with same-millisecond events
// in a different order on every other request.
type simKeycloak struct {
	mu       sync.Mutex
	clock    time.Time
	events   []simEvent
	pageSize int
	requests int
}

func (k *simKeycloak) get(_ context.Context, _ string, since time.Time, first int) ([]*keycloakadmin.EventRepresentation, bool, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.requests++
	var matched []*keycloakadmin.EventRepresentation
	for _, se := range k.events {
		if !se.visibleAt.After(k.clock) && se.ev.Time >= since.UnixMilli() {
			matched = append(matched, se.ev)
		}
	}
	flip := k.requests%2 == 0
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].Time != matched[j].Time {
			return matched[i].Time < matched[j].Time
		}
		return (matched[i].ID < matched[j].ID) != flip
	})
	if first >= len(matched) {
		return nil, false, nil
	}
	end := first + k.pageSize
	if end > len(matched) {
		end = len(matched)
	}
	page := matched[first:end]
	return page, len(page) == k.pageSize, nil
}

// Every event is collected exactly once over a stretch of simulated time with
// Keycloak nodes whose clocks differ from the monitor's, commit delays up to
// the transaction timeout, bursts larger than a cycle's or a sweep's page
// budget, and same-millisecond events reordered between requests.
func TestCollectEvents_SimulatedKeycloakCollectsEveryEventOnce(t *testing.T) {
	for seed := int64(1); seed <= 6; seed++ {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			simulateEventPolling(t, seed, time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local), time.Hour)
		})
	}
	// Across midnight with the one-minute lookback of config.yaml.example,
	// where the lookback floor jumps to the new day.
	for seed := int64(7); seed <= 9; seed++ {
		t.Run(fmt.Sprintf("seed %d across midnight", seed), func(t *testing.T) {
			simulateEventPolling(t, seed, time.Date(2026, 10, 14, 23, 35, 0, 0, time.Local), time.Minute)
		})
	}
}

func simulateEventPolling(t *testing.T, seed int64, start time.Time, lookback time.Duration) {
	const (
		pollEvery   = 30 * time.Second
		generateFor = 40 * time.Minute
		runFor      = 60 * time.Minute
	)
	rnd := rand.New(rand.NewSource(seed))
	nodeSkews := []time.Duration{-45 * time.Second, 0, 20 * time.Second}

	k := &simKeycloak{clock: start, pageSize: 20}
	repo := newSavedIDsEventRepo()
	client := newMockAdminAPI()
	client.GetEventsSinceFunc = k.get
	cfg := newTestConfig()
	cfg.Events.LookbackDuration = lookback
	m := NewMonitor("tenant-1", "Test Tenant", client, cfg, newTestLogger(),
		repo, newMockGeneralEventRepository(), newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(), nil, nil)
	m.now = func() time.Time {
		k.mu.Lock()
		defer k.mu.Unlock()
		return k.clock
	}

	n := 0
	for now := start; now.Before(start.Add(runFor)); now = now.Add(pollEvery) {
		if now.Before(start.Add(generateFor)) {
			count := 40 + rnd.Intn(40)
			minute := int(now.Sub(start) / time.Minute)
			if minute == 10 || minute == 25 {
				count = 1500 // a burst beyond one cycle's and one sweep cycle's pages
			}
			k.mu.Lock()
			for i := 0; i < count; i++ {
				happened := now.Add(time.Duration(rnd.Int63n(int64(pollEvery))))
				if count > 100 {
					happened = happened.Truncate(10 * time.Millisecond) // many ties
				}
				var delay time.Duration
				switch p := rnd.Float64(); {
				case p < 0.7:
					delay = time.Duration(rnd.Int63n(int64(500 * time.Millisecond)))
				case p < 0.9:
					delay = time.Duration(rnd.Int63n(int64(time.Minute)))
				default:
					delay = time.Duration(rnd.Int63n(int64(defaultLateCommitWindow - time.Second)))
				}
				stamped := happened.Add(nodeSkews[rnd.Intn(len(nodeSkews))])
				k.events = append(k.events, simEvent{
					ev:        &keycloakadmin.EventRepresentation{ID: fmt.Sprintf("e%06d", n), Time: stamped.UnixMilli(), Type: "LOGIN", RealmID: "realm-1"},
					visibleAt: happened.Add(delay),
				})
				n++
			}
			k.mu.Unlock()
		}
		k.mu.Lock()
		k.clock = now
		k.mu.Unlock()
		m.collectEvents(context.Background())
	}

	missing, repeated := 0, 0
	for _, se := range k.events {
		switch c := repo.count(se.ev.ID); {
		case c == 0:
			missing++
		case c > 1:
			repeated++
		}
	}
	if missing > 0 || repeated > 0 {
		t.Errorf("of %d events, %d were never saved and %d saved more than once", n, missing, repeated)
	}
}

// simMonitor is a monitor polling k on k's clock.
func simMonitor(k *simKeycloak, repo *savedIDsEventRepo, lookback time.Duration) *Monitor {
	client := newMockAdminAPI()
	client.GetEventsSinceFunc = k.get
	cfg := newTestConfig()
	cfg.Events.LookbackDuration = lookback
	m := NewMonitor("tenant-1", "Test Tenant", client, cfg, newTestLogger(),
		repo, newMockGeneralEventRepository(), newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(), nil, nil)
	m.now = func() time.Time {
		k.mu.Lock()
		defer k.mu.Unlock()
		return k.clock
	}
	return m
}

func (k *simKeycloak) setClock(t time.Time) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.clock = t
}

func (k *simKeycloak) addEvent(id string, stamped, visible time.Time) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.events = append(k.events, simEvent{ev: &keycloakadmin.EventRepresentation{ID: id, Time: stamped.UnixMilli(), Type: "LOGIN"}, visibleAt: visible})
}

// An event stamped before midnight and committed after it is collected even
// with a lookback so short that the floor has moved to the new day: sweeps
// are not held to the floor.
func TestCollectEvents_LateCommitAcrossMidnightWithAShortLookback(t *testing.T) {
	start := time.Date(2026, 10, 14, 23, 40, 0, 0, time.Local)
	k := &simKeycloak{clock: start, pageSize: 1000}
	for ts := start; ts.Before(start.Add(40 * time.Minute)); ts = ts.Add(5 * time.Second) {
		k.addEvent("bg"+ts.Format("150405"), ts, ts)
	}
	late := time.Date(2026, 10, 14, 23, 57, 0, 0, time.Local)
	k.addEvent("late", late, late.Add(4*time.Minute))
	repo := newSavedIDsEventRepo()
	m := simMonitor(k, repo, time.Minute)
	for now := start; now.Before(start.Add(40 * time.Minute)); now = now.Add(30 * time.Second) {
		k.setClock(now)
		m.collectEvents(context.Background())
	}
	if c := repo.count("late"); c != 1 {
		t.Errorf("event committed across midnight saved %d times, want 1", c)
	}
}

// With a Keycloak node an hour ahead, the cursor never passes the monitor's
// clock (also after partial fetches), so events from nodes with the right time
// stay prompt; the fast node's events are collected as its time comes round.
// The fetch history stays bounded, and every event is collected once.
func TestCollectEvents_NodeAnHourAheadKeepsTheCursorAtNow(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	start := time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local)
	k := &simKeycloak{clock: start, pageSize: 100}
	repo := newSavedIDsEventRepo()
	m := simMonitor(k, repo, time.Hour)
	n, maxCovered := 0, 0
	stamps := map[string]time.Time{}
	for now := start; now.Before(start.Add(130 * time.Minute)); now = now.Add(30 * time.Second) {
		if now.Before(start.Add(60 * time.Minute)) {
			for i := 0; i < 60; i++ {
				h := now.Add(time.Duration(rnd.Int63n(int64(30 * time.Second))))
				stamped := h
				if i%2 == 0 {
					stamped = h.Add(time.Hour)
				}
				id := fmt.Sprintf("e%06d", n)
				k.addEvent(id, stamped, h)
				stamps[id] = stamped
				n++
			}
		}
		k.setClock(now)
		m.collectEvents(context.Background())
		for id, st := range stamps {
			if st.Before(now.Add(-time.Minute)) && repo.count(id) == 0 {
				t.Fatalf("at +%v event %s, stamped +%v, is not collected", now.Sub(start), id, st.Sub(start))
			}
		}
		c := m.eventCursors["test-realm"]
		if c.since.After(now) {
			t.Fatalf("at +%v the cursor is at +%v, past the monitor's clock", now.Sub(start), c.since.Sub(start))
		}
		if len(c.covered) > maxCovered {
			maxCovered = len(c.covered)
		}
	}
	missing, repeated := 0, 0
	for i := 0; i < n; i++ {
		switch c := repo.count(fmt.Sprintf("e%06d", i)); {
		case c == 0:
			missing++
		case c > 1:
			repeated++
		}
	}
	if missing > 0 || repeated > 0 {
		t.Errorf("of %d events, %d missing and %d repeated", n, missing, repeated)
	}
	if maxCovered > 400 {
		t.Errorf("fetch history grew to %d entries", maxCovered)
	}
}

// On a quiet realm the fetch history does not grow with time.
func TestCollectEvents_FetchHistoryStaysBoundedOnAQuietRealm(t *testing.T) {
	start := time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local)
	k := &simKeycloak{clock: start, pageSize: 1000}
	m := simMonitor(k, newSavedIDsEventRepo(), time.Hour)
	maxFirstHour, maxLater := 0, 0
	for now := start; now.Before(start.Add(6 * time.Hour)); now = now.Add(30 * time.Second) {
		if int(now.Sub(start)/time.Minute)%10 == 0 {
			k.addEvent(now.Format("150405"), now, now)
		}
		k.setClock(now)
		m.collectEvents(context.Background())
		n := len(m.eventCursors["test-realm"].covered)
		if now.Sub(start) < time.Hour {
			maxFirstHour = max(maxFirstHour, n)
		} else {
			maxLater = max(maxLater, n)
		}
	}
	if maxLater > maxFirstHour {
		t.Errorf("fetch history peaked at %d entries in the first hour and %d later; want it bounded", maxFirstHour, maxLater)
	}
}

// When the monitor's clock steps back (NTP correcting it), sweeps still run
// and a late commit in the gap is collected.
func TestCollectEvents_ClockSteppingBackStillSweeps(t *testing.T) {
	start := time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local)
	k := &simKeycloak{clock: start, pageSize: 1000}
	repo := newSavedIDsEventRepo()
	m := simMonitor(k, repo, time.Hour)
	now := start
	for ; now.Before(start.Add(20 * time.Minute)); now = now.Add(30 * time.Second) {
		k.addEvent("bg"+now.Format("150405"), now, now)
		k.setClock(now)
		m.collectEvents(context.Background())
	}
	now = now.Add(-10 * time.Minute) // the clock steps back
	late := now.Add(-3 * time.Minute)
	k.addEvent("late", late, now)
	for end := now.Add(12 * time.Minute); now.Before(end); now = now.Add(30 * time.Second) {
		k.setClock(now)
		m.collectEvents(context.Background())
	}
	if c := repo.count("late"); c != 1 {
		t.Errorf("late event after the clock stepped back saved %d times, want 1", c)
	}
}
