// Coverage for the per-tenant AMFA mirror watermark. Shares the
// KMT_TEST_DATABASE_DSN-gated helper in rawdata_integration_test.go; skipped
// when that variable is unset.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestAmfaMirrorWatermark_IsolatesTenantsSharingARealmName is the regression
// test for the bug this table exists to fix.
//
// Realm names are unique per-tenant, not globally, and every Keycloak instance
// has a "master" realm. The mirror used to derive its position from
// MAX(timestamp) over events WHERE source = 'amfa:<realm>', which carries no
// tenant, so two tenants monitoring a realm of the same name shared one
// position: whichever polled second inherited the other's newer timestamp and
// skipped its own older events permanently.
func TestAmfaMirrorWatermark_IsolatesTenantsSharingARealmName(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	tenantA := fmt.Sprintf("wm-tenantA-%d", stamp)
	tenantB := fmt.Sprintf("wm-tenantB-%d", stamp)
	realm := "master" // the name every Keycloak shares
	t.Cleanup(func() {
		db.Exec(`DELETE FROM amfa_mirror_watermarks WHERE tenant_id IN (?, ?)`, tenantA, tenantB)
	})

	posA := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	posB := time.Now().UTC().Truncate(time.Second) // tenant B is far ahead

	if err := repo.SaveAmfaMirrorWatermark(ctx, tenantA, realm, posA); err != nil {
		t.Fatalf("save tenant A: %v", err)
	}
	if err := repo.SaveAmfaMirrorWatermark(ctx, tenantB, realm, posB); err != nil {
		t.Fatalf("save tenant B: %v", err)
	}

	gotA, err := repo.AmfaMirrorWatermark(ctx, tenantA, realm)
	if err != nil {
		t.Fatalf("read tenant A: %v", err)
	}
	gotB, err := repo.AmfaMirrorWatermark(ctx, tenantB, realm)
	if err != nil {
		t.Fatalf("read tenant B: %v", err)
	}

	if !gotA.UTC().Equal(posA) {
		t.Errorf("tenant A position: want %s, got %s. Tenant A must not inherit tenant B's %s, "+
			"or it silently skips its own older events.",
			posA.Format(time.RFC3339), gotA.UTC().Format(time.RFC3339), posB.Format(time.RFC3339))
	}
	if !gotB.UTC().Equal(posB) {
		t.Errorf("tenant B position: want %s, got %s", posB.Format(time.RFC3339), gotB.UTC().Format(time.RFC3339))
	}
}

// A tenant and realm with no recorded position reads as the zero time, which is
// what makes the mirror fall back to its backfill window rather than to the
// epoch or to another tenant's progress.
func TestAmfaMirrorWatermark_UnknownTenantRealmIsZero(t *testing.T) {
	repo, _ := newRawDataTestRepo(t)
	ctx := context.Background()

	got, err := repo.AmfaMirrorWatermark(ctx,
		fmt.Sprintf("wm-absent-%d", time.Now().UnixNano()), "master")
	if err != nil {
		t.Fatalf("read absent watermark: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected the zero time for an unrecorded (tenant, realm), got %s", got)
	}
}

// The upsert is monotonic so a stale writer cannot rewind a newer position.
// Rewinding would only cause harmless re-reads, but never skips, and this pins
// which direction is safe.
func TestAmfaMirrorWatermark_SaveIsMonotonic(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	tenant := fmt.Sprintf("wm-mono-%d", time.Now().UnixNano())
	realm := "master"
	t.Cleanup(func() {
		db.Exec(`DELETE FROM amfa_mirror_watermarks WHERE tenant_id = ?`, tenant)
	})

	newer := time.Now().UTC().Truncate(time.Second)
	older := newer.Add(-time.Hour)

	if err := repo.SaveAmfaMirrorWatermark(ctx, tenant, realm, newer); err != nil {
		t.Fatalf("save newer: %v", err)
	}
	// A stale writer tries to move the position backwards.
	if err := repo.SaveAmfaMirrorWatermark(ctx, tenant, realm, older); err != nil {
		t.Fatalf("save older: %v", err)
	}

	got, err := repo.AmfaMirrorWatermark(ctx, tenant, realm)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !got.UTC().Equal(newer) {
		t.Errorf("expected the newer position %s to survive a stale write, got %s",
			newer.Format(time.RFC3339), got.UTC().Format(time.RFC3339))
	}
}
