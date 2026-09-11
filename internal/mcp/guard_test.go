package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
)

func testGuard() *toolGuard {
	return newToolGuard(&config.MCPConfig{}, nil, testLogger())
}

func TestWrapToolRejectsOversizeResponse(t *testing.T) {
	g := newToolGuard(&config.MCPConfig{MaxResponseBytes: 256}, nil, testLogger())

	big := wrapTool(g, "big", func(context.Context, *mcpsdk.CallToolRequest, whoamiInput) (*mcpsdk.CallToolResult, whoamiOutput, error) {
		return nil, whoamiOutput{Subject: strings.Repeat("x", 200)}, nil
	})
	_, out, err := big(context.Background(), &mcpsdk.CallToolRequest{}, whoamiInput{})
	if !errors.Is(err, ErrResultTooLarge) {
		t.Fatalf("oversize result: err = %v, want %v", err, ErrResultTooLarge)
	}
	// The caller is told what to do about it and nothing else: no size, no
	// limit, no tool name, no fragment of the result.
	if msg := err.Error(); strings.Contains(msg, "256") || strings.Contains(msg, "big") || strings.Contains(msg, "x") {
		t.Fatalf("oversize error leaks detail: %q", msg)
	}
	if out.Subject != "" {
		t.Fatalf("oversize result must be dropped, got %+v", out)
	}

	overHalf := wrapTool(g, "over_half", func(context.Context, *mcpsdk.CallToolRequest, whoamiInput) (*mcpsdk.CallToolResult, whoamiOutput, error) {
		return nil, whoamiOutput{Subject: strings.Repeat("x", 140)}, nil
	})
	if _, _, err := overHalf(context.Background(), &mcpsdk.CallToolRequest{}, whoamiInput{}); err == nil {
		t.Fatal("result over half the cap must be rejected: the SDK doubles it on the wire")
	}

	withResult := wrapTool(g, "with_result", func(context.Context, *mcpsdk.CallToolRequest, whoamiInput) (*mcpsdk.CallToolResult, whoamiOutput, error) {
		res := &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: strings.Repeat("y", 140)}}}
		return res, whoamiOutput{Subject: "ok"}, nil
	})
	if _, _, err := withResult(context.Background(), &mcpsdk.CallToolRequest{}, whoamiInput{}); err == nil {
		t.Fatal("oversize handler-built result content must be rejected")
	}

	small := wrapTool(g, "small", func(context.Context, *mcpsdk.CallToolRequest, whoamiInput) (*mcpsdk.CallToolResult, whoamiOutput, error) {
		return nil, whoamiOutput{Subject: "ok"}, nil
	})
	_, out, err = small(context.Background(), &mcpsdk.CallToolRequest{}, whoamiInput{})
	if err != nil {
		t.Fatalf("result under the cap: unexpected error %v", err)
	}
	if out.Subject != "ok" {
		t.Fatalf("result under the cap must pass through, got %+v", out)
	}
}

func TestToolMetricsCountPerOutcome(t *testing.T) {
	m := metrics.New()
	ts := newTestServerCustom(t, defaultTestCfg(), testLogger(), m,
		tokenWithID(42), &mockPermissionService{}, readerFor(enabledTenant("tenant-a")), &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	callToolResult(t, ts, "whoami", nil)
	callToolResult(t, ts, "get_tenant_health", map[string]any{"tenant": "tenant-a"})
	callToolResult(t, ts, "get_tenant_health", map[string]any{"tenant": ""})

	if got := testutil.ToFloat64(m.MCPToolCalls.WithLabelValues("whoami", "allowed")); got != 1 {
		t.Errorf("whoami allowed = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.MCPToolCalls.WithLabelValues("get_tenant_health", "denied")); got != 1 {
		t.Errorf("get_tenant_health denied = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.MCPToolCalls.WithLabelValues("get_tenant_health", "invalid")); got != 1 {
		t.Errorf("get_tenant_health invalid = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(m.MCPToolCallDuration); got != 1 {
		t.Errorf("duration series = %d, want 1 (only the allowed whoami call is observed)", got)
	}
}

func TestToolMetricsNotFoundAndErrorOutcomes(t *testing.T) {
	m := metrics.New()
	reader := &mockAlertReader{
		getAlertFn: func(context.Context, string, string) (*domain.Alert, error) { return nil, nil },
	}
	ts := newTestServerCustom(t, defaultTestCfg(), testLogger(), m,
		tokenFor([]string{"tenant-a", "tenant-b"}), permsWithPolicies(nil), readerFor(enabledTenant("tenant-a")), &mockRealmReader{},
		reader, &mockAmfaStatsReader{}, &mockEventReader{})

	callToolResult(t, ts, "get_alert", map[string]any{"tenant": "tenant-a", "alert_id": "missing"})

	failing := &mockAlertReader{
		getAlertFn: func(context.Context, string, string) (*domain.Alert, error) {
			return nil, context.DeadlineExceeded
		},
	}
	tsFailing := newTestServerCustom(t, defaultTestCfg(), testLogger(), m,
		tokenFor([]string{"tenant-a", "tenant-b"}), permsWithPolicies(nil), readerFor(enabledTenant("tenant-a")), &mockRealmReader{},
		failing, &mockAmfaStatsReader{}, &mockEventReader{})

	callToolResult(t, tsFailing, "get_alert", map[string]any{"tenant": "tenant-a", "alert_id": "boom"})

	if got := testutil.ToFloat64(m.MCPToolCalls.WithLabelValues("get_alert", "not_found")); got != 1 {
		t.Errorf("get_alert not_found = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.MCPToolCalls.WithLabelValues("get_alert", "error")); got != 1 {
		t.Errorf("get_alert error = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(m.MCPToolCallDuration); got != 0 {
		t.Errorf("duration series = %d, want 0 (failed calls are never observed)", got)
	}
}

func TestMethodLabelBoundsMetricCardinality(t *testing.T) {
	for _, known := range []string{"initialize", "ping", "tools/list", "tools/call"} {
		if got := methodLabel(known); got != known {
			t.Errorf("methodLabel(%q) = %q, want identity", known, got)
		}
	}
	if got := methodLabel("made/up-method"); got != "other" {
		t.Errorf("methodLabel(unknown) = %q, want other", got)
	}
}

func TestRPCRequestsCounted(t *testing.T) {
	m := metrics.New()
	ts := newTestServerCustom(t, defaultTestCfg(), testLogger(), m,
		tokenWithID(42), &mockPermissionService{}, &mockTenantReader{}, &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	callToolResult(t, ts, "whoami", nil)

	// The v1.7.0 SDK client connects through server/discover instead of an
	// initialize round-trip.
	if got := testutil.ToFloat64(m.MCPRequests.WithLabelValues("server/discover")); got < 1 {
		t.Errorf("server/discover requests = %v, want at least 1", got)
	}
	if got := testutil.ToFloat64(m.MCPRequests.WithLabelValues("tools/call")); got != 1 {
		t.Errorf("tools/call requests = %v, want 1", got)
	}
}

func TestToolMetricsCountRateLimited(t *testing.T) {
	m := metrics.New()
	ts := newTestServerCustom(t, rateCfg(1, 0, 1), testLogger(), m,
		tokenWithID(42), &mockPermissionService{}, &mockTenantReader{}, &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	callToolResult(t, ts, "whoami", nil)
	callToolResult(t, ts, "whoami", nil)

	if got := testutil.ToFloat64(m.MCPToolCalls.WithLabelValues("whoami", "rate_limited")); got != 1 {
		t.Errorf("whoami rate_limited = %v, want 1", got)
	}
}

// unmarshalableOutput carries a channel, which encoding/json refuses.
type unmarshalableOutput struct {
	Ch chan int `json:"ch"`
}

func TestWrapToolRejectsAnUnmeasurableResponse(t *testing.T) {
	g := newToolGuard(&config.MCPConfig{MaxResponseBytes: 1 << 20}, nil, testLogger())

	broken := wrapTool(g, "broken", func(context.Context, *mcpsdk.CallToolRequest, whoamiInput) (*mcpsdk.CallToolResult, unmarshalableOutput, error) {
		return nil, unmarshalableOutput{Ch: make(chan int)}, nil
	})
	_, out, err := broken(context.Background(), &mcpsdk.CallToolRequest{}, whoamiInput{})
	if err == nil {
		t.Fatal("a result that cannot be marshaled must not pass the size cap as zero bytes")
	}
	if out.Ch != nil {
		t.Fatalf("rejected result must be dropped, got %+v", out)
	}
}
