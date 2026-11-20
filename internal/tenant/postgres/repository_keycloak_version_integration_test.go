// Coverage for storing the Keycloak version an operator enters for a tenant.
//
// Requires a real PostgreSQL instance, from KMT_TEST_DATABASE_DSN; skipped
// when unset. Like the enabled tests, it only removes the tenants it creates,
// so it does not skip under -short.
package postgres

import (
	"context"
	"testing"
	"time"
)

// The entered version and when it was set survive a save and a reload, and an
// update can change and remove them.
func TestRepository_StoresTheEnteredKeycloakVersion(t *testing.T) {
	repo, db := newEnabledTestRepo(t)
	ctx := context.Background()

	setAt := time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC)
	tn := newTestTenant(t, db, "kcversion", true)
	tn.KeycloakVersion = "26.7.5"
	tn.KeycloakVersionSetAt = &setAt
	if err := repo.Create(ctx, tn); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByTenantID(ctx, tn.TenantID)
	if err != nil {
		t.Fatalf("GetByTenantID: %v", err)
	}
	if got.KeycloakVersion != "26.7.5" || got.KeycloakVersionSetAt == nil || !got.KeycloakVersionSetAt.Equal(setAt) {
		t.Fatalf("read back %q set at %v, want 26.7.5 set at %v", got.KeycloakVersion, got.KeycloakVersionSetAt, setAt)
	}

	got.KeycloakVersion = ""
	got.KeycloakVersionSetAt = nil
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = repo.GetByTenantID(ctx, tn.TenantID)
	if err != nil {
		t.Fatalf("GetByTenantID after update: %v", err)
	}
	if got.KeycloakVersion != "" || got.KeycloakVersionSetAt != nil {
		t.Errorf("removed version read back as %q set at %v", got.KeycloakVersion, got.KeycloakVersionSetAt)
	}
}
