package amfa

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// fakeKCClient is a configurable in-memory KeycloakClient for tests.
type fakeKCClient struct {
	calls    atomic.Int64
	users    map[string]KeycloakUser
	notFound map[string]bool
	fail     error // if set, return this error
}

func (f *fakeKCClient) GetUser(_ context.Context, _, _, userID string) (*KeycloakUser, error) {
	f.calls.Add(1)
	if f.fail != nil {
		return nil, f.fail
	}
	if f.notFound[userID] {
		return nil, ErrKeycloakUserNotFound
	}
	u, ok := f.users[userID]
	if !ok {
		return nil, ErrKeycloakUserNotFound
	}
	return &u, nil
}

func strPtr(s string) *string { return &s }

func TestEnrichment_CachesAfterFirstFetch(t *testing.T) {
	fake := &fakeKCClient{
		users: map[string]KeycloakUser{
			"u1": {Username: strPtr("alice"), Email: strPtr("alice@example.com")},
		},
	}
	enr := NewEnrichment(fake, time.Minute, time.Minute)

	// First batch: same uid 3 times — dedup inside the batch.
	got, err := enr.BatchEnrich(context.Background(), "t1", "realm-1", []string{"u1", "u1", "u1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u, ok := got["u1"]; !ok || u.Username == nil || *u.Username != "alice" {
		t.Errorf("expected alice in result, got %+v", got)
	}
	if c := fake.calls.Load(); c != 1 {
		t.Errorf("expected 1 call after first batch, got %d", c)
	}

	// Second batch: cache hit, no additional calls.
	_, _ = enr.BatchEnrich(context.Background(), "t1", "realm-1", []string{"u1"})
	if c := fake.calls.Load(); c != 1 {
		t.Errorf("expected 1 call total (cached), got %d", c)
	}
}

func TestEnrichment_NegativeCacheFor404(t *testing.T) {
	fake := &fakeKCClient{
		notFound: map[string]bool{"ghost": true},
	}
	enr := NewEnrichment(fake, time.Minute, time.Minute)

	got, _ := enr.BatchEnrich(context.Background(), "t1", "realm-1", []string{"ghost"})
	if _, ok := got["ghost"]; ok {
		t.Errorf("expected ghost to be absent from result, got %+v", got)
	}
	if c := fake.calls.Load(); c != 1 {
		t.Errorf("expected 1 call after first batch, got %d", c)
	}

	// Second batch: negative cache hit, no new call.
	_, _ = enr.BatchEnrich(context.Background(), "t1", "realm-1", []string{"ghost"})
	if c := fake.calls.Load(); c != 1 {
		t.Errorf("expected 1 call total (negative cached), got %d", c)
	}
}

func TestEnrichment_TTLExpiry(t *testing.T) {
	fake := &fakeKCClient{
		users: map[string]KeycloakUser{
			"u1": {Username: strPtr("alice")},
		},
	}
	enr := NewEnrichment(fake, 10*time.Millisecond, 10*time.Millisecond)

	_, _ = enr.BatchEnrich(context.Background(), "t1", "realm-1", []string{"u1"})
	if c := fake.calls.Load(); c != 1 {
		t.Fatalf("expected 1 call after first batch, got %d", c)
	}

	time.Sleep(15 * time.Millisecond)

	_, _ = enr.BatchEnrich(context.Background(), "t1", "realm-1", []string{"u1"})
	if c := fake.calls.Load(); c != 2 {
		t.Errorf("expected 2 calls after TTL expiry, got %d", c)
	}
}

func TestEnrichment_SkipsEmptyUserIDs(t *testing.T) {
	fake := &fakeKCClient{}
	enr := NewEnrichment(fake, time.Minute, time.Minute)

	got, err := enr.BatchEnrich(context.Background(), "t1", "realm-1", []string{""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %+v", got)
	}
	if c := fake.calls.Load(); c != 0 {
		t.Errorf("expected 0 calls, got %d", c)
	}
}

func TestEnrichment_TransientErrorNotCached(t *testing.T) {
	fake := &fakeKCClient{
		fail: errors.New("network timeout"),
	}
	enr := NewEnrichment(fake, time.Minute, time.Minute)

	_, _ = enr.BatchEnrich(context.Background(), "t1", "realm-1", []string{"u1"})
	if c := fake.calls.Load(); c != 1 {
		t.Fatalf("expected 1 call after first batch, got %d", c)
	}

	// Transient errors are not cached: client called again.
	_, _ = enr.BatchEnrich(context.Background(), "t1", "realm-1", []string{"u1"})
	if c := fake.calls.Load(); c != 2 {
		t.Errorf("expected 2 calls (no caching of transient errors), got %d", c)
	}
}
