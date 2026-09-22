package amfa

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// stubRepo is a configurable Repository for service tests.
type stubRepo struct {
	listResult *ListEventsResult
	listErr    error
	stats      Stats
	statsErr   error
	geo        []GeoBucket
	geoErr     error
}

func (s *stubRepo) ListEvents(_ context.Context, _ ListEventsOptions) (*ListEventsResult, error) {
	return s.listResult, s.listErr
}
func (s *stubRepo) GetStats(_ context.Context, _ StatsOptions) (Stats, error) {
	return s.stats, s.statsErr
}
func (s *stubRepo) GetGeoBuckets(_ context.Context, _ GeoOptions) ([]GeoBucket, error) {
	return s.geo, s.geoErr
}
func (s *stubRepo) ListRejectedEventsSince(_ context.Context, _ string, _ time.Time) ([]EventRow, error) {
	return nil, nil
}
func (s *stubRepo) ListVPNRiskyEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]EventRow, error) {
	return nil, nil
}
func (s *stubRepo) CountRepeatedRiskyByUser(_ context.Context, _ string, _ time.Time, _, _ int) ([]UserRiskyCount, error) {
	return nil, nil
}
func (s *stubRepo) CountByEventTypeInWindow(_ context.Context, _, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}

func (s *stubRepo) ListEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]EventRow, error) {
	return nil, nil
}
func (s *stubRepo) CountByEventTypeAndClientInWindow(_ context.Context, _, _ string, _, _ time.Time, _ int) ([]ClientEventCount, error) {
	return nil, nil
}
func (s *stubRepo) CountDistinctRejectedUsersSince(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (s *stubRepo) CountByEventTypeByUser(_ context.Context, _, _ string, _ time.Time, _ int) ([]UserRiskyCount, error) {
	return nil, nil
}

// stubKCClient is a minimal KeycloakClient for service-level tests; declared
// here to avoid coupling to enrichment_test.go (Go test files in the same
// package see each other but keeping this independent helps readability).
type stubKCClient struct {
	calls atomic.Int64
	users map[string]KeycloakUser
}

func (s *stubKCClient) GetUser(_ context.Context, _, _, userID string) (*KeycloakUser, error) {
	s.calls.Add(1)
	u, ok := s.users[userID]
	if !ok {
		return nil, ErrKeycloakUserNotFound
	}
	return &u, nil
}

func svcStrPtr(s string) *string { return &s }

func TestService_ListEvents_ReturnsErrAmfaNotConfiguredForUnknownTenant(t *testing.T) {
	reg := NewRegistry()
	svc := NewService(reg, nil)

	_, _, err := svc.ListEvents(context.Background(), "missing", ListEventsOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrAmfaNotConfigured) {
		t.Errorf("expected errors.Is(err, ErrAmfaNotConfigured), got %v", err)
	}
}

func TestService_ListEvents_EnrichesUsernames(t *testing.T) {
	uid := "u1"
	repo := &stubRepo{
		listResult: &ListEventsResult{
			Items: []EventRow{
				{
					EventID:   "e1",
					EventTime: time.Now(),
					EventType: "LOGIN",
					UserID:    &uid,
					Client:    "web",
					IPAddress: "1.2.3.4",
				},
			},
			Total: 1,
		},
	}
	reg := NewRegistry()
	reg.Register("t1", repo)

	kc := &stubKCClient{
		users: map[string]KeycloakUser{
			"u1": {Username: svcStrPtr("alice"), Email: svcStrPtr("alice@example.com")},
		},
	}
	enr := NewEnrichment(kc, time.Minute, time.Minute)
	svc := NewService(reg, enr)

	events, total, err := svc.ListEvents(context.Background(), "t1", ListEventsOptions{RealmID: "realm-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Username == nil || *events[0].Username != "alice" {
		t.Errorf("expected Username=alice, got %v", events[0].Username)
	}
	if events[0].Email == nil || *events[0].Email != "alice@example.com" {
		t.Errorf("expected Email=alice@example.com, got %v", events[0].Email)
	}
}

func TestService_ListEvents_HandlesNilUserID(t *testing.T) {
	repo := &stubRepo{
		listResult: &ListEventsResult{
			Items: []EventRow{
				{
					EventID:   "e1",
					EventTime: time.Now(),
					EventType: "LOGIN_ERROR",
					UserID:    nil,
					Client:    "web",
					IPAddress: "1.2.3.4",
				},
			},
			Total: 1,
		},
	}
	reg := NewRegistry()
	reg.Register("t1", repo)

	kc := &stubKCClient{}
	enr := NewEnrichment(kc, time.Minute, time.Minute)
	svc := NewService(reg, enr)

	events, _, err := svc.ListEvents(context.Background(), "t1", ListEventsOptions{RealmID: "realm-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Username != nil {
		t.Errorf("expected Username=nil, got %v", *events[0].Username)
	}
	if events[0].Email != nil {
		t.Errorf("expected Email=nil, got %v", *events[0].Email)
	}
	if c := kc.calls.Load(); c != 0 {
		t.Errorf("expected 0 keycloak calls (nil UserID), got %d", c)
	}
}

func TestService_ListEvents_DedupsUsersBeforeEnrichment(t *testing.T) {
	uid := "u1"
	rows := make([]EventRow, 5)
	for i := range rows {
		rows[i] = EventRow{
			EventID:   "e",
			EventTime: time.Now(),
			EventType: "LOGIN",
			UserID:    &uid,
			Client:    "web",
			IPAddress: "1.2.3.4",
		}
	}
	repo := &stubRepo{
		listResult: &ListEventsResult{Items: rows, Total: 5},
	}
	reg := NewRegistry()
	reg.Register("t1", repo)

	kc := &stubKCClient{
		users: map[string]KeycloakUser{
			"u1": {Username: svcStrPtr("alice")},
		},
	}
	enr := NewEnrichment(kc, time.Minute, time.Minute)
	svc := NewService(reg, enr)

	events, _, err := svc.ListEvents(context.Background(), "t1", ListEventsOptions{RealmID: "realm-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}
	if c := kc.calls.Load(); c != 1 {
		t.Errorf("expected 1 keycloak call (dedup), got %d", c)
	}
	for i, e := range events {
		if e.Username == nil || *e.Username != "alice" {
			t.Errorf("event[%d]: expected Username=alice, got %v", i, e.Username)
		}
	}
}

func TestService_GetStats_PropagatesRepoResult(t *testing.T) {
	repo := &stubRepo{
		stats: Stats{Total: 100, Risky: 10, UniqueUsers: 25, FlaggedIPs: 3},
	}
	reg := NewRegistry()
	reg.Register("t1", repo)
	svc := NewService(reg, nil)

	got, err := svc.GetStats(context.Background(), "t1", StatsOptions{RealmID: "realm-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 100 || got.Risky != 10 || got.UniqueUsers != 25 || got.FlaggedIPs != 3 {
		t.Errorf("stats not propagated correctly: %+v", got)
	}
}

func TestService_GetGeoBuckets_ReturnsBuckets(t *testing.T) {
	repo := &stubRepo{
		geo: []GeoBucket{
			{Lat: 40.7, Long: -74.0, Count: 5, RiskyCount: 1},
			{Lat: 51.5, Long: -0.12, Count: 3, RiskyCount: 0},
		},
	}
	reg := NewRegistry()
	reg.Register("t1", repo)
	svc := NewService(reg, nil)

	got, err := svc.GetGeoBuckets(context.Background(), "t1", GeoOptions{RealmID: "realm-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(got))
	}
	if got[0].Count != 5 || got[1].Count != 3 {
		t.Errorf("bucket counts wrong: %+v", got)
	}
}

func TestService_GetStats_ReturnsErrAmfaNotConfigured(t *testing.T) {
	reg := NewRegistry()
	svc := NewService(reg, nil)

	_, err := svc.GetStats(context.Background(), "missing", StatsOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrAmfaNotConfigured) {
		t.Errorf("expected errors.Is(err, ErrAmfaNotConfigured), got %v", err)
	}
}
