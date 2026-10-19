package postgres

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// The two mirrors write independently, so a Keycloak row can land at any
// moment of a reconcile sweep. One that lands after the AMFA fields have been
// copied onto the existing twins, but before the absorbed AMFA rows are
// removed, must not cost its AMFA twin's data: either the sweep folds that row
// onto it, or it leaves the AMFA row for the next sweep.

func TestReconcileAMFAMerges_AKeycloakTwinArrivingMidSweepLosesNothing(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	ctx := context.Background()

	amfaEventID := testEventID(t, "amfa-corr")
	kcEventID := testEventID(t, "kc")
	amfaRowEventID := testEventID(t, "amfa-row")
	now := time.Now().UTC().Truncate(time.Millisecond)
	cleanupEventID(t, db, kcEventID)
	cleanupEventID(t, db, amfaRowEventID)

	// The AMFA half is already stored; its Keycloak twin is not yet.
	if err := repo.Save(ctx, &domain.Event{
		TenantID: fixtureTenant, EventID: amfaRowEventID, Type: "LOGIN", Category: "authentication",
		Severity: "info", Description: "AMFA LOGIN", Source: "amfa:test-realm", SourceSystem: "amfa",
		Status: "processed", Timestamp: now, AMFAEventID: &amfaEventID,
		Country: strPtr("DE"), City: strPtr("Berlin"), Device: strPtr("Desktop"), Browser: strPtr("Firefox"),
	}); err != nil {
		t.Fatalf("save amfa event: %v", err)
	}

	// The Keycloak monitor saves the twin right after the sweep's enrich
	// statement runs, as it can at any moment.
	var fired atomic.Bool
	insertTwin := func(tx *gorm.DB) {
		if !strings.Contains(tx.Statement.SQL.String(), "UPDATE events AS k") || !fired.CompareAndSwap(false, true) {
			return
		}
		other := &Repository{db: db.Session(&gorm.Session{NewDB: true}), logger: repo.logger}
		if err := other.Save(context.Background(), &domain.Event{
			TenantID: fixtureTenant, EventID: kcEventID, Type: "LOGIN", Category: "authentication",
			Severity: "info", Description: "Keycloak LOGIN", Source: "keycloak:test-realm",
			SourceSystem: "keycloak", Status: "processed", Timestamp: now, AMFAEventID: &amfaEventID,
		}); err != nil {
			t.Errorf("save keycloak twin mid-sweep: %v", err)
		}
	}
	if err := db.Callback().Raw().After("gorm:raw").Register("test:keycloak-twin-mid-sweep", insertTwin); err != nil {
		t.Fatalf("register callback: %v", err)
	}

	if _, err := repo.ReconcileAMFAMerges(ctx); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if !fired.Load() {
		t.Fatal("the twin was never inserted mid-sweep, so this test proves nothing")
	}
	// The next poll's sweep.
	if _, err := repo.ReconcileAMFAMerges(ctx); err != nil {
		t.Fatalf("second sweep: %v", err)
	}

	merged, err := repo.GetByTenantAndEventID(ctx, fixtureTenant, kcEventID)
	if err != nil {
		t.Fatalf("get keycloak twin: %v", err)
	}
	if merged.Country == nil || *merged.Country != "DE" || merged.Device == nil || *merged.Device != "Desktop" {
		var amfaRows int64
		db.Raw(`SELECT COUNT(*) FROM events WHERE event_id = ?`, amfaRowEventID).Scan(&amfaRows)
		t.Errorf("the AMFA geo and device data was lost: twin has country=%v device=%v, and %d AMFA row(s) remain",
			merged.Country, merged.Device, amfaRows)
	}
}

// Every tenant's mirror sweeps the whole table. A sweep that finds another
// one running leaves the rows to it rather than repeating it, and the rows are
// merged once the lock is free.
func TestReconcileAMFAMerges_OneSweepAtATime(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	ctx := context.Background()

	amfaEventID := testEventID(t, "amfa-corr")
	kcEventID := testEventID(t, "kc")
	amfaRowEventID := testEventID(t, "amfa-row")
	now := time.Now().UTC().Truncate(time.Millisecond)
	cleanupEventID(t, db, kcEventID)
	cleanupEventID(t, db, amfaRowEventID)
	for _, e := range []*domain.Event{
		{TenantID: fixtureTenant, EventID: kcEventID, Type: "LOGIN", Category: "authentication", Severity: "info",
			Description: "Keycloak LOGIN", Source: "keycloak:test-realm", SourceSystem: "keycloak", Status: "processed",
			Timestamp: now, AMFAEventID: &amfaEventID},
		{TenantID: fixtureTenant, EventID: amfaRowEventID, Type: "LOGIN", Category: "authentication", Severity: "info",
			Description: "AMFA LOGIN", Source: "amfa:test-realm", SourceSystem: "amfa", Status: "processed",
			Timestamp: now, AMFAEventID: &amfaEventID, Country: strPtr("DE")},
	} {
		if err := repo.Save(ctx, e); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	// Another sweep is running: it holds the lock in its own transaction.
	other := db.Begin()
	if err := other.Exec(`SELECT pg_advisory_xact_lock(hashtext(?)::bigint)`, amfaReconcileLockKey).Error; err != nil {
		t.Fatalf("take the lock: %v", err)
	}
	absorbed, err := repo.ReconcileAMFAMerges(ctx)
	if err != nil || absorbed != 0 {
		t.Fatalf("sweep while another runs = %d, %v; want 0, nil", absorbed, err)
	}
	other.Rollback()

	absorbed, err = repo.ReconcileAMFAMerges(ctx)
	if err != nil || absorbed != 1 {
		t.Fatalf("sweep once the lock is free = %d, %v; want 1, nil", absorbed, err)
	}
	merged, err := repo.GetByTenantAndEventID(ctx, fixtureTenant, kcEventID)
	if err != nil || merged.Country == nil || *merged.Country != "DE" {
		t.Errorf("twin after the sweep: %+v, %v", merged, err)
	}
}
