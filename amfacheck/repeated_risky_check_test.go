package amfacheck

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
)

func TestRepeatedRiskyCheck_OneAlertPerUserOverThreshold(t *testing.T) {
	repo := &checkRepoStub{userCounts: []amfa.UserRiskyCount{
		{UserID: "user-a", Count: 4},
		{UserID: "user-b", Count: 3},
	}}
	c := NewRepeatedRiskyCheck("tenant-1", singleRealm("master"), repo, 3, 24*time.Hour, 2, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Severity != "warning" {
		t.Errorf("severity = %q, want warning", a.Severity)
	}
	if a.CheckType != "amfa-repeated-risky" {
		t.Errorf("check_type = %q, want amfa-repeated-risky", a.CheckType)
	}
	if a.ResourceType != "amfa_user" {
		t.Errorf("resource_type = %q, want amfa_user", a.ResourceType)
	}
	if a.ResourceID != "user-a" {
		t.Errorf("resource_id = %q, want user-a", a.ResourceID)
	}
}

func TestRepeatedRiskyCheck_AlertIDIncludesUTCDate(t *testing.T) {
	repo := &checkRepoStub{userCounts: []amfa.UserRiskyCount{{UserID: "user-a", Count: 5}}}
	c := NewRepeatedRiskyCheck("tenant-1", singleRealm("master"), repo, 3, 24*time.Hour, 2, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	want := alertID("amfa-repeated-risky", "tenant-1", "master", "user-a", day)
	if alerts[0].AlertID != want {
		t.Errorf("AlertID = %q, want %q", alerts[0].AlertID, want)
	}
}

func TestRepeatedRiskyCheck_NoUsersNoAlerts(t *testing.T) {
	repo := &checkRepoStub{userCounts: nil}
	c := NewRepeatedRiskyCheck("tenant-1", singleRealm("master"), repo, 3, 24*time.Hour, 2, testLogger{})
	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}
