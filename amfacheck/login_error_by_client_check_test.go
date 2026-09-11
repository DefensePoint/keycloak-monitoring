package amfacheck

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
)

func TestLoginErrorByClientCheck_OneAlertPerClientAtThreshold(t *testing.T) {
	repo := &checkRepoStub{clientCounts: []amfa.ClientEventCount{
		{Client: "mobile-app", Count: 5},
	}}
	c := NewLoginErrorByClientCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

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
	if a.CheckType != "amfa-login-error-by-client" {
		t.Errorf("check_type = %q, want amfa-login-error-by-client", a.CheckType)
	}
	if a.ResourceType != "amfa_client" {
		t.Errorf("resource_type = %q, want amfa_client", a.ResourceType)
	}
	if a.ResourceID != "mobile-app" {
		t.Errorf("resource_id = %q, want mobile-app", a.ResourceID)
	}
}

func TestLoginErrorByClientCheck_NoClientsNoAlerts(t *testing.T) {
	repo := &checkRepoStub{clientCounts: nil}
	c := NewLoginErrorByClientCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})
	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestLoginErrorByClientCheck_AlertIDStableWithinWindow(t *testing.T) {
	repo := &checkRepoStub{clientCounts: []amfa.ClientEventCount{{Client: "mobile-app", Count: 8}}}
	c := NewLoginErrorByClientCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

	a1, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute(1): %v", err)
	}
	a2, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute(2): %v", err)
	}
	if a1[0].AlertID != a2[0].AlertID {
		t.Errorf("AlertID not stable: %q != %q", a1[0].AlertID, a2[0].AlertID)
	}
}

func TestLoginErrorByClientCheck_RepoErrorPropagates(t *testing.T) {
	repo := &checkRepoStub{clientCountsErr: amfa.ErrAmfaUnavailable}
	c := NewLoginErrorByClientCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})
	if _, err := c.Execute(context.Background()); err == nil {
		t.Error("expected error to propagate")
	}
}
