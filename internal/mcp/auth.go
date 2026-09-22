package mcp

import (
	"context"
	"net/http"
	"strconv"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DefensePoint/keycloak-monitoring/internal/apitoken"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// TokenValidator is the slice of apitoken.Service this package needs.
type TokenValidator interface {
	Validate(ctx context.Context, plaintext string) (*apitoken.Identity, error)
}

// Caller is the authenticated principal behind an MCP request.
type Caller struct {
	UserID  uint
	Subject string

	// TokenID identifies the exact API token behind this request. Pagination
	// cursors are MAC-bound to it, so one token's cursor never validates
	// under another token, the same user's included.
	TokenID uint

	// TenantIDs is the token's tenant allowlist, which every token carries
	// and which no tool call can reach outside of. Empty only ever means a
	// token that predates the allowlist requirement, and every scope check
	// treats it as granting nothing.
	TenantIDs []string

	IsAdmin bool
}

// callerExtraKey is where the Caller travels inside the SDK's TokenInfo.Extra.
const callerExtraKey = "mcp_caller"

// CallerFromRequest returns the authenticated caller of a tool call. The SDK
// attaches the verifier's TokenInfo to every JSON-RPC request, which is the
// only authentication state the stateless transport carries between the HTTP
// layer and the tool handler.
func CallerFromRequest(req *mcpsdk.CallToolRequest) *Caller {
	return callerOf(req)
}

func callerOf(req mcpsdk.Request) *Caller {
	if req == nil {
		return nil
	}
	extra := req.GetExtra()
	if extra == nil || extra.TokenInfo == nil {
		return nil
	}
	caller, _ := extra.TokenInfo.Extra[callerExtraKey].(*Caller)
	return caller
}

// AuthMiddleware requires "Authorization: Bearer <token>" on every request
// and method, validating through the apitoken service. It also enforces the
// Origin allowlist before authentication runs. On success the SDK stores the
// resulting TokenInfo in the request context and delivers it to tool handlers
// with each request (see CallerFromRequest).
func AuthMiddleware(tokens TokenValidator, perms PermissionService, originAllowlist []string, log *logger.Logger) func(http.Handler) http.Handler {
	verifier := func(ctx context.Context, plaintext string, _ *http.Request) (*sdkauth.TokenInfo, error) {
		identity, err := tokens.Validate(ctx, plaintext)
		if err != nil {
			log.Warn("MCP token rejected", logger.Err(err))
			return nil, sdkauth.ErrInvalidToken
		}

		caller := &Caller{
			UserID:    identity.User.ID,
			Subject:   identity.User.Subject,
			TokenID:   identity.TokenID,
			TenantIDs: identity.TenantIDs,
		}
		isAdmin, err := perms.IsAdmin(ctx, identity.User.ID)
		if err != nil {
			log.Warn("MCP admin check failed, treating caller as non-admin",
				logger.Uint("user_id", identity.User.ID),
				logger.Err(err))
		}
		caller.IsAdmin = isAdmin

		return &sdkauth.TokenInfo{
			UserID: strconv.FormatUint(uint64(identity.User.ID), 10),
			Extra:  map[string]any{callerExtraKey: caller},
		}, nil
	}

	// apitoken.Validate already enforced expiry; the Identity it returns
	// does not carry the expiration, so the SDK's own check is waived.
	requireBearer := sdkauth.RequireBearerToken(verifier, &sdkauth.RequireBearerTokenOptions{
		AllowMissingExpiration: true,
	})

	return func(next http.Handler) http.Handler {
		authed := requireBearer(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origin := r.Header.Get("Origin"); origin != "" && !originAllowed(origin, originAllowlist) {
				log.Warn("MCP request rejected: origin not in allowlist",
					logger.Str("origin", origin))
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			authed.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowlist []string) bool {
	for _, allowed := range allowlist {
		if origin == allowed {
			return true
		}
	}
	return false
}
