//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// TestRepository_CoordinatesFollowTheSchema runs every query that reads
// coordinates on one repository while location_network goes through engine
// migration 5ae8978564f9 and back: without lat/long, with them, after the
// downgrade drops them, and with them again. Each query must keep working
// through the downgrade without a restart, and a failed login AMFA stored
// without a risk decision must be on the map exactly while the columns are
// there. A "no" is trusted for networkCoordinatesRecheck, so after each upgrade
// the test ages it instead of waiting. The repository prepares its statements,
// as the one KMT opens does.
//
// It copies AMFA's tables from the public schema into its own schema, which it
// creates and drops, so it needs AMFA_DESTRUCTIVE_TEST_DSN pointing at a
// database AMFA has migrated, like TestCheckAmfaSchema_Integration.
func TestRepository_CoordinatesFollowTheSchema(t *testing.T) {
	dsn := os.Getenv("AMFA_DESTRUCTIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("AMFA_DESTRUCTIVE_TEST_DSN not set; skipping destructive integration test for the coordinate columns")
	}
	testdb.RequireDisposable(t, dsn)
	ctx := context.Background()

	const schema = "kmt_coordinates_schema_test"
	admin, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	var tables int64
	if err := admin.Raw(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('auth_event', 'auth_process', 'auth_context', 'location_network')`).Scan(&tables).Error; err != nil {
		t.Fatalf("look for AMFA tables: %v", err)
	}
	if tables != 4 {
		t.Skip("AMFA_DESTRUCTIVE_TEST_DSN has no AMFA tables in public; point it at a database AMFA has migrated")
	}
	exec := func(stmt string, args ...interface{}) {
		t.Helper()
		if err := admin.Exec(stmt, args...).Error; err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE") })
	exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
	exec("CREATE SCHEMA " + schema)
	for _, table := range []string{"auth_event", "auth_process", "auth_context", "location_network"} {
		exec("CREATE TABLE " + schema + "." + table + " (LIKE public." + table + " INCLUDING DEFAULTS)")
	}
	// Start from the schema before the migration, whatever public has.
	exec("ALTER TABLE " + schema + ".location_network DROP COLUMN IF EXISTS lat, DROP COLUMN IF EXISTS long")

	const (
		realm   = "tdd-coordinates-schema-realm"
		eventID = "ffffffff-0000-0000-0000-000000000001"
		netHash = "tdd-coordinates-schema-network-hash"
	)
	eventTime := time.Now().UTC()
	exec(`INSERT INTO `+schema+`.location_network (hash, geolocation_cluster_label, country, is_vpn_flag, ip_address)
		VALUES (?, 0, 'NL', 0, '198.51.100.79')`, netHash)
	exec(`INSERT INTO `+schema+`.auth_event (id, event_type, event_time, realm_id, network_location_hash)
		VALUES (?, 'LOGIN_ERROR', ?, ?, ?)`, eventID, eventTime, realm, netHash)

	db, err := gorm.Open(gormpg.Open(dsn+" search_path="+schema), &gorm.Config{PrepareStmt: true})
	if err != nil {
		t.Fatalf("open test DB in %s: %v", schema, err)
	}
	repo := NewRepository(db)
	start, end := eventTime.Add(-time.Minute), eventTime.Add(time.Minute)
	since := eventTime.Add(-time.Minute)

	check := func(stage string, onTheMap bool) {
		t.Helper()
		if _, err := repo.ListRejectedEventsSince(ctx, realm, since); err != nil {
			t.Fatalf("%s: ListRejectedEventsSince: %v", stage, err)
		}
		if _, err := repo.ListVPNRiskyEventsSince(ctx, realm, since, 0); err != nil {
			t.Fatalf("%s: ListVPNRiskyEventsSince: %v", stage, err)
		}
		mirrored, err := repo.ListEventsSince(ctx, realm, since, 10)
		if err != nil {
			t.Fatalf("%s: ListEventsSince: %v", stage, err)
		}
		res, err := repo.ListEvents(ctx, amfa.ListEventsOptions{RealmID: realm, Limit: 10})
		if err != nil {
			t.Fatalf("%s: ListEvents: %v", stage, err)
		}
		buckets, err := repo.GetGeoBuckets(ctx, amfa.GeoOptions{RealmID: realm, StartTime: &start, EndTime: &end})
		if err != nil {
			t.Fatalf("%s: GetGeoBuckets: %v", stage, err)
		}
		for name, rows := range map[string][]amfa.EventRow{"ListEventsSince": mirrored, "ListEvents": res.Items} {
			if len(rows) != 1 || rows[0].IPAddress != "198.51.100.79" {
				t.Fatalf("%s: %s did not list the failed login: %+v", stage, name, rows)
			}
			placed := rows[0].Lat != nil && *rows[0].Lat == 52.3676 && rows[0].Long != nil && *rows[0].Long == 4.9041
			if placed != onTheMap || (!onTheMap && (rows[0].Lat != nil || rows[0].Long != nil)) {
				t.Fatalf("%s: %s coordinates = %v/%v, want them only while location_network has them",
					stage, name, rows[0].Lat, rows[0].Long)
			}
		}
		if onTheMap && (len(buckets) != 1 || buckets[0].Lat != 52.4 || buckets[0].Long != 4.9 || buckets[0].Count != 1) {
			t.Fatalf("%s: failed login not on the map: %+v", stage, buckets)
		}
		if !onTheMap && len(buckets) != 0 {
			t.Fatalf("%s: map has markers without coordinates: %+v", stage, buckets)
		}
	}
	upgrade := func() {
		exec("ALTER TABLE " + schema + ".location_network ADD COLUMN lat float8, ADD COLUMN long float8")
		exec("UPDATE "+schema+".location_network SET lat = 52.3676, long = 4.9041 WHERE hash = ?", netHash)
		repo.schema.checkedAt = time.Now().Add(-networkCoordinatesRecheck - time.Second)
	}

	check("before the migration", false)
	upgrade()
	check("after the migration", true)
	exec("ALTER TABLE " + schema + ".location_network DROP COLUMN lat, DROP COLUMN long")
	check("after the downgrade", false)
	upgrade()
	check("after upgrading again", true)
}
