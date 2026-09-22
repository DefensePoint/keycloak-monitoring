package amfacheck

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
)

// checkRepoStub is a hand-written amfa.Repository stub for check tests. Only the
// methods a given check uses return data; the rest are no-ops. It is shared by
// the rule test files in this package.
type checkRepoStub struct {
	rejected         []amfa.EventRow
	rejectedErr      error
	vpnRisky         []amfa.EventRow
	vpnRiskyErr      error
	userCounts       []amfa.UserRiskyCount
	userErr          error
	typeCount        int64
	typeCountErr     error
	clientCounts     []amfa.ClientEventCount
	clientCountsErr  error
	distinctUsers    int64
	distinctUsersErr error
	byUserCounts     []amfa.UserRiskyCount
	byUserCountsErr  error
}

func (s *checkRepoStub) ListEvents(_ context.Context, _ amfa.ListEventsOptions) (*amfa.ListEventsResult, error) {
	return &amfa.ListEventsResult{}, nil
}
func (s *checkRepoStub) GetStats(_ context.Context, _ amfa.StatsOptions) (amfa.Stats, error) {
	return amfa.Stats{}, nil
}
func (s *checkRepoStub) GetGeoBuckets(_ context.Context, _ amfa.GeoOptions) ([]amfa.GeoBucket, error) {
	return nil, nil
}
func (s *checkRepoStub) ListRejectedEventsSince(_ context.Context, _ string, _ time.Time) ([]amfa.EventRow, error) {
	return s.rejected, s.rejectedErr
}
func (s *checkRepoStub) ListVPNRiskyEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]amfa.EventRow, error) {
	return s.vpnRisky, s.vpnRiskyErr
}
func (s *checkRepoStub) CountRepeatedRiskyByUser(_ context.Context, _ string, _ time.Time, _, _ int) ([]amfa.UserRiskyCount, error) {
	return s.userCounts, s.userErr
}
func (s *checkRepoStub) CountByEventTypeInWindow(_ context.Context, _, _ string, _, _ time.Time) (int64, error) {
	return s.typeCount, s.typeCountErr
}

func (s *checkRepoStub) ListEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]amfa.EventRow, error) {
	return nil, nil
}
func (s *checkRepoStub) CountByEventTypeAndClientInWindow(_ context.Context, _, _ string, _, _ time.Time, _ int) ([]amfa.ClientEventCount, error) {
	return s.clientCounts, s.clientCountsErr
}
func (s *checkRepoStub) CountDistinctRejectedUsersSince(_ context.Context, _ string, _ time.Time) (int64, error) {
	return s.distinctUsers, s.distinctUsersErr
}
func (s *checkRepoStub) CountByEventTypeByUser(_ context.Context, _, _ string, _ time.Time, _ int) ([]amfa.UserRiskyCount, error) {
	return s.byUserCounts, s.byUserCountsErr
}

func singleRealm(realms ...string) RealmsFunc {
	return func(_ context.Context) ([]string, error) { return realms, nil }
}

func TestRiskRejectedCheck_ProducesCriticalPerEventAlerts(t *testing.T) {
	uid := "user-1"
	repo := &checkRepoStub{rejected: []amfa.EventRow{
		{EventID: "ev-1", EventTime: time.Now(), UserID: &uid, IPAddress: "1.2.3.4"},
		{EventID: "ev-2", EventTime: time.Now(), IPAddress: "5.6.7.8"},
	}}
	c := NewRiskRejectedCheck("tenant-1", singleRealm("master"), repo, time.Minute, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Severity != "critical" {
		t.Errorf("severity = %q, want critical", a.Severity)
	}
	if a.Source != "event" {
		t.Errorf("source = %q, want event", a.Source)
	}
	if a.CheckType != "amfa-risk-rejected" {
		t.Errorf("check_type = %q, want amfa-risk-rejected", a.CheckType)
	}
	if a.EventID == nil || *a.EventID != "ev-1" {
		t.Errorf("EventID pointer = %v, want ev-1", a.EventID)
	}
	want := alertID("amfa-risk-rejected", "tenant-1", "ev-1")
	if a.AlertID != want {
		t.Errorf("AlertID = %q, want %q", a.AlertID, want)
	}
}

func TestRiskRejectedCheck_AdvancesCheckpoint(t *testing.T) {
	// Production SQL returns only events strictly newer than the current
	// checkpoint (event_time > since), so the checkpoint moves forward to the
	// newest event. Use a recent event (newer than the now-lookback floor) to
	// mirror that contract; the monotonic After(since) guard then advances it.
	newest := time.Now().Add(-10 * time.Second)
	repo := &checkRepoStub{rejected: []amfa.EventRow{
		{EventID: "ev-1", EventTime: newest.Add(-20 * time.Second)},
		{EventID: "ev-2", EventTime: newest},
	}}
	c := NewRiskRejectedCheck("tenant-1", singleRealm("master"), repo, time.Minute, testLogger{})

	if _, err := c.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := c.getLastSeen("master")
	if !got.Equal(newest) {
		t.Errorf("checkpoint = %v, want %v (newest event time)", got, newest)
	}
}

func TestRiskRejectedCheck_RepoErrorPropagates(t *testing.T) {
	repo := &checkRepoStub{rejectedErr: amfa.ErrAmfaUnavailable}
	c := NewRiskRejectedCheck("tenant-1", singleRealm("master"), repo, time.Minute, testLogger{})
	if _, err := c.Execute(context.Background()); err == nil {
		t.Error("expected error to propagate")
	}
}
