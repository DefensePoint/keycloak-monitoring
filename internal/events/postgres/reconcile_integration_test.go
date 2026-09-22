// Coverage for the AMFA<->Keycloak merge in ReconcileAMFAMerges and the
// Save upsert it depends on. This is the part of the events package that
// converges the two mirrors' independent writes onto one canonical row per
// login; it had no test coverage at all before this file, so a refactor of
// the exclusion list in Save or the SQL in ReconcileAMFAMerges could regress
// silently.
//
// Requires a real PostgreSQL instance. The connection DSN is taken from the
// KMT_TEST_DATABASE_DSN environment variable; the test is skipped when
// unset. Run with:
//
//	KMT_TEST_DATABASE_DSN="host=localhost port=55432 dbname=kmt_test user=kmt password=kmt sslmode=disable" \
//	    go test ./events/postgres/...
package postgres

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// newReconcileTestRepo opens a connection to KMT_TEST_DATABASE_DSN, ensures
// the events table exists, and returns a Repository plus the raw *gorm.DB for
// assertions the Repository interface doesn't expose (e.g. "does this row
// still exist at all").
func newReconcileTestRepo(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()

	dsn := os.Getenv("KMT_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_TEST_DATABASE_DSN not set; skipping integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	// Every table these suites touch, so a fresh database works. The
	// watermark tests used to fail on one because only Event was migrated
	// here, and they only passed locally on databases that already had the
	// table from an application run.
	if err := db.AutoMigrate(&database.Event{}, &database.AmfaMirrorWatermark{}); err != nil {
		t.Fatalf("failed to auto-migrate test tables: %v", err)
	}

	log := logger.New(zerolog.New(io.Discard))
	return &Repository{db: db, logger: log}, db
}

// testEventID returns a unique event_id namespaced to the test and a suffix,
// so concurrent/repeated runs never collide on the events.event_id unique
// index.
func testEventID(t *testing.T, suffix string) string {
	t.Helper()
	return fmt.Sprintf("reconcile-test-%s-%s-%d", t.Name(), suffix, time.Now().UnixNano())
}

func strPtr(s string) *string   { return &s }
func intPtr(i int) *int         { return &i }
func f64Ptr(f float64) *float64 { return &f }
func boolPtr(b bool) *bool      { return &b }

// cleanupEventID deletes a row by event_id, ignoring "already gone" — the
// whole point of these tests is that some rows get deleted mid-test.
func cleanupEventID(t *testing.T, db *gorm.DB, eventID string) {
	t.Helper()
	t.Cleanup(func() {
		db.Exec(`DELETE FROM events WHERE event_id = ?`, eventID)
	})
}

func TestReconcileAMFAMerges_MergesAmfaFieldsOntoKeycloakTwinAndDeletesAmfaRow(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	ctx := context.Background()

	amfaEventID := testEventID(t, "amfa-corr")
	kcEventID := testEventID(t, "kc")
	amfaRowEventID := testEventID(t, "amfa-row")
	now := time.Now().UTC().Truncate(time.Millisecond)

	cleanupEventID(t, db, kcEventID)
	cleanupEventID(t, db, amfaRowEventID)

	// Keycloak twin arrives first, with no risk_level of its own yet.
	kcEvent := &domain.Event{
		EventID:      kcEventID,
		Type:         "LOGIN",
		Category:     "authentication",
		Severity:     "info",
		Description:  "Keycloak LOGIN event",
		Source:       "keycloak:test-realm",
		SourceSystem: "keycloak",
		Status:       "processed",
		Timestamp:    now,
		AMFAEventID:  &amfaEventID,
	}
	if err := repo.Save(ctx, kcEvent); err != nil {
		t.Fatalf("save keycloak event: %v", err)
	}

	// AMFA row arrives second, carrying the AMFA-owned attributes.
	amfaEvent := &domain.Event{
		EventID:          amfaRowEventID,
		Type:             "LOGIN",
		Category:         "authentication",
		Severity:         "info",
		Description:      "AMFA LOGIN event in realm test-realm",
		Source:           "amfa:test-realm",
		SourceSystem:     "amfa",
		Status:           "processed",
		Timestamp:        now,
		AMFAEventID:      &amfaEventID,
		RiskLevel:        intPtr(3),
		FinalStatus:      strPtr("LOGIN"),
		IsVPN:            boolPtr(true),
		Country:          strPtr("DE"),
		City:             strPtr("Berlin"),
		Lat:              f64Ptr(52.52),
		Long:             f64Ptr(13.405),
		OperatingSystem:  strPtr("Linux"),
		Browser:          strPtr("Firefox"),
		Device:           strPtr("Desktop"),
		SystemLanguage:   strPtr("en-US"),
		ScreenResolution: strPtr("1920x1080"),
	}
	if err := repo.Save(ctx, amfaEvent); err != nil {
		t.Fatalf("save amfa event: %v", err)
	}

	absorbed, err := repo.ReconcileAMFAMerges(ctx)
	if err != nil {
		t.Fatalf("ReconcileAMFAMerges: %v", err)
	}
	if absorbed < 1 {
		t.Fatalf("expected at least 1 row absorbed, got %d", absorbed)
	}

	merged, err := repo.GetByID(ctx, kcEventID)
	if err != nil {
		t.Fatalf("get merged keycloak event: %v", err)
	}
	if merged.RiskLevel == nil || *merged.RiskLevel != 3 {
		t.Errorf("risk_level: expected 3 (from AMFA, since keycloak had none), got %v", merged.RiskLevel)
	}
	if merged.IsVPN == nil || !*merged.IsVPN {
		t.Errorf("is_vpn: expected true, got %v", merged.IsVPN)
	}
	if merged.Country == nil || *merged.Country != "DE" {
		t.Errorf("country: expected DE, got %v", merged.Country)
	}
	if merged.City == nil || *merged.City != "Berlin" {
		t.Errorf("city: expected Berlin, got %v", merged.City)
	}
	if merged.FinalStatus == nil || *merged.FinalStatus != "LOGIN" {
		t.Errorf("final_status: expected LOGIN, got %v", merged.FinalStatus)
	}
	if merged.Device == nil || *merged.Device != "Desktop" {
		t.Errorf("device: expected Desktop, got %v", merged.Device)
	}

	// The absorbed AMFA row must be gone (hard-deleted), not merely marked.
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM events WHERE event_id = ?`, amfaRowEventID).Scan(&count).Error; err != nil {
		t.Fatalf("count amfa row: %v", err)
	}
	if count != 0 {
		t.Errorf("expected the absorbed AMFA row to be deleted, found %d row(s)", count)
	}

	// Idempotent: running it again finds nothing left to absorb.
	absorbedAgain, err := repo.ReconcileAMFAMerges(ctx)
	if err != nil {
		t.Fatalf("second ReconcileAMFAMerges: %v", err)
	}
	if absorbedAgain != 0 {
		t.Errorf("expected 0 rows absorbed on the second run, got %d", absorbedAgain)
	}
}

func TestReconcileAMFAMerges_KeycloakRiskLevelWinsOverAmfa(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	ctx := context.Background()

	amfaEventID := testEventID(t, "amfa-corr")
	kcEventID := testEventID(t, "kc")
	amfaRowEventID := testEventID(t, "amfa-row")
	now := time.Now().UTC().Truncate(time.Millisecond)

	cleanupEventID(t, db, kcEventID)
	cleanupEventID(t, db, amfaRowEventID)

	// Keycloak already carries its own risk_level, stamped by the
	// adaptive-auth extension directly onto the login event.
	kcEvent := &domain.Event{
		EventID:      kcEventID,
		Type:         "LOGIN",
		Source:       "keycloak:test-realm",
		SourceSystem: "keycloak",
		Status:       "processed",
		Timestamp:    now,
		AMFAEventID:  &amfaEventID,
		RiskLevel:    intPtr(2),
	}
	if err := repo.Save(ctx, kcEvent); err != nil {
		t.Fatalf("save keycloak event: %v", err)
	}

	amfaEvent := &domain.Event{
		EventID:      amfaRowEventID,
		Type:         "LOGIN",
		Source:       "amfa:test-realm",
		SourceSystem: "amfa",
		Status:       "processed",
		Timestamp:    now,
		AMFAEventID:  &amfaEventID,
		RiskLevel:    intPtr(4),
	}
	if err := repo.Save(ctx, amfaEvent); err != nil {
		t.Fatalf("save amfa event: %v", err)
	}

	if _, err := repo.ReconcileAMFAMerges(ctx); err != nil {
		t.Fatalf("ReconcileAMFAMerges: %v", err)
	}

	merged, err := repo.GetByID(ctx, kcEventID)
	if err != nil {
		t.Fatalf("get merged keycloak event: %v", err)
	}
	if merged.RiskLevel == nil || *merged.RiskLevel != 2 {
		t.Errorf("risk_level: expected 2 (keycloak's own value wins), got %v", merged.RiskLevel)
	}
}

func TestReconcileAMFAMerges_LeavesAmfaOnlyEventsUntouched(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	ctx := context.Background()

	orphanAmfaEventID := testEventID(t, "amfa-corr-orphan")
	amfaRowEventID := testEventID(t, "amfa-row-orphan")
	now := time.Now().UTC().Truncate(time.Millisecond)

	cleanupEventID(t, db, amfaRowEventID)

	// No Keycloak twin exists for this amfa_event_id.
	amfaEvent := &domain.Event{
		EventID:      amfaRowEventID,
		Type:         "LOGIN",
		Source:       "amfa:test-realm",
		SourceSystem: "amfa",
		Status:       "processed",
		Timestamp:    now,
		AMFAEventID:  &orphanAmfaEventID,
		RiskLevel:    intPtr(1),
	}
	if err := repo.Save(ctx, amfaEvent); err != nil {
		t.Fatalf("save amfa event: %v", err)
	}

	if _, err := repo.ReconcileAMFAMerges(ctx); err != nil {
		t.Fatalf("ReconcileAMFAMerges: %v", err)
	}

	still, err := repo.GetByID(ctx, amfaRowEventID)
	if err != nil {
		t.Fatalf("expected the orphan AMFA row to still exist: %v", err)
	}
	if still.SourceSystem != "amfa" {
		t.Errorf("expected source_system to remain 'amfa', got %q", still.SourceSystem)
	}
}

// TestSave_KeycloakReupsertDoesNotClobberAmfaOwnedFields guards the exclusion
// list in Save's OnConflict DoUpdates. The Keycloak mirror re-upserts the
// same event_id on every poll cycle it still sees the event, and its own
// domain.Event never sets the AMFA-owned fields (is_vpn, country, etc. stay
// nil) — see keycloak/monitor.go. If those columns were ever added back to
// DoUpdates, this re-upsert would silently wipe out whatever
// ReconcileAMFAMerges had written.
func TestSave_KeycloakReupsertDoesNotClobberAmfaOwnedFields(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	ctx := context.Background()

	amfaEventID := testEventID(t, "amfa-corr")
	kcEventID := testEventID(t, "kc")
	amfaRowEventID := testEventID(t, "amfa-row")
	now := time.Now().UTC().Truncate(time.Millisecond)

	cleanupEventID(t, db, kcEventID)
	cleanupEventID(t, db, amfaRowEventID)

	kcEvent := &domain.Event{
		EventID:      kcEventID,
		Type:         "LOGIN",
		Source:       "keycloak:test-realm",
		SourceSystem: "keycloak",
		Status:       "processed",
		Timestamp:    now,
		AMFAEventID:  &amfaEventID,
	}
	if err := repo.Save(ctx, kcEvent); err != nil {
		t.Fatalf("save keycloak event: %v", err)
	}

	amfaEvent := &domain.Event{
		EventID:      amfaRowEventID,
		Type:         "LOGIN",
		Source:       "amfa:test-realm",
		SourceSystem: "amfa",
		Status:       "processed",
		Timestamp:    now,
		AMFAEventID:  &amfaEventID,
		IsVPN:        boolPtr(true),
		Country:      strPtr("DE"),
	}
	if err := repo.Save(ctx, amfaEvent); err != nil {
		t.Fatalf("save amfa event: %v", err)
	}
	if _, err := repo.ReconcileAMFAMerges(ctx); err != nil {
		t.Fatalf("ReconcileAMFAMerges: %v", err)
	}

	// Simulate the Keycloak monitor re-polling and re-saving the same login
	// event on its next cycle. Its domain.Event never carries the AMFA
	// fields, matching keycloak/monitor.go's generalEvent construction.
	kcReupsert := &domain.Event{
		EventID:      kcEventID,
		Type:         "LOGIN",
		Source:       "keycloak:test-realm",
		SourceSystem: "keycloak",
		Status:       "processed",
		Timestamp:    now,
		AMFAEventID:  &amfaEventID,
	}
	if err := repo.Save(ctx, kcReupsert); err != nil {
		t.Fatalf("re-save keycloak event: %v", err)
	}

	after, err := repo.GetByID(ctx, kcEventID)
	if err != nil {
		t.Fatalf("get event after re-upsert: %v", err)
	}
	if after.IsVPN == nil || !*after.IsVPN {
		t.Errorf("is_vpn: expected the merged value (true) to survive the Keycloak re-upsert, got %v", after.IsVPN)
	}
	if after.Country == nil || *after.Country != "DE" {
		t.Errorf("country: expected the merged value (DE) to survive the Keycloak re-upsert, got %v", after.Country)
	}
}
