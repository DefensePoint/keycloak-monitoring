package keycloak

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

func idlessEvent(at int64, user, ip string) *keycloakadmin.EventRepresentation {
	return &keycloakadmin.EventRepresentation{Time: at, Type: "LOGIN_ERROR", RealmID: "realm-1", ClientID: "app", UserID: user, IPAddress: ip, Error: "invalid_user_credentials", Details: map[string]string{"username": user, "auth_method": "openid-connect"}}
}

// Keycloak before 26.2 returns no event id. Two failed logins in one
// millisecond from different users or addresses must not share an id.
func TestEventKey_IdlessEventsInOneMillisecondKeepApart(t *testing.T) {
	a, b := idlessEvent(1791996699416, "alice", "10.0.0.1"), idlessEvent(1791996699416, "bob", "10.0.0.2")
	c := idlessEvent(1791996699416, "alice", "10.0.0.9")
	if legacyEventKey(a) != legacyEventKey(b) {
		t.Fatal("the test needs the legacy ids to collide")
	}
	if eventKey(a) == eventKey(b) || eventKey(a) == eventKey(c) {
		t.Errorf("eventKey = %s, %s, %s; want them apart", eventKey(a), eventKey(b), eventKey(c))
	}
	if !strings.HasPrefix(eventKey(a), syntheticEventIDPrefix) {
		t.Errorf("eventKey = %s, want the %s prefix", eventKey(a), syntheticEventIDPrefix)
	}
	same := idlessEvent(1791996699416, "alice", "10.0.0.1")
	same.Details = map[string]string{"auth_method": "openid-connect", "username": "alice"}
	if eventKey(a) != eventKey(same) {
		t.Error("the same event read again got another id")
	}
	if got := eventKey(&keycloakadmin.EventRepresentation{ID: "kc-id", Time: 1}); got != "kc-id" {
		t.Errorf("an event with a Keycloak id is stored as %q, want its id", got)
	}
}

func idsMonitor(repo *mockEventRepository, start time.Time) *Monitor {
	m := NewMonitor("tenant-1", "T", newMockAdminAPI(), newTestConfig(), newTestLogger(),
		repo, newMockGeneralEventRepository(), newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(), nil, nil)
	m.eventIDsHashedFrom = start
	return m
}

// After an upgrade the day is read again. Events stamped before the start
// that are stored under their legacy id keep it, so they are not stored twice;
// the others get the hashed id.
func TestEventIDs_KeepTheLegacyIDOfEventsStoredBeforeTheStart(t *testing.T) {
	start := time.UnixMilli(1791996700000)
	stored := idlessEvent(1791996699000, "alice", "10.0.0.1")
	notStored := idlessEvent(1791996699500, "bob", "10.0.0.2")
	after := idlessEvent(1791996700000+int64(2*time.Minute/time.Millisecond), "carol", "10.0.0.3") // past the clock margin
	repo := newMockEventRepository()
	var asked []string
	repo.StoredEventIDsFunc = func(_ context.Context, tenantID string, ids []string, _, _ time.Time) (map[string]bool, error) {
		asked = append(asked, ids...)
		return map[string]bool{legacyEventKey(stored): true}, nil
	}
	m := idsMonitor(repo, start)

	ids, _, _ := m.eventIDs(context.Background(), "realm", []*keycloakadmin.EventRepresentation{stored, notStored, after})

	if ids[0] != legacyEventKey(stored) || ids[1] != eventKey(notStored) || ids[2] != eventKey(after) {
		t.Errorf("ids = %q; want the legacy id, then hashed ids", ids)
	}
	if len(asked) != 2 || strings.Contains(strings.Join(asked, ","), legacyEventKey(after)) {
		t.Errorf("looked up %q; want the two events from before the start only", asked)
	}
}

// Events with a Keycloak id, or stamped after the start, need no lookup.
func TestEventIDs_NoLookupWhenNothingCanBeStoredUnderALegacyID(t *testing.T) {
	start := time.UnixMilli(1791996700000)
	repo := newMockEventRepository()
	m := idsMonitor(repo, start)
	_, _, _ = m.eventIDs(context.Background(), "realm", []*keycloakadmin.EventRepresentation{
		{ID: "kc-1", Time: 1791996699000, Type: "LOGIN"},
		idlessEvent(1791996700000+int64(2*time.Minute/time.Millisecond), "alice", "10.0.0.1"), // past the clock margin
	})
	if n := repo.calls["StoredEventIDs"]; n != 0 {
		t.Errorf("StoredEventIDs called %d times, want 0", n)
	}
}

// If the lookup fails, the events it was for are left unresolved, to be
// retried, rather than stored under a guess that could store them twice.
func TestEventIDs_LookupFailureLeavesTheEventsToRetry(t *testing.T) {
	start := time.UnixMilli(1791996700000)
	before := idlessEvent(1791996699000, "alice", "10.0.0.1")
	after := idlessEvent(start.Add(2*time.Minute).UnixMilli(), "bob", "10.0.0.2")
	repo := newMockEventRepository()
	repo.StoredEventIDsFunc = func(context.Context, string, []string, time.Time, time.Time) (map[string]bool, error) {
		return nil, errors.New("database unavailable")
	}
	m := idsMonitor(repo, start)
	m.now = func() time.Time { return start.Add(3 * time.Minute) }
	ids, unresolved, err := m.eventIDs(context.Background(), "realm", []*keycloakadmin.EventRepresentation{before, after})
	if err == nil || !unresolved[0] || unresolved[1] {
		t.Errorf("unresolved = %v, err = %v; want only the event from before the start", unresolved, err)
	}
	if ids[1] != eventKey(after) {
		t.Errorf("ids[1] = %q, want the hashed id", ids[1])
	}
}

// Through the poller: two failed logins in one millisecond from a Keycloak
// that returns no ids are both stored.
func TestCollectEvents_IdlessFailedLoginsInOneMillisecondAreBothStored(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	stream := &fakeEventStream{pageSize: 100}
	a, b := idlessEvent(now.Add(time.Second).UnixMilli(), "alice", "10.0.0.1"), idlessEvent(now.Add(time.Second).UnixMilli(), "bob", "10.0.0.2")
	repo := newSavedIDsEventRepo()
	m := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository())
	clock := now
	m.now = func() time.Time { return clock }
	m.collectEvents(context.Background())

	stream.add(a, b)
	clock = now.Add(5 * time.Second)
	m.collectEvents(context.Background())

	repo.mu.Lock()
	distinct, total := len(repo.saved), 0
	for _, n := range repo.saved {
		total += n
	}
	repo.mu.Unlock()
	if distinct != 2 || total != 2 {
		t.Errorf("stored %d events under %d ids; want both, under two ids", total, distinct)
	}
}

// Keycloak 26.2 returns ids, also for events it stored before. An event that
// now has one, stamped no later than the newest event stored without one,
// keeps the id it was stored under; one not stored takes its Keycloak id.
func TestEventIDs_KeycloakUpgradeKeepsTheIDsEventsWereStoredUnder(t *testing.T) {
	start := time.UnixMilli(1791996700000)
	asHash := idlessEvent(1791996699000, "alice", "10.0.0.1")
	asLegacy := idlessEvent(1791996699100, "bob", "10.0.0.2")
	notStored := idlessEvent(1791996699200, "carol", "10.0.0.3")
	later := idlessEvent(1791996699900, "dave", "10.0.0.4")
	hashID, legacyID := eventKey(asHash), legacyEventKey(asLegacy)
	for i, e := range []*keycloakadmin.EventRepresentation{asHash, asLegacy, notStored, later} {
		e.ID = fmt.Sprintf("keycloak-uuid-%d", i)
	}
	repo := newMockEventRepository()
	repo.IDlessFunc = func(context.Context, string, time.Time) (time.Time, time.Time, error) {
		return time.Time{}, time.UnixMilli(1791996699500), nil
	}
	var asked []string
	repo.StoredEventIDsFunc = func(_ context.Context, _ string, ids []string, _, _ time.Time) (map[string]bool, error) {
		asked = append(asked, ids...)
		return map[string]bool{hashID: true, legacyID: true}, nil
	}
	m := idsMonitor(repo, start)

	ids, _, _ := m.eventIDs(context.Background(), "realm", []*keycloakadmin.EventRepresentation{asHash, asLegacy, notStored, later})

	if ids[0] != hashID || ids[1] != legacyID || ids[2] != "keycloak-uuid-2" || ids[3] != "keycloak-uuid-3" {
		t.Errorf("ids = %q; want the stored hash, the stored legacy id, then Keycloak's ids", ids)
	}
	for _, id := range asked {
		if strings.Contains(id, "1791996699900") {
			t.Errorf("looked up %s, stamped after the newest event stored without an id", id)
		}
	}
	_, _, _ = m.eventIDs(context.Background(), "realm", []*keycloakadmin.EventRepresentation{asHash})
	if n := repo.calls["IDlessEventTimes"]; n != 1 {
		t.Errorf("IDlessEventTimes called %d times, want once", n)
	}
}

// A tenant whose Keycloak always returned ids pays no lookup.
func TestEventIDs_NoLookupForATenantThatNeverStoredEventsWithoutIDs(t *testing.T) {
	repo := newMockEventRepository()
	m := idsMonitor(repo, time.UnixMilli(1791996700000))
	e := idlessEvent(1791996699000, "alice", "10.0.0.1")
	e.ID = "keycloak-uuid"
	if ids, _, _ := m.eventIDs(context.Background(), "realm", []*keycloakadmin.EventRepresentation{e}); ids[0] != "keycloak-uuid" {
		t.Errorf("id = %q, want Keycloak's", ids[0])
	}
	if n := repo.calls["StoredEventIDs"]; n != 0 {
		t.Errorf("StoredEventIDs called %d times, want 0", n)
	}
}

// If the newest event stored without an id cannot be read, every event from
// before the start is checked; later ones are not.
func TestEventIDs_UnreadableIDlessTimeChecksEverythingBeforeTheStart(t *testing.T) {
	start := time.UnixMilli(1791996700000)
	before, after := idlessEvent(1791996699000, "alice", "10.0.0.1"), idlessEvent(1791996700500, "bob", "10.0.0.2")
	before.ID, after.ID = "kc-before", "kc-after"
	repo := newMockEventRepository()
	repo.IDlessFunc = func(context.Context, string, time.Time) (time.Time, time.Time, error) {
		return time.Time{}, time.Time{}, errors.New("database unavailable")
	}
	var asked []string
	repo.StoredEventIDsFunc = func(_ context.Context, _ string, ids []string, _, _ time.Time) (map[string]bool, error) {
		asked = append(asked, ids...)
		return map[string]bool{}, nil
	}
	m := idsMonitor(repo, start)
	ids, _, _ := m.eventIDs(context.Background(), "realm", []*keycloakadmin.EventRepresentation{before, after})
	if ids[0] != "kc-before" || ids[1] != "kc-after" || len(asked) != 2 {
		t.Errorf("ids = %q, looked up %q; want Keycloak's ids and only the event from before the start checked", ids, asked)
	}
}

// Through the poller: Keycloak is upgraded while the monitor runs, and the
// events it served without ids come back with them. None is stored twice.
func TestCollectEvents_KeycloakUpgradeUnderARunningMonitorStoresNothingTwice(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	stream := &fakeEventStream{pageSize: 100}
	var idless []*keycloakadmin.EventRepresentation
	for i := 0; i < 5; i++ {
		idless = append(idless, idlessEvent(now.Add(time.Duration(i+1)*time.Second).UnixMilli(), fmt.Sprintf("user-%d", i), "10.0.0.1"))
	}
	stream.add(idless...)
	repo := newSavedIDsEventRepo()
	m := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository())
	m.eventIDsHashedFrom = now
	clock := now.Add(10 * time.Second)
	m.now = func() time.Time { return clock }
	m.collectEvents(context.Background())

	// Upgraded: the same events, now with ids, and a new one.
	stream.mu.Lock()
	stream.events = nil
	stream.mu.Unlock()
	for i, e := range idless {
		withID := *e
		withID.ID = fmt.Sprintf("uuid-%d", i)
		stream.add(&withID)
	}
	stream.add(&keycloakadmin.EventRepresentation{ID: "uuid-new", Time: now.Add(20 * time.Second).UnixMilli(), Type: "LOGIN", RealmID: "realm-1"})
	m.eventCursors["test-realm"].lastSweepStart = clock.Add(-eventSweepInterval)
	clock = now.Add(25 * time.Second)
	m.eventCursors["test-realm"].seen = make(map[string]int64) // the ids are new to the seen set too
	m.collectEvents(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.saved) != 6 {
		t.Errorf("stored under %d ids, want 6 (5 events once, plus the new one): %v", len(repo.saved), repo.saved)
	}
	for id, n := range repo.saved {
		if n != 1 && !strings.HasPrefix(id, syntheticEventIDPrefix) {
			t.Errorf("%s saved %d times", id, n)
		}
	}
}

// simIDKeycloak is a Keycloak 26.0 that becomes 26.2 at upgradeAt: events
// visible from then come with ids, including the ones stored before.
type simIDKeycloak struct {
	k         *simKeycloak
	upgradeAt time.Time
	ids       map[*keycloakadmin.EventRepresentation]string
}

func (s *simIDKeycloak) get(ctx context.Context, realm string, since time.Time, first int) ([]*keycloakadmin.EventRepresentation, bool, error) {
	page, full, err := s.k.get(ctx, realm, since, first)
	s.k.mu.Lock()
	upgraded := !s.k.clock.Before(s.upgradeAt)
	s.k.mu.Unlock()
	out := make([]*keycloakadmin.EventRepresentation, len(page))
	for i, e := range page {
		c := *e
		if upgraded {
			c.ID = s.ids[e]
		}
		out[i] = &c
	}
	return out, full, err
}

// Keycloak is upgraded to return ids just after midnight, and the monitor
// restarts minutes later with the one-minute lookback of config.yaml.example.
// Its first sweep reads back before midnight, into the events stored the day
// before without ids: none may be stored twice. Also when the monitor is
// down during the upgrade, and a day later, when the last event without an id
// is older than anything read again.
func TestCollectEvents_KeycloakUpgradeAcrossMidnightStoresNothingTwice(t *testing.T) {
	for _, tc := range []struct {
		name      string
		upgradeAt time.Time
		restartAt time.Time
		down      bool // the monitor is stopped from the upgrade to the restart
	}{
		{"monitor running through an upgrade just after midnight", time.Date(2026, 10, 15, 0, 1, 0, 0, time.Local), time.Date(2026, 10, 15, 0, 5, 0, 0, time.Local), false},
		{"monitor down during an upgrade just after midnight", time.Date(2026, 10, 15, 0, 1, 0, 0, time.Local), time.Date(2026, 10, 15, 0, 5, 0, 0, time.Local), true},
		{"monitor down during an upgrade a day later", time.Date(2026, 10, 16, 9, 0, 0, 0, time.Local), time.Date(2026, 10, 16, 10, 0, 0, 0, time.Local), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Date(2026, 10, 14, 23, 50, 0, 0, time.Local)
			k := &simKeycloak{clock: start, pageSize: 1000}
			sk := &simIDKeycloak{k: k, upgradeAt: tc.upgradeAt, ids: map[*keycloakadmin.EventRepresentation]string{}}
			n := 0
			add := func(at time.Time) {
				e := idlessEvent(at.UnixMilli(), fmt.Sprintf("user-%d", n), "10.0.0.1")
				k.mu.Lock()
				k.events = append(k.events, simEvent{ev: e, visibleAt: at})
				k.mu.Unlock()
				sk.ids[e] = fmt.Sprintf("uuid-%d", n)
				n++
			}
			repo := newSavedIDsEventRepo()
			newMon := func(at time.Time) *Monitor {
				client := newMockAdminAPI()
				client.GetEventsSinceFunc = sk.get
				cfg := newTestConfig()
				cfg.Events.LookbackDuration = time.Minute
				m := NewMonitor("tenant-1", "T", client, cfg, newTestLogger(), repo, newMockGeneralEventRepository(), newMockMetricsRepository(), newMockHealthRepository(), newMockRealmRepository(), nil, nil)
				m.now = func() time.Time { k.mu.Lock(); defer k.mu.Unlock(); return k.clock }
				m.eventIDsHashedFrom = at
				return m
			}
			m := newMon(start)
			restarted, skipped := false, 0
			for now := start; now.Before(tc.restartAt.Add(20 * time.Minute)); now = now.Add(30 * time.Second) {
				// Events until just before midnight while Keycloak gives no ids,
				// and again from the upgrade.
				if now.Before(time.Date(2026, 10, 14, 23, 59, 40, 0, time.Local)) || !now.Before(tc.upgradeAt) {
					add(now.Add(-10 * time.Second))
				}
				k.setClock(now)
				if !now.Before(tc.restartAt) && !restarted {
					m, restarted = newMon(now), true // nothing in memory
				}
				if tc.down && !now.Before(tc.upgradeAt.Add(-time.Minute)) && now.Before(tc.restartAt) {
					skipped++
					continue
				}
				m.collectEvents(context.Background())
			}
			if tc.down && skipped == 0 {
				t.Fatal("the monitor was never down")
			}
			repo.mu.Lock()
			defer repo.mu.Unlock()
			if len(repo.saved) != n {
				t.Errorf("%d events stored under %d ids; want each once", n, len(repo.saved))
			}
		})
	}
}

// failingOnce makes StoredEventIDs fail the first time.
func failingOnce(repo *savedIDsEventRepo) {
	inner := repo.StoredEventIDsFunc
	failed := false
	repo.StoredEventIDsFunc = func(ctx context.Context, tenantID string, ids []string, from, to time.Time) (map[string]bool, error) {
		if !failed {
			failed = true
			return nil, errors.New("statement timeout")
		}
		return inner(ctx, tenantID, ids, from, to)
	}
}

// A second start of the new monitor reads the day again, stored under hashed
// ids. When that lookup fails, the events are retried, not stored under a
// guessed id next to the hashed one.
func TestCollectEvents_LookupFailureOnASecondStartStoresNothingTwice(t *testing.T) {
	start := time.Now().Truncate(time.Millisecond).Add(-time.Hour)
	stream := &fakeEventStream{pageSize: 100}
	for i := 0; i < 5; i++ {
		stream.add(idlessEvent(start.Add(time.Duration(i+1)*time.Minute).UnixMilli(), fmt.Sprintf("user-%d", i), "10.0.0.1"))
	}
	repo := newSavedIDsEventRepo()
	m := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository())
	m.eventIDsHashedFrom = start
	m.collectEvents(context.Background())

	failingOnce(repo)
	m2 := newCursorTestMonitor(stream, repo, newMockGeneralEventRepository()) // the second start
	for i := 0; i < 3; i++ {
		m2.collectEvents(context.Background())
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.saved) != 5 {
		t.Errorf("stored under %d ids, want 5: %v", len(repo.saved), repo.saved)
	}
}

// The old monitor's last rows can be stamped a little after the new one's
// start, when Keycloak's clock is ahead: they still match their legacy ids.
func TestEventIDs_LegacyRowsStampedJustAfterTheStartAreMatched(t *testing.T) {
	start := time.UnixMilli(1791996700000)
	ahead := idlessEvent(start.Add(20*time.Second).UnixMilli(), "alice", "10.0.0.1")
	repo := newMockEventRepository()
	repo.StoredEventIDsFunc = func(context.Context, string, []string, time.Time, time.Time) (map[string]bool, error) {
		return map[string]bool{legacyEventKey(ahead): true}, nil
	}
	m := idsMonitor(repo, start)
	m.now = func() time.Time { return start.Add(time.Minute) }
	if ids, _, _ := m.eventIDs(context.Background(), "realm", []*keycloakadmin.EventRepresentation{ahead}); ids[0] != legacyEventKey(ahead) {
		t.Errorf("id = %q, want the legacy id it is stored under", ids[0])
	}
}

// One event without an id stamped far ahead does not send every later event
// with an id to a lookup.
func TestEventIDs_AFarFutureEventDoesNotSendEverythingToALookup(t *testing.T) {
	now := time.UnixMilli(1791996700000)
	repo := newMockEventRepository()
	m := idsMonitor(repo, now.Add(-time.Hour))
	m.now = func() time.Time { return now }
	_, _, _ = m.eventIDs(context.Background(), "realm", []*keycloakadmin.EventRepresentation{idlessEvent(now.Add(365*24*time.Hour).UnixMilli(), "x", "10.0.0.9")})
	calls := repo.calls["StoredEventIDs"]
	later := idlessEvent(now.Add(10*time.Minute).UnixMilli(), "y", "10.0.0.8")
	later.ID = "kc-later"
	if ids, _, _ := m.eventIDs(context.Background(), "realm", []*keycloakadmin.EventRepresentation{later}); ids[0] != "kc-later" || repo.calls["StoredEventIDs"] != calls {
		t.Errorf("id %q, lookups %d -> %d; want Keycloak's id and no lookup", ids[0], calls, repo.calls["StoredEventIDs"])
	}
}
