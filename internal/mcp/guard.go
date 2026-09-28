package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
)

const (
	auditFieldCap = 256

	fallbackMaxResponseBytes = 1 << 20
)

// Tool call outcomes as they appear in audit lines and metric labels.
const (
	outcomeAllowed     = "allowed"
	outcomeDenied      = "denied"
	outcomeInvalid     = "invalid"
	outcomeNotFound    = "not_found"
	outcomeError       = "error"
	outcomeRateLimited = "rate_limited"
	outcomeTooLarge    = "too_large"
)

// ToolMetrics is the slice of metrics.Recorder this package needs.
type ToolMetrics interface {
	IncMCPRequest(method string)
	IncMCPToolCall(tool, outcome string)
	ObserveMCPToolCall(tool string, d time.Duration)
}

// toolGuard carries the cross-cutting state every wrapped tool handler
// shares: the server log, the audit log, the per-tool metrics recorder, the
// per-user rate limiter and the response size cap.
type toolGuard struct {
	log              *logger.Logger
	audit            *logger.Logger
	metrics          ToolMetrics
	users            *keyLimiter[uint]
	maxResponseBytes int
}

func newToolGuard(cfg *config.MCPConfig, m ToolMetrics, log *logger.Logger) *toolGuard {
	if m == nil {
		m = metrics.NopRecorder{}
	}
	maxBytes := fallbackMaxResponseBytes
	var users *keyLimiter[uint]
	if cfg != nil {
		if cfg.MaxResponseBytes > 0 {
			maxBytes = cfg.MaxResponseBytes
		}
		// Keyed by user ID rather than token ID, so all of a user's tokens
		// draw from one shared budget.
		users = newKeyLimiter[uint](cfg.Rate.PerUserPerMinute, cfg.Rate.Burst, maxTrackedUsers)
	}
	return &toolGuard{
		log: log,
		// The audit trail must survive operational log-level tuning, so its
		// level is pinned to info independently of the global zerolog level.
		audit:            log.WithComponent("mcp-audit").PinLevel("info"),
		metrics:          m,
		users:            users,
		maxResponseBytes: maxBytes,
	}
}

// rpcMethods is the fixed JSON-RPC method set the SDK server dispatches. The
// metrics label collapses anything else to "other" so a client cannot mint
// unbounded label values.
var rpcMethods = map[string]bool{
	"initialize":                       true,
	"ping":                             true,
	"server/discover":                  true,
	"tools/list":                       true,
	"tools/call":                       true,
	"prompts/list":                     true,
	"prompts/get":                      true,
	"resources/list":                   true,
	"resources/read":                   true,
	"resources/templates/list":         true,
	"resources/subscribe":              true,
	"resources/unsubscribe":            true,
	"subscriptions/listen":             true,
	"completion/complete":              true,
	"logging/setLevel":                 true,
	"notifications/initialized":        true,
	"notifications/cancelled":          true,
	"notifications/progress":           true,
	"notifications/roots/list_changed": true,
}

func methodLabel(method string) string {
	if rpcMethods[method] {
		return method
	}
	return "other"
}

// auditRPC is the SDK receiving middleware recording every inbound JSON-RPC
// method with the caller identity, before per-tool guardrails run.
func (g *toolGuard) auditRPC() mcpsdk.Middleware {
	return func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
			g.metrics.IncMCPRequest(methodLabel(method))

			fields := make([]logger.Field, 0, 4)
			fields = append(fields, logger.Str("method", Clean(method, auditFieldCap)))
			if caller := callerOf(req); caller != nil {
				fields = append(fields,
					logger.Uint("user_id", caller.UserID),
					logger.Str("subject", Clean(caller.Subject, auditFieldCap)),
					logger.Uint("token_id", caller.TokenID))
			}
			g.audit.Info("MCP request", fields...)

			return next(ctx, method, req)
		}
	}
}

// wrapTool is the guardrail every tool handler is registered through: it
// enforces the per-user rate limit, recovers panics (no stack trace reaches a
// client), replaces handler errors with their client-safe mapping, caps the
// serialized result size, and emits one audit line and per-tool metrics for
// every call.
func wrapTool[In, Out any](g *toolGuard, name string, h mcpsdk.ToolHandlerFor[In, Out]) mcpsdk.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest, in In) (res *mcpsdk.CallToolResult, out Out, err error) {
		start := time.Now()
		caller := CallerFromRequest(req)
		tenant, realm := scopeArgs(req)
		outcome := outcomeError

		ctx, rows := withRowCount(ctx)
		defer func() {
			if r := recover(); r != nil {
				g.log.Error("Panic in MCP tool",
					logger.Str("tool", name),
					logger.Str("panic", fmt.Sprint(r)),
					logger.Str("stack", string(debug.Stack())))
				var zero Out
				res, out, err = nil, zero, errInternal
				outcome = outcomeError
			}
			g.finish(name, outcome, caller, tenant, realm, rows.Load(), time.Since(start))
		}()

		// The per-user budget covers tool calls only: initialize and
		// tools/list must never be budget-limited, so a throttled client can
		// still hold a session and discover the tools.
		if caller != nil && !g.users.Allow(caller.UserID) {
			outcome = outcomeRateLimited
			var zero Out
			return nil, zero, ErrRateLimited
		}

		res, out, err = h(ctx, req, in)
		if err != nil {
			g.log.Warn("MCP tool call failed",
				logger.Str("tool", name),
				logger.Err(err))
			outcome = outcomeOf(err)
			var zero Out
			return nil, zero, toClientError(err)
		}

		// The SDK duplicates structured output into a text content block, so
		// the wire carries the marshaled result twice; the cap is therefore
		// enforced at half the configured maximum.
		size, sized := marshaledSize(out)
		if sized && res != nil {
			var contentSize int
			contentSize, sized = marshaledSize(res)
			size += contentSize
		}
		// A result that does not marshal has no measurable size. Leaving it
		// at zero would let it through the cap unmeasured, so it is rejected
		// here instead.
		if !sized {
			g.log.Error("MCP tool result could not be marshaled to measure it",
				logger.Str("tool", name))
			var zero Out
			return nil, zero, errInternal
		}
		if size > g.maxResponseBytes/2 {
			g.log.Error("MCP tool result exceeds the response size cap",
				logger.Str("tool", name),
				logger.Int("size_bytes", size),
				logger.Int("limit_bytes", g.maxResponseBytes/2))
			outcome = outcomeTooLarge
			var zero Out
			// The message carries no size, no limit and no tool: it says only
			// what the caller can act on.
			return nil, zero, ErrResultTooLarge
		}

		outcome = outcomeAllowed
		return res, out, nil
	}
}

// finish records the per-tool metrics and emits the call's audit line. Beyond
// the tenant and realm scope arguments no tool argument is ever logged:
// free-text arguments may carry sensitive queries.
func (g *toolGuard) finish(tool, outcome string, caller *Caller, tenant, realm string, rows int64, d time.Duration) {
	g.metrics.IncMCPToolCall(tool, outcome)
	if outcome == outcomeAllowed {
		g.metrics.ObserveMCPToolCall(tool, d)
	}

	fields := make([]logger.Field, 0, 9)
	fields = append(fields,
		logger.Str("tool", tool),
		logger.Str("outcome", outcome),
		logger.Int64("duration_ms", d.Milliseconds()))
	if caller != nil {
		fields = append(fields,
			logger.Uint("user_id", caller.UserID),
			logger.Str("subject", Clean(caller.Subject, auditFieldCap)),
			logger.Uint("token_id", caller.TokenID))
	}
	if tenant != "" {
		fields = append(fields, logger.Str("tenant", tenant))
	}
	if realm != "" {
		fields = append(fields, logger.Str("realm", realm))
	}
	if rows >= 0 {
		fields = append(fields, logger.Int64("rows", rows))
	}

	if outcome == outcomeAllowed {
		g.audit.Info("MCP tool call", fields...)
		return
	}
	g.audit.Warn("MCP tool call", fields...)
}

// marshaledSize reports the serialized size of a value, and whether it could
// be serialized at all.
func marshaledSize(v any) (int, bool) {
	raw, err := json.Marshal(v)
	if err != nil {
		return 0, false
	}
	return len(raw), true
}

func outcomeOf(err error) string {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return outcomeInvalid
	case errors.Is(err, ErrAlertNotFound), errors.Is(err, ErrEventNotFound):
		return outcomeNotFound
	case errors.Is(err, ErrUnauthenticated), errors.Is(err, ErrForbidden), errors.Is(err, ErrTenantNotAvailable):
		return outcomeDenied
	}
	return outcomeError
}

// scopeArgs extracts only the tenant and realm arguments from the raw tool
// call for auditing.
func scopeArgs(req *mcpsdk.CallToolRequest) (tenant, realm string) {
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return "", ""
	}
	var scope struct {
		Tenant string `json:"tenant"`
		Realm  string `json:"realm"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &scope); err != nil {
		return "", ""
	}
	return Clean(scope.Tenant, auditFieldCap), Clean(scope.Realm, auditFieldCap)
}

type rowCountKey struct{}

func withRowCount(ctx context.Context) (context.Context, *atomic.Int64) {
	n := &atomic.Int64{}
	n.Store(-1)
	return context.WithValue(ctx, rowCountKey{}, n), n
}

// reportRows records how many result rows a tool call returned; the count
// lands in the call's audit line.
func reportRows(ctx context.Context, n int) {
	if counter, ok := ctx.Value(rowCountKey{}).(*atomic.Int64); ok {
		counter.Store(int64(n))
	}
}
