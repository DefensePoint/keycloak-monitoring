# Keycloak Monitoring Tool MCP user guide

Keycloak Monitoring Tool (KMT) is the monitoring application for Keycloak
customers. It includes a separate, read-only Model Context Protocol (MCP)
server (`cmd/mcp-server`). The server exposes tenants, realm listings, tenant
health, alerts, events and Adaptive Multi-Factor Authentication (AMFA)
statistics using Streamable HTTP. Starting the main web/API process does not
start the MCP server. Clients authenticate with a personal access token; the
token's tenant allowlist and the user's role-based access control (RBAC)
permissions both apply.

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

## Customer connectivity and screenshot check

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
Capture `whoami`, the discovered tool names and a successful scoped read.
Keep the token, login response, cookies and secret values out of screenshots.
HTTP 200 alone is insufficient: JSON-RPC `error` or `result.isError: true`
means the tool failed. Empty result arrays can be valid.

## Retest checklist

Record application version, image digest/source SHA, environment, UTC time and
each actual result. Repeat after releases or MCP configuration/auth changes.

- Enable: process/pod is ready; `/health` and `/ready` return 200.
- Authenticate: missing, invalid and revoked tokens return 401.
- Protocol: initialization and `tools/list` succeed; check the advertised tools.
- Data: `whoami`, tenant/realm reads, alerts, events and configured AMFA reads
  succeed without JSON-RPC/tool errors. Compare representative results with
  the UI/API for the same scope. Test ID-based tools with real returned IDs.
- Scope: a restricted token cannot read outside its tenant/realm permissions.
  Include a multi-tenant token when checking cross-tenant denial; single-tenant
  tokens reject an explicitly supplied tenant argument at validation time.
- Database: confirm the MCP role can SELECT required tables and cannot write.
- Disable: stop/scale down MCP, confirm endpoint requests fail and the main
  application remains healthy.
- Re-enable: establish a new connection and repeat authenticated discovery
  and representative data calls. Revoke temporary test tokens afterward.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Process exits immediately | `mcp.enabled` and environment override; no hot reload |
| Startup refuses configuration | Read-only DB block, password, cursor key length |
| Ready but requests return 401 | Token expiry/revocation, nonempty allowlist, user state, SSO sign-in age; API schema and DB grants |
| HTTP 400 | Include both Accept media types; inspect request parameters |
| Browser request rejected | Exact Origin must be in `mcp.origin_allowlist`; keep empty for non-browser private clients |
| HTTP 200 but tool error | Inspect JSON-RPC error/isError, tenant/realm scope and permissions |
| jq parse error | Strip SSE `data:` framing as shown above |

Health/readiness demonstrate process/database availability, not a working token,
correct grants on every table, or a successful tool call.
