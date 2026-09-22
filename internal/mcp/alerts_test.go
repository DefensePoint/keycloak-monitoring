package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// mockAlertReader implements AlertReader for testing.
type mockAlertReader struct {
	getAlertFn      func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
	listAlertsFn    func(ctx context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error)
	getStatisticsFn func(ctx context.Context, tenantID string, realmName ...string) (*alerts.Statistics, error)
}

func (m *mockAlertReader) GetAlert(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	if m.getAlertFn != nil {
		return m.getAlertFn(ctx, tenantID, alertID)
	}
	return nil, errors.New("alert not found")
}

func (m *mockAlertReader) ListAlerts(ctx context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
	if m.listAlertsFn != nil {
		return m.listAlertsFn(ctx, tenantID, opts)
	}
	return nil, 0, nil
}

func (m *mockAlertReader) GetStatistics(ctx context.Context, tenantID string, realmName ...string) (*alerts.Statistics, error) {
	if m.getStatisticsFn != nil {
		return m.getStatisticsFn(ctx, tenantID, realmName...)
	}
	return &alerts.Statistics{BySeverity: map[string]int{}, ByType: map[string]int{}}, nil
}

func alertWithInternals(alertID, realm string, firstDetected time.Time) *domain.Alert {
	ruleID := "rule-42"
	eventID := "event-99"
	return &domain.Alert{
		ID:             1,
		TenantID:       "tenant-a",
		AlertID:        alertID,
		Source:         domain.AlertSourceEvent,
		Type:           "brute_force",
		Severity:       domain.AlertSeverityCritical,
		Status:         domain.AlertStatusActive,
		Title:          "Brute force detected",
		Description:    "Repeated login failures",
		ResourceType:   "realm",
		ResourceName:   realm,
		RealmName:      realm,
		CheckType:      "internal-check-name",
		RuleID:         &ruleID,
		EventID:        &eventID,
		Recommendation: "Lock the account",
		Metadata:       `{"internal_secret":"do-not-leak"}`,
		FirstDetected:  firstDetected,
		LastSeen:       firstDetected,
	}
}

func alertServer(t *testing.T, perms PermissionService, reader *mockAlertReader) *httptest.Server {
	t.Helper()
	return newTestServerFull(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		perms,
		readerFor(enabledTenant("tenant-a")),
		&mockRealmReader{},
		reader,
		&mockAmfaStatsReader{},
		&mockEventReader{})
}

func assertNoInternalAlertFields(t *testing.T, raw []byte) {
	t.Helper()
	for _, leaked := range []string{"metadata", "check_type", "rule_id", "event_id", "rule-42", "event-99", "internal-check-name", "do-not-leak"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("alert DTO leaks %q: %s", leaked, raw)
		}
	}
}

func TestListAlertsWireLevelSanitizedEvenForAdmin(t *testing.T) {
	perms := permsWithRoles(globalRole())
	perms.isAdminFn = func(context.Context, uint) (bool, error) { return true, nil }
	reader := &mockAlertReader{
		listAlertsFn: func(_ context.Context, _ string, _ *alerts.ListOptions) ([]*domain.Alert, int64, error) {
			return []*domain.Alert{alertWithInternals("alert-1", "prod", time.Now())}, 1, nil
		},
	}
	ts := alertServer(t, perms, reader)

	res := callToolResult(t, ts, "list_alerts", map[string]any{"tenant": "tenant-a"})
	if res.IsError {
		t.Fatalf("list_alerts returned tool error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	assertNoInternalAlertFields(t, raw)

	var out listAlertsOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal list output: %v", err)
	}
	if len(out.Alerts) != 1 || out.Alerts[0].AlertID != "alert-1" || out.Total != 1 {
		t.Fatalf("output = %+v, want one alert-1 with total 1", out)
	}
}

func TestGetAlertWireLevelSanitized(t *testing.T) {
	reader := &mockAlertReader{
		getAlertFn: func(_ context.Context, tenantID, alertID string) (*domain.Alert, error) {
			if tenantID == "tenant-a" && alertID == "alert-1" {
				return alertWithInternals("alert-1", "prod", time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)), nil
			}
			return nil, errors.New("record not found")
		},
	}
	ts := alertServer(t, permsWithPolicies(nil), reader)

	res := callToolResult(t, ts, "get_alert", map[string]any{"tenant": "tenant-a", "alert_id": "alert-1"})
	if res.IsError {
		t.Fatalf("get_alert returned tool error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	assertNoInternalAlertFields(t, raw)

	var out alertSummary
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal alert output: %v", err)
	}
	if out.AlertID != "alert-1" || out.RealmName != "prod" || out.FirstDetected != "2026-09-01T10:00:00Z" {
		t.Fatalf("alert = %+v, want alert-1 / prod / 2026-09-01T10:00:00Z", out)
	}
}

func TestGetAlertMissingForeignAndDisallowedRealmIndistinguishable(t *testing.T) {
	missing := &mockAlertReader{
		getAlertFn: func(context.Context, string, string) (*domain.Alert, error) {
			return nil, fmt.Errorf("%w: alert-x in tenant tenant-b", alerts.ErrNotFound)
		},
	}
	tsMissing := alertServer(t, permsWithPolicies(nil), missing)
	missingText := callToolErrText(t, tsMissing, "get_alert", map[string]any{"tenant": "tenant-a", "alert_id": "alert-x"})

	nilRow := &mockAlertReader{
		getAlertFn: func(context.Context, string, string) (*domain.Alert, error) { return nil, nil },
	}
	tsNil := alertServer(t, permsWithPolicies(nil), nilRow)
	nilText := callToolErrText(t, tsNil, "get_alert", map[string]any{"tenant": "tenant-a", "alert_id": "alert-x"})

	outsideRealm := &mockAlertReader{
		getAlertFn: func(context.Context, string, string) (*domain.Alert, error) {
			return alertWithInternals("alert-1", "dev", time.Now()), nil
		},
	}
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod"}}}
	tsOutside := alertServer(t, permsWithPolicies(policies), outsideRealm)
	outsideText := callToolErrText(t, tsOutside, "get_alert", map[string]any{"tenant": "tenant-a", "alert_id": "alert-1"})

	want := ErrAlertNotFound.Error()
	if missingText != want || nilText != want || outsideText != want {
		t.Fatalf("errors = %q / %q / %q, want all %q", missingText, nilText, outsideText, want)
	}
	if strings.Contains(missingText, "tenant-b") {
		t.Fatalf("not-found error leaks lookup detail: %q", missingText)
	}
}

func TestGetAlertRepositoryErrorIsInternal(t *testing.T) {
	reader := &mockAlertReader{
		getAlertFn: func(context.Context, string, string) (*domain.Alert, error) {
			return nil, errors.New("dial tcp 10.0.0.5:5432: connection refused")
		},
	}
	ts := alertServer(t, permsWithPolicies(nil), reader)

	text := callToolErrText(t, ts, "get_alert", map[string]any{"tenant": "tenant-a", "alert_id": "alert-x"})
	if text != errInternal.Error() {
		t.Fatalf("error = %q, want %q (only alerts.ErrNotFound maps to not found)", text, errInternal.Error())
	}
	if strings.Contains(text, "10.0.0.5") {
		t.Fatalf("error leaks connection detail: %q", text)
	}
}

func TestListAlertsDisallowedRealmArgDenied(t *testing.T) {
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod"}}}
	ts := alertServer(t, permsWithPolicies(policies), &mockAlertReader{})

	text := callToolErrText(t, ts, "list_alerts", map[string]any{"tenant": "tenant-a", "realm": "dev"})
	if text != ErrForbidden.Error() {
		t.Fatalf("error = %q, want %q", text, ErrForbidden.Error())
	}
}

func TestListAlertsRestrictedCallerFiltersRealmSetInOneQuery(t *testing.T) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	var got *alerts.ListOptions
	reader := &mockAlertReader{
		listAlertsFn: func(_ context.Context, _ string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
			got = opts
			return []*domain.Alert{
				alertWithInternals("staging-mid", "staging", base.Add(9*time.Hour)),
				alertWithInternals("prod-old", "prod", base.Add(8*time.Hour)),
			}, 3, nil
		},
	}
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod", "staging"}}}
	ts := alertServer(t, permsWithPolicies(policies), reader)

	var out listAlertsOutput
	callToolOK(t, ts, "list_alerts", map[string]any{"tenant": "tenant-a", "limit": 2, "offset": 1}, &out)

	if got == nil {
		t.Fatal("reader was not queried")
	}
	if got.RealmName != "" || !slices.Equal(got.RealmNames, []string{"prod", "staging"}) {
		t.Fatalf("opts = %+v, want the allowed realm set and no single-realm filter", got)
	}
	if got.Limit != 2 || got.Offset != 1 {
		t.Errorf("limit/offset = %d/%d, want 2/1 (database-level pagination)", got.Limit, got.Offset)
	}
	if out.Total != 3 {
		t.Errorf("total = %d, want 3", out.Total)
	}
	if len(out.Alerts) != 2 || out.Alerts[0].AlertID != "staging-mid" || out.Alerts[1].AlertID != "prod-old" {
		t.Fatalf("alerts = %+v, want [staging-mid prod-old]", out.Alerts)
	}
}

func TestListAlertsRestrictedCallerOffsetBeyondRows(t *testing.T) {
	reader := &mockAlertReader{
		listAlertsFn: func(_ context.Context, _ string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
			if opts.Offset != 50 {
				return nil, 0, fmt.Errorf("offset = %d, want 50", opts.Offset)
			}
			return nil, 3, nil
		},
	}
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod", "staging"}}}
	ts := alertServer(t, permsWithPolicies(policies), reader)

	var out listAlertsOutput
	callToolOK(t, ts, "list_alerts", map[string]any{"tenant": "tenant-a", "offset": 50}, &out)

	if len(out.Alerts) != 0 || out.Total != 3 || out.Offset != 50 {
		t.Fatalf("output = %+v, want empty page with total 3 at offset 50", out)
	}
}

func TestListAlertsAllowedRealmArgUsesDatabasePagination(t *testing.T) {
	var got *alerts.ListOptions
	reader := &mockAlertReader{
		listAlertsFn: func(_ context.Context, _ string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
			got = opts
			return nil, 0, nil
		},
	}
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod", "staging"}}}
	ts := alertServer(t, permsWithPolicies(policies), reader)

	var out listAlertsOutput
	callToolOK(t, ts, "list_alerts", map[string]any{"tenant": "tenant-a", "realm": "prod", "offset": 5}, &out)

	if got == nil || got.RealmName != "prod" || got.Offset != 5 {
		t.Fatalf("opts = %+v, want realm prod with offset 5", got)
	}
}

func TestListAlertsLimitClamped(t *testing.T) {
	var limits []int
	reader := &mockAlertReader{
		listAlertsFn: func(_ context.Context, _ string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
			limits = append(limits, opts.Limit)
			return nil, 0, nil
		},
	}
	ts := alertServer(t, permsWithPolicies(nil), reader)

	var out listAlertsOutput
	callToolOK(t, ts, "list_alerts", map[string]any{"tenant": "tenant-a", "limit": 10000}, &out)
	callToolOK(t, ts, "list_alerts", map[string]any{"tenant": "tenant-a"}, &out)

	if len(limits) != 2 || limits[0] != 500 || limits[1] != 50 {
		t.Fatalf("limits = %v, want [500 50] (absolute max, then default)", limits)
	}
}

func TestListAlertsOffsetCapped(t *testing.T) {
	ts := alertServer(t, permsWithPolicies(nil), &mockAlertReader{})

	text := callToolErrText(t, ts, "list_alerts", map[string]any{"tenant": "tenant-a", "offset": maxAlertOffset + 1})
	if text != "invalid input: offset must be between 0 and 10000" {
		t.Fatalf("error = %q, want offset bound message", text)
	}
}

func TestGetAlertStatsDisallowedRealmDenied(t *testing.T) {
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod"}}}
	ts := alertServer(t, permsWithPolicies(policies), &mockAlertReader{})

	text := callToolErrText(t, ts, "get_alert_stats", map[string]any{"tenant": "tenant-a", "realm": "dev"})
	if text != ErrForbidden.Error() {
		t.Fatalf("error = %q, want %q", text, ErrForbidden.Error())
	}
}

func TestGetAlertStatsRestrictedCallerQueriesAllowedRealmSet(t *testing.T) {
	var queried [][]string
	reader := &mockAlertReader{
		getStatisticsFn: func(_ context.Context, _ string, realmName ...string) (*alerts.Statistics, error) {
			queried = append(queried, realmName)
			return &alerts.Statistics{
				TotalActive: 3,
				BySeverity:  map[string]int{"critical": 2, "warning": 1},
				ByType:      map[string]int{"brute_force": 2, "config": 1},
			}, nil
		},
	}
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod", "staging"}}}
	ts := alertServer(t, permsWithPolicies(policies), reader)

	var out alertStatsOutput
	callToolOK(t, ts, "get_alert_stats", map[string]any{"tenant": "tenant-a"}, &out)

	if len(queried) != 1 || !slices.Equal(queried[0], []string{"prod", "staging"}) {
		t.Fatalf("queries = %v, want one query over the allowed realm set", queried)
	}
	if out.TotalActive != 3 {
		t.Errorf("total_active = %d, want 3", out.TotalActive)
	}
	if out.BySeverity["critical"] != 2 || out.BySeverity["warning"] != 1 {
		t.Errorf("by_severity = %v, want critical 2 warning 1", out.BySeverity)
	}
	if out.ByType["brute_force"] != 2 || out.ByType["config"] != 1 {
		t.Errorf("by_type = %v, want brute_force 2 config 1", out.ByType)
	}
}

func TestGetAlertStatsCollapsedKeysSum(t *testing.T) {
	reader := &mockAlertReader{
		getStatisticsFn: func(_ context.Context, _ string, _ ...string) (*alerts.Statistics, error) {
			return &alerts.Statistics{
				TotalActive: 5,
				BySeverity:  map[string]int{"critical": 2, "critical\u200b": 3},
				ByType:      map[string]int{},
			}, nil
		},
	}
	ts := alertServer(t, permsWithPolicies(nil), reader)

	var out alertStatsOutput
	callToolOK(t, ts, "get_alert_stats", map[string]any{"tenant": "tenant-a"}, &out)

	if len(out.BySeverity) != 1 || out.BySeverity["critical"] != 5 {
		t.Fatalf("by_severity = %v, want keys collapsed by Clean to sum to critical 5", out.BySeverity)
	}
}

func TestGetAlertStatsUnrestrictedOmittedRealmAggregatesTenant(t *testing.T) {
	var gotRealms []string
	reader := &mockAlertReader{
		getStatisticsFn: func(_ context.Context, _ string, realmName ...string) (*alerts.Statistics, error) {
			gotRealms = realmName
			return &alerts.Statistics{TotalActive: 7, BySeverity: map[string]int{"critical": 7}, ByType: map[string]int{}}, nil
		},
	}
	ts := alertServer(t, permsWithPolicies(nil), reader)

	var out alertStatsOutput
	callToolOK(t, ts, "get_alert_stats", map[string]any{"tenant": "tenant-a"}, &out)

	if len(gotRealms) != 0 {
		t.Fatalf("realms = %v, want none (tenant-wide query)", gotRealms)
	}
	if out.TotalActive != 7 || out.BySeverity["critical"] != 7 {
		t.Fatalf("output = %+v, want total 7 critical 7", out)
	}
}

func TestNoAlertRealmsAllowedFailsClosedOnAnEmptyScope(t *testing.T) {
	tests := []struct {
		name     string
		realmArg string
		allowed  []string
		all      bool
		want     bool
	}{
		{"restricted caller with no allowed realm", "", nil, false, true},
		{"restricted caller with allowed realms", "", []string{"prod"}, false, false},
		{"unrestricted caller", "", nil, true, false},
		{"realm argument carries its own scope", "prod", nil, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noAlertRealmsAllowed(tt.realmArg, tt.allowed, tt.all); got != tt.want {
				t.Errorf("noAlertRealmsAllowed = %v, want %v", got, tt.want)
			}
		})
	}
}
