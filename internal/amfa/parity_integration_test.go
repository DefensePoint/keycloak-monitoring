//go:build integration
// +build integration

// Parity between the two amfa.Repository implementations.
//
// The migration away from a direct database connection is only safe if the HTTP
// implementation answers the same questions with the same numbers. Unit tests on
// either side cannot show that: each one asserts against its own idea of what
// the other does. This test runs both against the same live data and compares.
//
// Requires both:
//
//	AMFA_TEST_DSN       host=localhost port=5455 dbname=adaptive_mfa user=... sslmode=disable
//	AMFA_TEST_API_URL   http://localhost:8000
//	AMFA_TEST_REALM     the realm to compare
//	AMFA_TEST_TOKEN     a bearer token valid for that realm
//
// Run with: go test -tags integration ./amfa/ -run TestParity -v
package amfa_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfa/httpclient"
	amfapg "github.com/DefensePoint/keycloak-monitoring/internal/amfa/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// parityFixture holds both implementations plus the realm and window they are
// compared over.
type parityFixture struct {
	pg    amfa.Repository
	api   amfa.Repository
	realm string
	start time.Time
	end   time.Time
}

func newParityFixture(t *testing.T) parityFixture {
	t.Helper()

	dsn := os.Getenv("AMFA_TEST_DSN")
	apiURL := os.Getenv("AMFA_TEST_API_URL")
	realm := os.Getenv("AMFA_TEST_REALM")
	token := os.Getenv("AMFA_TEST_TOKEN")

	if dsn == "" || apiURL == "" || realm == "" || token == "" {
		t.Skip("parity test needs AMFA_TEST_DSN, AMFA_TEST_API_URL, AMFA_TEST_REALM and AMFA_TEST_TOKEN")
	}

	testdb.ShareDatabase(t, dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect to AMFA database: %v", err)
	}

	api, err := httpclient.New(
		httpclient.Config{BaseURL: apiURL, Timeout: 30 * time.Second},
		httpclient.StaticTokenSource(token),
		nil,
	)
	if err != nil {
		t.Fatalf("build AMFA API client: %v", err)
	}

	// A window with a fixed end. Leaving it open would let rows arrive between
	// the two calls and show up as a parity failure that is really just a new
	// login.
	end := time.Now().UTC().Add(-1 * time.Minute)
	return parityFixture{
		pg:    amfapg.NewRepository(db),
		api:   api,
		realm: realm,
		start: end.AddDate(0, 0, -30),
		end:   end,
	}
}

// requireNonEmpty guards against a test that passes only because both sides
// returned nothing. Parity over an empty result set proves nothing.
func requireNonEmpty(t *testing.T, n int, what string) {
	t.Helper()
	if n == 0 {
		t.Skipf("no %s in the window; seed the AMFA database to make this meaningful", what)
	}
}

func TestParityStats(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()
	opts := amfa.StatsOptions{RealmID: f.realm, StartTime: &f.start, EndTime: &f.end}

	fromPG, err := f.pg.GetStats(ctx, opts)
	if err != nil {
		t.Fatalf("postgres GetStats: %v", err)
	}
	fromAPI, err := f.api.GetStats(ctx, opts)
	if err != nil {
		t.Fatalf("api GetStats: %v", err)
	}

	requireNonEmpty(t, int(fromPG.Total), "events")

	// Compared field by field: a struct diff would report "not equal" without
	// naming which KPI drifted, and each has a different cause.
	for _, c := range []struct {
		name    string
		pg, api int64
	}{
		{"Total", fromPG.Total, fromAPI.Total},
		{"Risky", fromPG.Risky, fromAPI.Risky},
		{"UniqueUsers", fromPG.UniqueUsers, fromAPI.UniqueUsers},
		{"FlaggedIPs", fromPG.FlaggedIPs, fromAPI.FlaggedIPs},
	} {
		if c.pg != c.api {
			t.Errorf("%s: postgres=%d api=%d", c.name, c.pg, c.api)
		}
	}
}

func TestParityGeoBuckets(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()
	opts := amfa.GeoOptions{RealmID: f.realm, StartTime: &f.start, EndTime: &f.end}

	fromPG, err := f.pg.GetGeoBuckets(ctx, opts)
	if err != nil {
		t.Fatalf("postgres GetGeoBuckets: %v", err)
	}
	fromAPI, err := f.api.GetGeoBuckets(ctx, opts)
	if err != nil {
		t.Fatalf("api GetGeoBuckets: %v", err)
	}

	requireNonEmpty(t, len(fromPG), "geolocated events")

	if len(fromPG) != len(fromAPI) {
		t.Fatalf("bucket count: postgres=%d api=%d", len(fromPG), len(fromAPI))
	}

	// Keyed by cell rather than compared positionally: both sides order by count
	// descending, so ties can legitimately come back in a different order.
	type cell struct {
		lat, long float64
	}
	pgByCell := make(map[cell]amfa.GeoBucket, len(fromPG))
	for _, b := range fromPG {
		pgByCell[cell{b.Lat, b.Long}] = b
	}

	for _, apiBucket := range fromAPI {
		key := cell{apiBucket.Lat, apiBucket.Long}
		pgBucket, ok := pgByCell[key]
		if !ok {
			t.Errorf("api returned cell %+v that postgres did not", key)
			continue
		}
		if pgBucket.Count != apiBucket.Count {
			t.Errorf("cell %+v count: postgres=%d api=%d", key, pgBucket.Count, apiBucket.Count)
		}
		if pgBucket.RiskyCount != apiBucket.RiskyCount {
			t.Errorf("cell %+v risky: postgres=%d api=%d", key, pgBucket.RiskyCount, apiBucket.RiskyCount)
		}
		if !equalStrPtr(pgBucket.Country, apiBucket.Country) {
			t.Errorf("cell %+v country: postgres=%v api=%v", key, deref(pgBucket.Country), deref(apiBucket.Country))
		}
	}
}

func TestParityListEvents(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()
	opts := amfa.ListEventsOptions{
		RealmID:   f.realm,
		StartTime: &f.start,
		EndTime:   &f.end,
		Limit:     50,
	}

	fromPG, err := f.pg.ListEvents(ctx, opts)
	if err != nil {
		t.Fatalf("postgres ListEvents: %v", err)
	}
	fromAPI, err := f.api.ListEvents(ctx, opts)
	if err != nil {
		t.Fatalf("api ListEvents: %v", err)
	}

	requireNonEmpty(t, len(fromPG.Items), "events")

	if fromPG.Total != fromAPI.Total {
		t.Errorf("total: postgres=%d api=%d", fromPG.Total, fromAPI.Total)
	}
	if len(fromPG.Items) != len(fromAPI.Items) {
		t.Fatalf("page size: postgres=%d api=%d", len(fromPG.Items), len(fromAPI.Items))
	}

	// Keyed by event id: postgres orders by event_time alone while the API adds
	// the id as a tiebreaker, so rows sharing a timestamp can appear in a
	// different order without either being wrong.
	pgByID := make(map[string]amfa.EventRow, len(fromPG.Items))
	for _, row := range fromPG.Items {
		pgByID[row.EventID] = row
	}

	for _, apiRow := range fromAPI.Items {
		pgRow, ok := pgByID[apiRow.EventID]
		if !ok {
			t.Errorf("api returned event %s that postgres did not", apiRow.EventID)
			continue
		}
		compareEventRow(t, pgRow, apiRow)
	}
}

func TestParityListEventsSince(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()
	since := f.start

	fromPG, err := f.pg.ListEventsSince(ctx, f.realm, since, 100)
	if err != nil {
		t.Fatalf("postgres ListEventsSince: %v", err)
	}
	fromAPI, err := f.api.ListEventsSince(ctx, f.realm, since, 100)
	if err != nil {
		t.Fatalf("api ListEventsSince: %v", err)
	}

	requireNonEmpty(t, len(fromPG), "events")

	// This one is order-sensitive on purpose. The events mirror advances a
	// watermark from the last row it saved, so a divergence in ordering here
	// would make it skip or replay events rather than merely display them
	// differently.
	if len(fromPG) != len(fromAPI) {
		t.Fatalf("row count: postgres=%d api=%d", len(fromPG), len(fromAPI))
	}
	for i := range fromPG {
		if !fromPG[i].EventTime.Equal(fromAPI[i].EventTime) {
			t.Errorf("row %d event_time: postgres=%v api=%v",
				i, fromPG[i].EventTime, fromAPI[i].EventTime)
		}
	}
	// Ascending order is what makes a watermark safe to advance.
	for i := 1; i < len(fromAPI); i++ {
		if fromAPI[i].EventTime.Before(fromAPI[i-1].EventTime) {
			t.Fatalf("api rows are not oldest-first at index %d", i)
		}
	}
}

func TestParityRejectedEvents(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()

	fromPG, err := f.pg.ListRejectedEventsSince(ctx, f.realm, f.start)
	if err != nil {
		t.Fatalf("postgres ListRejectedEventsSince: %v", err)
	}
	fromAPI, err := f.api.ListRejectedEventsSince(ctx, f.realm, f.start)
	if err != nil {
		t.Fatalf("api ListRejectedEventsSince: %v", err)
	}

	requireNonEmpty(t, len(fromPG), "risk-rejected events")
	compareIDSets(t, "rejected events", fromPG, fromAPI)
}

func TestParityVPNRiskyEvents(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()

	fromPG, err := f.pg.ListVPNRiskyEventsSince(ctx, f.realm, f.start, 3)
	if err != nil {
		t.Fatalf("postgres ListVPNRiskyEventsSince: %v", err)
	}
	fromAPI, err := f.api.ListVPNRiskyEventsSince(ctx, f.realm, f.start, 3)
	if err != nil {
		t.Fatalf("api ListVPNRiskyEventsSince: %v", err)
	}

	requireNonEmpty(t, len(fromPG), "VPN-flagged risky events")
	compareIDSets(t, "vpn risky events", fromPG, fromAPI)
}

func TestParityRepeatedRiskyByUser(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()

	fromPG, err := f.pg.CountRepeatedRiskyByUser(ctx, f.realm, f.start, 2, 1)
	if err != nil {
		t.Fatalf("postgres CountRepeatedRiskyByUser: %v", err)
	}
	fromAPI, err := f.api.CountRepeatedRiskyByUser(ctx, f.realm, f.start, 2, 1)
	if err != nil {
		t.Fatalf("api CountRepeatedRiskyByUser: %v", err)
	}

	requireNonEmpty(t, len(fromPG), "users with risky events")

	pgByUser := make(map[string]int64, len(fromPG))
	for _, c := range fromPG {
		pgByUser[c.UserID] = c.Count
	}
	if len(fromPG) != len(fromAPI) {
		t.Errorf("user count: postgres=%d api=%d", len(fromPG), len(fromAPI))
	}
	for _, c := range fromAPI {
		want, ok := pgByUser[c.UserID]
		if !ok {
			t.Errorf("api returned user %s that postgres did not", c.UserID)
			continue
		}
		if want != c.Count {
			t.Errorf("user %s count: postgres=%d api=%d", c.UserID, want, c.Count)
		}
	}
}

func TestParityCountByEventType(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()

	// Every type the alert rules count on, so a mismatch is attributed to a
	// type rather than to "some count somewhere".
	for _, eventType := range []string{"LOGIN", "LOGIN_ERROR", "CLIENT_LOGIN", "CLIENT_LOGIN_ERROR"} {
		fromPG, err := f.pg.CountByEventTypeInWindow(ctx, f.realm, eventType, f.start, f.end)
		if err != nil {
			t.Fatalf("postgres CountByEventTypeInWindow(%s): %v", eventType, err)
		}
		fromAPI, err := f.api.CountByEventTypeInWindow(ctx, f.realm, eventType, f.start, f.end)
		if err != nil {
			t.Fatalf("api CountByEventTypeInWindow(%s): %v", eventType, err)
		}
		if fromPG != fromAPI {
			t.Errorf("%s count: postgres=%d api=%d", eventType, fromPG, fromAPI)
		}
	}
}

// TestParityAllRealmsStatsDivergesDeliberately documents the one place the two
// implementations are not interchangeable.
//
// The postgres path treats an empty realm as "every realm", which it can do
// because it holds a connection to the whole database. The API cannot: its path
// is realm-scoped and its token is realm-bound. Returning one realm's numbers
// under an all-realms request would understate every KPI silently, so the client
// reports the gap instead.
func TestParityAllRealmsStatsDivergesDeliberately(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()
	opts := amfa.StatsOptions{StartTime: &f.start, EndTime: &f.end}

	if _, err := f.pg.GetStats(ctx, opts); err != nil {
		t.Errorf("postgres should still support all-realms stats: %v", err)
	}
	if _, err := f.api.GetStats(ctx, opts); !errors.Is(err, amfa.ErrAllRealmsUnsupported) {
		t.Errorf("api should refuse all-realms stats with ErrAllRealmsUnsupported, got: %v", err)
	}
}

// --- helpers --------------------------------------------------------------

func compareEventRow(t *testing.T, pg, api amfa.EventRow) {
	t.Helper()
	id := pg.EventID

	if !pg.EventTime.Equal(api.EventTime) {
		t.Errorf("event %s event_time: postgres=%v api=%v", id, pg.EventTime, api.EventTime)
	}
	for _, c := range []struct {
		field   string
		pg, api string
	}{
		{"event_type", pg.EventType, api.EventType},
		{"client", pg.Client, api.Client},
		{"ip_address", pg.IPAddress, api.IPAddress},
	} {
		if c.pg != c.api {
			t.Errorf("event %s %s: postgres=%q api=%q", id, c.field, c.pg, c.api)
		}
	}
	if pg.IsVPN != api.IsVPN {
		t.Errorf("event %s is_vpn: postgres=%v api=%v", id, pg.IsVPN, api.IsVPN)
	}
	for _, c := range []struct {
		field   string
		pg, api *string
	}{
		{"user_id", pg.UserID, api.UserID},
		{"country", pg.Country, api.Country},
		{"city", pg.City, api.City},
		{"final_status", pg.FinalStatus, api.FinalStatus},
		{"operating_system", pg.OperatingSystem, api.OperatingSystem},
		{"browser", pg.Browser, api.Browser},
		{"device", pg.Device, api.Device},
		{"system_language", pg.SystemLanguage, api.SystemLanguage},
		{"screen_resolution", pg.ScreenResolution, api.ScreenResolution},
	} {
		if !equalStrPtr(c.pg, c.api) {
			t.Errorf("event %s %s: postgres=%v api=%v", id, c.field, deref(c.pg), deref(c.api))
		}
	}
	if !reflect.DeepEqual(pg.RiskLevel, api.RiskLevel) {
		t.Errorf("event %s risk_level: postgres=%v api=%v", id, pg.RiskLevel, api.RiskLevel)
	}
	if !equalFloatPtr(pg.Lat, api.Lat) || !equalFloatPtr(pg.Long, api.Long) {
		t.Errorf("event %s coordinates differ: postgres=(%v,%v) api=(%v,%v)",
			id, pg.Lat, pg.Long, api.Lat, api.Long)
	}
}

// compareIDSets compares which events each side matched, ignoring order.
//
// Reported as which ids are missing from each side rather than as a count
// difference: "postgres had 12, api had 11" does not say which event an alert
// rule would have stopped firing on.
func compareIDSets(t *testing.T, what string, pg, api []amfa.EventRow) {
	t.Helper()

	pgIDs := make(map[string]struct{}, len(pg))
	for _, row := range pg {
		pgIDs[row.EventID] = struct{}{}
	}
	apiIDs := make(map[string]struct{}, len(api))
	for _, row := range api {
		apiIDs[row.EventID] = struct{}{}
	}

	for id := range pgIDs {
		if _, ok := apiIDs[id]; !ok {
			t.Errorf("%s: api missed event %s that postgres matched", what, id)
		}
	}
	for id := range apiIDs {
		if _, ok := pgIDs[id]; !ok {
			t.Errorf("%s: api matched event %s that postgres did not", what, id)
		}
	}
}

func equalStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// equalFloatPtr compares coordinates with a tolerance. Both sides read the same
// stored value, but it crosses a JSON float on one path and a driver float64 on
// the other, so an exact comparison could fail on representation alone.
func equalFloatPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	diff := *a - *b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-9
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// The three methods below were added to the HTTP client after the rest of this
// file was written, and were verified only against a stub server. They are the
// ones where the two implementations are most likely to diverge, because AMFA
// exposes no server-side endpoint that groups an arbitrary event type's counts,
// so the HTTP client drains matching events and aggregates them client-side
// while Postgres does the grouping in SQL.
//
// Note a known, deliberate boundary difference that these tests are written
// around: Postgres bounds the lower edge inclusively (>= / BETWEEN) while
// AMFA's API treats `since` as exclusive. An event landing exactly on the
// boundary is therefore counted by one and not the other. That predates these
// methods (CountByEventTypeInWindow and CountRepeatedRiskyByUser share it) and
// is immaterial with real timestamps, so the assertions below allow the two
// sides to differ by at most one event per group rather than pretending the
// semantics are identical.

// countsWithinOne reports whether two counts agree to within the single-event
// boundary tolerance described above.
func countsWithinOne(a, b int64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= 1
}

func TestParityCountByEventTypeAndClientInWindow(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()

	const eventType = "LOGIN_ERROR"
	// threshold 1 so every client with any matching event is returned, which
	// makes the two result sets comparable rather than both being empty.
	fromPG, err := f.pg.CountByEventTypeAndClientInWindow(ctx, f.realm, eventType, f.start, f.end, 1)
	if err != nil {
		t.Fatalf("postgres CountByEventTypeAndClientInWindow: %v", err)
	}
	fromAPI, err := f.api.CountByEventTypeAndClientInWindow(ctx, f.realm, eventType, f.start, f.end, 1)
	if err != nil {
		t.Fatalf("api CountByEventTypeAndClientInWindow: %v", err)
	}

	requireNonEmpty(t, len(fromPG), eventType+" events grouped by client")

	pgByClient := make(map[string]int64, len(fromPG))
	for _, c := range fromPG {
		pgByClient[c.Client] = c.Count
	}
	apiByClient := make(map[string]int64, len(fromAPI))
	for _, c := range fromAPI {
		apiByClient[c.Client] = c.Count
	}

	for client, want := range pgByClient {
		got, ok := apiByClient[client]
		if !ok {
			// A client whose only events sit on the boundary can legitimately
			// drop out of the API's exclusive-since result.
			if want > 1 {
				t.Errorf("api omitted client %q entirely, postgres counted %d", client, want)
			}
			continue
		}
		if !countsWithinOne(want, got) {
			t.Errorf("client %q count: postgres=%d api=%d", client, want, got)
		}
	}
	for client := range apiByClient {
		if _, ok := pgByClient[client]; !ok {
			t.Errorf("api returned client %q that postgres did not", client)
		}
	}
}

func TestParityCountDistinctRejectedUsersSince(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()

	fromPG, err := f.pg.CountDistinctRejectedUsersSince(ctx, f.realm, f.start)
	if err != nil {
		t.Fatalf("postgres CountDistinctRejectedUsersSince: %v", err)
	}
	fromAPI, err := f.api.CountDistinctRejectedUsersSince(ctx, f.realm, f.start)
	if err != nil {
		t.Fatalf("api CountDistinctRejectedUsersSince: %v", err)
	}

	requireNonEmpty(t, int(fromPG), "risk-rejected users")

	// This one counts distinct users rather than events, so the boundary
	// tolerance only bites when a user's sole rejection sits on the edge.
	if !countsWithinOne(fromPG, fromAPI) {
		t.Errorf("distinct rejected users: postgres=%d api=%d", fromPG, fromAPI)
	}
}

func TestParityCountByEventTypeByUser(t *testing.T) {
	f := newParityFixture(t)
	ctx := context.Background()

	const eventType = "LOGIN_ERROR"
	fromPG, err := f.pg.CountByEventTypeByUser(ctx, f.realm, eventType, f.start, 1)
	if err != nil {
		t.Fatalf("postgres CountByEventTypeByUser: %v", err)
	}
	fromAPI, err := f.api.CountByEventTypeByUser(ctx, f.realm, eventType, f.start, 1)
	if err != nil {
		t.Fatalf("api CountByEventTypeByUser: %v", err)
	}

	requireNonEmpty(t, len(fromPG), eventType+" events grouped by user")

	pgByUser := make(map[string]int64, len(fromPG))
	for _, c := range fromPG {
		pgByUser[c.UserID] = c.Count
	}
	apiByUser := make(map[string]int64, len(fromAPI))
	for _, c := range fromAPI {
		apiByUser[c.UserID] = c.Count
	}

	for user, want := range pgByUser {
		got, ok := apiByUser[user]
		if !ok {
			if want > 1 {
				t.Errorf("api omitted user %s entirely, postgres counted %d", user, want)
			}
			continue
		}
		if !countsWithinOne(want, got) {
			t.Errorf("user %s count: postgres=%d api=%d", user, want, got)
		}
	}
	for user := range apiByUser {
		if _, ok := pgByUser[user]; !ok {
			t.Errorf("api returned user %s that postgres did not", user)
		}
	}
}
