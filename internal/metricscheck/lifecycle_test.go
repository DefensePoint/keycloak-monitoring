package metricscheck

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// noopAlertStore satisfies AlertStore; the lifecycle test raises no alerts.
type noopAlertStore struct{}

func (noopAlertStore) GetAlertByID(context.Context, string, string) (*domain.Alert, error) {
	return nil, nil
}
func (noopAlertStore) SaveAlert(context.Context, *domain.Alert) error { return nil }

// discardLogger satisfies Logger without producing output.
type discardLogger struct{}

func (discardLogger) Debug(string, ...interface{})  {}
func (discardLogger) Info(string, ...interface{})   {}
func (discardLogger) Warn(string, ...interface{})   {}
func (discardLogger) Error(string, ...interface{})  {}
func (d discardLogger) WithComponent(string) Logger { return d }

// gatedCheck blocks inside Execute until the test releases it, so a test can
// hold a run in flight deterministically rather than racing a sleep.
type gatedCheck struct {
	entered        chan struct{}
	release        chan struct{}
	once           sync.Once
	completedCalls atomic.Int64
}

func newGatedCheck() *gatedCheck {
	return &gatedCheck{entered: make(chan struct{}), release: make(chan struct{})}
}

func (c *gatedCheck) GetCheckType() string   { return "gated-check" }
func (c *gatedCheck) GetDescription() string { return "blocks until the test releases it" }

func (c *gatedCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	c.once.Do(func() { close(c.entered) })
	<-c.release
	c.completedCalls.Add(1)
	return nil, nil
}

// Stop must not return while the immediate first run launched by Start is
// still in flight.
//
// Start fires runChecks in its own goroutine and then starts the poller. Only
// the poller signalled completion, so Stop drained the ticker and returned
// while that first run was still executing checks and writing alerts.
//
// The check is gated on channels rather than a sleep so the assertion cannot
// go soft under load: the run is provably in flight before Stop is called, and
// provably still in flight while Stop is blocked.
func TestService_Stop_WaitsForInitialRun(t *testing.T) {
	check := newGatedCheck()
	// A poll interval far longer than the test: the only run in flight is the
	// immediate one Start launches.
	svc := NewService([]Checker{check}, noopAlertStore{}, nil, discardLogger{}, time.Hour)

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	select {
	case <-check.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the initial run never reached the check")
	}

	stopped := make(chan struct{})
	go func() {
		_ = svc.Stop()
		close(stopped)
	}()

	// Stop must still be blocked, because the check has not been released.
	select {
	case <-stopped:
		t.Fatal("Stop() returned while the initial run was still in flight")
	case <-time.After(100 * time.Millisecond):
	}

	close(check.release)

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return after the initial run finished")
	}

	if got := check.completedCalls.Load(); got != 1 {
		t.Errorf("completed %d checks, want 1", got)
	}
}

// Stop must be safe to call twice.
//
// Every service here used to close an unguarded stop channel, so a second Stop
// panicked and took the process down. fx calls OnStop once, so this never
// fired in production, but it is a sharp edge for anything that retries a
// shutdown. lifecycle.Workers closes the channel under a sync.Once.
func TestService_Stop_IsIdempotent(t *testing.T) {
	check := newGatedCheck()
	close(check.release) // nothing to hold open here; this test is about Stop itself
	svc := NewService([]Checker{check}, noopAlertStore{}, nil, discardLogger{}, time.Hour)

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	if err := svc.Stop(); err != nil {
		t.Fatalf("first Stop() returned error: %v", err)
	}
	// Panics here if the stop channel is closed a second time.
	if err := svc.Stop(); err != nil {
		t.Fatalf("second Stop() returned error: %v", err)
	}
}
