package amfacheck

import (
	"context"
	"testing"
	"time"
)

func TestClientLoginErrorCheck_AlertWhenAtThreshold(t *testing.T) {
	repo := &checkRepoStub{typeCount: 7}
	c := NewClientLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].CheckType != "amfa-client-login-error" {
		t.Errorf("check_type = %q, want amfa-client-login-error", alerts[0].CheckType)
	}
}

func TestClientLoginErrorCheck_DistinctAlertIDFromRule4a(t *testing.T) {
	repo := &checkRepoStub{typeCount: 9}
	a4a := NewLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})
	a4b := NewClientLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

	x, err := a4a.Execute(context.Background())
	if err != nil {
		t.Fatalf("4a Execute: %v", err)
	}
	y, err := a4b.Execute(context.Background())
	if err != nil {
		t.Fatalf("4b Execute: %v", err)
	}
	if x[0].AlertID == y[0].AlertID {
		t.Error("rule 4a and 4b must produce distinct AlertIDs for the same realm/window")
	}
}
