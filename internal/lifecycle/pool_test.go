package lifecycle

import (
	"context"
	"sync"
	"testing"
)

// fakePoller records its Start and Stop calls and whether its run context was
// cancelled by the time it was stopped.
type fakePoller struct {
	mu              sync.Mutex
	starts, stops   int
	ctx             context.Context
	cancelledAtStop bool
}

func (f *fakePoller) Start(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	f.ctx = ctx
	return nil
}

func (f *fakePoller) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	f.cancelledAtStop = f.ctx != nil && f.ctx.Err() != nil
	return nil
}

func (f *fakePoller) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts, f.stops
}

func wantCounts(t *testing.T, name string, f *fakePoller, starts, stops int) {
	t.Helper()
	if s, st := f.counts(); s != starts || st != stops {
		t.Errorf("%s: starts=%d stops=%d, want %d and %d", name, s, st, starts, stops)
	}
}

// A poller added before Start waits for it, as fx starts the pool only once
// the app starts.
func TestPool_PutBeforeStartWaitsForStart(t *testing.T) {
	var p Pool[*fakePoller]
	a := &fakePoller{}
	p.Put("a", a)
	wantCounts(t, "before Start", a, 0, 0)

	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	wantCounts(t, "after Start", a, 1, 0)
	if a.ctx.Err() != nil {
		t.Error("run context already cancelled after Start")
	}
}

func TestPool_PutWhileRunningStartsAtOnce(t *testing.T) {
	var p Pool[*fakePoller]
	_ = p.Start()
	a := &fakePoller{}
	p.Put("a", a)
	wantCounts(t, "a", a, 1, 0)
}

// Replacing a tenant's poller stops the old one before the new one starts, so
// a tenant never has two.
func TestPool_PutReplacesAndStopsTheOldPoller(t *testing.T) {
	var p Pool[*fakePoller]
	_ = p.Start()
	old, replacement := &fakePoller{}, &fakePoller{}
	p.Put("a", old)
	p.Put("a", replacement)

	wantCounts(t, "old", old, 1, 1)
	wantCounts(t, "replacement", replacement, 1, 0)
	if !old.cancelledAtStop {
		t.Error("old poller's run context was not cancelled before Stop")
	}
	if p.Len() != 1 {
		t.Errorf("Len = %d, want 1", p.Len())
	}
}

func TestPool_RemoveStopsThePoller(t *testing.T) {
	var p Pool[*fakePoller]
	_ = p.Start()
	a := &fakePoller{}
	p.Put("a", a)
	p.Remove("a")

	wantCounts(t, "a", a, 1, 1)
	if !a.cancelledAtStop {
		t.Error("run context was not cancelled before Stop")
	}
	if p.Len() != 0 {
		t.Errorf("Len = %d, want 0", p.Len())
	}
	p.Remove("a") // removing again is a no-op
	wantCounts(t, "a after second Remove", a, 1, 1)
}

// A poller removed before Start was never started, so it is not stopped either.
func TestPool_RemoveBeforeStartNeverStarts(t *testing.T) {
	var p Pool[*fakePoller]
	a := &fakePoller{}
	p.Put("a", a)
	p.Remove("a")
	_ = p.Start()
	wantCounts(t, "a", a, 0, 0)
}

func TestPool_StopStopsEveryPoller(t *testing.T) {
	var p Pool[*fakePoller]
	a, b := &fakePoller{}, &fakePoller{}
	p.Put("a", a)
	_ = p.Start()
	p.Put("b", b)

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	wantCounts(t, "a", a, 1, 1)
	wantCounts(t, "b", b, 1, 1)
	if !a.cancelledAtStop || !b.cancelledAtStop {
		t.Error("run contexts were not cancelled before Stop")
	}
}

// A change that arrives during or after shutdown must not start anything.
func TestPool_PutAfterStopIsIgnored(t *testing.T) {
	var p Pool[*fakePoller]
	_ = p.Start()
	_ = p.Stop()
	late := &fakePoller{}
	p.Put("a", late)
	wantCounts(t, "late", late, 0, 0)
	if p.Len() != 0 {
		t.Errorf("Len = %d, want 0", p.Len())
	}
}

func TestPool_ConcurrentChanges(t *testing.T) {
	var p Pool[*fakePoller]
	_ = p.Start()
	var all []*fakePoller
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			f := &fakePoller{}
			mu.Lock()
			all = append(all, f)
			mu.Unlock()
			p.Put("a", f)
		}()
		go func() {
			defer wg.Done()
			p.Remove("a")
		}()
	}
	wg.Wait()
	_ = p.Stop()

	for i, f := range all {
		if s, st := f.counts(); s != st {
			t.Errorf("poller %d: starts=%d stops=%d, every started poller must be stopped", i, s, st)
		}
	}
}
