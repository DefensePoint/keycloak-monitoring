// Coverage for the predicate symmetry between the two steps of
// ReconcileAMFAMerges. Shares the KMT_TEST_DATABASE_DSN-gated helper in
// rawdata_integration_test.go; skipped when that variable is unset.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// TestReconcileAMFAMerges_DoesNotDestroySoftDeletedAmfaRowUnmerged pins the
// invariant that the sweep never deletes an AMFA row it did not actually
// absorb.
//
// Step 1 (enrich) filters on `a.deleted_at IS NULL`. If step 2 (delete) does
// not carry the same filter, a soft-deleted AMFA row whose Keycloak twin is
// live gets hard-deleted having never been folded onto that twin, destroying
// its AMFA fields instead of merging them. No production path soft-deletes
// events rows today, so this guards a latent hazard rather than a live bug.
func TestReconcileAMFAMerges_DoesNotDestroySoftDeletedAmfaRowUnmerged(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	amfaEventID := fmt.Sprintf("softdel-corr-%d", time.Now().UnixNano())
	kcEventID := fmt.Sprintf("softdel-kc-%d", time.Now().UnixNano())
	amfaRowEventID := fmt.Sprintf("softdel-amfa-row-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Millisecond)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM events WHERE event_id IN (?, ?)`, kcEventID, amfaRowEventID)
	})

	// Live Keycloak twin, carrying none of the AMFA-owned fields yet.
	kc := &domain.Event{
		EventID:      kcEventID,
		Type:         "LOGIN",
		Source:       "keycloak:test-realm",
		SourceSystem: "keycloak",
		Status:       "processed",
		Timestamp:    now,
		AMFAEventID:  &amfaEventID,
	}
	if err := repo.Save(ctx, kc); err != nil {
		t.Fatalf("save keycloak event: %v", err)
	}

	country := "DE"
	isVPN := true
	amfa := &domain.Event{
		EventID:      amfaRowEventID,
		Type:         "LOGIN",
		Source:       "amfa:test-realm",
		SourceSystem: "amfa",
		Status:       "processed",
		Timestamp:    now,
		AMFAEventID:  &amfaEventID,
		Country:      &country,
		IsVPN:        &isVPN,
	}
	if err := repo.Save(ctx, amfa); err != nil {
		t.Fatalf("save amfa event: %v", err)
	}

	// Soft-delete the AMFA row via raw SQL: GORM's soft-delete scoping is what
	// step 1 already honours, and this sets the column the sweep predicates on.
	if err := db.Exec(`UPDATE events SET deleted_at = now() WHERE event_id = ?`, amfaRowEventID).Error; err != nil {
		t.Fatalf("soft-delete amfa row: %v", err)
	}

	absorbed, err := repo.ReconcileAMFAMerges(ctx)
	if err != nil {
		t.Fatalf("ReconcileAMFAMerges: %v", err)
	}
	if absorbed != 0 {
		t.Errorf("expected 0 rows absorbed (the only candidate AMFA row is soft-deleted), got %d", absorbed)
	}

	// The row must survive: it was never merged, so deleting it would destroy
	// data. Counted with raw SQL to see through GORM's soft-delete scoping.
	var present int64
	if err := db.Raw(`SELECT count(*) FROM events WHERE event_id = ?`, amfaRowEventID).Scan(&present).Error; err != nil {
		t.Fatalf("count amfa row: %v", err)
	}
	if present != 1 {
		t.Errorf("soft-deleted AMFA row was hard-deleted without being absorbed; its AMFA fields are now unrecoverable (found %d rows)", present)
	}

	// And the twin must not have been enriched from a logically-deleted row.
	twin, err := repo.GetByID(ctx, kcEventID)
	if err != nil {
		t.Fatalf("get keycloak twin: %v", err)
	}
	if twin.Country != nil {
		t.Errorf("keycloak twin was enriched from a soft-deleted AMFA row: country=%q", *twin.Country)
	}
	if twin.IsVPN != nil {
		t.Errorf("keycloak twin was enriched from a soft-deleted AMFA row: is_vpn=%v", *twin.IsVPN)
	}
}
