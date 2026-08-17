package amfacheck

import (
	"context"
	"errors"
	"testing"
)

// runnerFakeService is a minimal Service for Runner tests; only RunCheckNow is exercised.
type runnerFakeService struct {
	calls int
	err   error
}

func (f *runnerFakeService) Start(context.Context) error { return nil }
func (f *runnerFakeService) Stop() error                 { return nil }
func (f *runnerFakeService) AddCheck(Check)              {}
func (f *runnerFakeService) RunCheckNow(context.Context) error {
	f.calls++
	return f.err
}

func TestRunner_Enabled(t *testing.T) {
	r := NewRunner(map[string]Service{"t1": &runnerFakeService{}})
	if !r.Enabled("t1") {
		t.Error("expected t1 enabled")
	}
	if r.Enabled("t2") {
		t.Error("expected t2 not enabled")
	}
}

func TestRunner_RunNow_DispatchesToTenant(t *testing.T) {
	svc := &runnerFakeService{}
	r := NewRunner(map[string]Service{"t1": svc})
	if err := r.RunNow(context.Background(), "t1"); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if svc.calls != 1 {
		t.Errorf("RunCheckNow calls = %d, want 1", svc.calls)
	}
}

func TestRunner_RunNow_UnknownTenant(t *testing.T) {
	r := NewRunner(nil) // nil map tolerated
	err := r.RunNow(context.Background(), "nope")
	if !errors.Is(err, ErrCheckerNotEnabled) {
		t.Errorf("expected ErrCheckerNotEnabled, got %v", err)
	}
}

func TestRunner_Services(t *testing.T) {
	r := NewRunner(map[string]Service{"a": &runnerFakeService{}, "b": &runnerFakeService{}})
	if len(r.Services()) != 2 {
		t.Errorf("Services() len = %d, want 2", len(r.Services()))
	}
}
