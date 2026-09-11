package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DefensePoint/keycloak-monitoring/apitoken"
	"github.com/DefensePoint/keycloak-monitoring/events"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// mockEventReader implements EventReader for testing.
type mockEventReader struct {
	getFn        func(ctx context.Context, tenantID, eventID string) (*domain.Event, error)
	listKeysetFn func(ctx context.Context, opts *events.ListOptions, after *events.KeysetPosition) (*events.KeysetPage, error)
	statsFn      func(ctx context.Context, opts *events.ListOptions) (*events.Stats, error)
}

func (m *mockEventReader) GetByTenantAndEventID(ctx context.Context, tenantID, eventID string) (*domain.Event, error) {
	if m.getFn != nil {
		return m.getFn(ctx, tenantID, eventID)
	}
	return nil, fmt.Errorf("%w: %s", events.ErrNotFound, eventID)
}

func (m *mockEventReader) ListKeyset(ctx context.Context, opts *events.ListOptions, after *events.KeysetPosition) (*events.KeysetPage, error) {
	if m.listKeysetFn != nil {
		return m.listKeysetFn(ctx, opts, after)
	}
	return &events.KeysetPage{}, nil
}

func (m *mockEventReader) Stats(ctx context.Context, opts *events.ListOptions) (*events.Stats, error) {
	if m.statsFn != nil {
		return m.statsFn(ctx, opts)
	}
	return &events.Stats{ByType: map[string]int64{}, BySeverity: map[string]int64{}, BySource: map[string]int64{}}, nil
}

func eventInRealm(id uint, eventID, realm string, ts time.Time) *domain.Event {
	risk := 42
	finalStatus := "allowed"
	isVPN := true
	country := "DK"
	city := "Copenhagen"
	osName := "macOS"
	browser := "Firefox"
	device := "desktop"
	return &domain.Event{
		ID:              id,
		TenantID:        "tenant-a",
		EventID:         eventID,
		Type:            "LOGIN",
		Category:        "authentication",
		Severity:        "info",
		Description:     "User logged in",
		Source:          events.SourceForKeycloakRealm(realm),
		SourceIP:        "10.1.2.3",
		SourceSystem:    "keycloak",
		UserID:          "kc-user-1",
		Username:        "alice",
		Email:           "alice@example.com",
		ClientID:        "web-app",
		RawData:         `{"internal_secret":"raw-data-must-not-leak"}`,
		Status:          "success",
		Timestamp:       ts,
		RiskLevel:       &risk,
		FinalStatus:     &finalStatus,
		IsVPN:           &isVPN,
		Country:         &country,
		City:            &city,
		OperatingSystem: &osName,
		Browser:         &browser,
		Device:          &device,
	}
}

func eventServer(t *testing.T, perms PermissionService, reader *mockEventReader) *httptest.Server {
	t.Helper()
	return newTestServerFull(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		perms,
		readerFor(enabledTenant("tenant-a")),
		realmsNamed("prod", "staging", "dev"),
		&mockAlertReader{},
		&mockAmfaStatsReader{},
		reader)
}

// eventServerClocked is eventServer reading the supplied clock, for tests that
// advance time between calls.
func eventServerClocked(t *testing.T, perms PermissionService, reader *mockEventReader, clock Clock) *httptest.Server {
	t.Helper()
	return newTestServerClocked(t,
		defaultTestCfg(),
		testLogger(),
		nil,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		perms,
		readerFor(enabledTenant("tenant-a")),
		realmsNamed("prod", "staging", "dev"),
		&mockAlertReader{},
		&mockAmfaStatsReader{},
		reader,
		clock)
}

func callToolResultAs(t *testing.T, ts *httptest.Server, token, name string, args map[string]any) *mcpsdk.CallToolResult {
	t.Helper()
	session := connectClient(t, ts, token)
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s call failed: %v", name, err)
	}
	return res
}

func errTextOf(t *testing.T, res *mcpsdk.CallToolResult, name string) string {
	t.Helper()
	if !res.IsError {
		t.Fatalf("%s: expected tool error, got %+v", name, res.StructuredContent)
	}
	if len(res.Content) == 0 {
		t.Fatalf("%s: tool error without content", name)
	}
	text, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("%s: unexpected error content %T", name, res.Content[0])
	}
	return text.Text
}

func assertNoRawEventData(t *testing.T, raw []byte) {
	t.Helper()
	for _, leaked := range []string{"raw_data", "internal_secret", "raw-data-must-not-leak"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("event DTO leaks %q: %s", leaked, raw)
		}
	}
}

func TestListEventsRestrictedCallerRealmOmittedScopesSources(t *testing.T) {
	var got *events.ListOptions
	reader := &mockEventReader{
		listKeysetFn: func(_ context.Context, opts *events.ListOptions, _ *events.KeysetPosition) (*events.KeysetPage, error) {
			got = opts
			return &events.KeysetPage{}, nil
		},
	}
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod", "staging"}}}
	ts := eventServer(t, permsWithPolicies(policies), reader)

	var out listEventsOutput
	callToolOK(t, ts, "list_events", map[string]any{"tenant": "tenant-a"}, &out)

	if got == nil {
		t.Fatal("reader was not queried")
	}
	want := []string{"keycloak:prod", "amfa:prod", "keycloak:staging", "amfa:staging"}
	if !slices.Equal(got.Sources, want) {
		t.Fatalf("sources = %v, want %v (allowed realms only, dev excluded)", got.Sources, want)
	}
	if got.TenantID != "tenant-a" {
		t.Fatalf("tenant = %q, want tenant-a", got.TenantID)
	}
}

func TestListEventsDisallowedRealmArgDenied(t *testing.T) {
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod"}}}
	ts := eventServer(t, permsWithPolicies(policies), &mockEventReader{})

	text := callToolErrText(t, ts, "list_events", map[string]any{"tenant": "tenant-a", "realm": "dev"})
	if text != ErrForbidden.Error() {
		t.Fatalf("error = %q, want %q", text, ErrForbidden.Error())
	}
}

func TestListEventsNoResolvableSourcesReturnsEmptyWithoutQuery(t *testing.T) {
	queried := false
	reader := &mockEventReader{
		listKeysetFn: func(context.Context, *events.ListOptions, *events.KeysetPosition) (*events.KeysetPage, error) {
			queried = true
			return &events.KeysetPage{}, nil
		},
	}
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"unrelated"}}}
	ts := eventServer(t, permsWithPolicies(policies), reader)

	var out listEventsOutput
	callToolOK(t, ts, "list_events", map[string]any{"tenant": "tenant-a"}, &out)

	if queried {
		t.Fatal("repository queried despite an empty resolved source set")
	}
	if len(out.Events) != 0 || out.NextCursor != "" {
		t.Fatalf("output = %+v, want empty page without cursor", out)
	}
}

func TestListEventsFilterAndWindowMapping(t *testing.T) {
	var got *events.ListOptions
	reader := &mockEventReader{
		listKeysetFn: func(_ context.Context, opts *events.ListOptions, _ *events.KeysetPosition) (*events.KeysetPage, error) {
			got = opts
			return &events.KeysetPage{}, nil
		},
	}
	ts := eventServer(t, permsWithPolicies(nil), reader)

	var out listEventsOutput
	callToolOK(t, ts, "list_events", map[string]any{
		"tenant":   "tenant-a",
		"from":     "2026-09-01T00:00:00Z",
		"to":       "2026-09-02T00:00:00Z",
		"type":     "LOGIN",
		"severity": "warning",
		"min_risk": 40,
		"limit":    25,
	}, &out)

	if got == nil {
		t.Fatal("reader was not queried")
	}
	if got.Type != "LOGIN" || got.Severity != "warning" {
		t.Errorf("type/severity = %q/%q, want LOGIN/warning", got.Type, got.Severity)
	}
	if got.MinRisk == nil || *got.MinRisk != 40 {
		t.Errorf("min_risk = %v, want 40", got.MinRisk)
	}
	if got.Limit != 25 {
		t.Errorf("limit = %d, want 25", got.Limit)
	}
	if got.StartTime == nil || *got.StartTime != "2026-09-01T00:00:00Z" {
		t.Errorf("start_time = %v, want 2026-09-01T00:00:00Z", got.StartTime)
	}
	if got.EndTime == nil || *got.EndTime != "2026-09-02T00:00:00Z" {
		t.Errorf("end_time = %v, want 2026-09-02T00:00:00Z", got.EndTime)
	}
}

func TestListEventsMalformedWindowRejected(t *testing.T) {
	queried := false
	reader := &mockEventReader{
		listKeysetFn: func(context.Context, *events.ListOptions, *events.KeysetPosition) (*events.KeysetPage, error) {
			queried = true
			return &events.KeysetPage{}, nil
		},
	}
	ts := eventServer(t, permsWithPolicies(nil), reader)

	for _, args := range []map[string]any{
		{"tenant": "tenant-a", "from": "yesterday"},
		{"tenant": "tenant-a", "to": "2026-09-01 10:00:00"},
	} {
		text := callToolErrText(t, ts, "list_events", args)
		if !strings.HasPrefix(text, "invalid input") {
			t.Fatalf("error = %q, want invalid input for %v", text, args)
		}
	}
	if queried {
		t.Fatal("repository queried despite malformed window")
	}
}

func TestListEventsCursorRoundTripAcrossPages(t *testing.T) {
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	midTS := base.Add(time.Hour).Add(123456789 * time.Nanosecond)
	newest := eventInRealm(3, "ev-3", "prod", base.Add(2*time.Hour))
	mid := eventInRealm(2, "ev-2", "prod", midTS)
	oldest := eventInRealm(1, "ev-1", "prod", base)

	var afterSeen []*events.KeysetPosition
	reader := &mockEventReader{
		listKeysetFn: func(_ context.Context, _ *events.ListOptions, after *events.KeysetPosition) (*events.KeysetPage, error) {
			afterSeen = append(afterSeen, after)
			if after == nil {
				return &events.KeysetPage{
					Events: []*domain.Event{newest, mid},
					Last:   &events.KeysetPosition{Timestamp: mid.Timestamp, ID: mid.ID},
				}, nil
			}
			return &events.KeysetPage{
				Events: []*domain.Event{oldest},
				Last:   &events.KeysetPosition{Timestamp: oldest.Timestamp, ID: oldest.ID},
			}, nil
		},
	}
	ts := eventServer(t, permsWithPolicies(nil), reader)

	args := map[string]any{
		"tenant": "tenant-a",
		"from":   "2026-09-01T00:00:00Z",
		"to":     "2026-09-02T00:00:00Z",
		"limit":  2,
	}
	var page1 listEventsOutput
	callToolOK(t, ts, "list_events", args, &page1)
	if len(page1.Events) != 2 || page1.Events[0].EventID != "ev-3" || page1.Events[1].EventID != "ev-2" {
		t.Fatalf("page 1 = %+v, want [ev-3 ev-2]", page1.Events)
	}
	if page1.NextCursor == "" {
		t.Fatal("page 1 returned no cursor for a full page")
	}

	args["cursor"] = page1.NextCursor
	var page2 listEventsOutput
	callToolOK(t, ts, "list_events", args, &page2)
	if len(page2.Events) != 1 || page2.Events[0].EventID != "ev-1" {
		t.Fatalf("page 2 = %+v, want [ev-1]", page2.Events)
	}
	if page2.NextCursor != "" {
		t.Fatalf("next_cursor = %q, want empty on a short page", page2.NextCursor)
	}

	if len(afterSeen) != 2 || afterSeen[0] != nil || afterSeen[1] == nil {
		t.Fatalf("after positions = %v, want [nil, non-nil]", afterSeen)
	}
	wantTS := time.UnixMicro(midTS.UnixMicro()).UTC()
	if !afterSeen[1].Timestamp.Equal(wantTS) || afterSeen[1].ID != 2 {
		t.Fatalf("page 2 resumed at %v id %d, want %v id 2 (microsecond truncation)",
			afterSeen[1].Timestamp, afterSeen[1].ID, wantTS)
	}
}

// windowedEventReader serves a fixed newest-first event set the way the
// repository does: rows outside the window in opts are excluded, and a
// position resumes strictly after it in (timestamp DESC, id DESC) order.
func windowedEventReader(t *testing.T, rows []*domain.Event) *mockEventReader {
	t.Helper()
	return &mockEventReader{
		listKeysetFn: func(_ context.Context, opts *events.ListOptions, after *events.KeysetPosition) (*events.KeysetPage, error) {
			start, err := time.Parse(time.RFC3339Nano, *opts.StartTime)
			if err != nil {
				return nil, err
			}
			end, err := time.Parse(time.RFC3339Nano, *opts.EndTime)
			if err != nil {
				return nil, err
			}

			page := &events.KeysetPage{}
			for _, row := range rows {
				at := row.Timestamp
				if at.Before(start) || at.After(end) {
					continue
				}
				if after != nil {
					// Keyset order is (timestamp DESC, id DESC), so a row
					// continues the page only when it sorts strictly after
					// the cursor position.
					pastCursor := at.Before(after.Timestamp) ||
						(at.Equal(after.Timestamp) && row.ID < after.ID)
					if !pastCursor {
						continue
					}
				}
				page.Events = append(page.Events, row)
				page.Last = &events.KeysetPosition{Timestamp: at, ID: row.ID}
				if len(page.Events) == opts.Limit {
					break
				}
			}
			return page, nil
		},
	}
}

func TestListEventsDefaultWindowPaginatesWholeSetDespiteInterPageDelay(t *testing.T) {
	const (
		seeded    = 25
		pageSize  = 10
		pageDelay = 400 * time.Millisecond
	)

	// The clock is the test's to move: the band below sits a fixed distance
	// inside the tail of the window the first page resolves, so setup latency
	// on a loaded machine cannot shift the seeded events out of it.
	clock := newFakeClock(time.Now().UTC().Truncate(time.Microsecond))

	// The seeded band sits just inside the tail of the default window, close
	// enough that a window re-resolved per page would leave its oldest events
	// behind the newer start.
	base := clock.Now().Add(-DefaultWindow).Add(300 * time.Millisecond)
	rows := make([]*domain.Event, 0, seeded)
	for id := seeded; id >= 1; id-- {
		rows = append(rows, eventInRealm(uint(id), fmt.Sprintf("ev-%d", id), "prod",
			base.Add(time.Duration(id)*10*time.Millisecond)))
	}
	ts := eventServerClocked(t, permsWithPolicies(nil), windowedEventReader(t, rows), clock.Now)

	args := map[string]any{"tenant": "tenant-a", "limit": pageSize}
	var got []string
	for page := 1; ; page++ {
		var out listEventsOutput
		callToolOK(t, ts, "list_events", args, &out)
		for _, event := range out.Events {
			got = append(got, event.EventID)
		}
		if out.NextCursor == "" {
			break
		}
		if page > seeded {
			t.Fatal("pagination did not terminate")
		}
		args["cursor"] = out.NextCursor
		// The client's think time between pages. Advancing the clock instead
		// of sleeping keeps the window's forward slide under test while the
		// test itself stays fast and deterministic.
		clock.Advance(pageDelay)
	}

	want := make([]string, 0, seeded)
	for id := seeded; id >= 1; id-- {
		want = append(want, fmt.Sprintf("ev-%d", id))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("paginated %d of %d events (%v); every seeded event must survive the inter-page delay",
			len(got), seeded, got)
	}
}

func fullPageReader(t *testing.T) *mockEventReader {
	t.Helper()
	row := eventInRealm(9, "ev-9", "prod", time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC))
	return &mockEventReader{
		listKeysetFn: func(context.Context, *events.ListOptions, *events.KeysetPosition) (*events.KeysetPage, error) {
			return &events.KeysetPage{
				Events: []*domain.Event{row},
				Last:   &events.KeysetPosition{Timestamp: row.Timestamp, ID: row.ID},
			}, nil
		},
	}
}

func mintCursor(t *testing.T, ts *httptest.Server, args map[string]any) string {
	t.Helper()
	var page listEventsOutput
	callToolOK(t, ts, "list_events", args, &page)
	if page.NextCursor == "" {
		t.Fatal("no cursor minted for a full page")
	}
	return page.NextCursor
}

func TestListEventsMalformedAndTamperedCursorsRejected(t *testing.T) {
	ts := eventServer(t, permsWithPolicies(nil), fullPageReader(t))
	args := map[string]any{
		"tenant": "tenant-a",
		"from":   "2026-09-01T00:00:00Z",
		"to":     "2026-09-02T00:00:00Z",
		"limit":  1,
	}
	cursor := mintCursor(t, ts, args)

	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	var payload cursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal cursor: %v", err)
	}
	payload.ID++
	tampered, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal tampered cursor: %v", err)
	}

	for name, bad := range map[string]string{
		"not base64":       "%%not-base64%%",
		"not json":         base64.RawURLEncoding.EncodeToString([]byte("plain text")),
		"tampered payload": base64.RawURLEncoding.EncodeToString(tampered),
	} {
		args["cursor"] = bad
		text := callToolErrText(t, ts, "list_events", args)
		if text != ErrInvalidInput.Error() {
			t.Fatalf("%s: error = %q, want %q", name, text, ErrInvalidInput.Error())
		}
	}
}

func TestListEventsCursorWithDifferentFiltersRejected(t *testing.T) {
	ts := eventServer(t, permsWithPolicies(nil), fullPageReader(t))
	args := map[string]any{
		"tenant": "tenant-a",
		"from":   "2026-09-01T00:00:00Z",
		"to":     "2026-09-02T00:00:00Z",
		"limit":  1,
	}
	cursor := mintCursor(t, ts, args)

	args["cursor"] = cursor
	args["type"] = "LOGIN_ERROR"
	text := callToolErrText(t, ts, "list_events", args)
	if text != ErrInvalidInput.Error() {
		t.Fatalf("error = %q, want %q (filter digest mismatch)", text, ErrInvalidInput.Error())
	}
}

func TestListEventsCursorBoundToToken(t *testing.T) {
	tokens := &mockTokenValidator{
		validateFn: func(_ context.Context, plaintext string) (*apitoken.Identity, error) {
			user := &domain.User{ID: 7, Subject: "user-7", IsActive: true}
			switch plaintext {
			case "pat_good":
				return &apitoken.Identity{TokenID: 1, User: user, TenantIDs: []string{"tenant-a", "tenant-b"}}, nil
			case "pat_other":
				return &apitoken.Identity{TokenID: 2, User: user, TenantIDs: []string{"tenant-a", "tenant-b"}}, nil
			}
			return nil, apitoken.ErrTokenNotFound
		},
	}
	ts := newTestServerFull(t,
		tokens,
		permsWithPolicies(nil),
		readerFor(enabledTenant("tenant-a")),
		realmsNamed("prod"),
		&mockAlertReader{},
		&mockAmfaStatsReader{},
		fullPageReader(t))

	args := map[string]any{
		"tenant": "tenant-a",
		"from":   "2026-09-01T00:00:00Z",
		"to":     "2026-09-02T00:00:00Z",
		"limit":  1,
	}
	args["cursor"] = mintCursor(t, ts, args)

	res := callToolResultAs(t, ts, "pat_other", "list_events", args)
	text := errTextOf(t, res, "list_events")
	if text != ErrInvalidInput.Error() {
		t.Fatalf("error = %q, want %q (cursor MAC binds the token id)", text, ErrInvalidInput.Error())
	}
}

func TestListEventsCursorOutsideWindowRejected(t *testing.T) {
	old := eventInRealm(4, "ev-old", "prod", time.Now().UTC().Add(-30*time.Hour))
	reader := &mockEventReader{
		listKeysetFn: func(context.Context, *events.ListOptions, *events.KeysetPosition) (*events.KeysetPage, error) {
			return &events.KeysetPage{
				Events: []*domain.Event{old},
				Last:   &events.KeysetPosition{Timestamp: old.Timestamp, ID: old.ID},
			}, nil
		},
	}
	ts := eventServer(t, permsWithPolicies(nil), reader)

	args := map[string]any{"tenant": "tenant-a", "limit": 1}
	args["cursor"] = mintCursor(t, ts, args)

	text := callToolErrText(t, ts, "list_events", args)
	if text != ErrInvalidInput.Error() {
		t.Fatalf("error = %q, want %q (position outside the default window)", text, ErrInvalidInput.Error())
	}
}

func TestListEventsWireLevelOmitsRawDataEvenForAdmin(t *testing.T) {
	perms := permsWithRoles(globalRole())
	perms.isAdminFn = func(context.Context, uint) (bool, error) { return true, nil }
	reader := &mockEventReader{
		listKeysetFn: func(context.Context, *events.ListOptions, *events.KeysetPosition) (*events.KeysetPage, error) {
			row := eventInRealm(1, "ev-1", "prod", time.Now().UTC())
			return &events.KeysetPage{
				Events: []*domain.Event{row},
				Last:   &events.KeysetPosition{Timestamp: row.Timestamp, ID: row.ID},
			}, nil
		},
	}
	ts := eventServer(t, perms, reader)

	res := callToolResult(t, ts, "list_events", map[string]any{"tenant": "tenant-a"})
	if res.IsError {
		t.Fatalf("list_events returned tool error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	assertNoRawEventData(t, raw)
}

func TestGetEventWireLevelOmitsRawData(t *testing.T) {
	reader := &mockEventReader{
		getFn: func(_ context.Context, tenantID, eventID string) (*domain.Event, error) {
			if tenantID == "tenant-a" && eventID == "ev-1" {
				return eventInRealm(1, "ev-1", "prod", time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)), nil
			}
			return nil, fmt.Errorf("%w: %s", events.ErrNotFound, eventID)
		},
	}
	ts := eventServer(t, permsWithPolicies(nil), reader)

	res := callToolResult(t, ts, "get_event", map[string]any{"tenant": "tenant-a", "event_id": "ev-1"})
	if res.IsError {
		t.Fatalf("get_event returned tool error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	assertNoRawEventData(t, raw)

	var out eventSummary
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal event output: %v", err)
	}
	if out.EventID != "ev-1" || out.Realm != "prod" || out.Timestamp != "2026-09-01T10:00:00Z" {
		t.Fatalf("event = %+v, want ev-1 / prod / 2026-09-01T10:00:00Z", out)
	}
	if out.Email != "alice@example.com" || out.RiskLevel == nil || *out.RiskLevel != 42 {
		t.Fatalf("event = %+v, want email and risk level present", out)
	}
}

func TestGetEventMissingForeignAndDisallowedRealmIndistinguishable(t *testing.T) {
	missing := &mockEventReader{
		getFn: func(context.Context, string, string) (*domain.Event, error) {
			return nil, fmt.Errorf("%w: ev-x in tenant tenant-b", events.ErrNotFound)
		},
	}
	tsMissing := eventServer(t, permsWithPolicies(nil), missing)
	missingText := callToolErrText(t, tsMissing, "get_event", map[string]any{"tenant": "tenant-a", "event_id": "ev-x"})

	nilRow := &mockEventReader{
		getFn: func(context.Context, string, string) (*domain.Event, error) { return nil, nil },
	}
	tsNil := eventServer(t, permsWithPolicies(nil), nilRow)
	nilText := callToolErrText(t, tsNil, "get_event", map[string]any{"tenant": "tenant-a", "event_id": "ev-x"})

	outsideRealm := &mockEventReader{
		getFn: func(context.Context, string, string) (*domain.Event, error) {
			return eventInRealm(1, "ev-1", "dev", time.Now().UTC()), nil
		},
	}
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod"}}}
	tsOutside := eventServer(t, permsWithPolicies(policies), outsideRealm)
	outsideText := callToolErrText(t, tsOutside, "get_event", map[string]any{"tenant": "tenant-a", "event_id": "ev-1"})

	want := ErrEventNotFound.Error()
	if missingText != want || nilText != want || outsideText != want {
		t.Fatalf("errors = %q / %q / %q, want all %q", missingText, nilText, outsideText, want)
	}
	if strings.Contains(missingText, "tenant-b") {
		t.Fatalf("not-found error leaks lookup detail: %q", missingText)
	}
}

func TestGetEventStatsScopedToAllowedSources(t *testing.T) {
	var got *events.ListOptions
	reader := &mockEventReader{
		statsFn: func(_ context.Context, opts *events.ListOptions) (*events.Stats, error) {
			got = opts
			return &events.Stats{ByType: map[string]int64{}, BySeverity: map[string]int64{}, BySource: map[string]int64{}}, nil
		},
	}
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod"}}}
	ts := eventServer(t, permsWithPolicies(policies), reader)

	var out eventStatsOutput
	callToolOK(t, ts, "get_event_stats", map[string]any{
		"tenant": "tenant-a",
		"from":   "2026-09-01T00:00:00Z",
		"to":     "2026-09-02T00:00:00Z",
	}, &out)

	if got == nil {
		t.Fatal("reader was not queried")
	}
	if !slices.Equal(got.Sources, []string{"keycloak:prod", "amfa:prod"}) || got.TenantID != "tenant-a" {
		t.Fatalf("opts = %+v, want tenant-a scoped to prod sources", got)
	}
	if got.StartTime == nil || *got.StartTime != "2026-09-01T00:00:00Z" || got.EndTime == nil || *got.EndTime != "2026-09-02T00:00:00Z" {
		t.Fatalf("window = %v..%v, want the requested window", got.StartTime, got.EndTime)
	}
}

func TestGetEventStatsNoResolvableSourcesReturnsEmptyWithoutQuery(t *testing.T) {
	queried := false
	reader := &mockEventReader{
		statsFn: func(context.Context, *events.ListOptions) (*events.Stats, error) {
			queried = true
			return &events.Stats{}, nil
		},
	}
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"unrelated"}}}
	ts := eventServer(t, permsWithPolicies(policies), reader)

	var out eventStatsOutput
	callToolOK(t, ts, "get_event_stats", map[string]any{"tenant": "tenant-a"}, &out)

	if queried {
		t.Fatal("repository queried despite an empty resolved source set")
	}
	if len(out.ByType) != 0 || len(out.BySeverity) != 0 || len(out.BySource) != 0 {
		t.Fatalf("output = %+v, want empty maps", out)
	}
}

func TestGetEventStatsDisallowedRealmDenied(t *testing.T) {
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod"}}}
	ts := eventServer(t, permsWithPolicies(policies), &mockEventReader{})

	text := callToolErrText(t, ts, "get_event_stats", map[string]any{"tenant": "tenant-a", "realm": "dev"})
	if text != ErrForbidden.Error() {
		t.Fatalf("error = %q, want %q", text, ErrForbidden.Error())
	}
}

func TestGetEventStatsCollapsedKeysSum(t *testing.T) {
	reader := &mockEventReader{
		statsFn: func(context.Context, *events.ListOptions) (*events.Stats, error) {
			return &events.Stats{
				ByType:     map[string]int64{"LOGIN": 2, "LOGIN\u200b": 3},
				BySeverity: map[string]int64{"info": 4, "info\u200b": 1},
				BySource:   map[string]int64{"keycloak:prod": 2, "keycloak:prod\u200b": 5},
			}, nil
		},
	}
	ts := eventServer(t, permsWithPolicies(nil), reader)

	var out eventStatsOutput
	callToolOK(t, ts, "get_event_stats", map[string]any{"tenant": "tenant-a"}, &out)

	if len(out.ByType) != 1 || out.ByType["LOGIN"] != 5 {
		t.Fatalf("by_type = %v, want keys collapsed by Clean to sum to LOGIN 5", out.ByType)
	}
	if len(out.BySeverity) != 1 || out.BySeverity["info"] != 5 {
		t.Fatalf("by_severity = %v, want info 5", out.BySeverity)
	}
	if len(out.BySource) != 1 || out.BySource["keycloak:prod"] != 7 {
		t.Fatalf("by_source = %v, want keycloak:prod 7", out.BySource)
	}
}
