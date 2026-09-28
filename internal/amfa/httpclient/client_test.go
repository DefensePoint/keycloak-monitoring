package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
)

// recorder captures what the client asked for, so tests can assert on the
// request rather than only the decoded result.
type recorder struct {
	paths   []string
	queries []url.Values
	auth    []string
}

// newTestClient starts a stub AMFA and returns a client pointed at it. The
// handler receives the request count so a test can vary responses per call.
func newTestClient(t *testing.T, rec *recorder, handler func(w http.ResponseWriter, r *http.Request, call int)) *Client {
	t.Helper()

	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.paths = append(rec.paths, r.URL.Path)
		rec.queries = append(rec.queries, r.URL.Query())
		rec.auth = append(rec.auth, r.Header.Get("Authorization"))
		handler(w, r, call)
		call++
	}))
	t.Cleanup(srv.Close)

	c, err := New(Config{BaseURL: srv.URL}, StaticTokenSource("test-token"), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode: %v", err)
	}
}

func TestClientSatisfiesRepository(t *testing.T) {
	// Also asserted at compile time in client.go; kept here so the intent is
	// visible to anyone reading the tests.
	var _ amfa.Repository = (*Client)(nil)
}

func TestNewRequiresBaseURLAndTokenSource(t *testing.T) {
	if _, err := New(Config{}, StaticTokenSource("t"), nil); err == nil {
		t.Error("expected error for empty base_url")
	}
	if _, err := New(Config{BaseURL: "https://amfa"}, nil, nil); err == nil {
		t.Error("expected error for nil token source")
	}
}

func TestEveryRequestCarriesTheBearerToken(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{})
	})

	if _, err := c.ListEventsSince(context.Background(), "acme", time.Now(), 10); err != nil {
		t.Fatalf("ListEventsSince: %v", err)
	}
	if got := rec.auth[0]; got != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
	}
}

func TestRealmIsPathScoped(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{})
	})

	if _, err := c.ListEventsSince(context.Background(), "acme", time.Now(), 10); err != nil {
		t.Fatalf("ListEventsSince: %v", err)
	}
	if want := "/acme/monitoring/events"; rec.paths[0] != want {
		t.Errorf("path = %q, want %q", rec.paths[0], want)
	}
}

func TestRealmWithPathSeparatorIsRejected(t *testing.T) {
	// Without this guard the request silently retargets a different endpoint:
	// url.PathEscape does not escape "/". No Keycloak realm can contain one, so
	// reject rather than encode.
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{})
	})

	for _, realm := range []string{"a/b", "a\\b", "../admin"} {
		if _, err := c.ListEventsSince(context.Background(), realm, time.Now(), 10); err == nil {
			t.Errorf("realm %q: expected rejection", realm)
		}
	}
	if len(rec.paths) != 0 {
		t.Errorf("made %d requests for invalid realms, want 0", len(rec.paths))
	}
}

func TestOrdinaryRealmNamesArePreserved(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{})
	})

	if _, err := c.ListEventsSince(context.Background(), "acme-prod_1", time.Now(), 10); err != nil {
		t.Fatalf("ListEventsSince: %v", err)
	}
	if want := "/acme-prod_1/monitoring/events"; rec.paths[0] != want {
		t.Errorf("path = %q, want %q", rec.paths[0], want)
	}
}

func TestEmptyRealmIsRejectedBeforeAnyRequest(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{})
	})

	if _, err := c.ListEventsSince(context.Background(), "", time.Now(), 10); err == nil {
		t.Fatal("expected error for empty realm")
	}
	if len(rec.paths) != 0 {
		t.Errorf("made %d requests for an empty realm, want 0", len(rec.paths))
	}
}

func TestEventRowMapsEveryField(t *testing.T) {
	// The row shape is the contract between the two services. A field added to
	// one side and missed here would decode as a zero value rather than fail.
	userID, country, city := "3ab1", "Philippines", "Cebu City"
	lat, long := 10.3157, 123.8854
	risk := 2
	status, os, browser := "SUCCESS", "macOS", "Safari"
	device, lang, res := "desktop", "en-US", "2560x1440"

	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{
			Items: []eventRow{{
				EventID: "9f2c", EventTime: time.Unix(1787038200, 0).UTC(),
				EventType: "LOGIN", UserID: &userID, Client: "account-console",
				IPAddress: "203.0.113.42", Country: &country, City: &city,
				Lat: &lat, Long: &long, IsVPN: true, RiskLevel: &risk,
				FinalStatus: &status, OperatingSystem: &os, Browser: &browser,
				Device: &device, SystemLanguage: &lang, ScreenResolution: &res,
			}},
			Total: 1,
		})
	})

	rows, err := c.ListEventsSince(context.Background(), "acme", time.Now(), 10)
	if err != nil {
		t.Fatalf("ListEventsSince: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}

	got := rows[0]
	checks := []struct {
		name string
		ok   bool
	}{
		{"EventID", got.EventID == "9f2c"},
		{"EventType", got.EventType == "LOGIN"},
		{"UserID", got.UserID != nil && *got.UserID == userID},
		{"Client", got.Client == "account-console"},
		{"IPAddress", got.IPAddress == "203.0.113.42"},
		{"Country", got.Country != nil && *got.Country == country},
		{"City", got.City != nil && *got.City == city},
		{"Lat", got.Lat != nil && *got.Lat == lat},
		{"Long", got.Long != nil && *got.Long == long},
		{"IsVPN", got.IsVPN},
		{"RiskLevel", got.RiskLevel != nil && *got.RiskLevel == risk},
		{"FinalStatus", got.FinalStatus != nil && *got.FinalStatus == status},
		{"OperatingSystem", got.OperatingSystem != nil && *got.OperatingSystem == os},
		{"Browser", got.Browser != nil && *got.Browser == browser},
		{"Device", got.Device != nil && *got.Device == device},
		{"SystemLanguage", got.SystemLanguage != nil && *got.SystemLanguage == lang},
		{"ScreenResolution", got.ScreenResolution != nil && *got.ScreenResolution == res},
	}
	for _, ch := range checks {
		if !ch.ok {
			t.Errorf("%s did not round-trip", ch.name)
		}
	}
}

func TestNullableFieldsStayNil(t *testing.T) {
	// An event whose auth_process never linked has no risk level or status.
	// Those must arrive as nil, not as 0 and "", which would read as a real
	// risk-level-zero login.
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, map[string]any{
			"items": []map[string]any{{
				"event_id": "9f2c", "event_time": "2026-08-19T04:12:07Z",
				"event_type": "LOGIN", "user_id": nil, "risk_level": nil,
				"final_status": nil, "country": nil,
			}},
			"total": 1,
		})
	})

	rows, err := c.ListEventsSince(context.Background(), "acme", time.Now(), 10)
	if err != nil {
		t.Fatalf("ListEventsSince: %v", err)
	}
	got := rows[0]
	if got.RiskLevel != nil || got.FinalStatus != nil || got.UserID != nil || got.Country != nil {
		t.Error("absent fields should decode as nil, not zero values")
	}
}

func TestListEventsSinceReturnsOnlyOnePage(t *testing.T) {
	// The mirror advances its watermark from the rows it saves and re-invokes.
	// Draining here would hand back more than it asked for and desynchronise
	// the watermark from the returned rows.
	next := "2026-08-19T00:00:00Z|9f2c"
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{
			Items:      []eventRow{{EventID: "9f2c", EventTime: time.Now()}},
			Total:      100,
			NextCursor: &next,
		})
	})

	rows, err := c.ListEventsSince(context.Background(), "acme", time.Now(), 10)
	if err != nil {
		t.Fatalf("ListEventsSince: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("got %d rows, want 1 page only", len(rows))
	}
	if len(rec.paths) != 1 {
		t.Errorf("made %d requests, want 1", len(rec.paths))
	}
}

func TestAlertRulePullsDrainEveryPage(t *testing.T) {
	// A rule that saw only the first page would stop alerting during a burst,
	// which is precisely when it must not.
	first := "2026-08-19T00:00:00Z|a"
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, call int) {
		if call == 0 {
			writeJSON(t, w, eventsPage{
				Items:      []eventRow{{EventID: "a", EventTime: time.Now()}},
				NextCursor: &first,
			})
			return
		}
		writeJSON(t, w, eventsPage{Items: []eventRow{{EventID: "b", EventTime: time.Now()}}})
	})

	rows, err := c.ListRejectedEventsSince(context.Background(), "acme", time.Now())
	if err != nil {
		t.Fatalf("ListRejectedEventsSince: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("got %d rows, want 2 across both pages", len(rows))
	}
	if len(rec.queries) < 2 || rec.queries[1].Get("cursor") != first {
		t.Error("second request did not carry the cursor from the first")
	}
}

func TestPageDrainIsBounded(t *testing.T) {
	// A realm with an endless backlog must not loop forever holding a
	// connection; the remainder is read on the next cycle.
	forever := "2026-08-19T00:00:00Z|x"
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{
			Items:      []eventRow{{EventID: "x", EventTime: time.Now()}},
			NextCursor: &forever,
		})
	})

	if _, err := c.ListRejectedEventsSince(context.Background(), "acme", time.Now()); err != nil {
		t.Fatalf("ListRejectedEventsSince: %v", err)
	}
	if len(rec.paths) != maxPagesPerCall {
		t.Errorf("made %d requests, want the %d-page cap", len(rec.paths), maxPagesPerCall)
	}
}

func TestRejectedEventsFilterOnExactRiskLevel(t *testing.T) {
	// The rule fires on risk-rejected logins specifically. A ">=" filter would
	// silently absorb any future higher level.
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{})
	})

	if _, err := c.ListRejectedEventsSince(context.Background(), "acme", time.Now()); err != nil {
		t.Fatalf("ListRejectedEventsSince: %v", err)
	}
	q := rec.queries[0]
	if q.Get("risk_decision") != "4" {
		t.Errorf("risk_decision = %q, want 4", q.Get("risk_decision"))
	}
	if q.Get("min_risk") != "" {
		t.Error("min_risk must not be set: it would widen an exact-level rule")
	}
}

func TestVPNRiskyPullSetsBothFilters(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{})
	})

	if _, err := c.ListVPNRiskyEventsSince(context.Background(), "acme", time.Now(), 3); err != nil {
		t.Fatalf("ListVPNRiskyEventsSince: %v", err)
	}
	q := rec.queries[0]
	if q.Get("is_vpn") != "true" || q.Get("min_risk") != "3" {
		t.Errorf("is_vpn=%q min_risk=%q, want true/3", q.Get("is_vpn"), q.Get("min_risk"))
	}
}

func TestIncrementalPullsWalkForwardInTime(t *testing.T) {
	// Oldest-first is what lets a consumer advance a watermark safely.
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{})
	})

	for _, call := range []func() error{
		func() error { _, e := c.ListEventsSince(context.Background(), "acme", time.Now(), 10); return e },
		func() error { _, e := c.ListRejectedEventsSince(context.Background(), "acme", time.Now()); return e },
		func() error {
			_, e := c.ListVPNRiskyEventsSince(context.Background(), "acme", time.Now(), 3)
			return e
		},
	} {
		if err := call(); err != nil {
			t.Fatalf("call: %v", err)
		}
	}
	for i, q := range rec.queries {
		if q.Get("ascending") != "true" {
			t.Errorf("request %d: ascending = %q, want true", i, q.Get("ascending"))
		}
	}
}

func TestUIListingIsNewestFirst(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{})
	})

	if _, err := c.ListEvents(context.Background(), amfa.ListEventsOptions{RealmID: "acme"}); err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if rec.queries[0].Get("ascending") != "false" {
		t.Error("UI listing should request newest first")
	}
}

// pageOfEvents builds a fake events page of n rows, numbered from `from`, so
// a test can assert on which absolute rows came back.
func pageOfEvents(from, n int, next *string, total int64) eventsPage {
	items := make([]eventRow, n)
	for i := range items {
		items[i] = eventRow{EventID: fmt.Sprintf("e%03d", from+i), EventTime: time.Now()}
	}
	return eventsPage{Items: items, NextCursor: next, Total: total}
}

func TestListEventsOffsetSpanningAPageBoundaryReturnsTheRightWindow(t *testing.T) {
	// Limit=25, Offset=10 with a 25-row page size must return absolute rows
	// 10-34: the tail of page one plus the head of page two.
	cursor := "cursor-1"
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, call int) {
		if call == 0 {
			writeJSON(t, w, pageOfEvents(0, 25, &cursor, 1000))
			return
		}
		// A continuation page's total must never be trusted; make it wrong so a
		// regression that reads it back would be caught.
		writeJSON(t, w, pageOfEvents(25, 25, nil, 999999))
	})

	res, err := c.ListEvents(context.Background(), amfa.ListEventsOptions{
		RealmID: "acme", Limit: 25, Offset: 10,
	})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(res.Items) != 25 {
		t.Fatalf("got %d items, want 25", len(res.Items))
	}
	if got, want := res.Items[0].EventID, "e010"; got != want {
		t.Errorf("first item = %q, want %q", got, want)
	}
	if got, want := res.Items[len(res.Items)-1].EventID, "e034"; got != want {
		t.Errorf("last item = %q, want %q", got, want)
	}
	if res.Total != 1000 {
		t.Errorf("Total = %d, want 1000 (from the first page, not a continuation page)", res.Total)
	}
}

func TestListEventsFirstPageMakesOneRequest(t *testing.T) {
	// Offset<=0 must not pay for a second request just to confirm there's
	// nothing more to skip.
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		next := "cursor-1"
		writeJSON(t, w, pageOfEvents(0, 25, &next, 100))
	})

	if _, err := c.ListEvents(context.Background(), amfa.ListEventsOptions{RealmID: "acme", Limit: 25}); err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(rec.paths) != 1 {
		t.Errorf("made %d requests for offset 0, want 1", len(rec.paths))
	}
	if rec.queries[0].Get("include_total") != "true" {
		t.Error("the only page fetched must ask for the total")
	}
}

func TestListEventsContinuationPagesSkipTheTotal(t *testing.T) {
	cursor := "cursor-1"
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, call int) {
		if call == 0 {
			writeJSON(t, w, pageOfEvents(0, 25, &cursor, 1000))
			return
		}
		writeJSON(t, w, pageOfEvents(25, 25, nil, 1000))
	})

	if _, err := c.ListEvents(context.Background(), amfa.ListEventsOptions{
		RealmID: "acme", Limit: 25, Offset: 10,
	}); err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(rec.queries) < 2 {
		t.Fatalf("expected a continuation request, got %d", len(rec.queries))
	}
	if rec.queries[0].Get("include_total") != "true" {
		t.Error("the first page must ask for the total")
	}
	if rec.queries[1].Get("include_total") != "false" {
		t.Error("a continuation page must not re-ask for the total")
	}
}

func TestListEventsOffsetTooDeepFailsFastInsteadOfWalkingForever(t *testing.T) {
	next := "always-more"
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, pageOfEvents(0, 25, &next, 1_000_000))
	})

	_, err := c.ListEvents(context.Background(), amfa.ListEventsOptions{
		RealmID: "acme", Limit: 25, Offset: 49_000,
	})
	if !errors.Is(err, amfa.ErrOffsetTooLarge) {
		t.Errorf("expected ErrOffsetTooLarge, got %v", err)
	}
	if len(rec.paths) != maxOffsetWalkPages {
		t.Errorf("made %d requests, want the %d-page cap", len(rec.paths), maxOffsetWalkPages)
	}
}

func TestStatsAndGeoDecode(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, r *http.Request, _ int) {
		if strings.HasSuffix(r.URL.Path, "/stats") {
			writeJSON(t, w, amfa.Stats{Total: 10, Risky: 2, UniqueUsers: 5, FlaggedIPs: 1})
			return
		}
		country := "Philippines"
		writeJSON(t, w, []amfa.GeoBucket{{
			Country: &country, Lat: 10.3, Long: 123.9, Count: 4, RiskyCount: 1,
		}})
	})

	stats, err := c.GetStats(context.Background(), amfa.StatsOptions{RealmID: "acme"})
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.Total != 10 || stats.Risky != 2 || stats.UniqueUsers != 5 || stats.FlaggedIPs != 1 {
		t.Errorf("stats did not round-trip: %+v", stats)
	}

	buckets, err := c.GetGeoBuckets(context.Background(), amfa.GeoOptions{RealmID: "acme"})
	if err != nil {
		t.Fatalf("GetGeoBuckets: %v", err)
	}
	if len(buckets) != 1 || buckets[0].Count != 4 || buckets[0].RiskyCount != 1 {
		t.Errorf("geo did not round-trip: %+v", buckets)
	}
}

func TestAllRealmsStatsIsReportedNotFaked(t *testing.T) {
	// The postgres path supports an empty realm as "all realms". The API cannot:
	// the path is realm-scoped and the token is realm-bound. Answering for one
	// realm instead would understate every KPI without saying so.
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, amfa.Stats{})
	})

	_, err := c.GetStats(context.Background(), amfa.StatsOptions{RealmID: ""})
	if err == nil {
		t.Fatal("expected an error for all-realms stats over the API")
	}
	if !errors.Is(err, amfa.ErrAllRealmsUnsupported) {
		t.Errorf("error should wrap ErrAllRealmsUnsupported, got %v", err)
	}
	if errors.Is(err, amfa.ErrAmfaNotConfigured) {
		t.Error("should not be reported as ErrAmfaNotConfigured: AMFA is configured, this aggregate just isn't available over this transport")
	}
	if len(rec.paths) != 0 {
		t.Error("should not have issued a request")
	}
}

func TestRiskyUsersAndEventCountsDecode(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, r *http.Request, _ int) {
		if strings.HasSuffix(r.URL.Path, "/risky-users") {
			// Written as the API actually serialises it (snake_case), not via
			// the untagged domain struct, whose Go field names would not
			// exercise the decode path the client really takes.
			writeJSON(t, w, []map[string]any{{"user_id": "3ab1", "count": 7}})
			return
		}
		writeJSON(t, w, map[string]any{"event_type": "LOGIN_ERROR", "count": 42})
	})

	counts, err := c.CountRepeatedRiskyByUser(context.Background(), "acme", time.Now(), 2, 5)
	if err != nil {
		t.Fatalf("CountRepeatedRiskyByUser: %v", err)
	}
	// UserID is the field to watch: amfa.UserRiskyCount carries no JSON tags,
	// so decoding straight into it silently yields an empty user while Count
	// still binds case-insensitively. An alert rule would then fire against a
	// blank user id.
	if len(counts) != 1 || counts[0].UserID != "3ab1" || counts[0].Count != 7 {
		t.Errorf("risky users did not round-trip: %+v", counts)
	}
	q := rec.queries[0]
	if q.Get("min_risk") != "2" || q.Get("threshold") != "5" {
		t.Errorf("min_risk=%q threshold=%q, want 2/5", q.Get("min_risk"), q.Get("threshold"))
	}

	start := time.Now().Add(-time.Hour)
	total, err := c.CountByEventTypeInWindow(context.Background(), "acme", "LOGIN_ERROR", start, time.Now())
	if err != nil {
		t.Fatalf("CountByEventTypeInWindow: %v", err)
	}
	if total != 42 {
		t.Errorf("count = %d, want 42", total)
	}
}

func TestServerErrorsAreUnavailable(t *testing.T) {
	// 5xx, 408 and 429 mean AMFA is reachable but cannot answer; a handler
	// should surface 503 and a caller may retry.
	for _, status := range []int{500, 502, 503, 504, 408, 429} {
		rec := &recorder{}
		c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("upstream failure"))
		})

		_, err := c.ListEventsSince(context.Background(), "acme", time.Now(), 10)
		if err == nil {
			t.Fatalf("status %d: expected an error", status)
		}
		if !errors.Is(err, amfa.ErrAmfaUnavailable) {
			t.Errorf("status %d: should wrap ErrAmfaUnavailable, got %v", status, err)
		}
	}
}

func TestAuthFailuresAreNotReportedAsUnavailable(t *testing.T) {
	// A rejected or wrong-realm token is a misconfiguration that retrying
	// cannot fix. Labelling it "unavailable" would hide it behind what looks
	// like a transient outage.
	for _, status := range []int{401, 403} {
		rec := &recorder{}
		c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("token realm does not match request realm"))
		})

		_, err := c.ListEventsSince(context.Background(), "acme", time.Now(), 10)
		if err == nil {
			t.Fatalf("status %d: expected an error", status)
		}
		if errors.Is(err, amfa.ErrAmfaUnavailable) {
			t.Errorf("status %d: must not be classified as unavailable: %v", status, err)
		}
	}
}

func TestUnreachableServerIsUnavailable(t *testing.T) {
	c, err := New(
		Config{BaseURL: "http://127.0.0.1:1", Timeout: time.Second},
		StaticTokenSource("t"), nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.ListEventsSince(context.Background(), "acme", time.Now(), 10)
	if err == nil {
		t.Fatal("expected an error against a closed port")
	}
	if !errors.Is(err, amfa.ErrAmfaUnavailable) {
		t.Errorf("should wrap ErrAmfaUnavailable, got %v", err)
	}
}

func TestCancelledContextIsUnavailable(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{})
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.ListEventsSince(ctx, "acme", time.Now(), 10)
	if err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
	if !errors.Is(err, amfa.ErrAmfaUnavailable) {
		t.Errorf("should wrap ErrAmfaUnavailable, got %v", err)
	}
}

func TestMalformedJSONIsNotSilentlyEmpty(t *testing.T) {
	// Returning no rows and no error would read downstream as "a quiet period"
	// rather than a broken response.
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not json"))
	})

	if _, err := c.ListEventsSince(context.Background(), "acme", time.Now(), 10); err == nil {
		t.Fatal("expected a decode error")
	}
}

func TestWindowIsAlwaysBounded(t *testing.T) {
	// An unbounded window would let one request read a realm's whole history.
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, amfa.Stats{})
	})

	if _, err := c.GetStats(context.Background(), amfa.StatsOptions{RealmID: "acme"}); err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	q := rec.queries[0]
	if q.Get("since") == "" || q.Get("until") == "" {
		t.Error("stats request must carry both window bounds")
	}
}

func TestLookbackIsCappedAtMaximum(t *testing.T) {
	c, err := New(
		Config{BaseURL: "https://amfa", LookbackDays: 9999},
		StaticTokenSource("t"), nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.lookbackDay != maxLookbackDays {
		t.Errorf("lookback = %d, want the %d-day cap", c.lookbackDay, maxLookbackDays)
	}
}

func TestWindowMatchesThePostgresImplementation(t *testing.T) {
	// Both implementations must window identically, or the same request returns
	// different data depending on which is wired up.
	end := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	start, gotEnd := normalizeTimeWindow(nil, &end, 30)

	if !gotEnd.Equal(end) {
		t.Errorf("end = %v, want %v", gotEnd, end)
	}
	if want := end.AddDate(0, 0, -30); !start.Equal(want) {
		t.Errorf("start = %v, want %v", start, want)
	}

	tooWide := end.AddDate(0, 0, -365)
	start, _ = normalizeTimeWindow(&tooWide, &end, 30)
	if end.Sub(start) > time.Duration(maxLookbackDays)*24*time.Hour {
		t.Error("an over-wide window was not trimmed to the cap")
	}
}

func TestCountByEventTypeAndClientInWindowAggregatesAndFiltersByThreshold(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(t, w, eventsPage{Items: []eventRow{
			{EventID: "1", EventTime: time.Now(), Client: "app-a"},
			{EventID: "2", EventTime: time.Now(), Client: "app-a"},
			{EventID: "3", EventTime: time.Now(), Client: "app-a"},
			{EventID: "4", EventTime: time.Now(), Client: "app-b"},
			{EventID: "5", EventTime: time.Now(), Client: ""}, // must not surface as a client
		}})
	})

	start := time.Now().Add(-time.Hour)
	counts, err := c.CountByEventTypeAndClientInWindow(context.Background(), "acme", "LOGIN_ERROR", start, time.Now(), 3)
	if err != nil {
		t.Fatalf("CountByEventTypeAndClientInWindow: %v", err)
	}
	if len(counts) != 1 || counts[0].Client != "app-a" || counts[0].Count != 3 {
		t.Errorf("counts = %+v, want exactly one entry {app-a 3} (app-b is below threshold, empty client excluded)", counts)
	}

	q := rec.queries[0]
	if q.Get("event_type") != "LOGIN_ERROR" {
		t.Errorf("event_type = %q, want LOGIN_ERROR", q.Get("event_type"))
	}
	if q.Get("ascending") != "true" {
		t.Errorf("ascending = %q, want true", q.Get("ascending"))
	}
}

func TestCountByEventTypeByUserAggregatesAndFiltersByThreshold(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		u1, u2, empty := "user-1", "user-2", ""
		writeJSON(t, w, eventsPage{Items: []eventRow{
			{EventID: "1", EventTime: time.Now(), UserID: &u1},
			{EventID: "2", EventTime: time.Now(), UserID: &u1},
			{EventID: "3", EventTime: time.Now(), UserID: &u2},
			{EventID: "4", EventTime: time.Now(), UserID: nil},
			{EventID: "5", EventTime: time.Now(), UserID: &empty},
		}})
	})

	counts, err := c.CountByEventTypeByUser(context.Background(), "acme", "LOGIN_ERROR", time.Now().Add(-time.Hour), 2)
	if err != nil {
		t.Fatalf("CountByEventTypeByUser: %v", err)
	}
	if len(counts) != 1 || counts[0].UserID != "user-1" || counts[0].Count != 2 {
		t.Errorf("counts = %+v, want exactly one entry {user-1 2} (user-2 below threshold, nil/empty excluded)", counts)
	}

	q := rec.queries[0]
	if q.Get("event_type") != "LOGIN_ERROR" {
		t.Errorf("event_type = %q, want LOGIN_ERROR", q.Get("event_type"))
	}
	if q.Get("until") != "" {
		t.Error("until must not be set: the postgres implementation has no upper bound for this query")
	}
}

func TestCountDistinctRejectedUsersSinceCountsDistinctUsersOnly(t *testing.T) {
	rec := &recorder{}
	c := newTestClient(t, rec, func(w http.ResponseWriter, _ *http.Request, _ int) {
		u1, u2 := "user-1", "user-2"
		writeJSON(t, w, eventsPage{Items: []eventRow{
			{EventID: "1", EventTime: time.Now(), UserID: &u1},
			{EventID: "2", EventTime: time.Now(), UserID: &u1}, // duplicate, must count once
			{EventID: "3", EventTime: time.Now(), UserID: &u2},
			{EventID: "4", EventTime: time.Now(), UserID: nil}, // must be excluded
		}})
	})

	count, err := c.CountDistinctRejectedUsersSince(context.Background(), "acme", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("CountDistinctRejectedUsersSince: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 distinct users", count)
	}

	// Reuses ListRejectedEventsSince, so it must carry the exact-level filter.
	q := rec.queries[0]
	if q.Get("risk_decision") != "4" {
		t.Errorf("risk_decision = %q, want 4", q.Get("risk_decision"))
	}
}
