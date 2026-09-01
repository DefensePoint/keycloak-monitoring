package amfa

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeRepo is a no-op Repository used for registry tests.
type fakeRepo struct {
	id string
}

func (fakeRepo) ListEvents(_ context.Context, _ ListEventsOptions) (*ListEventsResult, error) {
	return nil, nil
}
func (fakeRepo) GetStats(_ context.Context, _ StatsOptions) (Stats, error) {
	return Stats{}, nil
}
func (fakeRepo) GetGeoBuckets(_ context.Context, _ GeoOptions) ([]GeoBucket, error) {
	return nil, nil
}
func (fakeRepo) ListRejectedEventsSince(_ context.Context, _ string, _ time.Time) ([]EventRow, error) {
	return nil, nil
}
func (fakeRepo) ListVPNRiskyEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]EventRow, error) {
	return nil, nil
}
func (fakeRepo) CountRepeatedRiskyByUser(_ context.Context, _ string, _ time.Time, _, _ int) ([]UserRiskyCount, error) {
	return nil, nil
}
func (fakeRepo) CountByEventTypeInWindow(_ context.Context, _, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}
func (fakeRepo) ListEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]EventRow, error) {
	return nil, nil
}
func (fakeRepo) CountByEventTypeAndClientInWindow(_ context.Context, _, _ string, _, _ time.Time, _ int) ([]ClientEventCount, error) {
	return nil, nil
}
func (fakeRepo) CountDistinctRejectedUsersSince(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (fakeRepo) CountByEventTypeByUser(_ context.Context, _, _ string, _ time.Time, _ int) ([]UserRiskyCount, error) {
	return nil, nil
}

func TestRegistry_ReturnsRepoForRegisteredTenant(t *testing.T) {
	r := NewRegistry()
	want := fakeRepo{id: "tenant-a"}
	r.Register("tenant-a", want)

	got, err := r.RepositoryFor("tenant-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotRepo, ok := got.(fakeRepo)
	if !ok {
		t.Fatalf("expected fakeRepo, got %T", got)
	}
	if gotRepo.id != want.id {
		t.Errorf("expected repo id %q, got %q", want.id, gotRepo.id)
	}
}

func TestRegistry_ReturnsErrAmfaNotConfiguredForUnknownTenant(t *testing.T) {
	r := NewRegistry()

	_, err := r.RepositoryFor("missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrAmfaNotConfigured) {
		t.Errorf("expected errors.Is(err, ErrAmfaNotConfigured), got %v", err)
	}
}

func TestRegistry_TenantIDsReturnsRegisteredSorted(t *testing.T) {
	r := NewRegistry()
	r.Register("b", fakeRepo{id: "b"})
	r.Register("a", fakeRepo{id: "a"})

	ids := r.TenantIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	if ids[0] != "a" || ids[1] != "b" {
		t.Errorf("expected [a b], got %v", ids)
	}
}

func TestRegistry_ConcurrentRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n * 2)

	// Register goroutines
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("tenant-%d", i)
			r.Register(id, fakeRepo{id: id})
		}(i)
	}

	// Lookup goroutines (may or may not find the tenant — we just verify no race)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("tenant-%d", i)
			_, _ = r.RepositoryFor(id)
		}(i)
	}

	wg.Wait()

	// After all registers complete, every tenant should resolve.
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("tenant-%d", i)
		if _, err := r.RepositoryFor(id); err != nil {
			t.Errorf("expected tenant %s registered, got error: %v", id, err)
		}
	}

	if got := len(r.TenantIDs()); got != n {
		t.Errorf("expected %d registered tenants, got %d", n, got)
	}
}
