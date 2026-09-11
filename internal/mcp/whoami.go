package mcp

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DefensePoint/keycloak-monitoring/internal/version"
)

const identityFieldCap = 256

type whoamiInput struct{}

type whoamiOutput struct {
	Subject         string   `json:"subject" jsonschema:"the authenticated caller's subject"`
	TenantAllowlist []string `json:"tenant_allowlist" jsonschema:"tenant IDs the token is restricted to; when it holds exactly one, tools take that tenant from the token and reject a tenant argument"`
	Admin           bool     `json:"admin" jsonschema:"whether the caller is a platform administrator"`
	ServerVersion   string   `json:"server_version" jsonschema:"version of the Keycloak Monitoring Tool MCP server"`
}

// registerWhoami adds the whoami connectivity-check tool. It reads no
// monitoring data; it exists to prove the auth, error-mapping and
// sanitization pipeline end to end.
func registerWhoami(srv *mcpsdk.Server, g *toolGuard) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "whoami",
		Title:       "Who Am I (connectivity check)",
		Description: "Connectivity check: returns the authenticated caller's identity, the token's tenant allowlist and the server version. Reads no monitoring data.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, wrapTool(g, "whoami", handleWhoami))
}

func handleWhoami(_ context.Context, req *mcpsdk.CallToolRequest, _ whoamiInput) (*mcpsdk.CallToolResult, whoamiOutput, error) {
	caller := CallerFromRequest(req)
	if caller == nil {
		return nil, whoamiOutput{}, ErrUnauthenticated
	}

	out := whoamiOutput{
		Subject:         Clean(caller.Subject, identityFieldCap),
		TenantAllowlist: CleanSlice(caller.TenantIDs, identityFieldCap),
		Admin:           caller.IsAdmin,
		ServerVersion:   version.Version,
	}
	if out.TenantAllowlist == nil {
		out.TenantAllowlist = []string{}
	}
	return nil, out, nil
}
