package amfacheck

import (
	"context"
	"testing"
	"time"
)

func TestLoginErrorCheck_AlertWhenCountAtThreshold(t *testing.T) {
	repo := &checkRepoStub{typeCount: 5}
	c := NewLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Severity != "warning" {
		t.Errorf("severity = %q, want warning", a.Severity)
	}
	if a.CheckType != "amfa-login-error" {
		t.Errorf("check_type = %q, want amfa-login-error", a.CheckType)
	}
	if a.RealmName != "master" {
		t.Errorf("realm = %q, want master", a.RealmName)
	}
}

func TestLoginErrorCheck_NoAlertBelowThreshold(t *testing.T) {
	repo := &checkRepoStub{typeCount: 4}
	c := NewLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})
	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts below threshold, got %d", len(alerts))
	}
}

func TestLoginErrorCheck_AlertIDStableWithinWindow(t *testing.T) {
	repo := &checkRepoStub{typeCount: 10}
	c := NewLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

	a1, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute(1): %v", err)
	}
	a2, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute(2): %v", err)
	}
	// Two calls inside the same wall-clock 5-minute window must produce the
	// same AlertID (dedup). This is non-flaky except at a 5-minute boundary; if
	// it ever flakes, that's the boundary - re-run.
	if a1[0].AlertID != a2[0].AlertID {
		t.Errorf("AlertID not stable within window: %q != %q", a1[0].AlertID, a2[0].AlertID)
	}
}
