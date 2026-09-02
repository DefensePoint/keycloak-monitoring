package amfa

import (
	"context"
	"testing"
	"time"
)

func TestEventRow_ZeroValue(t *testing.T) {
	var e EventRow
	if e.EventID != "" {
		t.Errorf("expected empty EventID, got %q", e.EventID)
	}
	if !e.EventTime.IsZero() {
		t.Errorf("expected zero EventTime, got %v", e.EventTime)
	}
	if e.EventType != "" {
		t.Errorf("expected empty EventType, got %q", e.EventType)
	}
	if e.UserID != nil {
		t.Errorf("expected nil UserID, got %v", *e.UserID)
	}
	if e.Country != nil {
		t.Errorf("expected nil Country pointer, got %v", *e.Country)
	}
	if e.Lat != nil {
		t.Errorf("expected nil Lat pointer, got %v", *e.Lat)
	}
	if e.Long != nil {
		t.Errorf("expected nil Long pointer, got %v", *e.Long)
	}
	if e.IsVPN {
		t.Errorf("expected IsVPN=false, got true")
	}
	if e.RiskLevel != nil {
		t.Errorf("expected nil RiskLevel, got %v", *e.RiskLevel)
	}
	if e.FinalStatus != nil {
		t.Errorf("expected nil FinalStatus, got %v", *e.FinalStatus)
	}
}

func TestStats_ZeroValue(t *testing.T) {
	var s Stats
	if s.Total != 0 {
		t.Errorf("expected Total=0, got %d", s.Total)
	}
	if s.Risky != 0 {
		t.Errorf("expected Risky=0, got %d", s.Risky)
	}
	if s.UniqueUsers != 0 {
		t.Errorf("expected UniqueUsers=0, got %d", s.UniqueUsers)
	}
	if s.FlaggedIPs != 0 {
		t.Errorf("expected FlaggedIPs=0, got %d", s.FlaggedIPs)
	}
}

func TestGeoBucket_HoldsCoordinates(t *testing.T) {
	b := GeoBucket{Lat: 40.7, Long: -74.0, Count: 5, RiskyCount: 2}
	const epsilon = 0.001
	if diff := b.Lat - 40.7; diff < -epsilon || diff > epsilon {
		t.Errorf("expected Lat~40.7, got %v", b.Lat)
	}
	if diff := b.Long - (-74.0); diff < -epsilon || diff > epsilon {
		t.Errorf("expected Long~-74.0, got %v", b.Long)
	}
	if b.Count != 5 {
		t.Errorf("expected Count=5, got %d", b.Count)
	}
	if b.RiskyCount != 2 {
		t.Errorf("expected RiskyCount=2, got %d", b.RiskyCount)
	}
}

// Compile-time check that ErrAmfaNotConfigured satisfies the error interface.
var _ error = ErrAmfaNotConfigured

func TestErrAmfaNotConfigured_HasMessage(t *testing.T) {
	if ErrAmfaNotConfigured.Error() == "" {
		t.Errorf("expected non-empty error message")
	}
	if ErrAmfaUnavailable.Error() == "" {
		t.Errorf("expected non-empty ErrAmfaUnavailable message")
	}
}

func TestListEventsOptions_ZeroValue(t *testing.T) {
	opts := ListEventsOptions{}
	if opts.Limit != 0 {
		t.Errorf("expected Limit=0, got %d", opts.Limit)
	}
	if opts.RealmID != "" {
		t.Errorf("expected empty RealmID, got %q", opts.RealmID)
	}
	if opts.StartTime != nil {
		t.Errorf("expected nil StartTime, got %v", opts.StartTime)
	}
	if opts.EndTime != nil {
		t.Errorf("expected nil EndTime, got %v", opts.EndTime)
	}
}

// Compile-time check that Repository and Service interfaces are satisfiable.
func TestRepositoryInterface_Satisfiable(t *testing.T) {
	var _ Repository = (*nopRepository)(nil)
}

func TestServiceInterface_Satisfiable(t *testing.T) {
	var _ Service = (*nopService)(nil)
}

// nopRepository is a no-op stub used to verify the Repository interface compiles.
type nopRepository struct{}

func (nopRepository) ListEvents(_ context.Context, _ ListEventsOptions) (*ListEventsResult, error) {
	return nil, nil
}
func (nopRepository) GetStats(_ context.Context, _ StatsOptions) (Stats, error) {
	return Stats{}, nil
}
func (nopRepository) GetGeoBuckets(_ context.Context, _ GeoOptions) ([]GeoBucket, error) {
	return nil, nil
}
func (nopRepository) ListRejectedEventsSince(_ context.Context, _ string, _ time.Time) ([]EventRow, error) {
	return nil, nil
}
func (nopRepository) ListVPNRiskyEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]EventRow, error) {
	return nil, nil
}
func (nopRepository) CountRepeatedRiskyByUser(_ context.Context, _ string, _ time.Time, _, _ int) ([]UserRiskyCount, error) {
	return nil, nil
}
func (nopRepository) CountByEventTypeInWindow(_ context.Context, _, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}
func (nopRepository) ListEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]EventRow, error) {
	return nil, nil
}
func (nopRepository) CountByEventTypeAndClientInWindow(_ context.Context, _, _ string, _, _ time.Time, _ int) ([]ClientEventCount, error) {
	return nil, nil
}
func (nopRepository) CountDistinctRejectedUsersSince(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (nopRepository) CountByEventTypeByUser(_ context.Context, _, _ string, _ time.Time, _ int) ([]UserRiskyCount, error) {
	return nil, nil
}

// nopService is a no-op stub used to verify the Service interface compiles.
type nopService struct{}

func (nopService) ListEvents(_ context.Context, _ string, _ ListEventsOptions) ([]Event, int64, error) {
	return nil, 0, nil
}
func (nopService) GetStats(_ context.Context, _ string, _ StatsOptions) (Stats, error) {
	return Stats{}, nil
}
func (nopService) GetGeoBuckets(_ context.Context, _ string, _ GeoOptions) ([]GeoBucket, error) {
	return nil, nil
}
