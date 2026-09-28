// Package lifecycle provides the shared start/stop plumbing for the
// platform's long-running poller services.
package lifecycle

import (
	"sync"
	"time"
)

// Workers tracks the background goroutines a service starts so that its
// shutdown can wait for every one of them.
//
// It exists because six services each hand-rolled this and five of them got it
// wrong the same way: they signalled completion from the polling goroutine
// alone, so a service that also kicked off an immediate first run on startup
// returned from Stop while that run was still writing to the database. Go is
// the only way to start a worker here, which is what makes an untracked one
// impossible rather than merely discouraged.
//
// The zero value is ready to use. A Workers must not be copied after first
// use; go vet's copylocks check enforces this.
type Workers struct {
	initOnce sync.Once
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// init lazily builds the stop channel so the zero value works and callers do
// not have to remember a constructor.
func (w *Workers) init() {
	w.initOnce.Do(func() {
		w.stop = make(chan struct{})
	})
}

// Go starts fn as a tracked worker. Drain will not return until fn does.
func (w *Workers) Go(fn func()) {
	w.init()
	// Add before the goroutine starts, not inside it: a Drain racing this call
	// must never observe a WaitGroup that is briefly back at zero.
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		fn()
	}()
}

// Stopping returns a channel closed when shutdown begins. Workers select on it
// to leave their loops.
func (w *Workers) Stopping() <-chan struct{} {
	w.init()
	return w.stop
}

// Drain signals shutdown and waits up to timeout for every worker started by
// Go to return. It reports whether they all finished; false means the timeout
// expired and at least one worker was abandoned, which is what the callers log
// as a stop timeout.
//
// Safe to call more than once: closing an already-closed channel is a panic,
// and every one of the six services this replaced would have crashed on a
// second Stop.
func (w *Workers) Drain(timeout time.Duration) bool {
	w.init()
	w.stopOnce.Do(func() {
		close(w.stop)
	})

	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
