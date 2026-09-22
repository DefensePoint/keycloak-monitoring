package mcp

import (
	"sync"
	"time"
)

// fakeClock is a clock the test moves by hand. The tools read it from the
// server's request goroutines while the test advances it between calls, so
// every access is mutex-guarded.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

// Now satisfies Clock.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward, standing in for time the client spent
// between two pages without making the test actually wait.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
