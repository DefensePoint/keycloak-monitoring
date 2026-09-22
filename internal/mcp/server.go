// Package mcp assembles the Model Context Protocol server: the streamable
// HTTP transport behind API-token authentication, the per-tool authorization
// helper, and the guardrails (error mapping, panic recovery, parameter caps,
// string sanitization, rate limiting, audit logging, per-tool metrics and the
// response size cap) every tool call passes through.
package mcp

import (
	"fmt"
	"net/http"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/DefensePoint/keycloak-monitoring/internal/version"
)

const (
	serverName = "keycloak-monitoring-tool"

	// Endpoint is the path the MCP streamable transport is served on.
	Endpoint = "/mcp"
)

// Server is the assembled MCP server and the HTTP mux it is served on.
type Server struct {
	handler http.Handler
	port    int
}

// Clock is the source of "now" for the tools that resolve a defaulted time
// window. Production passes time.Now; tests pass a clock they advance by hand
// so window behaviour is exercised without depending on wall-clock timing.
type Clock func() time.Time

// New assembles the MCP server: tools registered through the guardrail
// pipeline, and the streamable HTTP handler behind the pre-auth per-IP rate
// limiter, the request body cap and the auth middleware. /health, /ready and
// /metrics live on the same mux outside any rate limiting: they do no work,
// and starving them would break probes and the host-local metrics scrape.
func New(
	cfg *config.AppConfig,
	log *logger.Logger,
	tokens TokenValidator,
	perms PermissionService,
	tenants TenantReader,
	realms RealmReader,
	alertReader AlertReader,
	amfaStats AmfaStatsReader,
	eventReader EventReader,
	m *metrics.Registry,
) *Server {
	return newServerWithClock(cfg, log, tokens, perms, tenants, realms, alertReader, amfaStats, eventReader, m, time.Now)
}

// newServerWithClock is New with the clock left injectable. Only tests pass
// anything but time.Now.
func newServerWithClock(
	cfg *config.AppConfig,
	log *logger.Logger,
	tokens TokenValidator,
	perms PermissionService,
	tenants TenantReader,
	realms RealmReader,
	alertReader AlertReader,
	amfaStats AmfaStatsReader,
	eventReader EventReader,
	m *metrics.Registry,
	clock Clock,
) *Server {
	authz := NewAuthorizer(perms, tenants, log)
	var rec ToolMetrics
	if m != nil {
		rec = m
	}
	guard := newToolGuard(&cfg.MCP, rec, log)

	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    serverName,
		Title:   "Keycloak Monitoring Tool",
		Version: version.Version,
	}, nil)
	srv.AddReceivingMiddleware(guard.auditRPC())
	registerTools(srv, authz, realms, alertReader, amfaStats, eventReader, newCursorSigner(cfg.MCP.CursorHMACKey, log), &cfg.MCP, guard, clock)

	streamable := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return srv },
		&mcpsdk.StreamableHTTPOptions{
			// Stateless: each POST runs in a temporary session, so nothing
			// accumulates server-side between calls, no Mcp-Session-Id is
			// issued, and GET/DELETE answer 405.
			Stateless: true,
		})

	requireAuth := AuthMiddleware(tokens, perms, cfg.MCP.OriginAllowlist, log)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/ready", handleReady)
	if cfg.MCP.Metrics.Enabled && m != nil {
		mux.Handle("/metrics", metrics.GuardedHandler(m, cfg.MCP.Metrics))
	}
	ips := newKeyLimiter[string](cfg.MCP.Rate.PerIPPerMinute, cfg.MCP.Rate.Burst, maxTrackedIPs)
	mux.Handle(Endpoint, rateLimitByIP(ips, capRequestBody(requireAuth(streamable))))

	return &Server{handler: mux, port: cfg.MCP.Port}
}

// maxRequestBodyBytes caps any MCP request body before authentication runs,
// so an oversized POST cannot exhaust memory ahead of the rate limiter.
const maxRequestBodyBytes = 1 << 20

func capRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// registerTools adds every tool through the guardrail pipeline. The
// Authorizer is part of the registration contract: tools that take a tenant
// argument must authorize it through authz before touching data. whoami takes
// no tenant; list_tenants takes none either and derives its scope through
// authz.VisibleTenants instead.
func registerTools(srv *mcpsdk.Server, authz *Authorizer, realms RealmReader, alertReader AlertReader, amfaStats AmfaStatsReader, eventReader EventReader, signer *cursorSigner, cfg *config.MCPConfig, g *toolGuard, clock Clock) {
	registerWhoami(srv, g)
	registerListTenants(srv, authz, g)
	registerGetTenantHealth(srv, authz, g)
	registerListRealms(srv, authz, realms, g)
	registerListAlerts(srv, authz, alertReader, cfg, g)
	registerGetAlert(srv, authz, alertReader, g)
	registerGetAlertStats(srv, authz, alertReader, g)
	registerGetAmfaStats(srv, authz, amfaStats, g)
	registerListEvents(srv, authz, realms, eventReader, signer, cfg, g, clock)
	registerGetEvent(srv, authz, eventReader, g)
	registerGetEventStats(srv, authz, realms, eventReader, g, clock)
}

// Handler returns the HTTP handler serving the MCP endpoint plus
// health/readiness/metrics.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// Addr returns the listen address.
func (s *Server) Addr() string {
	return fmt.Sprintf(":%d", s.port)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"healthy"}`))
}

func handleReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}
