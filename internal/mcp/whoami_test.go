package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/DefensePoint/keycloak-monitoring/internal/version"
)

// bearerTransport injects the Authorization header into every client request.
type bearerTransport struct {
	token string
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if t.token != "" {
		clone.Header.Set("Authorization", "Bearer "+t.token)
	}
	return http.DefaultTransport.RoundTrip(clone)
}

func newTestServer(t *testing.T, tokens TokenValidator) *httptest.Server {
	t.Helper()
	return newTestServerWith(t, tokens, &mockPermissionService{}, &mockTenantReader{}, &mockRealmReader{})
}

func newTestServerWith(t *testing.T, tokens TokenValidator, perms PermissionService, tenants TenantReader, realms RealmReader) *httptest.Server {
	t.Helper()
	return newTestServerFull(t, tokens, perms, tenants, realms, &mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})
}

func newTestServerFull(t *testing.T, tokens TokenValidator, perms PermissionService, tenants TenantReader, realms RealmReader, alertReader AlertReader, amfaStats AmfaStatsReader, eventReader EventReader) *httptest.Server {
	t.Helper()
	return newTestServerCustom(t, defaultTestCfg(), testLogger(), nil, tokens, perms, tenants, realms, alertReader, amfaStats, eventReader)
}

func defaultTestCfg() *config.AppConfig {
	return &config.AppConfig{
		MCP: config.MCPConfig{Port: 0, DefaultPageSize: 50, MaxPageSize: 500},
	}
}

func newTestServerCustom(t *testing.T, cfg *config.AppConfig, log *logger.Logger, m *metrics.Registry, tokens TokenValidator, perms PermissionService, tenants TenantReader, realms RealmReader, alertReader AlertReader, amfaStats AmfaStatsReader, eventReader EventReader) *httptest.Server {
	t.Helper()
	return newTestServerClocked(t, cfg, log, m, tokens, perms, tenants, realms, alertReader, amfaStats, eventReader, time.Now)
}

// newTestServerClocked is newTestServerCustom with the server's clock left to
// the caller, for tests that need to control the passage of time.
func newTestServerClocked(t *testing.T, cfg *config.AppConfig, log *logger.Logger, m *metrics.Registry, tokens TokenValidator, perms PermissionService, tenants TenantReader, realms RealmReader, alertReader AlertReader, amfaStats AmfaStatsReader, eventReader EventReader, clock Clock) *httptest.Server {
	t.Helper()
	server := newServerWithClock(cfg, log, tokens, perms, tenants, realms, alertReader, amfaStats, eventReader, m, clock)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func connectClient(t *testing.T, ts *httptest.Server, token string) *mcpsdk.ClientSession {
	t.Helper()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
		Endpoint:   ts.URL + Endpoint,
		HTTPClient: &http.Client{Transport: &bearerTransport{token: token}},
	}, nil)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestWhoamiEndToEnd(t *testing.T) {
	ts := newTestServer(t, acceptingValidator("pat_good"))
	session := connectClient(t, ts, "pat_good")

	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("whoami call failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("whoami returned tool error: %+v", res.Content)
	}

	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out whoamiOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal whoami output: %v", err)
	}

	if out.Subject != "user-7" {
		t.Errorf("subject = %q, want user-7", out.Subject)
	}
	if len(out.TenantAllowlist) != 1 || out.TenantAllowlist[0] != "tenant-a" {
		t.Errorf("tenant allowlist = %v, want [tenant-a]", out.TenantAllowlist)
	}
	if out.ServerVersion != version.Version {
		t.Errorf("server version = %q, want %q", out.ServerVersion, version.Version)
	}
}

func TestWhoamiRejectsMissingToken(t *testing.T) {
	ts := newTestServer(t, acceptingValidator("pat_good"))

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	_, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
		Endpoint: ts.URL + Endpoint,
	}, nil)
	if err == nil {
		t.Fatal("expected connect to fail without a token")
	}
}

func TestMCPEndpointIsStateless(t *testing.T) {
	ts := newTestServer(t, acceptingValidator("pat_good"))

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req, err := http.NewRequest(method, ts.URL+Endpoint, nil)
		if err != nil {
			t.Fatalf("build %s request: %v", method, err)
		}
		req.Header.Set("Authorization", "Bearer pat_good")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, Endpoint, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status %d, want %d", method, Endpoint, resp.StatusCode, http.StatusMethodNotAllowed)
		}
	}
}

func TestMCPEndpointCapsRequestBody(t *testing.T) {
	ts := newTestServer(t, acceptingValidator("pat_good"))

	req, err := http.NewRequest(http.MethodPost, ts.URL+Endpoint, bytes.NewReader(make([]byte, maxRequestBodyBytes+1)))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer pat_good")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("oversized POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized POST: status %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
}

func TestHealthAndReadyServeWithoutAuth(t *testing.T) {
	ts := newTestServer(t, acceptingValidator("pat_good"))

	for _, path := range []string{"/health", "/ready"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", path, resp.StatusCode)
		}
	}
}
