package amfacheck

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
)

func TestLoginErrorByAccountCheck_OneAlertPerUserAtThreshold(t *testing.T) {
	repo := &checkRepoStub{byUserCounts: []amfa.UserRiskyCount{
		{UserID: "user-brute", Count: 5},
	}}
	c := NewLoginErrorByAccountCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

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
	if a.CheckType != "amfa-login-error-by-account" {
		t.Errorf("check_type = %q, want amfa-login-error-by-account", a.CheckType)
	}
	if a.ResourceType != "amfa_user" {
		t.Errorf("resource_type = %q, want amfa_user", a.ResourceType)
	}
	if a.ResourceID != "user-brute" {
		t.Errorf("resource_id = %q, want user-brute", a.ResourceID)
	}
}

func TestLoginErrorByAccountCheck_AlertIDIncludesUTCDate(t *testing.T) {
	repo := &checkRepoStub{byUserCounts: []amfa.UserRiskyCount{{UserID: "user-brute", Count: 7}}}
	c := NewLoginErrorByAccountCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	want := alertID("amfa-login-error-by-account", "tenant-1", "master", "user-brute", day)
	if alerts[0].AlertID != want {
		t.Errorf("AlertID = %q, want %q", alerts[0].AlertID, want)
	}
}

func TestLoginErrorByAccountCheck_NoUsersNoAlerts(t *testing.T) {
	repo := &checkRepoStub{byUserCounts: nil}
	c := NewLoginErrorByAccountCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})
	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestLoginErrorByAccountCheck_RepoErrorPropagates(t *testing.T) {
	repo := &checkRepoStub{byUserCountsErr: amfa.ErrAmfaUnavailable}
	c := NewLoginErrorByAccountCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})
	if _, err := c.Execute(context.Background()); err == nil {
		t.Error("expected error to propagate")
	}
}
