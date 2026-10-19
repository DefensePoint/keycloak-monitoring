package amfacheck

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	alertstore "github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// timedRepo serves per-realm events honouring the production contract:
// event_time strictly newer than since, oldest first.
type timedRepo struct {
	checkRepoStub
	mu      sync.Mutex
	events  map[string][]amfa.EventRow
	failing map[string]bool
}

func (r *timedRepo) list(realm string, since time.Time) ([]amfa.EventRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failing[realm] {
		return nil, errors.New("realm query failed")
	}
	var out []amfa.EventRow
	for _, ev := range r.events[realm] {
		if ev.EventTime.After(since) {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (r *timedRepo) ListRejectedEventsSince(_ context.Context, realm string, since time.Time) ([]amfa.EventRow, error) {
	return r.list(realm, since)
}

func (r *timedRepo) ListVPNRiskyEventsSince(_ context.Context, realm string, since time.Time, _ int) ([]amfa.EventRow, error) {
	return r.list(realm, since)
}

func (r *timedRepo) add(realm, id string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[realm] = append(r.events[realm], amfa.EventRow{EventID: id, EventTime: at})
}

func (r *timedRepo) setFailing(realm string, failing bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failing[realm] = failing
}

// cursorStore is an in-memory AlertStore whose saves can be made to fail.
type cursorStore struct {
	mu        sync.Mutex
	alerts    map[string]*domain.Alert
	failSave  map[string]bool
	failFetch map[string]bool
	deleted   map[string]*domain.Alert
}

// AlertExists counts deleted alerts, as the real repository does.
func (s *cursorStore) AlertExists(_ context.Context, _, alertID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failFetch[alertID] {
		return false, errors.New("connection reset")
	}
	_, live := s.alerts[alertID]
	_, gone := s.deleted[alertID]
	return live || gone, nil
}

// remove soft-deletes an alert, as an operator does from the Alerts page.
func (s *cursorStore) remove(alertID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted[alertID] = s.alerts[alertID]
	delete(s.alerts, alertID)
}

func (s *cursorStore) GetAlertByID(_ context.Context, _, alertID string) (*domain.Alert, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failFetch[alertID] {
		return nil, errors.New("connection reset")
	}
	if a, ok := s.alerts[alertID]; ok {
		return a, nil
	}
	return nil, alertstore.ErrNotFound
}

func (s *cursorStore) SaveAlert(_ context.Context, a *domain.Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failSave[a.AlertID] {
		return errors.New("save failed")
	}
	a.UpdatedAt = time.Now()
	s.alerts[a.AlertID] = a
	return nil
}

func (s *cursorStore) latest(_ context.Context, checkType, realm string) (time.Time, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var at time.Time
	for _, a := range s.alerts {
		if a.CheckType == checkType && a.RealmName == realm && a.FirstDetected.After(at) {
			at = a.FirstDetected
		}
	}
	return at, !at.IsZero(), nil
}

func (s *cursorStore) ListOpenAlertsByCheck(context.Context, string, string) ([]*domain.Alert, error) {
	return nil, nil
}
func (s *cursorStore) ResolveAlert(context.Context, string, string) error { return nil }

type countingNotifier struct {
	mu   sync.Mutex
	sent map[string]int
}

func (n *countingNotifier) NotifyAlert(_ context.Context, a *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent[a.AlertID]++
	return nil
}
func (n *countingNotifier) NotifyAlertResolution(context.Context, *domain.Alert) error { return nil }

type eventCheckCase struct {
	name     string
	newCheck func(realms RealmsFunc, repo amfa.Repository, opts ...EventCheckOption) Check
}

func eventCheckCases() []eventCheckCase {
	return []eventCheckCase{
		{name: "risk-rejected", newCheck: func(realms RealmsFunc, repo amfa.Repository, opts ...EventCheckOption) Check {
			return NewRiskRejectedCheck("t1", realms, repo, time.Minute, testLogger{}, opts...)
		}},
		{name: "vpn-risky", newCheck: func(realms RealmsFunc, repo amfa.Repository, opts ...EventCheckOption) Check {
			return NewVPNRiskyCheck("t1", realms, repo, time.Minute, testLogger{}, opts...)
		}},
	}
}

type cursorHarness struct {
	t        *testing.T
	check    Check
	repo     *timedRepo
	store    *cursorStore
	notifier *countingNotifier
	svc      Service
}

func newCursorHarness(t *testing.T, tc eventCheckCase, realms ...string) *cursorHarness {
	t.Helper()
	repo := &timedRepo{events: map[string][]amfa.EventRow{}, failing: map[string]bool{}}
	h := &cursorHarness{
		t:        t,
		repo:     repo,
		store:    &cursorStore{alerts: map[string]*domain.Alert{}, failSave: map[string]bool{}, failFetch: map[string]bool{}, deleted: map[string]*domain.Alert{}},
		notifier: &countingNotifier{sent: map[string]int{}},
	}
	h.check = tc.newCheck(singleRealm(realms...), repo)
	h.svc = NewService("t1", []Check{h.check}, h.store, h.notifier, testLogger{}, time.Minute)
	return h
}

// restart replaces the check and service with fresh ones over the same AMFA
// events and alert store, as a server restart does.
func (h *cursorHarness) restart(tc eventCheckCase, resume bool, realms ...string) {
	var opts []EventCheckOption
	if resume {
		opts = append(opts, WithResume(h.store.latest))
	}
	h.check = tc.newCheck(singleRealm(realms...), h.repo, opts...)
	h.svc = NewService("t1", []Check{h.check}, h.store, h.notifier, testLogger{}, time.Minute)
}

func (h *cursorHarness) run() {
	_ = h.svc.RunCheckNow(context.Background())
}

func (h *cursorHarness) alertFor(eventID string) string {
	return alertID(h.check.GetCheckType(), "t1", eventID)
}

// expectAlerted asserts each event has exactly one stored alert and was
// notified exactly once.
func (h *cursorHarness) expectAlerted(eventIDs ...string) {
	h.t.Helper()
	for _, id := range eventIDs {
		aid := h.alertFor(id)
		if _, ok := h.store.alerts[aid]; !ok {
			h.t.Fatalf("event %s has no alert", id)
		}
		if n := h.notifier.sent[aid]; n != 1 {
			h.t.Fatalf("event %s notified %d times, want 1", id, n)
		}
	}
}

func TestEventChecks_FailedSaveIsRetried(t *testing.T) {
	for _, tc := range eventCheckCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newCursorHarness(t, tc, "r1")
			now := time.Now()
			// ev-1 is older than ev-2 by more than lateEventWindow, so only the
			// cursor stopping before it brings it back.
			h.repo.add("r1", "ev-1", now.Add(-lateEventWindow-50*time.Second))
			h.repo.add("r1", "ev-2", now.Add(-10*time.Second))
			h.store.failSave[h.alertFor("ev-1")] = true

			h.run()
			if _, ok := h.store.alerts[h.alertFor("ev-1")]; ok {
				t.Fatal("precondition: the first save should fail")
			}

			delete(h.store.failSave, h.alertFor("ev-1"))
			h.run()
			h.run()
			h.expectAlerted("ev-1", "ev-2")
		})
	}
}

func TestEventChecks_FailingRealmLosesNothing(t *testing.T) {
	for _, tc := range eventCheckCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newCursorHarness(t, tc, "r1", "r2")
			now := time.Now()
			h.repo.add("r1", "ev-r1", now.Add(-10*time.Second))
			h.repo.add("r2", "ev-r2", now.Add(-10*time.Second))
			h.repo.setFailing("r2", true)

			h.run()
			h.expectAlerted("ev-r1")

			h.repo.setFailing("r2", false)
			h.run()
			h.run()
			h.expectAlerted("ev-r1", "ev-r2")
		})
	}
}

func TestEventChecks_LateEventIsRead(t *testing.T) {
	for _, tc := range eventCheckCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newCursorHarness(t, tc, "r1")
			now := time.Now()
			h.repo.add("r1", "ev-2", now.Add(-10*time.Second))
			h.run()

			h.repo.add("r1", "ev-1-late", now.Add(-30*time.Second))
			h.repo.add("r1", "ev-2-same-time", now.Add(-10*time.Second))
			h.run()
			h.run()
			h.expectAlerted("ev-2", "ev-1-late", "ev-2-same-time")
		})
	}
}

func TestEventChecks_RestartReadsTheDowntime(t *testing.T) {
	for _, tc := range eventCheckCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newCursorHarness(t, tc, "r1")
			now := time.Now()
			h.repo.add("r1", "ev-before", now.Add(-130*time.Minute))
			h.restart(tc, true, "r1")
			h.store.alerts[h.alertFor("ev-before")] = &domain.Alert{
				AlertID: h.alertFor("ev-before"), CheckType: h.check.GetCheckType(), RealmName: "r1",
				FirstDetected: now.Add(-125 * time.Minute), Status: domain.AlertStatusActive,
				UpdatedAt: now.Add(-125 * time.Minute),
			}
			h.notifier.sent[h.alertFor("ev-before")] = 1

			h.repo.add("r1", "ev-while-down", now.Add(-30*time.Minute))
			h.restart(tc, true, "r1")
			h.run()
			h.expectAlerted("ev-before", "ev-while-down")
		})
	}
}

func TestEventChecks_FirstStartReadsOnlyTheLookback(t *testing.T) {
	for _, tc := range eventCheckCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newCursorHarness(t, tc, "r1")
			now := time.Now()
			h.repo.add("r1", "ev-old", now.Add(-30*time.Minute))
			h.repo.add("r1", "ev-recent", now.Add(-30*time.Second))
			h.restart(tc, true, "r1")
			h.run()
			h.expectAlerted("ev-recent")
			if _, ok := h.store.alerts[h.alertFor("ev-old")]; ok {
				t.Fatal("first start alerted an event older than the lookback and late-event window")
			}
		})
	}
}

func cursorOf(t *testing.T, c Check) *eventCursor {
	t.Helper()
	switch c := c.(type) {
	case *RiskRejectedCheck:
		return c.events
	case *VPNRiskyCheck:
		return c.events
	}
	t.Fatalf("not a per-event check: %T", c)
	return nil
}

func TestEventChecks_CursorAdvancesOnceStored(t *testing.T) {
	for _, tc := range eventCheckCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newCursorHarness(t, tc, "r1")
			newest := time.Now().Add(-10 * time.Second)
			h.repo.add("r1", "ev-1", newest.Add(-time.Minute))
			h.repo.add("r1", "ev-2", newest)
			h.run()
			if got := cursorOf(t, h.check).cursor["r1"]; !got.Equal(newest) {
				t.Fatalf("cursor = %v, want the newest stored event %v", got, newest)
			}
		})
	}
}

func TestEventChecks_LookupFailureDoesNotRenotify(t *testing.T) {
	for _, tc := range eventCheckCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newCursorHarness(t, tc, "r1")
			h.repo.add("r1", "ev-1", time.Now().Add(-10*time.Second))
			h.run()

			h.store.failFetch[h.alertFor("ev-1")] = true
			h.repo.add("r1", "ev-2", time.Now().Add(-5*time.Second))
			h.run()
			delete(h.store.failFetch, h.alertFor("ev-1"))
			h.run()
			h.expectAlerted("ev-1", "ev-2")
		})
	}
}

func TestEventChecks_DeletedAlertStaysDeleted(t *testing.T) {
	for _, tc := range eventCheckCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newCursorHarness(t, tc, "r1")
			h.repo.add("r1", "ev-1", time.Now().Add(-10*time.Second))
			h.run()
			h.expectAlerted("ev-1")

			h.store.remove(h.alertFor("ev-1"))
			h.run()
			h.run()
			if _, ok := h.store.alerts[h.alertFor("ev-1")]; ok {
				t.Fatal("the deleted alert was raised again")
			}
			if n := h.notifier.sent[h.alertFor("ev-1")]; n != 1 {
				t.Fatalf("event notified %d times, want 1", n)
			}
		})
	}
}

type realmSilencer struct {
	mu     sync.Mutex
	realms map[string]bool
}

func (s *realmSilencer) set(realm string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.realms[realm] = on
}

func (s *realmSilencer) ShouldSilence(a *domain.Alert) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.realms[a.RealmName], "silence-" + a.RealmName
}

// A silenced event is handled, not pending: it never alerts, even after the
// silence ends, and the cursor moves past it, so later events in the realm
// are not reread on every tick.
func TestEventChecks_SilencedEventIsHandled(t *testing.T) {
	for _, tc := range eventCheckCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newCursorHarness(t, tc, "r1")
			silencer := &realmSilencer{realms: map[string]bool{"r1": true}}
			h.svc = NewService("t1", []Check{h.check}, h.store, h.notifier, testLogger{}, time.Minute, WithSilencer(silencer))

			silencedAt := time.Now().Add(-20 * time.Second)
			h.repo.add("r1", "ev-silenced", silencedAt)
			h.run()
			if got := cursorOf(t, h.check).cursor["r1"]; !got.Equal(silencedAt) {
				t.Fatalf("cursor = %v, want it past the silenced event (%v)", got, silencedAt)
			}

			silencer.set("r1", false)
			h.repo.add("r1", "ev-after", time.Now().Add(-5*time.Second))
			h.run()
			h.run()
			if _, ok := h.store.alerts[h.alertFor("ev-silenced")]; ok || h.notifier.sent[h.alertFor("ev-silenced")] != 0 {
				t.Fatal("the silenced event was alerted after the silence ended")
			}
			h.expectAlerted("ev-after")
		})
	}
}
