package amfacheck

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// pausingAlertStore pauses the first run that looks an alert up, right after
// it has seen the alert as new and before it saves it, until release is
// closed. Any other lookup goes straight through.
type pausingAlertStore struct {
	*fakeAlertStore
	once    sync.Once
	paused  chan struct{}
	release chan struct{}
}

func newPausingAlertStore() *pausingAlertStore {
	return &pausingAlertStore{fakeAlertStore: newFakeAlertStore(),
		paused: make(chan struct{}), release: make(chan struct{})}
}

func (s *pausingAlertStore) GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	a, err := s.fakeAlertStore.GetAlertByID(ctx, tenantID, alertID)
	first := false
	s.once.Do(func() { first = true })
	if first {
		close(s.paused)
		<-s.release
	}
	return a, err
}

// A tenant save replaces its checker while a manual run ("Reload AMFA alerts")
// may still be running on the old one, and the new checker runs at once. With a
// lock per checker the two runs overlap, each sees the alert as new, and each
// announces it. Checkers that share the tenant's run lock take turns.
func TestService_CheckersSharingARunLockTakeTurns(t *testing.T) {
	store := newPausingAlertStore()
	notifier := newFakeNotifier()
	var tenantLock sync.Mutex
	newChecker := func() Service {
		chk := &fakeCheck{checkType: "amfa-test", alerts: []*domain.Alert{newAlert("a1")}}
		return NewService("tenant-1", []Check{chk}, store, notifier, testLogger{}, time.Minute,
			WithRunLock(&tenantLock))
	}
	oldChecker, newCheckerAfterSave := newChecker(), newChecker()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = oldChecker.RunCheckNow(context.Background()) }()
	<-store.paused // the manual run has seen a1 as new and not saved it yet
	go func() { defer wg.Done(); _ = newCheckerAfterSave.RunCheckNow(context.Background()) }()
	time.Sleep(100 * time.Millisecond) // room for the new checker's run to overlap, if it can
	close(store.release)
	wg.Wait()

	if got := notifier.notifs("a1"); got != 1 {
		t.Errorf("a1 announced %d times, want 1", got)
	}
}
