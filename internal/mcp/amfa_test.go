package mcp

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// mockAmfaStatsReader implements AmfaStatsReader for testing.
type mockAmfaStatsReader struct {
	getStatsFn func(ctx context.Context, tenantID string, opts amfa.StatsOptions) (amfa.Stats, error)
}

func (m *mockAmfaStatsReader) GetStats(ctx context.Context, tenantID string, opts amfa.StatsOptions) (amfa.Stats, error) {
	if m.getStatsFn != nil {
		return m.getStatsFn(ctx, tenantID, opts)
	}
	return amfa.Stats{}, nil
}

func amfaServer(t *testing.T, perms PermissionService, reader *mockAmfaStatsReader) *httptest.Server {
	t.Helper()
	return newTestServerFull(t,
		tokenFor([]string{"tenant-a", "tenant-b"}),
		perms,
		readerFor(enabledTenant("tenant-a")),
		&mockRealmReader{},
		&mockAlertReader{},
		reader,
		&mockEventReader{})
}

func TestGetAmfaStatsRealmRequired(t *testing.T) {
	called := false
	reader := &mockAmfaStatsReader{
		getStatsFn: func(context.Context, string, amfa.StatsOptions) (amfa.Stats, error) {
			called = true
			return amfa.Stats{}, nil
		},
	}
	ts := amfaServer(t, permsWithPolicies(nil), reader)

	text := callToolErrText(t, ts, "get_amfa_stats", map[string]any{"tenant": "tenant-a", "realm": ""})
	if text != "invalid input: realm is required" {
		t.Fatalf("error = %q, want %q", text, "invalid input: realm is required")
	}

	session := connectClient(t, ts, "pat_good")
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "get_amfa_stats",
		Arguments: map[string]any{"tenant": "tenant-a"},
	})
	if err == nil && !res.IsError {
		t.Fatal("expected omitting realm to fail (schema-level required field)")
	}
	if called {
		t.Fatal("GetStats must never run without a realm")
	}
}

func TestGetAmfaStatsDisallowedRealmDenied(t *testing.T) {
	policies := []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"prod"}}}
	ts := amfaServer(t, permsWithPolicies(policies), &mockAmfaStatsReader{})

	text := callToolErrText(t, ts, "get_amfa_stats", map[string]any{"tenant": "tenant-a", "realm": "dev"})
	if text != ErrForbidden.Error() {
		t.Fatalf("error = %q, want %q", text, ErrForbidden.Error())
	}
}

func TestGetAmfaStatsTenantWithoutAmfaIsGeneric(t *testing.T) {
	reader := &mockAmfaStatsReader{
		getStatsFn: func(context.Context, string, amfa.StatsOptions) (amfa.Stats, error) {
			return amfa.Stats{}, fmt.Errorf("%w: tenant=tenant-a", amfa.ErrAmfaNotConfigured)
		},
	}
	ts := amfaServer(t, permsWithPolicies(nil), reader)

	text := callToolErrText(t, ts, "get_amfa_stats", map[string]any{"tenant": "tenant-a", "realm": "prod"})
	if text != ErrAmfaNotAvailable.Error() {
		t.Fatalf("error = %q, want %q", text, ErrAmfaNotAvailable.Error())
	}
	if strings.Contains(text, "tenant=") || strings.Contains(text, "amfa:") {
		t.Fatalf("error leaks internals: %q", text)
	}
}

func TestGetAmfaStatsUnavailableIsGeneric(t *testing.T) {
	reader := &mockAmfaStatsReader{
		getStatsFn: func(context.Context, string, amfa.StatsOptions) (amfa.Stats, error) {
			return amfa.Stats{}, fmt.Errorf("%w: dial tcp 10.0.0.9:5432: connection refused", amfa.ErrAmfaUnavailable)
		},
	}
	ts := amfaServer(t, permsWithPolicies(nil), reader)

	text := callToolErrText(t, ts, "get_amfa_stats", map[string]any{"tenant": "tenant-a", "realm": "prod"})
	if text != ErrAmfaSourceUnavailable.Error() {
		t.Fatalf("error = %q, want %q", text, ErrAmfaSourceUnavailable.Error())
	}
	if strings.Contains(text, "10.0.0.9") || strings.Contains(text, "dial tcp") {
		t.Fatalf("error leaks connection detail: %q", text)
	}
}

func TestGetAmfaStatsWindowCapEnforced(t *testing.T) {
	ts := amfaServer(t, permsWithPolicies(nil), &mockAmfaStatsReader{})

	text := callToolErrText(t, ts, "get_amfa_stats", map[string]any{
		"tenant": "tenant-a",
		"realm":  "prod",
		"from":   "2026-06-01T00:00:00Z",
		"to":     "2026-08-31T00:00:00Z",
	})
	if text != "invalid input: time window must not exceed 90 days" {
		t.Fatalf("error = %q, want window cap message", text)
	}
}

func TestGetAmfaStatsHappyPath(t *testing.T) {
	var got amfa.StatsOptions
	reader := &mockAmfaStatsReader{
		getStatsFn: func(_ context.Context, tenantID string, opts amfa.StatsOptions) (amfa.Stats, error) {
			if tenantID != "tenant-a" {
				return amfa.Stats{}, fmt.Errorf("unexpected tenant %q", tenantID)
			}
			got = opts
			return amfa.Stats{Total: 100, Risky: 5, UniqueUsers: 42, FlaggedIPs: 3}, nil
		},
	}
	ts := amfaServer(t, permsWithPolicies(nil), reader)

	var out amfaStatsOutput
	callToolOK(t, ts, "get_amfa_stats", map[string]any{
		"tenant": "tenant-a",
		"realm":  "prod",
		"from":   "2026-06-01T00:00:00Z",
		"to":     "2026-06-10T00:00:00Z",
	}, &out)

	if got.RealmID != "prod" {
		t.Errorf("realm = %q, want prod", got.RealmID)
	}
	wantFrom := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	if got.StartTime == nil || !got.StartTime.Equal(wantFrom) || got.EndTime == nil || !got.EndTime.Equal(wantTo) {
		t.Errorf("window = %v..%v, want %v..%v", got.StartTime, got.EndTime, wantFrom, wantTo)
	}
	if out.Total != 100 || out.Risky != 5 || out.UniqueUsers != 42 || out.FlaggedIPs != 3 {
		t.Fatalf("output = %+v, want 100/5/42/3", out)
	}
}
