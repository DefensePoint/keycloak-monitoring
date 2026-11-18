//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// TestNetworkCoordinates_FollowsAnUpgrade checks the lat/long probe against a
// real schema: an AMFA without the columns answers no, an upgrade is picked up
// once the "no" is older than networkCoordinatesRecheck, a "yes" is kept, and a
// failed probe answers no without being remembered.
//
// It creates and drops its own schema, so it needs AMFA_DESTRUCTIVE_TEST_DSN,
// like TestCheckAmfaSchema_Integration.
func TestNetworkCoordinates_FollowsAnUpgrade(t *testing.T) {
	dsn := os.Getenv("AMFA_DESTRUCTIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("AMFA_DESTRUCTIVE_TEST_DSN not set; skipping destructive integration test for the coordinates probe")
	}
	testdb.RequireDisposable(t, dsn)
	ctx := context.Background()

	const schema = "kmt_coordinates_probe_test"
	admin, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE") })
	for _, stmt := range []string{
		"DROP SCHEMA IF EXISTS " + schema + " CASCADE",
		"CREATE SCHEMA " + schema,
		"CREATE TABLE " + schema + ".location_network (hash text PRIMARY KEY, ip_address text)",
	} {
		if err := admin.Exec(stmt).Error; err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	db, err := gorm.Open(gormpg.Open(dsn+" search_path="+schema), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test DB in %s: %v", schema, err)
	}
	repo := NewRepository(db)

	if repo.networkCoordinates(ctx) {
		t.Fatal("an AMFA without lat/long reported them")
	}

	if err := admin.Exec("ALTER TABLE " + schema + ".location_network ADD COLUMN lat float8, ADD COLUMN long float8").Error; err != nil {
		t.Fatalf("add columns: %v", err)
	}
	if repo.networkCoordinates(ctx) {
		t.Fatal("a recent no was checked again on every query")
	}
	repo.schema.checkedAt = time.Now().Add(-networkCoordinatesRecheck - time.Second)
	if !repo.networkCoordinates(ctx) {
		t.Fatal("an AMFA upgraded while KMT runs was not picked up after the recheck interval")
	}

	if err := admin.Exec("ALTER TABLE " + schema + ".location_network DROP COLUMN long").Error; err != nil {
		t.Fatalf("drop column: %v", err)
	}
	if !repo.networkCoordinates(ctx) {
		t.Fatal("a yes was checked again")
	}

	broken, err := gorm.Open(gormpg.Open(dsn+" search_path="+schema), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, _ := broken.DB()
	_ = sqlDB.Close()
	failing := NewRepository(broken)
	if failing.networkCoordinates(ctx) {
		t.Fatal("a failed probe answered yes")
	}
	if !failing.schema.checkedAt.IsZero() {
		t.Fatal("a failed probe was remembered")
	}
}
