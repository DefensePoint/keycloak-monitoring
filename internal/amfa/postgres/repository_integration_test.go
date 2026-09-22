//go:build integration
// +build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// openAmfaTestDB connects to the AMFA test database identified by the
// AMFA_TEST_DSN environment variable, skipping the test if the variable is
// unset. Example DSN:
//
//	host=localhost port=5455 dbname=adaptive_mfa user=keycloak password=password sslmode=disable
func openAmfaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AMFA_TEST_DSN")
	if dsn == "" {
		t.Skip("AMFA_TEST_DSN not set; skipping integration test. " +
			"Example: 'host=localhost port=5455 dbname=adaptive_mfa user=keycloak password=password sslmode=disable'")
	}

	testdb.ShareDatabase(t, dsn)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect to AMFA DB: %v", err)
	}
	return db
}

func TestRepository_ListEvents_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.ListEvents(context.Background(), amfa.ListEventsOptions{Limit: 10})
	if err == nil {
		t.Error("expected error when RealmID is empty")
	}
}

func TestRepository_GetStats_AllRealmsWhenRealmEmpty(t *testing.T) {
	// An empty RealmID means "all realms": GetStats drops the realm filter and
	// aggregates across every realm in the AMFA DB, so it must succeed (no error)
	// rather than reject the request the way ListEvents/GetGeoBuckets do.
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.GetStats(context.Background(), amfa.StatsOptions{})
	if err != nil {
		t.Errorf("GetStats with empty RealmID (all realms): unexpected error %v", err)
	}
}

func TestRepository_GetGeoBuckets_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.GetGeoBuckets(context.Background(), amfa.GeoOptions{})
	if err == nil {
		t.Error("expected error when RealmID is empty")
	}
}

// TestRepository_ListEvents_SmokeQuery exercises the full ListEvents query
// path against a real AMFA database. It only checks that the query is
// syntactically valid and returns a result; it does not assert on contents
// because the test database may be empty.
func TestRepository_ListEvents_SmokeQuery(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	res, err := repo.ListEvents(context.Background(), amfa.ListEventsOptions{
		RealmID: "master",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.Total < 0 {
		t.Errorf("expected total >= 0, got %d", res.Total)
	}
}

// TestRepository_GetStats_SmokeQuery exercises the GetStats query path.
func TestRepository_GetStats_SmokeQuery(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	stats, err := repo.GetStats(context.Background(), amfa.StatsOptions{
		RealmID: "master",
	})
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.Total < 0 || stats.Risky < 0 || stats.UniqueUsers < 0 || stats.FlaggedIPs < 0 {
		t.Errorf("expected non-negative stats, got %+v", stats)
	}
}

// TestRepository_GetGeoBuckets_SmokeQuery exercises the GetGeoBuckets query
// path.
func TestRepository_GetGeoBuckets_SmokeQuery(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	buckets, err := repo.GetGeoBuckets(context.Background(), amfa.GeoOptions{
		RealmID: "master",
	})
	if err != nil {
		t.Fatalf("GetGeoBuckets: %v", err)
	}
	// buckets may be empty if the test DB has no geo data; just verify the
	// query executed successfully.
	_ = buckets
}

// TestRepository_ListEvents_SourcesContextFromJSON verifies that client, IP,
// country, geo, and VPN are populated from auth_process.auth_context_json when
// the denormalized auth_context table has no matching row. The adaptive-auth
// service never writes auth_context rows (its AuthContextFactory is a stub), so
// the events list must fall back to the JSON snapshot it always writes.
//
// It seeds a uniquely-realmed auth_process + auth_event (reusing existing
// device/location_network/config rows to satisfy FKs) whose auth_context_hash
// intentionally matches no auth_context row, then asserts the fields resolve
// from the JSON. The seed is removed on cleanup.
func TestRepository_ListEvents_SourcesContextFromJSON(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	// Borrow FK-satisfying values from any existing process so we don't have to
	// seed device/location_network/decision_params_config ourselves.
	var deviceHash, netHash, configID string
	row := db.Raw(`SELECT device_info_hash, network_location_hash, parameters_config_id::text
	               FROM auth_process LIMIT 1`).Row()
	if err := row.Scan(&deviceHash, &netHash, &configID); err != nil {
		t.Skipf("no existing auth_process to borrow FK values from: %v", err)
	}

	const (
		realm   = "tdd-json-realm"
		procID  = "aaaaaaaa-0000-0000-0000-000000000001"
		eventID = "aaaaaaaa-0000-0000-0000-000000000002"
		userID  = "aaaaaaaa-0000-0000-0000-000000000003"
		ctxHash = "tdd-json-context-hash-matches-no-auth_context-row"
	)
	// Everything the projection should recover lives only in this JSON.
	const ctxJSON = `{"realm_id":"tdd-json-realm","client":"tdd-client",` +
		`"ip_address":"10.11.12.13","lat":12.5,"long":-8.25,` +
		`"is_vpn":true,"country_name":"Testland"}`
	now := time.Now().UTC()

	t.Cleanup(func() {
		db.Exec(`DELETE FROM auth_event WHERE id = ?`, eventID)
		db.Exec(`DELETE FROM auth_process WHERE id = ?`, procID)
	})

	if err := db.Exec(`
		INSERT INTO auth_process
		  (id, user_id, auth_context_hash, device_info_hash, network_location_hash,
		   auth_context_json, pre_auth_risk_decision, parameters_config_id,
		   final_status, started_at)
		VALUES (?, ?, ?, ?, ?, ?::json, ?, ?, ?, ?)`,
		procID, userID, ctxHash, deviceHash, netHash, ctxJSON, 2, configID, "LOGIN", now,
	).Error; err != nil {
		t.Fatalf("seed auth_process: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO auth_event
		  (id, auth_process, user_id, event_type, event_time,
		   auth_context_hash, device_info_hash, network_location_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID, procID, userID, "LOGIN", now, ctxHash, deviceHash, netHash,
	).Error; err != nil {
		t.Fatalf("seed auth_event: %v", err)
	}

	res, err := repo.ListEvents(ctx, amfa.ListEventsOptions{RealmID: realm, Limit: 10})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	var got *amfa.EventRow
	for i := range res.Items {
		if res.Items[i].EventID == eventID {
			got = &res.Items[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("seeded event %s not returned (total=%d)", eventID, res.Total)
	}

	if got.Client != "tdd-client" {
		t.Errorf("Client = %q, want %q (from auth_context_json)", got.Client, "tdd-client")
	}
	if got.IPAddress != "10.11.12.13" {
		t.Errorf("IPAddress = %q, want %q (from auth_context_json)", got.IPAddress, "10.11.12.13")
	}
	if got.Country == nil || *got.Country != "Testland" {
		t.Errorf("Country = %v, want %q (from auth_context_json)", got.Country, "Testland")
	}
	if got.Lat == nil || *got.Lat != 12.5 {
		t.Errorf("Lat = %v, want 12.5 (from auth_context_json)", got.Lat)
	}
	if got.Long == nil || *got.Long != -8.25 {
		t.Errorf("Long = %v, want -8.25 (from auth_context_json)", got.Long)
	}
	if !got.IsVPN {
		t.Errorf("IsVPN = false, want true (from auth_context_json)")
	}
}

// TestRepository_ListEvents_PrefersAuthContextTableForDeviceFields covers the
// opposite case from TestRepository_ListEvents_SourcesContextFromJSON: a login
// whose auth_context_hash DOES match a real auth_context row. The five
// device/agent fields (OS, browser, device, system language, screen
// resolution) must read from that row, not the auth_context_json snapshot -
// exercising exactly the gap the parity_integration_test.go suite (run
// against the AMFA HTTP API) caught: this repository used to read those five
// fields from the JSON unconditionally, diverging from the API's own
// AuthContext-table-first behavior whenever a real row existed.
func TestRepository_ListEvents_PrefersAuthContextTableForDeviceFields(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	var deviceHash, netHash, configID string
	row := db.Raw(`SELECT device_info_hash, network_location_hash, parameters_config_id::text
	               FROM auth_process LIMIT 1`).Row()
	if err := row.Scan(&deviceHash, &netHash, &configID); err != nil {
		t.Skipf("no existing auth_process to borrow FK values from: %v", err)
	}

	const (
		realm   = "tdd-authctx-table-realm"
		procID  = "bbbbbbbb-0000-0000-0000-000000000001"
		eventID = "bbbbbbbb-0000-0000-0000-000000000002"
		userID  = "bbbbbbbb-0000-0000-0000-000000000003"
		ctxHash = "tdd-authctx-table-hash"
	)
	// The JSON snapshot carries deliberately different, stale-looking values, so
	// a test that reads them instead of the table fails loudly rather than by
	// coincidence.
	const ctxJSON = `{"realm_id":"tdd-authctx-table-realm",` +
		`"operating_system":"JSON-STALE-OS","browser":"JSON-STALE-BROWSER",` +
		`"device":"JSON-STALE-DEVICE","system_language":"JSON-STALE-LANG",` +
		`"screen_resolution":"JSON-STALE-RES"}`
	now := time.Now().UTC()

	t.Cleanup(func() {
		db.Exec(`DELETE FROM auth_event WHERE id = ?`, eventID)
		db.Exec(`DELETE FROM auth_process WHERE id = ?`, procID)
		db.Exec(`DELETE FROM auth_context WHERE hash = ?`, ctxHash)
	})

	if err := db.Exec(`
		INSERT INTO auth_context
		  (hash, client, ip_address, system_language, screen_resolution,
		   operating_system, browser, device)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ctxHash, "table-client", "203.0.113.9", "fr-FR", "2560x1440",
		"macOS", "Safari", "table-device",
	).Error; err != nil {
		t.Fatalf("seed auth_context: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO auth_process
		  (id, user_id, auth_context_hash, device_info_hash, network_location_hash,
		   auth_context_json, pre_auth_risk_decision, parameters_config_id,
		   final_status, started_at)
		VALUES (?, ?, ?, ?, ?, ?::json, ?, ?, ?, ?)`,
		procID, userID, ctxHash, deviceHash, netHash, ctxJSON, 2, configID, "LOGIN", now,
	).Error; err != nil {
		t.Fatalf("seed auth_process: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO auth_event
		  (id, auth_process, user_id, event_type, event_time,
		   auth_context_hash, device_info_hash, network_location_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID, procID, userID, "LOGIN", now, ctxHash, deviceHash, netHash,
	).Error; err != nil {
		t.Fatalf("seed auth_event: %v", err)
	}

	res, err := repo.ListEvents(ctx, amfa.ListEventsOptions{RealmID: realm, Limit: 10})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	var got *amfa.EventRow
	for i := range res.Items {
		if res.Items[i].EventID == eventID {
			got = &res.Items[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("seeded event %s not returned (total=%d)", eventID, res.Total)
	}

	checks := []struct {
		name string
		got  *string
		want string
	}{
		{"OperatingSystem", got.OperatingSystem, "macOS"},
		{"Browser", got.Browser, "Safari"},
		{"Device", got.Device, "table-device"},
		{"SystemLanguage", got.SystemLanguage, "fr-FR"},
		{"ScreenResolution", got.ScreenResolution, "2560x1440"},
	}
	for _, c := range checks {
		if c.got == nil || *c.got != c.want {
			t.Errorf("%s = %v, want %q (from auth_context, not the JSON snapshot)", c.name, c.got, c.want)
		}
	}
}

// TestRepository_ListEvents_FindsOrphanEventByRealmIDColumn covers the gap
// found by actually running a real login through Keycloak's SPI + this
// engine end to end: a LOGIN_ERROR fired before the user ever reached the
// password prompt (e.g. an invalid_redirect_uri) has no auth_process at all,
// so it has no auth_context_json snapshot to read a realm from either. AMFA's
// login-event webhook still stamps auth_event.realm_id directly for exactly
// this reason. Before realmFilterExpr existed, every query here filtered
// only on auth_process's JSON snapshot and silently could not see this row -
// confirmed live: AMFA's own HTTP API returned it, this repository did not.
func TestRepository_ListEvents_FindsOrphanEventByRealmIDColumn(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	const (
		realm   = "tdd-orphan-realm"
		eventID = "cccccccc-0000-0000-0000-000000000001"
	)
	eventTime := time.Now().UTC()

	t.Cleanup(func() {
		db.Exec(`DELETE FROM auth_event WHERE id = ?`, eventID)
	})

	// No auth_process, no auth_context_hash: exactly what a pre-password
	// LOGIN_ERROR looks like. Only auth_event.realm_id carries the realm.
	if err := db.Exec(`
		INSERT INTO auth_event (id, event_type, event_time, realm_id)
		VALUES (?, ?, ?, ?)`,
		eventID, "LOGIN_ERROR", eventTime, realm,
	).Error; err != nil {
		t.Fatalf("seed orphan auth_event: %v", err)
	}

	res, err := repo.ListEvents(ctx, amfa.ListEventsOptions{RealmID: realm, Limit: 10})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	found := false
	for _, item := range res.Items {
		if item.EventID == eventID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("orphan event %s (realm attributed only via auth_event.realm_id) not returned; total=%d", eventID, res.Total)
	}

	count, err := repo.CountByEventTypeInWindow(ctx, realm, "LOGIN_ERROR",
		eventTime.Add(-time.Minute), eventTime.Add(time.Minute))
	if err != nil {
		t.Fatalf("CountByEventTypeInWindow: %v", err)
	}
	if count != 1 {
		t.Errorf("CountByEventTypeInWindow = %d, want 1", count)
	}
}

// seedJSONOnlyLogin inserts one auth_process + auth_event pair whose
// auth_context_hash deliberately matches no auth_context row, so every context
// field can only be resolved from auth_context_json. It returns the realm and
// event id, and registers cleanup.
//
// idPrefix must be unique per test (it forms the UUIDs), and ctxJSON must carry
// a realm_id equal to the returned realm.
func seedJSONOnlyLogin(t *testing.T, db *gorm.DB, idPrefix, realm, ctxJSON string, risk int) (string, string) {
	t.Helper()

	var deviceHash, netHash, configID string
	row := db.Raw(`SELECT device_info_hash, network_location_hash, parameters_config_id::text
	               FROM auth_process LIMIT 1`).Row()
	if err := row.Scan(&deviceHash, &netHash, &configID); err != nil {
		t.Skipf("no existing auth_process to borrow FK values from: %v", err)
	}

	procID := idPrefix + "-0000-0000-0000-000000000001"
	eventID := idPrefix + "-0000-0000-0000-000000000002"
	userID := idPrefix + "-0000-0000-0000-000000000003"
	ctxHash := idPrefix + "-context-hash-matches-no-auth_context-row"
	now := time.Now().UTC()

	t.Cleanup(func() {
		db.Exec(`DELETE FROM auth_event WHERE id = ?`, eventID)
		db.Exec(`DELETE FROM auth_process WHERE id = ?`, procID)
	})

	if err := db.Exec(`
		INSERT INTO auth_process
		  (id, user_id, auth_context_hash, device_info_hash, network_location_hash,
		   auth_context_json, pre_auth_risk_decision, parameters_config_id,
		   final_status, started_at)
		VALUES (?, ?, ?, ?, ?, ?::json, ?, ?, ?, ?)`,
		procID, userID, ctxHash, deviceHash, netHash, ctxJSON, risk, configID, "LOGIN", now,
	).Error; err != nil {
		t.Fatalf("seed auth_process: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO auth_event
		  (id, auth_process, user_id, event_type, event_time,
		   auth_context_hash, device_info_hash, network_location_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID, procID, userID, "LOGIN", now, ctxHash, deviceHash, netHash,
	).Error; err != nil {
		t.Fatalf("seed auth_event: %v", err)
	}
	return realm, eventID
}

// seedJSONOnlyEvent is seedJSONOnlyLogin generalized to a caller-chosen
// event_type and user_id (seedJSONOnlyLogin hardcodes both to "LOGIN" and an
// id derived from idPrefix). Used by tests that need LOGIN_ERROR rows or need
// to control which user/realm an event belongs to (e.g. grouping by client or
// by user, or seeding several distinct users for the reject-burst rule).
func seedJSONOnlyEvent(t *testing.T, db *gorm.DB, idPrefix, realm, userID, eventType, ctxJSON string, risk int) (outRealm, eventID string) {
	t.Helper()

	var deviceHash, netHash, configID string
	row := db.Raw(`SELECT device_info_hash, network_location_hash, parameters_config_id::text
	               FROM auth_process LIMIT 1`).Row()
	if err := row.Scan(&deviceHash, &netHash, &configID); err != nil {
		t.Skipf("no existing auth_process to borrow FK values from: %v", err)
	}

	procID := idPrefix + "-0000-0000-0000-000000000001"
	eventID = idPrefix + "-0000-0000-0000-000000000002"
	ctxHash := idPrefix + "-context-hash-matches-no-auth_context-row"
	now := time.Now().UTC()

	t.Cleanup(func() {
		db.Exec(`DELETE FROM auth_event WHERE id = ?`, eventID)
		db.Exec(`DELETE FROM auth_process WHERE id = ?`, procID)
	})

	if err := db.Exec(`
		INSERT INTO auth_process
		  (id, user_id, auth_context_hash, device_info_hash, network_location_hash,
		   auth_context_json, pre_auth_risk_decision, parameters_config_id,
		   final_status, started_at)
		VALUES (?, ?, ?, ?, ?, ?::json, ?, ?, ?, ?)`,
		procID, userID, ctxHash, deviceHash, netHash, ctxJSON, risk, configID, eventType, now,
	).Error; err != nil {
		t.Fatalf("seed auth_process: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO auth_event
		  (id, auth_process, user_id, event_type, event_time,
		   auth_context_hash, device_info_hash, network_location_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID, procID, userID, eventType, now, ctxHash, deviceHash, netHash,
	).Error; err != nil {
		t.Fatalf("seed auth_event: %v", err)
	}
	return realm, eventID
}

// TestRepository_GetGeoBuckets_SourcesGeoFromJSON is the geo-map counterpart of
// TestRepository_ListEvents_SourcesContextFromJSON. Because adaptive-auth never
// writes auth_context rows, a geo query reading only ac.lat/ac.long returns zero
// buckets for every realm and the map renders empty however much geolocated
// traffic AMFA holds.
func TestRepository_GetGeoBuckets_SourcesGeoFromJSON(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)

	const realm = "tdd-geo-json-realm"
	ctxJSON := `{"realm_id":"` + realm + `","client":"tdd-client",` +
		`"ip_address":"10.11.12.13","lat":12.5,"long":-8.2,` +
		`"is_vpn":true,"country_name":"Testland"}`
	seedJSONOnlyLogin(t, db, "bbbbbbbb", realm, ctxJSON, 4)

	buckets, err := repo.GetGeoBuckets(context.Background(), amfa.GeoOptions{RealmID: realm})
	if err != nil {
		t.Fatalf("GetGeoBuckets: %v", err)
	}
	if len(buckets) != 1 {
		t.Fatalf("expected exactly 1 bucket for the seeded realm, got %d (%+v)", len(buckets), buckets)
	}
	got := buckets[0]
	if got.Lat != 12.5 {
		t.Errorf("Lat = %v, want 12.5 (from auth_context_json)", got.Lat)
	}
	if got.Long != -8.2 {
		t.Errorf("Long = %v, want -8.2 (from auth_context_json)", got.Long)
	}
	if got.Country == nil || *got.Country != "Testland" {
		t.Errorf("Country = %v, want %q (from auth_context_json)", got.Country, "Testland")
	}
	if got.Count != 1 {
		t.Errorf("Count = %d, want 1", got.Count)
	}
	// risky_count counts pre_auth_risk_decision >= 3; the seed is risk 4.
	if got.RiskyCount != 1 {
		t.Errorf("RiskyCount = %d, want 1", got.RiskyCount)
	}
}

// TestRepository_GetGeoBuckets_ExcludesNullIsland pins the 0,0 guard. AMFA's
// geocoder writes lat/long 0 when the lookup fails or times out, which is the
// normal result for the private IP a local browser login reports. Those logins
// must not become a marker in the Gulf of Guinea.
func TestRepository_GetGeoBuckets_ExcludesNullIsland(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)

	const realm = "tdd-geo-nullisland-realm"
	ctxJSON := `{"realm_id":"` + realm + `","client":"tdd-client",` +
		`"ip_address":"172.19.0.1","lat":0,"long":0,` +
		`"is_vpn":false,"country_name":""}`
	seedJSONOnlyLogin(t, db, "cccccccc", realm, ctxJSON, 3)

	buckets, err := repo.GetGeoBuckets(context.Background(), amfa.GeoOptions{RealmID: realm})
	if err != nil {
		t.Fatalf("GetGeoBuckets: %v", err)
	}
	if len(buckets) != 0 {
		t.Errorf("expected 0 buckets for a 0,0 login, got %d (%+v)", len(buckets), buckets)
	}
}

// TestRepository_GetStats_CountsFlaggedIPFromJSON pins the Flagged IPs KPI to
// the JSON fallback; reading ac.ip_address/ac.is_vpn alone held it at 0 forever.
func TestRepository_GetStats_CountsFlaggedIPFromJSON(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)

	const realm = "tdd-stats-json-realm"
	ctxJSON := `{"realm_id":"` + realm + `","client":"tdd-client",` +
		`"ip_address":"198.51.100.7","lat":40.1,"long":-74.0,` +
		`"is_vpn":true,"country_name":"Testland"}`
	seedJSONOnlyLogin(t, db, "dddddddd", realm, ctxJSON, 3)

	stats, err := repo.GetStats(context.Background(), amfa.StatsOptions{RealmID: realm})
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.Total != 1 {
		t.Fatalf("Total = %d, want 1 for the seeded realm", stats.Total)
	}
	if stats.FlaggedIPs != 1 {
		t.Errorf("FlaggedIPs = %d, want 1 (VPN flag from auth_context_json)", stats.FlaggedIPs)
	}
	if stats.Risky != 1 {
		t.Errorf("Risky = %d, want 1 (risk 3 seed)", stats.Risky)
	}
}

// TestRepository_ListVPNRiskyEventsSince_SourcesVPNFromJSON pins the query
// behind the vpn_risky alert rule to the JSON fallback. Filtering on ac.is_vpn
// alone matched nothing, so that rule could never raise an alert.
func TestRepository_ListVPNRiskyEventsSince_SourcesVPNFromJSON(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)

	const realm = "tdd-vpn-json-realm"
	ctxJSON := `{"realm_id":"` + realm + `","client":"tdd-client",` +
		`"ip_address":"198.51.100.9","lat":48.9,"long":2.4,` +
		`"is_vpn":true,"country_name":"Testland"}`
	_, eventID := seedJSONOnlyLogin(t, db, "eeeeeeee", realm, ctxJSON, 3)

	rows, err := repo.ListVPNRiskyEventsSince(
		context.Background(), realm, time.Now().Add(-1*time.Hour), 3)
	if err != nil {
		t.Fatalf("ListVPNRiskyEventsSince: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.EventID == eventID {
			found = true
			if !r.IsVPN {
				t.Errorf("IsVPN = false, want true (from auth_context_json)")
			}
		}
	}
	if !found {
		t.Errorf("seeded VPN login %s not returned (%d rows) - vpn_risky would never alert",
			eventID, len(rows))
	}
}

func TestRepository_ListRejectedEventsSince_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.ListRejectedEventsSince(context.Background(), "", time.Unix(0, 0))
	if err == nil {
		t.Error("expected error when realm is empty")
	}
}

func TestRepository_ListVPNRiskyEventsSince_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.ListVPNRiskyEventsSince(context.Background(), "", time.Unix(0, 0), 3)
	if err == nil {
		t.Error("expected error when realm is empty")
	}
}

func TestRepository_CountRepeatedRiskyByUser_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.CountRepeatedRiskyByUser(context.Background(), "", time.Unix(0, 0), 2, 3)
	if err == nil {
		t.Error("expected error when realm is empty")
	}
}

func TestRepository_CountByEventTypeInWindow_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.CountByEventTypeInWindow(context.Background(), "", "LOGIN_ERROR", time.Unix(0, 0), time.Now())
	if err == nil {
		t.Error("expected error when realm is empty")
	}
}

func TestRepository_CountByEventTypeAndClientInWindow_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.CountByEventTypeAndClientInWindow(context.Background(), "", "LOGIN_ERROR", time.Unix(0, 0), time.Now(), 1)
	if err == nil {
		t.Error("expected error when realm is empty")
	}
}

func TestRepository_CountByEventTypeAndClientInWindow_GroupsByClient(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)

	const realm = "tdd-client-error-realm"
	ctxJSON := `{"realm_id":"` + realm + `","client":"tdd-attacked-client"}`
	seedJSONOnlyEvent(t, db, "ffffffff", realm, "ffffffff-0000-0000-0000-00000000000a", "LOGIN_ERROR", ctxJSON, 1)
	seedJSONOnlyEvent(t, db, "11111111", realm, "11111111-0000-0000-0000-00000000000b", "LOGIN_ERROR", ctxJSON, 1)

	rows, err := repo.CountByEventTypeAndClientInWindow(
		context.Background(), realm, "LOGIN_ERROR", time.Now().Add(-time.Hour), time.Now(), 2)
	if err != nil {
		t.Fatalf("CountByEventTypeAndClientInWindow: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 client row, got %d (%+v)", len(rows), rows)
	}
	if rows[0].Client != "tdd-attacked-client" {
		t.Errorf("Client = %q, want tdd-attacked-client", rows[0].Client)
	}
	if rows[0].Count != 2 {
		t.Errorf("Count = %d, want 2", rows[0].Count)
	}
}

func TestRepository_CountDistinctRejectedUsersSince_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.CountDistinctRejectedUsersSince(context.Background(), "", time.Unix(0, 0))
	if err == nil {
		t.Error("expected error when realm is empty")
	}
}

func TestRepository_CountDistinctRejectedUsersSince_CountsDistinctUsersOnly(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)

	const realm = "tdd-reject-burst-realm"
	ctxJSON := `{"realm_id":"` + realm + `"}`
	// Two rejections for the same user must count once; a second user adds one more.
	seedJSONOnlyEvent(t, db, "22222222", realm, "22222222-0000-0000-0000-00000000000a", "LOGIN_ERROR", ctxJSON, 4)
	seedJSONOnlyEvent(t, db, "33333333", realm, "22222222-0000-0000-0000-00000000000a", "LOGIN_ERROR", ctxJSON, 4)
	seedJSONOnlyEvent(t, db, "44444444", realm, "44444444-0000-0000-0000-00000000000b", "LOGIN_ERROR", ctxJSON, 4)

	count, err := repo.CountDistinctRejectedUsersSince(context.Background(), realm, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("CountDistinctRejectedUsersSince: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 distinct users", count)
	}
}

func TestRepository_CountByEventTypeByUser_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.CountByEventTypeByUser(context.Background(), "", "LOGIN_ERROR", time.Unix(0, 0), 1)
	if err == nil {
		t.Error("expected error when realm is empty")
	}
}

func TestRepository_CountByEventTypeByUser_GroupsByUserAboveThreshold(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)

	const realm = "tdd-account-error-realm"
	ctxJSON := `{"realm_id":"` + realm + `"}`
	seedJSONOnlyEvent(t, db, "55555555", realm, "55555555-0000-0000-0000-00000000000a", "LOGIN_ERROR", ctxJSON, 1)
	seedJSONOnlyEvent(t, db, "66666666", realm, "55555555-0000-0000-0000-00000000000a", "LOGIN_ERROR", ctxJSON, 1)
	seedJSONOnlyEvent(t, db, "77777777", realm, "77777777-0000-0000-0000-00000000000b", "LOGIN_ERROR", ctxJSON, 1)

	rows, err := repo.CountByEventTypeByUser(context.Background(), realm, "LOGIN_ERROR", time.Now().Add(-time.Hour), 2)
	if err != nil {
		t.Fatalf("CountByEventTypeByUser: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 user row above threshold, got %d (%+v)", len(rows), rows)
	}
	if rows[0].UserID != "55555555-0000-0000-0000-00000000000a" {
		t.Errorf("UserID = %q, want 55555555-0000-0000-0000-00000000000a", rows[0].UserID)
	}
	if rows[0].Count != 2 {
		t.Errorf("Count = %d, want 2", rows[0].Count)
	}
}

// TestRepository_NewQueries_SmokeAgainstRealDB exercises each query against the
// seeded AMFA test DB. It asserts only that the queries execute without error
// and return sane (non-negative) shapes - the row contents depend on seed data.
func TestRepository_NewQueries_SmokeAgainstRealDB(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	const realm = "master" // adjust to a realm present in the test DB if needed
	since := time.Now().Add(-90 * 24 * time.Hour)

	if _, err := repo.ListRejectedEventsSince(ctx, realm, since); err != nil {
		t.Errorf("ListRejectedEventsSince: %v", err)
	}
	if _, err := repo.ListVPNRiskyEventsSince(ctx, realm, since, 3); err != nil {
		t.Errorf("ListVPNRiskyEventsSince: %v", err)
	}
	if _, err := repo.CountRepeatedRiskyByUser(ctx, realm, since, 2, 1); err != nil {
		t.Errorf("CountRepeatedRiskyByUser: %v", err)
	}
	n, err := repo.CountByEventTypeInWindow(ctx, realm, "LOGIN_ERROR", since, time.Now())
	if err != nil {
		t.Errorf("CountByEventTypeInWindow: %v", err)
	}
	if n < 0 {
		t.Errorf("CountByEventTypeInWindow returned negative count %d", n)
	}
	if _, err := repo.CountByEventTypeAndClientInWindow(ctx, realm, "LOGIN_ERROR", since, time.Now(), 1); err != nil {
		t.Errorf("CountByEventTypeAndClientInWindow: %v", err)
	}
	if _, err := repo.CountDistinctRejectedUsersSince(ctx, realm, since); err != nil {
		t.Errorf("CountDistinctRejectedUsersSince: %v", err)
	}
	if _, err := repo.CountByEventTypeByUser(ctx, realm, "LOGIN_ERROR", since, 1); err != nil {
		t.Errorf("CountByEventTypeByUser: %v", err)
	}
}
