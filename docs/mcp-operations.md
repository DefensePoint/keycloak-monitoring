# Keycloak Monitoring Tool MCP user guide

Keycloak Monitoring Tool (KMT) is the monitoring application for Keycloak
customers. It includes a separate, read-only Model Context Protocol (MCP)
server (`cmd/mcp-server`). The server exposes tenants, realm listings, tenant
health, alerts, events and Adaptive Multi-Factor Authentication (AMFA)
statistics using Streamable HTTP. Starting the main web/API process does not
start the MCP server. Clients authenticate with a personal access token; the
token's tenant allowlist and the user's role-based access control (RBAC)
permissions both apply.

AMFA statistics require AMFA to be configured for the tenant. If your tenant
does not use AMFA, `get_amfa_stats` returns the tool error
`AMFA not available for this tenant`. This is expected and does not indicate
a broken MCP connection. You can still use the other monitoring tools within
your permissions. To retrieve AMFA statistics, ask your administrator to
configure AMFA for the tenant. After AMFA is configured, restart the MCP
server; it reads AMFA settings only at startup.

See the [server reference](../cmd/mcp-server/README.md) for complete prerequisites,
token administration, client configuration and tool parameters.

## Enable

1. Run the main API against the database first so its schema exists.
2. Provision a dedicated read-only role using
   [the PostgreSQL instructions](../deployments/postgres/README.md).
   Set the `mcp.database` block to that role. Keep its `password: ""` key in
   the config when supplying `MONITORING_MCP_DATABASE_PASSWORD`; absent keys
   in this block are not bound automatically from the environment.
3. If stored tenant client secrets are encrypted, supply the same
   `MONITORING_SECURITY_ENCRYPTION_KEY` used by the API to the MCP process.
   Without it, tenant reads can fail even while health checks pass.
4. Set `mcp.cursor_hmac_key` to a persistent random value of at least 32 bytes
   (for example, generate one with `openssl rand -base64 32`). Keep passwords,
   signing keys and API tokens in your secret manager.
5. Set `mcp.enabled: true`, or pass `MONITORING_MCP_ENABLED=true` to the
   **MCP process**. Start that process explicitly:

   ```bash
   make build-mcp-server
   MONITORING_MCP_ENABLED=true ./bin/mcp-server -config config.yaml
   ```

   For the local Compose deployment:

   ```bash
   # Set mcp.enabled: true in config.yaml first.
   docker compose -f deployments/local/docker-compose.yml up -d --build mcp-server
   ```

   A shell export does not automatically enter a Compose container: place the
   variable in the service's environment/env_file or use the mounted config.
   Packaged server Compose has no MCP port published; use a private network
   route or the SSH forwarding procedure in the server reference.
6. As a platform administrator, create a short-lived token for the intended
   user using `POST /api/tokens` on the **main API**, with a nonempty
   `tenant_ids` allowlist. Save the plaintext returned once. Do not grant
   broader user permissions simply to make a token work.

## Disable and re-enable

For a binary deployment, stop the MCP process, set `mcp.enabled: false` (and
remove any `MONITORING_MCP_ENABLED=true` override), then restart it if needed.
The disabled binary exits successfully without opening the MCP listener.
Changing a config value does not stop an already running process.

For local Compose:

```bash
docker compose -f deployments/local/docker-compose.yml stop mcp-server
# Set mcp.enabled: false and remove any true environment override.
```

Re-enable by restoring the true setting and starting `mcp-server` again.
This affects MCP access, not the main application. Disabling the service does
not revoke its tokens: revoke unused tokens separately with
`DELETE /api/tokens/<id>` on the main API.

## Add the server to an AI client

Install your chosen client and sign in to its AI provider account. Create a
KMT personal access token as described above; it is separate from your
AI provider credentials. The examples use the command-line clients Claude
Code, Codex CLI and Grok Build.

In Bash, set the endpoint and enter the token without displaying it:

```bash
export KMT_MCP_URL=http://127.0.0.1:7889/mcp
read -r -s -p 'KMT MCP token: ' KMT_MCP_TOKEN; echo
export KMT_MCP_TOKEN
```

Replace the URL with your operator-provided endpoint, including `/mcp`. Keep
any required VPN or port-forward running. The client process must be able to
reach this address. A hosted client cannot use your workstation's loopback
address. For a remote connection, use HTTPS or an encrypted private tunnel.

Choose the instructions for your client below. Start a new client session
from the terminal where these variables are exported. A desktop application
opened separately does not automatically inherit that terminal's environment.

### Claude Code

Add the server to your user configuration:

```bash
claude mcp add --transport http --scope user kmt "$KMT_MCP_URL" \
  --header 'Authorization: Bearer ${KMT_MCP_TOKEN}'
claude
```

Keep the header in single quotes so the saved configuration refers to the
environment variable instead of storing its current token value. Inside
Claude Code, use `/mcp` to check the connection and approve the server if asked.
See [Claude Code MCP setup](https://code.claude.com/docs/en/mcp).

### Codex CLI

```bash
codex mcp add kmt --url "$KMT_MCP_URL" \
  --bearer-token-env-var KMT_MCP_TOKEN
codex
```

The option takes the environment variable's name, without `$`. Codex reads
its value when connecting. Use `/mcp` in the new session to inspect the tools.
`codex mcp list` confirms registration; an actual tool call confirms access.
See [Codex MCP setup](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

### Grok Build

```bash
grok mcp add --transport http --scope user kmt "$KMT_MCP_URL" \
  --header 'Authorization: Bearer ${KMT_MCP_TOKEN}'
grok mcp doctor kmt
grok
```

The single-quoted header preserves the environment reference; Grok expands
it when loading the configuration. Inside Grok, use `/mcps` to inspect the
server and refresh its connection. These instructions are for Grok Build's
CLI, not the Grok website. See [Grok MCP setup](https://docs.x.ai/build/features/mcp-servers).

### Claude Desktop

Follow the [Claude Desktop configuration](../cmd/mcp-server/README.md#claude-desktop)
to connect through the local `mcp-remote` bridge. It requires Node.js with
`npx`. Keep the token in your private client configuration and restart Desktop
after changing it. The bridge connects to the same HTTP endpoint.

### First request

In your chosen client, ask:

> Use the kmt MCP server to call whoami, then list my available tenants
> and realms. Use actual MCP tools and report any connection or tool errors.

For a single-tenant token, omit the `tenant` argument in tenant-scoped calls;
the server supplies it. A multi-tenant token must specify an allowed tenant.
If a token expires or is revoked, obtain a replacement, update the environment
variable and restart the client. Do not paste the token into the conversation
or commit it to a repository.

### Remove a client connection

Run the removal command for the client you configured:

```bash
claude mcp remove --scope user kmt
codex mcp remove kmt
grok mcp remove --scope user kmt
unset KMT_MCP_TOKEN KMT_MCP_URL
```

Removing client configuration or unsetting a variable does not revoke the
token. Revoke unused tokens through the main API as described above.

## Connect using curl

Use an approved private route or a TLS endpoint supplied by your operator.
For a local binary or local Compose service:

```bash
export MCP_URL=http://127.0.0.1:7889
read -r -s -p 'MCP token: ' MCP_TOKEN; echo
export MCP_TOKEN
curl -fsS "$MCP_URL/health"
curl -fsS "$MCP_URL/ready"

mcp_call() {
  curl -fsS "$MCP_URL/mcp" \
    -H "Authorization: Bearer $MCP_TOKEN" \
    -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    --data "$1" | sed -n 's/^data: //p' | jq .
}
mcp_call '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"customer-check","version":"1"}}}'
mcp_call '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
mcp_call '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"whoami","arguments":{}}}'
mcp_call '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_tenants","arguments":{}}}'
mcp_call '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_realms","arguments":{}}}'
unset MCP_TOKEN
```

These Bash examples require curl, sed and jq. The final example assumes a
single-tenant token. For a multi-tenant token, supply `tenant` in tenant-scoped
tool arguments. For a single-tenant token, omit it; the server injects it.
A successful `whoami` response confirms authentication. The tenant and realm
responses show which monitoring data the token can access.
HTTP 200 alone is insufficient: JSON-RPC `error` or `result.isError: true`
means the tool failed. Empty result arrays can be valid.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Process exits immediately | `mcp.enabled` and environment override; no hot reload |
| Startup refuses configuration | Read-only DB block, password, cursor key length |
| Ready but requests return 401 | Token expiry/revocation, nonempty allowlist, user state, SSO sign-in age; API schema and DB grants |
| HTTP 400 | Include both Accept media types; inspect request parameters |
| Browser request rejected | Exact Origin must be in `mcp.origin_allowlist`; keep empty for non-browser private clients |
| `AMFA not available for this tenant` | Expected for a tenant without AMFA configured. Configure AMFA for the tenant to use `get_amfa_stats`; other tools remain available within your permissions. After AMFA is configured, restart the MCP server; it reads AMFA settings only at startup. |
| HTTP 200 but tool error | Inspect JSON-RPC error/isError, tenant/realm scope and permissions |
| jq parse error | Strip SSE `data:` framing as shown above |

Health/readiness demonstrate process/database availability, not a working token,
correct grants on every table, or a successful tool call.
