package tenant

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// waitClosed fails the test if ch is not closed or sent on within a second.
func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// A health check that lands while a rename is saving must survive it. The
// rename copied the entry before the check wrote "unhealthy", and storing that
// copy put "healthy" back.
func TestRenameKeepsAHealthCheckThatLandedDuringItsSave(t *testing.T) {
	saving := make(chan struct{})
	release := make(chan struct{})
	svc := NewService(&mockRepository{
		updateFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error {
			close(saving)
			<-release
			return nil
		},
	}).(*service)
	healthy := cachedTenant()
	healthy.HealthStatus = "healthy"
	healthy.HealthMessage = "ok"
	svc.cache.Store("tenant-1", healthy)
	ctx := context.Background()

	renamed := make(chan error, 1)
	go func() {
		_, err := svc.UpdateTenant(ctx, "tenant-1", &UpdateRequest{Name: strPtr("Renamed")})
		renamed <- err
	}()
	waitClosed(t, saving, "the rename to reach its save")

	healthDone := make(chan struct{})
	go func() {
		if err := svc.UpdateHealth(ctx, "tenant-1", "unhealthy", "connection refused"); err != nil {
			t.Errorf("UpdateHealth: %v", err)
		}
		close(healthDone)
	}()
	waitClosed(t, healthDone, "the health check, which must not wait for the rename")

	close(release)
	if err := <-renamed; err != nil {
		t.Fatalf("UpdateTenant: %v", err)
	}

	got, _ := svc.GetTenant(ctx, "tenant-1")
	if got.Name != "Renamed" {
		t.Errorf("name = %q, want Renamed", got.Name)
	}
	if got.HealthStatus != "unhealthy" || got.HealthMessage != "connection refused" {
		t.Errorf("health = %q %q, want the check's unhealthy, connection refused", got.HealthStatus, got.HealthMessage)
	}
}

// Two edits of one tenant must reach the cache in the order they reached the
// database. A saved, then B copied, saved and stored, then A stored: the cache
// kept A while the database had B, until a restart.
func TestTwoEditsLeaveTheCacheMatchingTheDatabase(t *testing.T) {
	var db *domain.KeycloakTenant // what the database holds: Save writes every column
	aSaved := make(chan struct{})
	releaseA := make(chan struct{})
	calls := 0
	svc := NewService(&mockRepository{
		updateFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error {
			db = cloneTenant(tenant)
			calls++
			if calls == 1 {
				close(aSaved)
				<-releaseA // A's save is committed; A has not stored yet
			}
			return nil
		},
	}).(*service)
	svc.cache.Store("tenant-1", cachedTenant())
	ctx := context.Background()

	aDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdateTenant(ctx, "tenant-1", &UpdateRequest{
			Name:         strPtr("Edit A"),
			ClientID:     strPtr("client-a"),
			ServerURL:    strPtr("https://a.example.com"),
			ClientSecret: strPtr("secret-a"),
		})
		aDone <- err
	}()
	waitClosed(t, aSaved, "edit A to save")

	bDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdateTenant(ctx, "tenant-1", &UpdateRequest{Name: strPtr("Edit B")})
		bDone <- err
	}()
	// Give B every chance to run ahead of A's store, as it did before.
	time.Sleep(100 * time.Millisecond)
	close(releaseA)

	for _, done := range []chan error{aDone, bDone} {
		if err := <-done; err != nil {
			t.Fatalf("UpdateTenant: %v", err)
		}
	}

	got, _ := svc.GetTenant(ctx, "tenant-1")
	if got.Name != db.Name || got.ClientID != db.ClientID || got.ServerURL != db.ServerURL || got.Enabled != db.Enabled {
		t.Errorf("cache and database disagree:\n cache: name=%q client_id=%q server_url=%q\n    db: name=%q client_id=%q server_url=%q",
			got.Name, got.ClientID, got.ServerURL, db.Name, db.ClientID, db.ServerURL)
	}
	if got.Name != "Edit B" || got.ClientID != "client-a" {
		t.Errorf("final = name %q client_id %q, want B's rename on top of A's client_id", got.Name, got.ClientID)
	}
}
