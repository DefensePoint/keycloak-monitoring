package amfacheck

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
)

func TestRealmRejectBurstCheck_AlertWhenDistinctUsersAtThreshold(t *testing.T) {
	repo := &checkRepoStub{distinctUsers: 10}
	c := NewRealmRejectBurstCheck("tenant-1", singleRealm("master"), repo, 10, 10*time.Minute, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Severity != "critical" {
		t.Errorf("severity = %q, want critical", a.Severity)
	}
	if a.CheckType != "amfa-realm-reject-burst" {
		t.Errorf("check_type = %q, want amfa-realm-reject-burst", a.CheckType)
	}
	if a.ResourceType != "realm" {
		t.Errorf("resource_type = %q, want realm", a.ResourceType)
	}
	if a.ResourceID != "master" {
		t.Errorf("resource_id = %q, want master", a.ResourceID)
	}
}

func TestRealmRejectBurstCheck_NoAlertBelowThreshold(t *testing.T) {
	repo := &checkRepoStub{distinctUsers: 9}
	c := NewRealmRejectBurstCheck("tenant-1", singleRealm("master"), repo, 10, 10*time.Minute, testLogger{})
	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts below threshold, got %d", len(alerts))
	}
}

func TestRealmRejectBurstCheck_AlertIDIncludesUTCDate(t *testing.T) {
	repo := &checkRepoStub{distinctUsers: 15}
	c := NewRealmRejectBurstCheck("tenant-1", singleRealm("master"), repo, 10, 10*time.Minute, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	want := alertID("amfa-realm-reject-burst", "tenant-1", "master", day)
	if alerts[0].AlertID != want {
		t.Errorf("AlertID = %q, want %q", alerts[0].AlertID, want)
	}
}

func TestRealmRejectBurstCheck_RepoErrorPropagates(t *testing.T) {
	repo := &checkRepoStub{distinctUsersErr: amfa.ErrAmfaUnavailable}
	c := NewRealmRejectBurstCheck("tenant-1", singleRealm("master"), repo, 10, 10*time.Minute, testLogger{})
	if _, err := c.Execute(context.Background()); err == nil {
		t.Error("expected error to propagate")
	}
}
