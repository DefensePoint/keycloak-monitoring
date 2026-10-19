package eventscheck

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// A threshold alert describes a condition, "N errors in the last few
// minutes". These tests drive the service with a real check at threshold 1,
// the setting where an alert that never resolves hurts most.

// realmCounts answers CountEventsByType per realm, failing for realms in errs.
type realmCounts struct {
	mu     sync.Mutex
	counts map[string]int64
	errs   map[string]error
}

func (r *realmCounts) CountEventsByType(_ context.Context, _, realm, _ string, _, _ time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.errs[realm]; err != nil {
		return 0, err
	}
	return r.counts[realm], nil
}

func (r *realmCounts) set(realm string, count int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[realm] = count
}

func (r *realmCounts) fail(realm string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs[realm] = err
}

// memStore keeps alerts the way the alerts repository does: a save of an
// existing alert writes only status, last_seen, resolved_at and the
// acknowledgement, and the rest of the row stays as first saved.
type memStore struct {
	mu     sync.Mutex
	alerts map[string]*domain.Alert
}

func newMemStore() *memStore { return &memStore{alerts: map[string]*domain.Alert{}} }

func (m *memStore) GetAlertByID(_ context.Context, _, id string) (*domain.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.alerts[id]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, nil
}

func (m *memStore) SaveAlert(_ context.Context, a *domain.Alert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.alerts[a.AlertID]; ok {
		existing.Status, existing.LastSeen, existing.ResolvedAt = a.Status, a.LastSeen, a.ResolvedAt
		existing.AcknowledgedAt, existing.AcknowledgedBy = a.AcknowledgedAt, a.AcknowledgedBy
		return nil
	}
	cp := *a
	m.alerts[a.AlertID] = &cp
	return nil
}

func (m *memStore) ListOpenAlertsByCheck(_ context.Context, tenantID, checkType string) ([]*domain.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.Alert
	for _, a := range m.alerts {
		if a.TenantID == tenantID && a.CheckType == checkType &&
			(a.Status == domain.AlertStatusActive || a.Status == domain.AlertStatusAcknowled) {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *memStore) ResolveAlert(_ context.Context, _, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.alerts[id]; ok {
		a.Status = domain.AlertStatusResolved
		now := time.Now()
		a.ResolvedAt = &now
	}
	return nil
}

// only returns the single stored alert for realm.
func (m *memStore) only(t *testing.T, realm string) *domain.Alert {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	var found *domain.Alert
	for _, a := range m.alerts {
		if a.RealmName == realm {
			if found != nil {
				t.Fatalf("realm %s has more than one alert", realm)
			}
			cp := *a
			found = &cp
		}
	}
	if found == nil {
		t.Fatalf("realm %s has no alert", realm)
	}
	return found
}

// age moves an alert's last detection into the past, as time passing would.
func (m *memStore) age(realm string, by time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.alerts {
		if a.RealmName == realm {
			a.LastSeen = a.LastSeen.Add(-by)
		}
	}
}

func (m *memStore) setStatus(realm string, status domain.AlertStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.alerts {
		if a.RealmName == realm {
			a.Status = status
			if status == domain.AlertStatusAcknowled {
				now := time.Now()
				a.AcknowledgedAt = &now
			}
		}
	}
}

type countingNotifier struct {
	mu                   sync.Mutex
	raised, resolutions  int
	lastResolutionStatus domain.AlertStatus
}

func (n *countingNotifier) NotifyAlert(context.Context, *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.raised++
	return nil
}

func (n *countingNotifier) NotifyAlertResolution(_ context.Context, a *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.resolutions++
	n.lastResolutionStatus = a.Status
	return nil
}

func (n *countingNotifier) counts() (raised, resolutions int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.raised, n.resolutions
}

const testResolveAfter = 15 * time.Minute

type resolutionFixture struct {
	counts   *realmCounts
	store    *memStore
	notifier *countingNotifier
	svc      Service
}

func newResolutionFixture(realms ...string) *resolutionFixture {
	f := &resolutionFixture{
		counts:   &realmCounts{counts: map[string]int64{}, errs: map[string]error{}},
		store:    newMemStore(),
		notifier: &countingNotifier{},
	}
	check := NewLoginErrorCheck(f.counts, discardLogger{}, "t1", staticRealms(realms...), 1, 5*time.Minute)
	f.svc = NewService([]Check{check}, f.store, f.notifier, discardLogger{}, time.Hour, WithResolveAfter(testResolveAfter))
	return f
}

func (f *resolutionFixture) poll(t *testing.T) {
	t.Helper()
	if err := f.svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}
}

func TestResolution_AClearedConditionResolvesOnlyAfterTheQuietPeriod(t *testing.T) {
	f := newResolutionFixture("master")

	f.counts.set("master", 1)
	f.poll(t)
	if a := f.store.only(t, "master"); a.Status != domain.AlertStatusActive {
		t.Fatalf("status after detection = %s, want active", a.Status)
	}

	// The errors age out of the window, but the alert was seen moments ago.
	f.counts.set("master", 0)
	f.poll(t)
	if a := f.store.only(t, "master"); a.Status != domain.AlertStatusActive {
		t.Fatalf("status one quiet poll later = %s, want still active", a.Status)
	}

	f.store.age("master", testResolveAfter)
	f.poll(t)
	if a := f.store.only(t, "master"); a.Status != domain.AlertStatusResolved {
		t.Fatalf("status after the quiet period = %s, want resolved", a.Status)
	}
	raised, resolutions := f.notifier.counts()
	if raised != 1 || resolutions != 1 {
		t.Errorf("notifications: raised %d, resolutions %d; want 1 and 1", raised, resolutions)
	}

	// Already resolved: a further quiet poll does nothing more.
	f.poll(t)
	if _, resolutions := f.notifier.counts(); resolutions != 1 {
		t.Errorf("resolution notifications = %d after another quiet poll, want still 1", resolutions)
	}
}

func TestResolution_ARecurrenceAfterResolvingReopensAndNotifies(t *testing.T) {
	f := newResolutionFixture("master")
	f.counts.set("master", 1)
	f.poll(t)
	f.counts.set("master", 0)
	f.store.age("master", testResolveAfter)
	f.poll(t)

	f.counts.set("master", 1)
	f.poll(t)
	a := f.store.only(t, "master")
	if a.Status != domain.AlertStatusActive {
		t.Fatalf("status after the condition recurred = %s, want active again", a.Status)
	}
	if raised, _ := f.notifier.counts(); raised != 2 {
		t.Errorf("raise notifications = %d, want 2: the recurrence must be announced", raised)
	}
}

// An operator who resolves an alert while the burst is still going on has
// decided about this burst. It reopens only once the condition has actually
// cleared and come back.
func TestResolution_AnAlertResolvedByHandMidBurstStaysResolved(t *testing.T) {
	f := newResolutionFixture("master")
	f.counts.set("master", 1)
	f.poll(t)
	f.store.setStatus("master", domain.AlertStatusResolved)

	f.poll(t)
	if a := f.store.only(t, "master"); a.Status != domain.AlertStatusResolved {
		t.Fatalf("status one poll after a manual resolve, burst ongoing = %s, want still resolved", a.Status)
	}
	if raised, _ := f.notifier.counts(); raised != 1 {
		t.Errorf("raise notifications = %d, want 1: the ongoing burst must not reopen it", raised)
	}

	f.counts.set("master", 0)
	f.store.age("master", testResolveAfter)
	f.poll(t)
	f.counts.set("master", 1)
	f.poll(t)
	if a := f.store.only(t, "master"); a.Status != domain.AlertStatusActive {
		t.Errorf("status after the burst ended and a new one began = %s, want active", a.Status)
	}
}

func TestResolution_AnAcknowledgedAlertResolvesWhenCleared(t *testing.T) {
	f := newResolutionFixture("master")
	f.counts.set("master", 1)
	f.poll(t)
	f.store.setStatus("master", domain.AlertStatusAcknowled)

	f.counts.set("master", 0)
	f.store.age("master", testResolveAfter)
	f.poll(t)
	if a := f.store.only(t, "master"); a.Status != domain.AlertStatusResolved {
		t.Errorf("status = %s, want resolved", a.Status)
	}
}

func TestResolution_AnIgnoredAlertIsLeftAlone(t *testing.T) {
	f := newResolutionFixture("master")
	f.counts.set("master", 1)
	f.poll(t)
	f.store.setStatus("master", domain.AlertStatusIgnored)

	f.counts.set("master", 0)
	f.store.age("master", testResolveAfter)
	f.poll(t)
	if a := f.store.only(t, "master"); a.Status != domain.AlertStatusIgnored {
		t.Errorf("status = %s, want still ignored", a.Status)
	}
}

// A realm whose events could not be counted was not seen to clear: during an
// outage its alert must not resolve, while a realm that was read does.
func TestResolution_ARealmThatCouldNotBeReadIsNotResolved(t *testing.T) {
	f := newResolutionFixture("readable", "unreadable")
	f.counts.set("readable", 1)
	f.counts.set("unreadable", 1)
	f.poll(t)

	f.counts.set("readable", 0)
	f.counts.fail("unreadable", errors.New("database unavailable"))
	f.store.age("readable", testResolveAfter)
	f.store.age("unreadable", testResolveAfter)
	f.poll(t)

	if a := f.store.only(t, "readable"); a.Status != domain.AlertStatusResolved {
		t.Errorf("readable realm: status = %s, want resolved", a.Status)
	}
	if a := f.store.only(t, "unreadable"); a.Status != domain.AlertStatusActive {
		t.Errorf("unreadable realm: status = %s, want still active: it was not seen to clear", a.Status)
	}
}

// A check that does not evaluate keeps the old behaviour: nothing resolves.
func TestResolution_ANonEvaluatingCheckResolvesNothing(t *testing.T) {
	store := newMemStore()
	notifier := &countingNotifier{}
	old := time.Now().Add(-time.Hour)
	store.alerts["legacy"] = &domain.Alert{TenantID: "t1", AlertID: "legacy", CheckType: "legacy-check",
		RealmName: "master", Status: domain.AlertStatusActive, LastSeen: old}

	svc := NewService([]Check{legacyCheck{}}, store, notifier, discardLogger{}, time.Hour, WithResolveAfter(time.Minute))
	if err := svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}
	if a := store.only(t, "master"); a.Status != domain.AlertStatusActive {
		t.Errorf("status = %s, want still active", a.Status)
	}
}

type legacyCheck struct{}

func (legacyCheck) GetCheckType() string                             { return "legacy-check" }
func (legacyCheck) GetDescription() string                           { return "legacy" }
func (legacyCheck) Execute(context.Context) ([]*domain.Alert, error) { return nil, nil }

// typeCounts answers per event type, failing for types in errs.
type typeCounts struct {
	mu     sync.Mutex
	counts map[string]int64
	errs   map[string]error
}

func (r *typeCounts) CountEventsByType(_ context.Context, _, _, eventType string, _, _ time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.errs[eventType]; err != nil {
		return 0, err
	}
	return r.counts[eventType], nil
}

// A check that reads several event types per realm evaluates each separately:
// one type failing to count must not hold back, or resolve, another.
func TestResolution_EachEventTypeIsEvaluatedOnItsOwn(t *testing.T) {
	reader := &typeCounts{counts: map[string]int64{"IDENTITY_PROVIDER_LOGIN_ERROR": 1, "IDENTITY_PROVIDER_FIRST_LOGIN_ERROR": 1}, errs: map[string]error{}}
	store := newMemStore()
	check := NewIdpErrorCheck(reader, discardLogger{}, "t1", staticRealms("master"), 1, 5*time.Minute,
		[]string{"IDENTITY_PROVIDER_LOGIN_ERROR", "IDENTITY_PROVIDER_FIRST_LOGIN_ERROR"})
	svc := NewService([]Check{check}, store, &countingNotifier{}, discardLogger{}, time.Hour, WithResolveAfter(testResolveAfter))
	run := func() {
		if err := svc.RunCheckNow(context.Background()); err != nil {
			t.Fatalf("RunCheckNow: %v", err)
		}
	}
	run()
	if len(store.alerts) != 2 {
		t.Fatalf("stored %d alerts, want one per event type", len(store.alerts))
	}

	reader.mu.Lock()
	reader.counts["IDENTITY_PROVIDER_LOGIN_ERROR"] = 0
	reader.errs["IDENTITY_PROVIDER_FIRST_LOGIN_ERROR"] = errors.New("count failed")
	reader.mu.Unlock()
	store.age("master", testResolveAfter)
	run()

	cleared := store.alerts[check.generateAlertID("master", "IDENTITY_PROVIDER_LOGIN_ERROR")]
	unread := store.alerts[check.generateAlertID("master", "IDENTITY_PROVIDER_FIRST_LOGIN_ERROR")]
	if cleared == nil || unread == nil {
		t.Fatal("alerts are not keyed by the check's own alert IDs")
	}
	if cleared.Status != domain.AlertStatusResolved {
		t.Errorf("cleared event type: status = %s, want resolved", cleared.Status)
	}
	if unread.Status != domain.AlertStatusActive {
		t.Errorf("event type that failed to count: status = %s, want still active", unread.Status)
	}
}

// A manual run can overlap the poll loop. However many runs overlap, a
// cleared condition resolves once and is announced once, and a new one is
// announced once.
func TestResolution_OverlappingRunsNotifyOnce(t *testing.T) {
	for round := 0; round < 20; round++ {
		f := newResolutionFixture("master")

		f.counts.set("master", 1)
		overlap(t, f.svc, 10)
		if raised, _ := f.notifier.counts(); raised != 1 {
			t.Fatalf("round %d: %d raise notifications from overlapping runs, want 1", round, raised)
		}

		f.counts.set("master", 0)
		f.store.age("master", testResolveAfter)
		overlap(t, f.svc, 10)
		if _, resolutions := f.notifier.counts(); resolutions != 1 {
			t.Fatalf("round %d: %d resolution notifications from overlapping runs, want 1", round, resolutions)
		}
	}
}

func overlap(t *testing.T, svc Service, n int) {
	t.Helper()
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = svc.RunCheckNow(context.Background())
		}()
	}
	close(start)
	wg.Wait()
}
