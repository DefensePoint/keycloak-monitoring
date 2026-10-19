package amfacheck

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

const testResolveAfter = 15 * time.Minute

type amfaResolutionFixture struct {
	repo     *checkRepoStub
	store    *fakeAlertStore
	notifier *fakeNotifier
	svc      Service
	alertID  string
}

// newAMFAResolutionFixture runs the real rule 4a check, login errors at
// threshold 1, through the service.
func newAMFAResolutionFixture() *amfaResolutionFixture {
	f := &amfaResolutionFixture{repo: &checkRepoStub{}, store: newFakeAlertStore(), notifier: newFakeNotifier()}
	realms := func(context.Context) ([]string, error) { return []string{"master"}, nil }
	check := NewLoginErrorCheck("t1", realms, f.repo, 1, 5*time.Minute, testLogger{})
	f.svc = NewService("t1", []Check{check}, f.store, f.notifier, testLogger{}, time.Hour, WithResolveAfter(testResolveAfter))
	return f
}

func (f *amfaResolutionFixture) poll(t *testing.T) {
	t.Helper()
	if err := f.svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}
}

// only returns the single stored alert.
func (f *amfaResolutionFixture) only(t *testing.T) *domain.Alert {
	t.Helper()
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	if len(f.store.existing) != 1 {
		t.Fatalf("store holds %d alerts, want 1", len(f.store.existing))
	}
	for id, a := range f.store.existing {
		f.alertID = id
		cp := *a
		return &cp
	}
	return nil
}

func (f *amfaResolutionFixture) age(by time.Duration) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	for _, a := range f.store.existing {
		a.LastSeen = a.LastSeen.Add(-by)
	}
}

func TestAMFAResolution_AClearedBurstResolvesAfterTheQuietPeriod(t *testing.T) {
	f := newAMFAResolutionFixture()
	f.repo.typeCount = 1
	f.poll(t)
	if a := f.only(t); a.Status != domain.AlertStatusActive {
		t.Fatalf("status after detection = %s, want active", a.Status)
	}

	f.repo.typeCount = 0
	f.poll(t)
	if a := f.only(t); a.Status != domain.AlertStatusActive {
		t.Fatalf("status one quiet poll later = %s, want still active", a.Status)
	}

	f.age(testResolveAfter)
	f.poll(t)
	if a := f.only(t); a.Status != domain.AlertStatusResolved {
		t.Fatalf("status after the quiet period = %s, want resolved", a.Status)
	}
	if n := f.notifier.resolutions(f.alertID); n != 1 {
		t.Errorf("resolution notifications = %d, want 1", n)
	}
}

// The case resolution must not get wrong: AMFA unreachable is not AMFA quiet.
func TestAMFAResolution_AnOutageResolvesNothing(t *testing.T) {
	f := newAMFAResolutionFixture()
	f.repo.typeCount = 1
	f.poll(t)

	f.repo.typeCountErr = errors.New("amfa unreachable")
	f.age(time.Hour)
	if err := f.svc.RunCheckNow(context.Background()); err == nil {
		t.Fatal("a run during the outage reported no error")
	}
	if a := f.only(t); a.Status != domain.AlertStatusActive {
		t.Errorf("status during an outage = %s, want still active", a.Status)
	}
	if n := f.notifier.resolutions(f.alertID); n != 0 {
		t.Errorf("resolution notifications during an outage = %d, want 0", n)
	}
}

func TestAMFAResolution_ARecurrenceReopensAndNotifies(t *testing.T) {
	f := newAMFAResolutionFixture()
	f.repo.typeCount = 1
	f.poll(t)
	f.repo.typeCount = 0
	f.age(testResolveAfter)
	f.poll(t)

	f.repo.typeCount = 1
	f.poll(t)
	if a := f.only(t); a.Status != domain.AlertStatusActive {
		t.Fatalf("status after the burst recurred = %s, want active", a.Status)
	}
	if n := f.notifier.notifs(f.alertID); n != 2 {
		t.Errorf("raise notifications = %d, want 2", n)
	}
}

// A per-event alert records one security event someone should review, so it
// does not resolve when the event leaves the lookback window.
func TestAMFAResolution_APerEventAlertDoesNotResolve(t *testing.T) {
	store := newFakeAlertStore()
	store.existing["evt"] = &domain.Alert{TenantID: "t1", AlertID: "evt", CheckType: "amfa-risk-rejected",
		RealmName: "master", Status: domain.AlertStatusActive, LastSeen: time.Now().Add(-24 * time.Hour)}
	realms := func(context.Context) ([]string, error) { return []string{"master"}, nil }
	check := NewRiskRejectedCheck("t1", realms, &checkRepoStub{}, time.Hour, testLogger{})
	svc := NewService("t1", []Check{check}, store, newFakeNotifier(), testLogger{}, time.Hour, WithResolveAfter(time.Minute))

	if err := svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}
	if got := store.existing["evt"].Status; got != domain.AlertStatusActive {
		t.Errorf("per-event alert status = %s, want still active", got)
	}
}

// As in eventscheck: overlapping runs, a manual one over the poll loop,
// announce a raise and a resolution once each.
func TestAMFAResolution_OverlappingRunsNotifyOnce(t *testing.T) {
	for round := 0; round < 20; round++ {
		f := newAMFAResolutionFixture()
		f.repo.typeCount = 1
		overlapRuns(f.svc, 10)
		a := f.only(t)
		if n := f.notifier.notifs(a.AlertID); n != 1 {
			t.Fatalf("round %d: %d raise notifications from overlapping runs, want 1", round, n)
		}

		f.repo.typeCount = 0
		f.age(testResolveAfter)
		overlapRuns(f.svc, 10)
		if n := f.notifier.resolutions(a.AlertID); n != 1 {
			t.Fatalf("round %d: %d resolution notifications from overlapping runs, want 1", round, n)
		}
	}
}

func overlapRuns(svc Service, n int) {
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
