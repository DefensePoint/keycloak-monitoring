package lifecycle

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The zero value must work without a constructor, since the services embed
// Workers as a plain field.
func TestWorkers_ZeroValueIsUsable(t *testing.T) {
	var w Workers

	var ran atomic.Bool
	w.Go(func() { ran.Store(true) })

	if !w.Drain(time.Second) {
		t.Fatal("Drain() timed out draining a trivial worker")
	}
	if !ran.Load() {
		t.Error("worker never ran")
	}
}

// The bug this type exists to prevent: Drain must wait for every worker, not
// just the last one started.
func TestWorkers_DrainWaitsForAllWorkers(t *testing.T) {
	var w Workers

	var completed atomic.Int64
	for i := 0; i < 4; i++ {
		w.Go(func() {
			time.Sleep(100 * time.Millisecond)
			completed.Add(1)
		})
	}

	if !w.Drain(5 * time.Second) {
		t.Fatal("Drain() timed out")
	}
	if got := completed.Load(); got != 4 {
		t.Errorf("Drain() returned with %d of 4 workers finished", got)
	}
}

// A worker that outlives the timeout is abandoned, and Drain says so rather
// than blocking forever.
func TestWorkers_DrainReportsTimeout(t *testing.T) {
	var w Workers

	release := make(chan struct{})
	w.Go(func() { <-release })
	defer close(release)

	start := time.Now()
	if w.Drain(50 * time.Millisecond) {
		t.Error("Drain() reported success while a worker was still blocked")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Drain() took %s, want it to give up at the timeout", elapsed)
	}
}

// Workers leave their loops on Stopping.
func TestWorkers_StoppingUnblocksWorkers(t *testing.T) {
	var w Workers

	w.Go(func() {
		<-w.Stopping()
	})

	if !w.Drain(time.Second) {
		t.Error("Drain() timed out: the worker did not observe Stopping()")
	}
}

// Stopping must hand out the same channel every time, including before any
// worker has started and after the lazy init has run.
func TestWorkers_StoppingIsStable(t *testing.T) {
	var w Workers

	first := w.Stopping()
	w.Go(func() {})
	if second := w.Stopping(); first != second {
		t.Error("Stopping() returned a different channel across calls")
	}
}

// A second Stop must not panic. Every hand-rolled version this replaced closed
// its stop channel unguarded, so a double Stop took the process down.
func TestWorkers_DrainIsIdempotent(t *testing.T) {
	var w Workers
	w.Go(func() {})

	if !w.Drain(time.Second) {
		t.Fatal("first Drain() timed out")
	}
	// Panics here if the stop channel is closed twice.
	if !w.Drain(time.Second) {
		t.Error("second Drain() timed out")
	}
}

// Draining a service that was never started is a no-op, not a hang.
func TestWorkers_DrainWithNoWorkers(t *testing.T) {
	var w Workers

	start := time.Now()
	if !w.Drain(5 * time.Second) {
		t.Error("Drain() reported a timeout with no workers to wait for")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Drain() blocked for %s with no workers", elapsed)
	}
}

// Go and Drain racing must not trip the race detector or lose a worker.
// Run under -race.
func TestWorkers_ConcurrentGoAndDrain(t *testing.T) {
	var w Workers

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Go(func() { <-w.Stopping() })
		}()
	}
	wg.Wait()

	if !w.Drain(5 * time.Second) {
		t.Error("Drain() timed out draining concurrently started workers")
	}
}
