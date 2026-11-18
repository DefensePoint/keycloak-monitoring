package fx

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
)

// pausingRejectedRepo pauses the first run that asks it for rejected events
// until release is closed, and counts every ask.
type pausingRejectedRepo struct {
	acRepoStub
	calls   *atomic.Int32
	once    *sync.Once
	paused  chan struct{}
	release chan struct{}
}

func (r pausingRejectedRepo) ListRejectedEventsSince(context.Context, string, time.Time) ([]amfa.EventRow, error) {
	r.calls.Add(1)
	r.once.Do(func() {
		close(r.paused)
		<-r.release
	})
	return nil, nil
}

// Every checker built for a tenant takes turns with the tenant's earlier ones:
// a save replaces the checker while a manual run may still be going on the old
// one. Checkers of different tenants do not wait for each other.
func TestAmfaCheckBuilder_ChecksOfOneTenantTakeTurnsAcrossRebuilds(t *testing.T) {
	cfg := checkerConfig(true)
	build := amfaCheckBuilder(cfg, acRealmRepoStub{}, acFakeAlertStore{}, acFakeNotifier{}, nil, nil)
	repo := pausingRejectedRepo{calls: &atomic.Int32{}, once: &sync.Once{},
		paused: make(chan struct{}), release: make(chan struct{})}

	oldChecker := build("t1", repo)
	newChecker := build("t1", repo)
	otherTenant := build("t2", repo)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); _ = oldChecker.RunCheckNow(context.Background()) }()
	<-repo.paused // the old checker's manual run is mid-way
	go func() { defer wg.Done(); _ = newChecker.RunCheckNow(context.Background()) }()
	go func() { defer wg.Done(); _ = otherTenant.RunCheckNow(context.Background()) }()

	deadline := time.Now().Add(2 * time.Second)
	for repo.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // room for t1's new checker to get in, if it can
	got := repo.calls.Load()
	close(repo.release)
	wg.Wait()

	if got != 2 {
		t.Errorf("rejected-event reads while t1's old run was paused = %d, want 2 (that run and t2's)", got)
	}
}
