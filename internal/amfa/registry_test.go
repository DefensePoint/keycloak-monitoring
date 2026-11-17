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

// registryChange is one notification a Subscribe listener received.
type registryChange struct {
	tenantID string
	repo     Repository
}

func TestRegistry_SubscribeReportsRegisterAndUnregister(t *testing.T) {
	r := NewRegistry()
	var got []registryChange
	r.Subscribe(func(tenantID string, repo Repository) {
		got = append(got, registryChange{tenantID, repo})
	})

	r.Register("a", fakeRepo{id: "a1"})
	r.Register("a", fakeRepo{id: "a2"}) // replaced, e.g. a new endpoint
	r.Unregister("a")

	want := []registryChange{{"a", fakeRepo{id: "a1"}}, {"a", fakeRepo{id: "a2"}}, {"a", nil}}
	if len(got) != len(want) {
		t.Fatalf("got %d notifications %v, want %v", len(got), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("notification %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// Unregister runs on every save of a tenant without AMFA, so it only reports
// a tenant that had a repository.
func TestRegistry_UnregisterOfUnknownTenantIsNotReported(t *testing.T) {
	r := NewRegistry()
	calls := 0
	r.Subscribe(func(string, Repository) { calls++ })

	r.Unregister("never-registered")

	if calls != 0 {
		t.Errorf("listener called %d times, want 0", calls)
	}
}

// A listener sees the changes in the order they were made, even when they come
// from different goroutines, so the last one it sees is the registry's state.
func TestRegistry_ListenerSeesChangesInOrder(t *testing.T) {
	r := NewRegistry()
	var mu sync.Mutex
	last := map[string]Repository{}
	r.Subscribe(func(tenantID string, repo Repository) {
		mu.Lock()
		defer mu.Unlock()
		last[tenantID] = repo
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			r.Register("a", fakeRepo{id: fmt.Sprint(i)})
		}(i)
		go func() {
			defer wg.Done()
			r.Unregister("a")
		}()
	}
	wg.Wait()

	current, err := r.RepositoryFor("a")
	if err != nil {
		current = nil
	}
	mu.Lock()
	defer mu.Unlock()
	if last["a"] != current {
		t.Errorf("listener's last view %v differs from the registry's %v", last["a"], current)
	}
}
