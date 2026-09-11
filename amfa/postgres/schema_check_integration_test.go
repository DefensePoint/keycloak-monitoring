//go:build integration

// Build tag is intentional: TestCheckAmfaSchema_Integration runs destructive
// SQL (DROP TABLE alembic_version) against the database pointed at by
// AMFA_DESTRUCTIVE_TEST_DSN. Without the tag this would run on a plain
// `go test ./...` invocation and could wipe alembic state on a developer's AMFA
// dev DB. To exercise it explicitly, run:
//
//	AMFA_DESTRUCTIVE_TEST_DSN="..." go test -tags=integration ./amfa/postgres/...

package postgres

import (
	"os"
	"testing"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// TestCheckAmfaSchema_Integration exercises the function against a real
// PostgreSQL instance. It is skipped unless AMFA_DESTRUCTIVE_TEST_DSN is set;
// the build tag above is the primary safety guard.
//
// The variable is deliberately not AMFA_TEST_DSN, which the read-only
// repository tests use: this test drops AMFA's alembic_version bookkeeping
// table, so it needs a database nobody minds losing.
//
// Example to run locally:
//
//	AMFA_DESTRUCTIVE_TEST_DSN="host=localhost port=5432 dbname=amfa_test user=postgres password=postgres sslmode=disable" \
//	    go test -tags=integration ./amfa/postgres/ -run TestCheckAmfaSchema_Integration -v
//
// The DSN must point at a database where you can CREATE TABLE.
func TestCheckAmfaSchema_Integration(t *testing.T) {
	dsn := os.Getenv("AMFA_DESTRUCTIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("AMFA_DESTRUCTIVE_TEST_DSN not set; skipping destructive integration test for CheckAmfaSchema")
	}
	testdb.RequireDisposable(t, dsn)

	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	// Best-effort teardown.
	t.Cleanup(func() {
		_ = db.Exec("DROP TABLE IF EXISTS alembic_version").Error
		sqlDB, derr := db.DB()
		if derr == nil {
			_ = sqlDB.Close()
		}
	})

	// Fresh table for the test.
	if err := db.Exec("DROP TABLE IF EXISTS alembic_version").Error; err != nil {
		t.Fatalf("failed to drop alembic_version: %v", err)
	}

	log := logger.NewNoop()

	// Case 1: table missing — function must return nil and merely log a warning.
	if err := CheckAmfaSchema(db, "c00d6d7d197c", log); err != nil {
		t.Errorf("expected nil error when table missing, got %v", err)
	}

	// Create the table for the remaining cases.
	if err := db.Exec("CREATE TABLE alembic_version (version_num VARCHAR(32) PRIMARY KEY)").Error; err != nil {
		t.Fatalf("failed to create alembic_version: %v", err)
	}

	// Case 2: drift — actual differs from expected; must still return nil.
	if err := db.Exec("INSERT INTO alembic_version VALUES ('different_hash')").Error; err != nil {
		t.Fatalf("failed to insert drifted version: %v", err)
	}
	if err := CheckAmfaSchema(db, "c00d6d7d197c", log); err != nil {
		t.Errorf("expected nil error on drift, got %v", err)
	}

	// Case 3: match — replace the row with the matching version and retry.
	if err := db.Exec("DELETE FROM alembic_version").Error; err != nil {
		t.Fatalf("failed to clear alembic_version: %v", err)
	}
	if err := db.Exec("INSERT INTO alembic_version VALUES ('c00d6d7d197c')").Error; err != nil {
		t.Fatalf("failed to insert matching version: %v", err)
	}
	if err := CheckAmfaSchema(db, "c00d6d7d197c", log); err != nil {
		t.Errorf("expected nil error on match, got %v", err)
	}
}
