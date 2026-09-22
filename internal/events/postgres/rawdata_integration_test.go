// Regression coverage for preserving raw_data through the AMFA/Keycloak
// reconcile merge (see the fix in ReconcileAMFAMerges's enrich UPDATE and the
// Save DoUpdates exclusion list).
//
// Requires a real PostgreSQL instance. The connection DSN is taken from the
// KMT_TEST_DATABASE_DSN environment variable; the test is skipped when
// unset. Run with:
//
//	KMT_TEST_DATABASE_DSN="host=localhost port=55432 dbname=kmt_test user=kmt password=kmt sslmode=disable" \
//	    go test ./events/postgres/... -run TestReconcileAMFAMerges_PreservesRawData
package postgres

import (
	"context"
	"encoding/json"
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
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// assertSameJSON compares two JSON documents by value, not by byte layout:
// the raw_data column is jsonb, and Postgres re-serializes it on storage
// (key order, spacing), so a literal string comparison against what we wrote
// would fail even when the merge preserved every field correctly.
func assertSameJSON(t *testing.T, context string, want, got string) {
	t.Helper()
	var wantVal, gotVal any
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("%s: invalid want JSON: %v", context, err)
	}
	if err := json.Unmarshal([]byte(got), &gotVal); err != nil {
		t.Fatalf("%s: raw_data is not valid JSON: %v (got %q)", context, err, got)
	}
	wantJSON, _ := json.Marshal(wantVal)
	gotJSON, _ := json.Marshal(gotVal)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("%s: expected JSON equivalent to %s, got %s", context, wantJSON, gotJSON)
	}
}

func newRawDataTestRepo(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()

	dsn := os.Getenv("KMT_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_TEST_DATABASE_DSN not set; skipping integration test")
	}

	testdb.ShareDatabase(t, dsn)

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

func TestReconcileAMFAMerges_PreservesRawData(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	amfaEventID := fmt.Sprintf("rawdata-test-corr-%d", time.Now().UnixNano())
	kcEventID := fmt.Sprintf("rawdata-test-kc-%d", time.Now().UnixNano())
	amfaRowEventID := fmt.Sprintf("rawdata-test-amfa-row-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Millisecond)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM events WHERE event_id IN (?, ?)`, kcEventID, amfaRowEventID)
	})

	const amfaPayload = `{"amfa_event_id":"abc-123","risk_level":3,"country":"DE"}`

	kcEvent := &domain.Event{
		EventID:      kcEventID,
		Type:         "LOGIN",
		Source:       "keycloak:test-realm",
		SourceSystem: "keycloak",
		Status:       "processed",
		Timestamp:    now,
		AMFAEventID:  &amfaEventID,
		RawData:      "", // keycloak/monitor.go always saves an empty raw_data
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
		RawData:      amfaPayload,
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
	assertSameJSON(t, "raw_data after merge", amfaPayload, merged.RawData)

	// A later Keycloak re-upsert of the same event (its domain.Event always
	// carries RawData: "") must not wipe the merged payload back out.
	kcReupsert := &domain.Event{
		EventID:      kcEventID,
		Type:         "LOGIN",
		Source:       "keycloak:test-realm",
		SourceSystem: "keycloak",
		Status:       "processed",
		Timestamp:    now,
		AMFAEventID:  &amfaEventID,
		RawData:      "",
	}
	if err := repo.Save(ctx, kcReupsert); err != nil {
		t.Fatalf("re-save keycloak event: %v", err)
	}

	after, err := repo.GetByID(ctx, kcEventID)
	if err != nil {
		t.Fatalf("get event after re-upsert: %v", err)
	}
	assertSameJSON(t, "raw_data after keycloak re-upsert", amfaPayload, after.RawData)
}
