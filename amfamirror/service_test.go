package amfamirror

import (
	"context"
	"sync"
	"testing"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// ---------------------------------------------------------------------------
// fakes
// ---------------------------------------------------------------------------

// fakeAmfaRepo implements amfa.Repository. Only ListEventsSince is exercised by
// the mirror; the rest satisfy the interface.
type fakeAmfaRepo struct {
	mu          sync.Mutex
	rowsByRealm map[string][]amfa.EventRow
	calls       []string

	// inFlight/maxInFlight track how many cycles are inside the repo at once,
	// so a test can assert that cycles genuinely serialise rather than merely
	// avoiding a detected race.
	inFlight    int
	maxInFlight int
	// delay holds a cycle open long enough for another to collide with it.
	delay time.Duration
}

func (f *fakeAmfaRepo) ListEventsSince(_ context.Context, realm string, since time.Time, limit int) ([]amfa.EventRow, error) {
	f.mu.Lock()
	f.calls = append(f.calls, realm)
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	delay := f.delay
	f.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}

	f.mu.Lock()
	defer func() {
		f.inFlight--
		f.mu.Unlock()
	}()

	var out []amfa.EventRow
	for _, row := range f.rowsByRealm[realm] {
		if row.EventTime.After(since) {
			out = append(out, row)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeAmfaRepo) sinceCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeAmfaRepo) peakConcurrency() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInFlight
}

func (f *fakeAmfaRepo) ListEvents(context.Context, amfa.ListEventsOptions) (*amfa.ListEventsResult, error) {
	return nil, nil
}
func (f *fakeAmfaRepo) GetStats(context.Context, amfa.StatsOptions) (amfa.Stats, error) {
	return amfa.Stats{}, nil
}
func (f *fakeAmfaRepo) GetGeoBuckets(context.Context, amfa.GeoOptions) ([]amfa.GeoBucket, error) {
	return nil, nil
}
func (f *fakeAmfaRepo) ListRejectedEventsSince(context.Context, string, time.Time) ([]amfa.EventRow, error) {
	return nil, nil
}
func (f *fakeAmfaRepo) ListVPNRiskyEventsSince(context.Context, string, time.Time, int) ([]amfa.EventRow, error) {
	return nil, nil
}
func (f *fakeAmfaRepo) CountRepeatedRiskyByUser(context.Context, string, time.Time, int, int) ([]amfa.UserRiskyCount, error) {
	return nil, nil
}
func (f *fakeAmfaRepo) CountByEventTypeInWindow(context.Context, string, string, time.Time, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeAmfaRepo) CountByEventTypeAndClientInWindow(context.Context, string, string, time.Time, time.Time, int) ([]amfa.ClientEventCount, error) {
	return nil, nil
}
func (f *fakeAmfaRepo) CountDistinctRejectedUsersSince(context.Context, string, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeAmfaRepo) CountByEventTypeByUser(context.Context, string, string, time.Time, int) ([]amfa.UserRiskyCount, error) {
	return nil, nil
}

// fakeStore implements EventStore, recording the positions it is asked to
// persist. Keyed by tenant and realm so a test can assert isolation.
type fakeStore struct {
	mu       sync.Mutex
	saved    map[string]time.Time // "tenant|realm" -> position
	preset   map[string]time.Time // positions that already exist on "disk"
	merged   []*domain.Event
	saves    int
	reconcil int
}

func newFakeStore() *fakeStore {
	return &fakeStore{saved: map[string]time.Time{}, preset: map[string]time.Time{}}
}

func wmKey(tenantID, realm string) string { return tenantID + "|" + realm }

func (f *fakeStore) MergeAmfaEvent(_ context.Context, event *domain.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.merged = append(f.merged, event)
	return nil
}

func (f *fakeStore) AmfaMirrorWatermark(_ context.Context, tenantID, realm string) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if wm, ok := f.saved[wmKey(tenantID, realm)]; ok {
		return wm, nil
	}
	return f.preset[wmKey(tenantID, realm)], nil
}

func (f *fakeStore) SaveAmfaMirrorWatermark(_ context.Context, tenantID, realm string, eventTime time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saves++
	// Monotonic, mirroring the real implementation.
	if existing, ok := f.saved[wmKey(tenantID, realm)]; ok && !eventTime.After(existing) {
		return nil
	}
	f.saved[wmKey(tenantID, realm)] = eventTime
	return nil
}

func (f *fakeStore) savedFor(tenantID, realm string) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	wm, ok := f.saved[wmKey(tenantID, realm)]
	return wm, ok
}

func (f *fakeStore) saveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saves
}

func (f *fakeStore) ReconcileAMFAMerges(context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reconcil++
	return 0, nil
}

func (f *fakeStore) mergedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.merged)
}

// discardLogger satisfies Logger without emitting anything.
type discardLogger struct{}

func (discardLogger) Info(string, ...any)           {}
func (discardLogger) Error(string, ...any)          {}
func (discardLogger) Warn(string, ...any)           {}
func (discardLogger) Debug(string, ...any)          {}
func (d discardLogger) WithComponent(string) Logger { return d }

func realmsFor(names ...string) RealmsFunc {
	return func(context.Context) ([]string, error) { return names, nil }
}

func eventRows(n int) []amfa.EventRow {
	base := time.Now().UTC().Add(-time.Hour)
	rows := make([]amfa.EventRow, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, amfa.EventRow{
			EventID:   string(rune('a' + i%26)),
			EventTime: base.Add(time.Duration(i) * time.Second),
			EventType: "LOGIN",
		})
	}
	return rows
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

// Cycles must not overlap.
//
// Start launches an immediate cycle in its own goroutine while poll tickers
// further ones, so a first backfill that outlives one poll interval puts two
// cycles in flight together. Both then touch the unsynchronised watermarks
// map, and a concurrent map write in Go is a fatal panic that takes the whole
// process down rather than returning an error.
//
// Run under `go test -race` to catch the map access, and note the peak-depth
// assertion below: passing the race detector alone would not catch two cycles
// duplicating every AMFA read.
func TestMirror_ConcurrentCyclesSerialise(t *testing.T) {
	store := newFakeStore()
	repo := &fakeAmfaRepo{
		rowsByRealm: map[string][]amfa.EventRow{
			"master": eventRows(50),
			"other":  eventRows(50),
		},
		// Hold each cycle open so the others genuinely collide rather than
		// completing one after another by luck.
		delay: 20 * time.Millisecond,
	}

	svc := NewService("tenantA", realmsFor("master", "other"), repo, store, nil,
		discardLogger{}, time.Hour, 24*time.Hour)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.RunOnce(context.Background())
		}()
	}
	wg.Wait()

	if peak := repo.peakConcurrency(); peak != 1 {
		t.Errorf("cycles overlapped: peak concurrency inside the AMFA repo was %d, want 1", peak)
	}
	if repo.sinceCallCount() == 0 {
		t.Error("expected the fake AMFA repo to have been queried at least once")
	}
	if store.mergedCount() == 0 {
		t.Error("expected the cycle that ran to have mirrored events")
	}
}

// A cycle that runs while another is in flight returns nil rather than an
// error: the in-flight cycle is already doing the work, so a skip is a
// non-event for the caller.
func TestMirror_SkippedCycleReturnsNil(t *testing.T) {
	store := newFakeStore()
	release := make(chan struct{})
	repo := &fakeAmfaRepo{rowsByRealm: map[string][]amfa.EventRow{"master": eventRows(5)}}

	svc := NewService("tenantA", realmsFor("master"), repo, store, nil,
		discardLogger{}, time.Hour, 24*time.Hour)

	// Hold one cycle open, then confirm a second returns nil promptly.
	repo.mu.Lock()
	repo.delay = 150 * time.Millisecond
	repo.mu.Unlock()

	go func() {
		_ = svc.RunOnce(context.Background())
		close(release)
	}()

	// Give the first cycle time to take the lock.
	time.Sleep(30 * time.Millisecond)

	start := time.Now()
	if err := svc.RunOnce(context.Background()); err != nil {
		t.Errorf("a skipped cycle should return nil, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("a skipped cycle should return promptly rather than blocking; took %s", elapsed)
	}
	<-release
}

// Sequential cycles still run: the guard must not wedge the mirror shut after
// the first cycle releases it.
func TestMirror_SequentialCyclesStillRun(t *testing.T) {
	store := newFakeStore()
	repo := &fakeAmfaRepo{rowsByRealm: map[string][]amfa.EventRow{"master": eventRows(3)}}

	svc := NewService("tenantA", realmsFor("master"), repo, store, nil,
		discardLogger{}, time.Hour, 24*time.Hour)

	for i := 0; i < 3; i++ {
		if err := svc.RunOnce(context.Background()); err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
	}
	if got := repo.sinceCallCount(); got < 3 {
		t.Errorf("expected at least one AMFA query per sequential cycle, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// per-tenant read position
// ---------------------------------------------------------------------------

// A cold start with nothing persisted falls back to the backfill window, not
// to the epoch and not to another tenant's progress.
func TestMirror_ColdStartUsesBackfillWindow(t *testing.T) {
	store := newFakeStore()
	repo := &fakeAmfaRepo{rowsByRealm: map[string][]amfa.EventRow{}}
	backfill := 24 * time.Hour

	svc := NewService("tenantA", realmsFor("master"), repo, store, nil,
		discardLogger{}, time.Hour, backfill).(*service)

	before := time.Now().Add(-backfill)
	wm, err := svc.watermarkFor(context.Background(), "master")
	if err != nil {
		t.Fatalf("watermarkFor: %v", err)
	}

	if wm.Before(before.Add(-time.Minute)) || wm.After(time.Now().Add(-backfill).Add(time.Minute)) {
		t.Errorf("cold start should resume near now-backfill (%s), got %s",
			before.Format(time.RFC3339), wm.Format(time.RFC3339))
	}
}

// A restart with a persisted position resumes from it rather than re-reading
// the whole backfill window.
func TestMirror_ResumesFromPersistedPosition(t *testing.T) {
	store := newFakeStore()
	persisted := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
	store.preset[wmKey("tenantA", "master")] = persisted

	repo := &fakeAmfaRepo{rowsByRealm: map[string][]amfa.EventRow{}}
	svc := NewService("tenantA", realmsFor("master"), repo, store, nil,
		discardLogger{}, time.Hour, 24*time.Hour).(*service)

	wm, err := svc.watermarkFor(context.Background(), "master")
	if err != nil {
		t.Fatalf("watermarkFor: %v", err)
	}
	if !wm.Equal(persisted) {
		t.Errorf("expected resume from persisted %s, got %s",
			persisted.Format(time.RFC3339), wm.Format(time.RFC3339))
	}
}

// The bug this table exists to fix. Realm names are unique per-tenant, not
// globally, and every Keycloak has a "master" realm, so two tenants monitoring
// a realm of the same name must not share a position.
func TestMirror_TwoTenantsSharingRealmNameKeepSeparatePositions(t *testing.T) {
	store := newFakeStore()
	now := time.Now().UTC().Truncate(time.Second)

	// Tenant B is far ahead on its own "master". Tenant A has never run.
	store.preset[wmKey("tenantB", "master")] = now

	repo := &fakeAmfaRepo{rowsByRealm: map[string][]amfa.EventRow{}}
	backfill := 24 * time.Hour

	svcA := NewService("tenantA", realmsFor("master"), repo, store, nil,
		discardLogger{}, time.Hour, backfill).(*service)
	svcB := NewService("tenantB", realmsFor("master"), repo, store, nil,
		discardLogger{}, time.Hour, backfill).(*service)

	wmA, err := svcA.watermarkFor(context.Background(), "master")
	if err != nil {
		t.Fatalf("tenant A: %v", err)
	}
	wmB, err := svcB.watermarkFor(context.Background(), "master")
	if err != nil {
		t.Fatalf("tenant B: %v", err)
	}

	if wmA.Equal(now) {
		t.Errorf("tenant A inherited tenant B's position %s and would skip its own older events",
			now.Format(time.RFC3339))
	}
	if !wmB.Equal(now) {
		t.Errorf("tenant B should resume from %s, got %s",
			now.Format(time.RFC3339), wmB.Format(time.RFC3339))
	}
}

// An idle cycle finds nothing new, so it must not write, or every realm takes a
// write on every poll forever.
func TestMirror_IdleCycleDoesNotPersist(t *testing.T) {
	store := newFakeStore()
	repo := &fakeAmfaRepo{rowsByRealm: map[string][]amfa.EventRow{}}

	svc := NewService("tenantA", realmsFor("master"), repo, store, nil,
		discardLogger{}, time.Hour, 24*time.Hour)

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if got := store.saveCount(); got != 0 {
		t.Errorf("idle cycle wrote the position %d time(s); expected none", got)
	}
}

// A cycle that mirrors new events persists the advanced position, so a restart
// does not re-read them.
func TestMirror_AdvancePersistsPosition(t *testing.T) {
	store := newFakeStore()
	rows := eventRows(3)
	newest := rows[len(rows)-1].EventTime
	repo := &fakeAmfaRepo{rowsByRealm: map[string][]amfa.EventRow{"master": rows}}

	svc := NewService("tenantA", realmsFor("master"), repo, store, nil,
		discardLogger{}, time.Hour, 24*time.Hour)

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	got, ok := store.savedFor("tenantA", "master")
	if !ok {
		t.Fatal("expected the advanced position to be persisted")
	}
	if !got.Equal(newest) {
		t.Errorf("persisted position: want %s, got %s",
			newest.Format(time.RFC3339), got.Format(time.RFC3339))
	}
}
