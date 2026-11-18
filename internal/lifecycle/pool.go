package lifecycle

import (
	"context"
	"errors"
	"sync"
)

// Poller is a per-tenant background service: it polls until Stop.
type Poller interface {
	Start(ctx context.Context) error
	Stop() error
}

// Pool runs one Poller per tenant and follows tenant changes: Put starts a
// tenant's poller, or replaces the one it had, and Remove stops it.
//
// Each poller runs on its own context, cancelled just before its Stop, never
// on the context of the caller that started it: fx cancels its OnStart
// context once startup completes, which would stop the poll loop moments
// after boot.
//
// A poller put before Start waits for Start, since fx only starts services
// once the whole app has started. After Stop, Put is ignored, so a change that
// arrives during shutdown starts nothing.
//
// The zero value is ready to use. A Pool must not be copied after first use.
type Pool[P Poller] struct {
	mu      sync.Mutex
	started bool
	stopped bool
	pollers map[string]*pooled[P]
}

type pooled[P Poller] struct {
	poller P
	// cancel is nil until the poller is started.
	cancel context.CancelFunc
}

// Put makes p the tenant's poller, stopping the one it replaces first, so a
// tenant never has two running. It returns the error of starting p.
func (pl *Pool[P]) Put(tenantID string, p P) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.stopped {
		return nil
	}
	if pl.pollers == nil {
		pl.pollers = make(map[string]*pooled[P])
	}
	if old, ok := pl.pollers[tenantID]; ok {
		delete(pl.pollers, tenantID)
		old.stop()
	}
	entry := &pooled[P]{poller: p}
	pl.pollers[tenantID] = entry
	if pl.started {
		return entry.start()
	}
	return nil
}

// Remove stops the tenant's poller, if it has one.
func (pl *Pool[P]) Remove(tenantID string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if entry, ok := pl.pollers[tenantID]; ok {
		delete(pl.pollers, tenantID)
		entry.stop()
	}
}

// Len reports how many tenants have a poller.
func (pl *Pool[P]) Len() int {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	return len(pl.pollers)
}

// Start starts every poller put so far, and every one put from now on.
func (pl *Pool[P]) Start() error {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.started || pl.stopped {
		return nil
	}
	pl.started = true
	var errs []error
	for _, entry := range pl.pollers {
		errs = append(errs, entry.start())
	}
	return errors.Join(errs...)
}

// Stop stops every poller, all at once so one slow drain does not hold up the
// others, and ignores any Put from now on.
func (pl *Pool[P]) Stop() error {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.stopped = true
	var wg sync.WaitGroup
	errs := make([]error, 0, len(pl.pollers))
	var errMu sync.Mutex
	for tenantID, entry := range pl.pollers {
		delete(pl.pollers, tenantID)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := entry.stop(); err != nil {
				errMu.Lock()
				errs = append(errs, err)
				errMu.Unlock()
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (e *pooled[P]) start() error {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	return e.poller.Start(ctx)
}

// stop cancels the run context so in-flight work unwinds, then lets Stop
// drain it. A poller that was never started is left alone.
func (e *pooled[P]) stop() error {
	if e.cancel == nil {
		return nil
	}
	e.cancel()
	return e.poller.Stop()
}
