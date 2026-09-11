package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/apitoken"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// logCapture is a race-safe sink for the JSON lines the server emits.
type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *logCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *logCapture) contents() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func (c *logCapture) auditLines(t *testing.T) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(c.contents(), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("unparseable log line %q: %v", raw, err)
		}
		if line["component"] == "mcp-audit" {
			lines = append(lines, line)
		}
	}
	return lines
}

func auditLineFor(t *testing.T, c *logCapture, tool string) map[string]any {
	t.Helper()
	for _, line := range c.auditLines(t) {
		if line["tool"] == tool {
			return line
		}
	}
	t.Fatalf("no audit line for tool %q in: %s", tool, c.contents())
	return nil
}

// captureLogger builds a logger writing JSON into a capture buffer.
func captureLogger(t *testing.T) (*logCapture, *logger.Logger) {
	t.Helper()
	c := &logCapture{}
	return c, logger.New(zerolog.New(c))
}

func tokenWithID(tokenID uint) *mockTokenValidator {
	return &mockTokenValidator{
		validateFn: func(_ context.Context, plaintext string) (*apitoken.Identity, error) {
			if plaintext == "pat_good" {
				return &apitoken.Identity{
					TokenID:   tokenID,
					User:      &domain.User{ID: 7, Subject: "user-7", IsActive: true},
					TenantIDs: []string{"tenant-a", "tenant-b"},
				}, nil
			}
			return nil, apitoken.ErrTokenNotFound
		},
	}
}

func TestAuditDeniedCallAlwaysLogged(t *testing.T) {
	capture, log := captureLogger(t)
	ts := newTestServerCustom(t, defaultTestCfg(), log, nil,
		tokenWithID(42), &mockPermissionService{}, readerFor(enabledTenant("tenant-a")), &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	if text := callToolErrText(t, ts, "get_tenant_health", map[string]any{"tenant": "tenant-a"}); text != ErrForbidden.Error() {
		t.Fatalf("error = %q, want %q", text, ErrForbidden.Error())
	}

	line := auditLineFor(t, capture, "get_tenant_health")
	if line["outcome"] != "denied" {
		t.Errorf("outcome = %v, want denied", line["outcome"])
	}
	if line["tenant"] != "tenant-a" {
		t.Errorf("tenant = %v, want tenant-a", line["tenant"])
	}
	if line["user_id"] != float64(7) {
		t.Errorf("user_id = %v, want 7", line["user_id"])
	}
	if line["token_id"] != float64(42) {
		t.Errorf("token_id = %v, want 42", line["token_id"])
	}
	if line["subject"] != "user-7" {
		t.Errorf("subject = %v, want user-7", line["subject"])
	}
	if _, ok := line["duration_ms"]; !ok {
		t.Error("audit line is missing duration_ms")
	}
}

func TestAuditAllowedCallCarriesToolTenantDuration(t *testing.T) {
	capture, log := captureLogger(t)
	ts := newTestServerCustom(t, defaultTestCfg(), log, nil,
		tokenWithID(42), grantAll(), readerFor(enabledTenant("tenant-a")), &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	var out tenantHealthOutput
	callToolOK(t, ts, "get_tenant_health", map[string]any{"tenant": "tenant-a"}, &out)

	line := auditLineFor(t, capture, "get_tenant_health")
	if line["outcome"] != "allowed" {
		t.Errorf("outcome = %v, want allowed", line["outcome"])
	}
	if line["tenant"] != "tenant-a" {
		t.Errorf("tenant = %v, want tenant-a", line["tenant"])
	}
	if _, ok := line["duration_ms"]; !ok {
		t.Error("audit line is missing duration_ms")
	}
}

func TestAuditListToolsReportRowCounts(t *testing.T) {
	capture, log := captureLogger(t)
	tenants := enabledTenantsLister(enabledTenant("tenant-a"), enabledTenant("tenant-b"))
	ts := newTestServerCustom(t, defaultTestCfg(), log, nil,
		tokenFor([]string{"tenant-a", "tenant-b"}), permsWithRoles(globalRole()), tenants, &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	var out listTenantsOutput
	callToolOK(t, ts, "list_tenants", nil, &out)

	line := auditLineFor(t, capture, "list_tenants")
	if line["rows"] != float64(2) {
		t.Errorf("rows = %v, want 2", line["rows"])
	}
}

func TestAuditNeverLogsFreeTextArguments(t *testing.T) {
	capture, log := captureLogger(t)
	ts := newTestServerCustom(t, defaultTestCfg(), log, nil,
		tokenWithID(42), grantAll(), readerFor(enabledTenant("tenant-a")), realmsNamed("prod"),
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	var out listEventsOutput
	callToolOK(t, ts, "list_events", map[string]any{
		"tenant": "tenant-a",
		"realm":  "prod",
		"type":   "SECRET_TYPE_MARKER",
	}, &out)

	line := auditLineFor(t, capture, "list_events")
	if line["outcome"] != "allowed" {
		t.Errorf("outcome = %v, want allowed", line["outcome"])
	}
	if line["realm"] != "prod" {
		t.Errorf("realm = %v, want prod", line["realm"])
	}
	if strings.Contains(capture.contents(), "SECRET_TYPE_MARKER") {
		t.Fatalf("free-text tool argument leaked into the log: %s", capture.contents())
	}
}

func TestAuditSurvivesGlobalErrorLevel(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	capture, log := captureLogger(t)
	ts := newTestServerCustom(t, defaultTestCfg(), log, nil,
		tokenWithID(42), &mockPermissionService{}, &mockTenantReader{}, &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	callToolResult(t, ts, "whoami", nil)

	line := auditLineFor(t, capture, "whoami")
	if line["outcome"] != "allowed" {
		t.Errorf("outcome = %v, want allowed", line["outcome"])
	}
	if line["level"] != "info" {
		t.Errorf("level = %v, want info", line["level"])
	}
}

func TestAuditRecordsEveryRPCMethod(t *testing.T) {
	capture, log := captureLogger(t)
	ts := newTestServerCustom(t, defaultTestCfg(), log, nil,
		tokenWithID(42), &mockPermissionService{}, &mockTenantReader{}, &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	session := connectClient(t, ts, "pat_good")
	if _, err := session.ListTools(context.Background(), nil); err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	if _, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "whoami"}); err != nil {
		t.Fatalf("whoami call failed: %v", err)
	}

	byMethod := map[string]map[string]any{}
	for _, line := range capture.auditLines(t) {
		if method, ok := line["method"].(string); ok {
			byMethod[method] = line
		}
	}
	// The v1.7.0 SDK client connects through server/discover instead of an
	// initialize round-trip.
	for _, want := range []string{"server/discover", "tools/list", "tools/call"} {
		line, ok := byMethod[want]
		if !ok {
			t.Fatalf("no request audit line for method %q in: %s", want, capture.contents())
		}
		if line["user_id"] != float64(7) {
			t.Errorf("%s user_id = %v, want 7", want, line["user_id"])
		}
		if line["token_id"] != float64(42) {
			t.Errorf("%s token_id = %v, want 42", want, line["token_id"])
		}
		if line["subject"] != "user-7" {
			t.Errorf("%s subject = %v, want user-7", want, line["subject"])
		}
	}
}

func TestAuditRateLimitedOutcome(t *testing.T) {
	capture, log := captureLogger(t)
	ts := newTestServerCustom(t, rateCfg(1, 0, 1), log, nil,
		tokenWithID(42), &mockPermissionService{}, &mockTenantReader{}, &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	callToolResult(t, ts, "whoami", nil)
	callToolResult(t, ts, "whoami", nil)

	var limited map[string]any
	for _, line := range capture.auditLines(t) {
		if line["tool"] == "whoami" && line["outcome"] == "rate_limited" {
			limited = line
		}
	}
	if limited == nil {
		t.Fatalf("no rate_limited audit line in: %s", capture.contents())
	}
	if limited["user_id"] != float64(7) {
		t.Errorf("user_id = %v, want 7", limited["user_id"])
	}
	if limited["token_id"] != float64(42) {
		t.Errorf("token_id = %v, want 42", limited["token_id"])
	}
}
