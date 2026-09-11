# Keycloak Monitoring Tool MCP Server

`cmd/mcp-server` is a read-only Model Context Protocol (MCP) server exposing monitoring data from the Keycloak Monitoring Tool to MCP clients such as Claude Code and Claude Desktop. It serves tenants, Keycloak realms, alerts, events and Adaptive Multi-Factor Authentication (AMFA) statistics over the streamable HTTP transport, authenticated with personal access tokens.

Every tool is read-only. The server runs as a separate process from the main API server, performs no schema migrations and never writes monitoring data.

## Running the server

Start the main API server against the database at least once first. Schema creation happens through GORM's `AutoMigrate` during the main server's startup, and the MCP server opens a read-only client that migrates nothing, so it never creates the tables it reads.

Skipping that step fails misleadingly. Against an un-migrated database the MCP server starts normally, `/health` and `/ready` both answer 200, and every request comes back as a bare 401 `invalid token`, which is indistinguishable from a genuinely bad token. The real cause appears only in the MCP server's own log, as `relation "api_tokens" does not exist`.

`make build-all` covers the server and web binaries only, so name the MCP server target explicitly:

```bash
make build-mcp-server
./bin/mcp-server -config config.yaml
```

The server is disabled by default. Enable it in the `mcp` block of `config.yaml` (see `config.yaml.example` for the full commented reference):

```yaml
mcp:
  enabled: true
  port: 7889
  cursor_hmac_key: "<random string, at least 32 bytes>"
```

- With `mcp.enabled: false` (the default) the binary exits immediately.
- The MCP endpoint is `POST /mcp` on `mcp.port`. `/health`, `/ready` and, when `mcp.metrics.enabled` is set, Prometheus `/metrics` are served on the same port.
- `cursor_hmac_key` signs pagination cursors and is required: at least 32 bytes, from `openssl rand -base64 32`. The server refuses to start on an empty or shorter key rather than fall back to a per-process one, whose cursors would stop validating across a restart and force clients back to the first page.

### Read-only database role

The `mcp.database` block (same shape as the top-level `database` block) points the server at the same database under a dedicated read-only role instead of the main application role, so the process cannot write even if it were compromised. It is required, and the server refuses to start without it:

```yaml
mcp:
  database:
    host: "your-postgres-host"
    port: 5432
    database: "monitoring"
    user: "monitoring_readonly"
    password: ""
    ssl_mode: "require"
```

Nothing in the config loader expands `${...}`, so a `"${VAR}"` value is sent to PostgreSQL literally. Supply the password through `MONITORING_MCP_DATABASE_PASSWORD` instead, and keep the empty `password: ""` key in the file: Viper's `AutomaticEnv` binds an environment variable only for a key that is already registered as a default or present in `config.yaml`, and `mcp.database.*` keys are deliberately not registered as defaults (a registered default would make the block always non-nil, so a config that names no role would look like one that does), so an absent key means the environment variable is silently ignored.

Use `ssl_mode: "require"` unless the database is reachable only over a private container network, which is the one case `"disable"` suits.

The role needs `CONNECT` on the database, `USAGE` on the schema and `SELECT` on the tables the tools read, and nothing beyond that. Deployments that ship a `deployments/` tree carry the provisioning script and its notes at `deployments/postgres/mcp-readonly-role.sql` and `deployments/postgres/README.md`; otherwise create the role by hand. There is no fallback to the main `database` block: an omitted `mcp.database` fails startup rather than quietly running the whole assembly as the read-write application role.

## Creating a personal access token

Personal access tokens authenticate MCP clients. Only platform administrators can create, list and revoke them, and the endpoints live on the main API server (port 7888), not on the MCP port. That server has to be running for any of the calls below.

A fresh database also has no administrator to log in as. The first admin user is created during the main server's startup from `auth.simple.default_user`, `auth.simple.default_pass` and `auth.simple.default_email`, and only when the users table is still empty. Of the three, only `default_pass` has no default (`default_user` defaults to `admin`, `default_email` to `admin@example.com`), so with simple authentication enabled and no users yet, the main server fails to start until it is set. The value must satisfy the platform's password policy: at least 12 characters, with an uppercase letter, a lowercase letter, a digit and a special character, and not one of the common weak passwords the validator rejects outright.

The login step below assumes simple authentication is enabled; `/auth/login/simple` answers 403 when it is not. In that case authenticate in the web UI and reuse the session cookie, or send `Authorization: Bearer <OAuth2 access token>` on the `/api/tokens` calls in place of `-b cookies.txt`.

The login also writes a session cookie, so `auth.session.secret` has to be set before the call can succeed at all. Left unset, a correct username and password still answer `500 {"error":"Failed to create session"}`, with `securecookie: hash key is not set` in the main server's log. `config.yaml.example` ships the key with the placeholder `change-this-to-a-random-secret-key-at-least-32-chars`; replace it with at least 32 random characters (`openssl rand -base64 32`). Supplying the value through `MONITORING_AUTH_SESSION_SECRET` works only while the key stays in `config.yaml`, for the `AutomaticEnv` reason above: no default is registered for it either.

Log in and keep the session cookie:

```bash
curl -sS -c cookies.txt -X POST http://your-monitoring-host:7888/auth/login/simple \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "<admin-password>"}'
```

Create the token. `user_id` is the platform user the token acts as (find it via `GET /api/users`); a token can never do more than that user's role-based access control (RBAC) roles and tenant policies allow.

```bash
curl -sS -b cookies.txt -X POST http://your-monitoring-host:7888/api/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "name": "analyst-mcp",
    "user_id": 42,
    "tenant_ids": ["tenant-a", "tenant-b"],
    "expires_at": "2027-03-01T00:00:00Z"
  }'
```

- `tenant_ids` is the token's tenant allowlist and is required: a request that omits it, or sends an empty array, is rejected with 400. The token is restricted to the listed tenants, and effective scope is always the intersection of this list and the user's RBAC scope. Name the fewest tenants the token needs, which is usually one.
- `expires_at` is optional RFC3339. Omitted, the token expires after 90 days; a requested expiry is capped at 365 days.

The `201` response is the only place the plaintext token ever appears:

```json
{
  "id": 7,
  "user_id": 42,
  "name": "analyst-mcp",
  "tenant_ids": ["tenant-a", "tenant-b"],
  "expires_at": "2027-03-01T00:00:00Z",
  "created_at": "2026-09-06T10:00:00Z",
  "token": "<plaintext token, shown exactly once>"
}
```

A token created before the allowlist became mandatory carries none, and is rejected at validation time rather than grandfathered: every request it makes answers 401, and the fix is to mint a replacement with `tenant_ids` set.

Store it in a secrets manager immediately. `GET /api/tokens` returns metadata only; neither the plaintext nor its digest can be retrieved again.

Revoke a token by its `id`. Revocation is immediate:

```bash
curl -sS -b cookies.txt -X DELETE http://your-monitoring-host:7888/api/tokens/7
```

## Connecting a client

### Reaching the endpoint

Which of the two deployments you run decides how you get to the endpoint. The examples below all use `http://127.0.0.1:7889/mcp`; substitute the address your own route produces.

**Local development.** `deployments/local/docker-compose.yml` publishes the port (`ports: - "7889:7889"`), so the endpoint is directly reachable at `http://127.0.0.1:7889/mcp` with no forwarding and no VPN. A binary started with `./bin/mcp-server` listens on the host directly and behaves the same way. Use the examples below unchanged.

**Packaged server deployment.** `deployments/server/docker-compose.yml` publishes no port and adds no reverse-proxy route on purpose: the service is attached to the internal Docker network only, so nothing is listening on `<host>:7889` and the endpoint is unreachable from outside that network. Reach it from another container on the network, over a VPN route into it, or through an SSH local forward.

The forward needs the container's address on that network, not its name. Docker's embedded DNS resolves `kmt-mcp-server` only from inside the network, so the Docker host itself, which is where the forward terminates, cannot use the name as a target. Read the address on the host:

```bash
docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' kmt-mcp-server
```

Then forward to it from the workstation. The address is reassigned whenever the container is recreated, so re-read it rather than storing it:

```bash
ssh -L 7889:<container-ip>:7889 <user>@<host>
```

### Claude Code

```bash
claude mcp add --transport http kmt http://127.0.0.1:7889/mcp \
  --header "Authorization: Bearer <token>"
```

Then ask Claude to run the `whoami` tool to verify connectivity.

### Claude Desktop

Claude Desktop launches local MCP servers; bridge to the HTTP endpoint with `mcp-remote` in `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "kmt": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "http://127.0.0.1:7889/mcp",
        "--transport",
        "http-only",
        "--header",
        "Authorization:${MONITORING_MCP_AUTH}"
      ],
      "env": {
        "MONITORING_MCP_AUTH": "Bearer <token>"
      }
    }
  }
}
```

The header argument carries no space: Claude Desktop's argument handling breaks on a space inside the value, so the `Bearer ` prefix travels in the environment variable instead. `--transport http-only` keeps the bridge from probing for Server-Sent Events (SSE) against an endpoint whose `GET` answers 405.

### Generic MCP clients

The server implements the streamable HTTP transport in stateless mode:

- Send JSON-RPC 2.0 requests as `POST /mcp` with `Authorization: Bearer <token>`, `Content-Type: application/json` and `Accept: application/json, text/event-stream`.
- `Accept` must contain both of those media types. Sending only `application/json` answers 400 with `Accept must contain both 'application/json' and 'text/event-stream'`. Omitting the header entirely works by accident, because curl then sends `Accept: */*`, which satisfies both; a client that sets a narrower header gets the 400.
- Every POST runs in its own temporary session: no `Mcp-Session-Id` is issued, nothing accumulates server-side between calls, and `GET` or `DELETE` on `/mcp` answer 405.
- Request bodies are capped at 1 MiB.
- Requests carrying an `Origin` header are rejected unless the value is listed in `mcp.origin_allowlist` (empty by default, rejecting all of them).

Connectivity check:

```bash
curl -sS http://127.0.0.1:7889/mcp \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"whoami","arguments":{}}}'
```

Responses are Server-Sent Events framed, stateless mode included, so the body is never bare JSON:

```
event: message
data: {"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"admin\":false,\"server_version\":\"0.1.0\",\"subject\":\"user-7\",\"tenant_allowlist\":[\"tenant-a\"]}"}],"structuredContent":{"admin":false,"server_version":"0.1.0","subject":"user-7","tenant_allowlist":["tenant-a"]}}}
```

Piping that into `jq` fails on the `event:` line with a parse error. Strip the framing by appending this to the command above:

```bash
| sed -n 's/^data: //p' | jq .
```

The tool's own result is JSON twice over: once as `structuredContent`, and again as a JSON string inside the text content block. Read `structuredContent`.

## Tool reference

Semantics shared by every tool:

- All timestamps, inbound and outbound, are RFC3339 in UTC, for example `2026-01-02T15:04:05Z`. Any offset is accepted on input and normalized to UTC; only a timestamp carrying no offset at all is rejected.
- Tools taking a time window: `to` defaults to now, `from` defaults to 24 hours before `to`, and the window spans at most 90 days.
- The `tenant` argument is not a free parameter. A token whose allowlist holds exactly one tenant has that tenant supplied by the server, and a call that names a tenant anyway is rejected as invalid input, its own tenant included. A token whose allowlist holds several must name one of them, and naming anything outside the list is denied. Either way the tenant must exist and be enabled, and RBAC must grant the tool's permission for it. Results are further narrowed to the realms the caller's tenant policy allows; a `realm` argument outside that set is denied.
- A resource outside the caller's tenant or realm scope is indistinguishable from a missing one: both report not found.
- Returned strings are sanitized (bidirectional overrides, zero-width characters and control characters other than newline and tab are stripped) and length-capped: 256 characters for identifier-like fields, 2048 for description-like text.
- Where `limit` is accepted, an omitted or non-positive value gets the configured default (`mcp.default_page_size`, 50 by default) and any requested value is capped at `mcp.max_page_size` (500 by default). These two replace the older `max_page_size` (the default) and `absolute_max_page_size` (the cap), and a config still carrying `absolute_max_page_size` fails startup: under the current names its `max_page_size` would cap every page at what it used to default to.
- A tool result whose serialized size exceeds half of `mcp.max_response_bytes` is replaced with `result too large, retry with a smaller limit`, which is distinct from the generic internal error and names no size, limit or content. The transport carries the marshaled result twice, as structured output and again as its text content block, so the cap is applied at half the configured value: 512 KiB per result at the 1 MiB default.

Required RBAC permission per tool:

| Tool | Permission |
|---|---|
| `whoami` | none beyond a valid token |
| `list_tenants`, `get_tenant_health` | `tenants:read` |
| `list_realms` | `tenants:read` and `keycloak:read` |
| `list_alerts`, `get_alert`, `get_alert_stats` | `alerts:read` |
| `get_amfa_stats` | `amfa:read` |
| `list_events`, `get_event`, `get_event_stats` | `keycloak:read` |

### whoami

Connectivity check. Takes no parameters and reads no monitoring data; returns the authenticated caller's `subject`, the token's `tenant_allowlist`, an `admin` flag and the `server_version`. Read `tenant_allowlist` first: it decides whether the tenant-scoped tools below take a `tenant` argument at all.

### list_tenants

Lists the enabled tenants (monitored Keycloak instances) the caller may access: the token's tenant allowlist intersected with the caller's RBAC scope. Takes no parameters. Returns `tenants`, each with `tenant_id`, `name` and `health_status` (`healthy`, `unhealthy` or `unknown`).

### get_tenant_health

| Parameter | Type | Required | Description |
|---|---|---|---|
| `tenant` | string | only for a multi-tenant token | Tenant ID to report health for |

Returns `health_status` and `last_health_check` (RFC3339 UTC; empty when the tenant has never been checked).

### list_realms

| Parameter | Type | Required | Description |
|---|---|---|---|
| `tenant` | string | only for a multi-tenant token | Tenant ID whose realms to list |

Returns `realms`: the tenant's Keycloak realm names, narrowed to the realms the caller's tenant policy allows.

### list_alerts

Lists a tenant's alerts, newest first, with limit/offset pagination. Internal alert fields (metadata, rule and event linkage) are always stripped, for administrators included.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `tenant` | string | only for a multi-tenant token | Tenant ID whose alerts to list |
| `realm` | string | no | Realm name filter |
| `status` | string | no | `active`, `resolved`, `acknowledged` or `ignored` |
| `severity` | string | no | `info`, `warning`, `error` or `critical` |
| `limit` | int | no | Page size; server default and cap apply |
| `offset` | int | no | Rows to skip; 0 to 10000 |

Returns `alerts` (each with `alert_id`, `severity`, `status`, `type`, `title`, `description`, `recommendation`, `resource_type`, `resource_name`, `realm_name`, `first_detected`, `last_seen`), `total`, and the `limit` and `offset` actually applied.

### get_alert

| Parameter | Type | Required | Description |
|---|---|---|---|
| `tenant` | string | only for a multi-tenant token | Tenant ID the alert belongs to |
| `alert_id` | string | yes | The alert's tenant-unique identifier |

Returns a single alert in the `list_alerts` shape.

### get_alert_stats

| Parameter | Type | Required | Description |
|---|---|---|---|
| `tenant` | string | only for a multi-tenant token | Tenant ID to report statistics for |
| `realm` | string | no | Scope the statistics to one realm |

Returns active-alert statistics: `total_active`, `by_severity` and `by_type`. A caller restricted to specific realms gets the aggregate over exactly those realms.

### get_amfa_stats

AMFA key performance indicators for one realm over a time window. The realm is required: AMFA statistics are always realm-scoped over MCP. Beyond the 90-day window cap, each tenant's own AMFA lookback (30 days by default, at most 90) bounds how far back data exists.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `tenant` | string | only for a multi-tenant token | Tenant ID to report AMFA statistics for |
| `realm` | string | yes | Realm to scope to |
| `from` | string | no | RFC3339 window start; defaults to 24 hours before `to` |
| `to` | string | no | RFC3339 window end; defaults to now |

Returns `total`, `risky`, `unique_users` and `flagged_ips` (distinct VPN-flagged IP addresses). Tenants without AMFA configured report "AMFA not available for this tenant".

The per-tenant AMFA registry is built once at startup and registers no tenant-change callbacks, so a tenant whose AMFA configuration changes after boot keeps reporting not available until the process restarts.

### list_events

Lists a tenant's Keycloak and AMFA events over a time window, newest first, with cursor pagination. The raw event payload is never included.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `tenant` | string | only for a multi-tenant token | Tenant ID whose events to list |
| `realm` | string | no | Realm to scope to |
| `from` | string | no | RFC3339 window start; defaults to 24 hours before `to` |
| `to` | string | no | RFC3339 window end; defaults to now |
| `type` | string | no | Exact event type, for example `LOGIN` |
| `severity` | string | no | Exact severity |
| `min_risk` | int | no | Minimum AMFA risk level; events without a risk level never match |
| `limit` | int | no | Page size; server default and cap apply |
| `cursor` | string | no | Opaque cursor from a previous page's `next_cursor` |

Returns `events` (each with `event_id`, `timestamp`, `type`, `category`, `severity`, `description`, `source`, `source_ip`, `username`, `email`, `client_id`, `status`, `realm`, and the AMFA enrichment fields `risk_level`, `final_status`, `is_vpn`, `country`, `city`, `os`, `browser`, `device` where present) and `next_cursor`.

Cursor contract:

- An empty `next_cursor` means there are no more events; otherwise pass it back as `cursor` to fetch the next page.
- Every filter parameter (`tenant`, `realm`, `from`, `to`, `type`, `severity`, `min_risk`) must be identical to the call that returned the cursor. Cursors are signed with a keyed-hash message authentication code (HMAC) bound to both the exact filter set and the exact API token, so changing any filter, or presenting the cursor under a different token, rejects it as invalid input.
- The cursor carries the window its first page resolved, and the signature covers that window too. A listing that omits `from` and `to` therefore keeps querying the window the first page resolved instead of one that moves with wall-clock time, however long the client takes between pages.
- Cursors survive a restart, because `mcp.cursor_hmac_key` is a required setting and the same key signs them before and after.

### get_event

| Parameter | Type | Required | Description |
|---|---|---|---|
| `tenant` | string | only for a multi-tenant token | Tenant ID the event belongs to |
| `event_id` | string | yes | The event's tenant-unique identifier |

Returns a single event in the `list_events` shape. The raw event payload is never included.

### get_event_stats

| Parameter | Type | Required | Description |
|---|---|---|---|
| `tenant` | string | only for a multi-tenant token | Tenant ID to report statistics for |
| `realm` | string | no | Scope the statistics to one realm |
| `from` | string | no | RFC3339 window start; defaults to 24 hours before `to` |
| `to` | string | no | RFC3339 window end; defaults to now |

Returns event counts over the window: `by_type`, `by_severity` and `by_source`. A caller restricted to specific realms gets the aggregate over exactly those realms.

## Rate limits

Two token buckets, configured under `mcp.rate`. Setting a rate to 0 disables that limiter; `burst` is the instantaneous allowance of both.

| Limit | Default | Applies to | Keyed by | On exceed |
|---|---|---|---|---|
| `per_ip_per_minute` | 300 | Every request, before authentication | The connection's direct remote address: the exact address for IPv4, the masked /64 prefix for IPv6, so an entire /64 shares one bucket. `X-Forwarded-For` is deliberately ignored, because the server does not assume a trusted reverse proxy and a spoofable header would let a client pick its own bucket | HTTP 429 |
| `per_user_per_minute` | 120 | Tool calls, after authentication | User ID, so all of a user's tokens share one budget | Tool error `rate limited, retry later` |

`initialize` and `tools/list` are never budget-limited, so a throttled client can still hold a session and discover the tools.

## Audit logging and metrics

- Every inbound JSON-RPC method is logged with the caller's user ID, subject and token ID.
- Every tool call emits exactly one audit line: tool, outcome (`allowed`, `denied`, `invalid`, `not_found`, `error`, `rate_limited`, `too_large`), duration, caller identity, the `tenant` and `realm` arguments, and a row count for list tools. No other tool argument is ever logged; free-text parameters never reach the log.
- Audit lines carry the `mcp-audit` component and are pinned to info level: they keep being emitted when the global log level is raised to warn or error, so tightening logging operationally cannot silence the trail. Allowed calls log at info; denied, invalid, not-found, rate-limited and error calls log at warn.
- With `mcp.metrics.enabled`, `/metrics` exposes `pmp_mcp_requests_total{method}`, `pmp_mcp_tool_calls_total{tool,outcome}` and the latency histogram `pmp_mcp_tool_call_duration_seconds{tool}`, which is observed for allowed calls only. The endpoint is served outside the authentication middleware, so `mcp.metrics.auth_token` is required whenever metrics are enabled: the server refuses to start on an enabled endpoint with no token, and scrapes send it as `Authorization: Bearer <token>`.

## Security

- **Tokens are secrets.** The plaintext is shown exactly once at creation and only a digest is stored. Send it only in the `Authorization` header, never in URLs or query strings, keep it in a secrets manager, and revoke it the moment it is no longer needed. Prefer short expiries and per-purpose tokens with the narrowest `tenant_ids` allowlist that works.
- **Tool results are data, not instructions.** Alert titles, event descriptions, usernames and similar fields originate in monitored systems and can contain text crafted by whoever can write to those systems, for example a username chosen at self-registration. The server strips bidirectional overrides, zero-width characters and control characters other than newline and tab, and it caps field lengths. Newlines survive on purpose, so that multi-line text stays readable, which also means a hostile field can still be laid out to imitate a new conversational turn. No sanitization makes hostile text safe to obey: the consuming model must treat every tool result strictly as data, and a residual risk of prompt injection through monitored data remains to be handled on the client side.
- **Internal network only by default.** The listener serves plain HTTP without TLS. Expose the MCP port only on internal networks, and terminate TLS at a reverse proxy if a client must connect across a trust boundary. The empty-by-default `mcp.origin_allowlist` rejects every browser-origin request as a DNS-rebinding defense; MCP clients are not browsers and send no `Origin` header.
- **No write path.** All tools are read-only and the assembly runs without schema migrations. The monitoring database is reached under the read-only role `mcp.database` names, and a tenant's AMFA database, when it is read directly rather than through AMFA's own read-only API, is reached in a session Postgres holds read-only, so neither connection can write whatever its credentials allow.
