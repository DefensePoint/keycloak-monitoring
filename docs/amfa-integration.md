# AMFA Integration Guide

Keycloak Monitoring Tool's **AMFA Events** feature reads from each tenant's AMFA (Adaptive MFA) PostgreSQL database. This guide walks an operator through enabling it.

## Architecture summary

- One AMFA service per KMT tenant (one AMFA per Keycloak instance).
- KMT opens a per-tenant read-only PostgreSQL connection to each enabled tenant's AMFA database.
- KMT never modifies AMFA's code or data — the integration is purely read-only.
- AMFA Events is disabled by default. It is enabled per tenant via config.

## Prerequisites

- One AMFA service per KMT tenant (one AMFA per Keycloak instance).
- Network reachability from KMT to each AMFA's PostgreSQL.
- Optionally a read replica per AMFA database (recommended for production deployments).
- The `amfa:read` permission (added automatically to the default `admin`, `operator`, and `viewer` roles by an idempotent migration on KMT startup).

## One-time setup on each AMFA database

Run on every AMFA PostgreSQL instance KMT will read from. The dedicated role enforces least privilege at the database level — even a bug in KMT cannot write to AMFA.

```sql
CREATE USER kmt_ro WITH PASSWORD '<strong-password>';
GRANT CONNECT ON DATABASE adaptive_mfa TO kmt_ro;
GRANT USAGE ON SCHEMA public TO kmt_ro;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO kmt_ro;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT ON TABLES TO kmt_ro;

-- Recommended for AMFA Events query performance (filters via
-- auth_context_json->>'realm_id' on every query):
CREATE INDEX IF NOT EXISTS idx_auth_process_realm_id
    ON auth_process ((auth_context_json->>'realm_id'));
```

## KMT configuration (per tenant)

Add an `amfa:` block under any tenant in `config.yaml`. Tenants without an `amfa:` block — or with `enabled: false` — get a "not configured" empty state on the AMFA Events page; everything else continues to work.

```yaml
keycloak:
  tenants:
    prod-keycloak:
      enabled: true
      server_url: "https://kc-prod.example.com"
      admin_username: "admin"
      admin_password: "${KC_PROD_ADMIN_PASSWORD}"
      amfa:
        enabled: true
        events_lookback_days: 30                # default 30; capped at 90 (drives normalizeTimeWindow)
        expected_schema_version: c00d6d7d197c   # alembic_version this AMFA was tested against; empty = skip check
        database:
          host: amfa-prod.internal
          port: 5432
          database: adaptive_mfa
          user: kmt_ro
          password: ${PROD_AMFA_DB_PASSWORD}
          sslmode: require
          max_conns: 5
          min_conns: 1
          timeout: 10s
          # read_replica_host: amfa-prod-replica.internal   # optional; preferred when set
    legacy-tenant:
      enabled: true
      server_url: "https://kc-legacy.example.com"
      admin_password: "${KC_LEGACY_ADMIN_PASSWORD}"
      # No amfa: block → AMFA Events page returns "not configured" for this tenant
```

Sensitive values (DB passwords) come from environment variables using KMT's existing per-tenant override pattern. Do not commit secrets to `config.yaml`.

## RBAC

The `amfa:read` permission is added by an idempotent migration on KMT startup:

- The permission row is created if it doesn't exist.
- The default `admin`, `operator`, and `viewer` roles each get granted `amfa:read` (skipped if already granted).
- Custom roles must be granted `amfa:read` explicitly via the admin UI or the RBAC API.

## Verification

Expected KMT startup log lines (per enabled tenant):

```
[INFO] AMFA enabled tenant detected: tenant_id=prod-keycloak
[INFO] AMFA database connected: tenant_id=prod-keycloak host=amfa-prod.internal database=adaptive_mfa
[INFO] AMFA schema version matches: tenant_id=prod-keycloak version=c00d6d7d197c
[INFO] AMFA registered for tenant: tenant_id=prod-keycloak
[INFO] AMFA module ready: enabled_tenant_count=1
```

Then open the AMFA Events page from the sidebar (`/{tenantId}/amfa-events`) and verify:

1. KPI cards show non-zero numbers if AMFA has events for the selected realm.
2. The events table populates and is filterable / paginatable.
3. The world map shows marker clusters at the geo points reported by AMFA's `auth_context.lat/long`.
4. Switching tenants (via the `TenantSelector`) routes you to the correct tenant's AMFA data; tenants without AMFA configured show a clear empty state.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| Page returns 404 `amfa_not_configured` for a tenant | That tenant has no `amfa:` block, or `amfa.enabled: false`. | Add or enable the block. |
| AMFA endpoint returns 503 `amfa_unavailable` | The tenant's AMFA DB is temporarily unreachable: connection refused, dial timeout, dropped connection, or a Postgres SQLSTATE class `08` (Connection Exception). | Check `docker logs` for the underlying error. Verify host/port reachability and that the `kmt_ro` role still works. Distinct from `500 internal error`, which indicates a query/schema bug rather than availability. |
| AMFA endpoint returns 400 with `invalid start_time` / `invalid end_time` / `invalid time range` | Frontend or curl sent a non-RFC3339 timestamp, or a window where start > end. | Use RFC3339 (e.g. `2026-05-01T00:00:00Z`) and make sure start ≤ end. |
| AMFA endpoint returns 400 with `offset must be <= 50000` | Pagination offset is too deep — this is a guard against full-scan DoS on AMFA's `auth_event/auth_process` join. | Use a tighter time range to surface older events instead of paging deep. 50k offset is 250 pages at the max page size (200). |
| Log line `AMFA schema version drift detected — KMT may need updating` | AMFA's database schema has moved past the version KMT was built against. The feature still works for the fields KMT knows about, but new columns or renames will not be picked up. | Test against the new AMFA schema and bump `expected_schema_version` once verified. |
| Log line `Could not read AMFA alembic_version table` | The `alembic_version` table is missing from AMFA's DB (e.g., a partial volume wipe). The schema check is non-fatal — KMT continues to operate. | Re-create the table from the current head: `CREATE TABLE alembic_version (version_num VARCHAR(32) PRIMARY KEY); INSERT INTO alembic_version VALUES ('<current_head_hash>');` Confirm the head by inspecting `migrations/versions/` in the AMFA repo. |
| Events table shows raw UUIDs instead of usernames | Either the Keycloak admin client isn't healthy for that tenant, or the AMFA event references a `user_id` that no longer exists in Keycloak. AMFA Events falls back to UUID display so the page still loads. | Check Keycloak is up and the monitor pool reports `Keycloak client initialized successfully` at startup. If the user genuinely doesn't exist in Keycloak (e.g. deleted), the UUID display is correct. |
| Page shows nothing when "All Realms" is selected in the dropdown | AMFA endpoints require a specific `realm_id` — they don't aggregate cross-realm. The hooks are intentionally disabled when no specific realm is picked. | Select a specific realm. An info banner now explains this directly on the page. |
| Geolocation map area is empty (no tiles, no markers) | If you're maintaining the map component: react-leaflet 4 strips non-`height/width` keys from its `style` prop, and a percentage height races MUI's layout pass at first paint. Leaflet measures `0×0` and never re-measures, so the map renders invisible. | Always pass an explicit pixel height to `MapContainer style={{ height: <number>, width: "100%" }}`. See the inline comment in `web/src/features/amfa/components/AmfaGeoMap.tsx`. |
| Map shows fewer markers than the events count | AMFA rows with `NULL` lat/long are excluded from the geo aggregation. The events table still shows them. | Expected behavior. |
| Counts lower than expected on older AMFA data | Historical AMFA rows may not have the `realm_id` key in `auth_context_json`. | Expected for legacy data. New events will include the key. |

## Seeding demo data for UI development

For development / UI work without waiting for real AMFA traffic, seed a few synthetic `auth_process` rows and events directly:

```sql
-- Insert one realistic record set:
--   1× auth_context (lat/long for the map, IP, country)
--   1× device + 1× location_network (FK targets)
--   1× auth_process (links them, sets realm + risk_level)
--   1× auth_event  (the displayable event row)
```

For two real users to appear with names instead of UUIDs, look up their UUIDs from Keycloak and use those `user_id` values in your seed.

## Rollback

To disable for a single tenant:

```yaml
amfa:
  enabled: false
```

Restart KMT. Other tenants are unaffected. AMFA itself was never touched — there is nothing to roll back on the AMFA side. The read-only DB role on AMFA can stay (it's harmless) or be revoked at leisure.

To disable globally: remove or set `enabled: false` on every tenant's `amfa:` block.

## Known limitation (follow-up)

In the current implementation, AMFA event enrichment with Keycloak usernames/emails requires a Keycloak admin client to be available to the AMFA fx module. KMT currently constructs Keycloak admin clients per-tenant inside the monitor pool, not as a process-scope singleton, so AMFA enrichment falls back to UUID-only display until one of the following is done:

1. Expose a per-tenant Keycloak admin client lookup function to `internal/fx/amfa.go` (small refactor of `internal/fx/monitoring.go`'s `MonitorPoolManager`).
2. Or construct a dedicated process-scope admin client for AMFA in `internal/fx/amfa.go`.

The UUID-only fallback is a deliberate degraded mode, not a bug: enrichment quality drops, correctness does not.

## Alerts

The AMFA risk-alert checker (`amfacheck/`) is a per-tenant poll loop that turns high-risk AMFA login events into KMT alerts. Alerts flow through the normal alert lifecycle and notification fan-out, so they appear on the Alerts page and trigger any configured notifications (e.g. Slack) exactly like Keycloak event-checker alerts. The checker reads AMFA's database read-only through the same `kmt_ro` role used by the rest of the integration; it never writes to AMFA.

### Rules and severities

| Rule | What it detects | Severity |
|---|---|---|
| `risk_rejected` (rule 1) | Logins rejected by AMFA's risk engine. | critical |
| `repeated_risky` (rule 2) | A user accumulating repeated risky attempts within a window (default 3 within 24h at `min_risk_level` 2+). | warning |
| `vpn_risky` (rule 3) | A risky login originating from a VPN or anonymizing IP. | warning |
| `login_error` (rule 4a) | A burst of `LOGIN_ERROR` events (default 5 within 5m). | warning |
| `client_login_error` (rule 4b) | A burst of `CLIENT_LOGIN_ERROR` events (default 5 within 5m). | warning |

### Configuration

The checker lives under `keycloak.global` in your KMT config, at the same level as `event_checker`. The master switch is off by default; opt in per check:

```yaml
      # AMFA risk-alert checker. Converts high-risk AMFA login events into KMT
      # alerts that flow through the normal alert lifecycle and notification
      # fan-out. Disabled by default; opt in per check. See docs/amfa-integration.md.
      amfa_checker:
        enabled: false            # MASTER SWITCH - off by default
        poll_interval: 1m
        checks:
          risk_rejected:      { enabled: true }                                        # rule 1: risk-engine rejections (critical)
          repeated_risky:     { enabled: true, threshold: 3, window: 24h, min_risk_level: 2 }  # rule 2: repeated risky attempts per user/day
          vpn_risky:          { enabled: true }                                        # rule 3: risky login from VPN/anonymizing IP
          login_error:        { enabled: true, threshold: 5, window: 5m }              # rule 4a: LOGIN_ERROR burst
          client_login_error: { enabled: true, threshold: 5, window: 5m }              # rule 4b: CLIENT_LOGIN_ERROR burst
```

### Expected startup logs

With the checker enabled, KMT logs something like:

```
amfacheck service constructed tenant_id=<id> checks=5 poll_interval=30s
AMFA checker services constructed count=N
amfacheck lifecycle hooks registered service_count=N
amfacheck service starting tenant_id=<id> num_checks=5 poll_interval=30s
```

When an alert is created, you will also see (per alert):

```
amfacheck: new alert tenant_id=<id> alert_id=<sha256> check=amfa-risk-rejected severity=critical
```

With the master switch off (the default), you will instead see a single line and no per-tenant loops:

```
AMFA checker globally disabled; no alert services constructed
amfacheck lifecycle hooks registered service_count=0
```

### Manual smoke test

1. Set `amfa_checker.enabled: true` for a tenant whose `amfa:` block is enabled, and restart KMT.
2. Trigger one risk-engine rejection via AMFA's `/decision` endpoint (a login that AMFA's risk engine rejects).
3. Wait one poll interval (default 1m).
4. Confirm the alert appears on KMT's Alerts page and that a notification lands in the configured Slack channel.

### Reload AMFA alerts (on demand)

The Alerts page shows a **Reload AMFA alerts** button next to Refresh, but only
for tenants where the AMFA checker is enabled (both the tenant's `amfa.enabled`
and the global `amfa_checker.enabled`). Clicking it runs the AMFA checker for
that tenant immediately - the same checks the background poller runs - then
reloads the list, so new AMFA alerts appear without waiting for the next poll
interval. It does not touch the other checkers, and it never removes alerts
(the checker is additive); notifications fire exactly as they do when polled.

Endpoints (both require `amfa:read`, under `/api/tenants/{tenantId}`):

- `GET  /amfa-checker`      returns `{ "enabled": true|false }` (drives button visibility)
- `POST /amfa-checker/run`  returns `{ "status": "ok" }`; `409 amfa_checker_not_enabled`
  if the checker is off for the tenant; `503 amfa_unavailable` if AMFA is unreachable
  or the run times out (so a manual trigger surfaces an error toast instead of a
  silent success); `500` for any other run error.

### Rollback

To disable the checker, set `enabled: false` on the `amfa_checker` block and restart KMT. No alerts are generated while it is off, and AMFA itself is never touched.

### Troubleshooting

| Symptom | Likely cause | Action |
|---|---|---|
| No `amfacheck service starting` log lines | `amfa_checker.enabled: false` (default) | Set it true and restart |
| Services start but no alerts ever | No qualifying events, or all per-check toggles disabled | Verify check toggles; confirm risky events exist in the window |
| `skipping tenant with no AMFA repository` warning | Tenant has `amfa.enabled: true` but its AMFA DB is unreachable | Check the tenant's `amfa.database` connection |
| Alert flood right after enabling | Backlog of historical risky events within the first-tick window | Expected for per-event rules; use the Alerts page silence manager to suppress noisy IPs/realms |
| Rule 2 misses a burst spanning midnight UTC | Day boundary is UTC; counts split across two days | Documented limitation; tune `window`/`threshold` if needed |
| One login appears twice on the unified Events page, as both `keycloak:<realm>` and `amfa:<realm>`, then becomes one row shortly after | The two mirrors write independently and a periodic sweep (`ReconcileAMFAMerges`) folds the AMFA row onto its Keycloak twin at the end of each mirror cycle. Between the AMFA write and that sweep, both halves of the same login are visible, and the event count is correspondingly high. | Expected behavior, bounded by `keycloak.global.amfa_mirror.poll_interval` (default 30s) and self-healing. Raising `poll_interval` widens the window proportionally. |
| One login appears twice on the unified Events page and **stays** that way | The two rows never merge because they share no correlation key. The merge is keyed on `amfa_event_id`, stamped onto the interactive login event by an Adaptive MFA Keycloak extension. The stock open-source Adaptive MFA SPI does not stamp it, so without an extension that does, KMT cannot tell that the Keycloak and AMFA rows describe the same login, and both are shown and counted. | Not the transient case above. Compare both sides for one realm: `SELECT source_system, count(*) AS rows, count(*) FILTER (WHERE amfa_event_id IS NOT NULL) AS with_merge_key FROM events WHERE source IN ('keycloak:<realm>','amfa:<realm>') GROUP BY source_system;` If the `amfa` row reports `with_merge_key > 0` but the `keycloak` row reports 0, no extension is stamping the key and every AMFA-covered login in that realm is counted twice indefinitely. A zero on the Keycloak side alone is not conclusive: only interactive logins that ran the AMFA flow carry the key, so `CLIENT_LOGIN`, token refreshes and non-AMFA flows legitimately have none. |

### Implementation notes

- **Per-tenant lifecycle.** One `amfacheck.Service` goroutine is constructed per tenant whose `amfa.enabled: true` AND `keycloak.global.amfa_checker.enabled: true`. The fx module starts each on `OnStart` and stops it on `OnStop`.
- **Poll-loop context.** The poll loop runs on a long-lived background context owned by the fx hook, NOT fx's `OnStart` context. fx cancels the `OnStart` context once startup finishes, so passing it to the loop would stop polling moments after boot (the loop would log `amfacheck polling cancelled`). Shutdown happens via `OnStop`, which cancels the background context and calls `Stop()` for a graceful drain.
- **Realm discovery.** Each tick the checker resolves the tenant's realms via `keycloak.Service.ListRealms` (the realms KMT has discovered/stored), so realms added or removed in Keycloak are picked up without a restart. AMFA events are matched by `auth_context_json->>'realm_id'`, so a realm only produces alerts if KMT has discovered a realm of that name.
- **Checkpoints.** Per-event rules (1, 3) keep an in-memory per-realm "last seen event time" and query `event_time > since`. The checkpoint is lost on restart; the first tick after a restart re-scans the last `poll_interval` and per-event `AlertID` dedup absorbs the replay. Because the checkpoint advances even when a downstream `SaveAlert` fails, a transient save failure is not retried for that exact event (rare; acceptable given dedup).
- **Threshold rules (4a, 4b) use a rolling window.** `login_error` and `client_login_error` count events of their type over a rolling `[now - window, now]` window (the same rolling style as rule 2), NOT a fixed clock-aligned bucket. This means a recent burst is always counted regardless of when the check runs, so a manual "Reload AMFA alerts" click (or a poll) reliably catches a burst even if it landed just before a clock boundary. The `AlertID` is stable per `(realm, eventType)`, so repeated polls and manual runs dedup to one alert row (the service bumps `LastSeen` on recurrence), mirroring how `eventscheck` handles its error-threshold alerts. (An earlier design used a fixed window-aligned bucket keyed into the `AlertID`; it was changed because a manual trigger could miss a burst that fell in the previous bucket.)
- **Alert storage.** Alerts are `domain.Alert` rows in KMT's `configuration_alerts` table with `source=event` and `check_type` one of `amfa-risk-rejected`, `amfa-repeated-risky`, `amfa-vpn-risky`, `amfa-login-error`, `amfa-client-login-error`.

### Testing the checker

There are three layers. All Go commands run in Docker (no host toolchain required):

```bash
gotest() { docker run --rm -v "$PWD:/app" -v go-mod-cache:/go/pkg/mod -w /app \
  golang:1.25.14-alpine sh -c "apk add --no-cache git build-base >/dev/null 2>&1 && $*"; }
```

**1. Unit tests** (stubbed repository, no database):

```bash
gotest go test ./amfacheck/ ./internal/config/ ./internal/fx/
```

Covers config defaults, all five rules (alert shape, severity, `AlertID`, thresholds, checkpoint advance, rolling-window threshold counting), the service save/notify/dedup orchestration, and the per-tenant fx builder.

**2. Integration tests against a real AMFA database** (gated by `//go:build integration`, skipped without `AMFA_TEST_DSN`). Point them at AMFA's PostgreSQL. From a test container, the host-published AMFA port is reachable via `host.docker.internal`.

> **Port note:** the [AMFA engine](https://github.com/DefensePoint/keycloak-adaptive-mfa-engine)'s own Docker Compose stack also publishes its Postgres on host port `5433`, same as KMT's `deployments/local/docker-compose.yml`. The two collide if both are running at once — stop one before starting the other, or edit one of the two compose files' `ports:` mapping.

```bash
docker run --rm -v "$PWD:/app" -v go-mod-cache:/go/pkg/mod -w /app \
  -e AMFA_TEST_DSN="host=host.docker.internal port=5433 dbname=adaptive_mfa user=keycloak password=password sslmode=disable" \
  golang:1.25.14-alpine sh -c "apk add --no-cache git build-base >/dev/null 2>&1 && \
    go test -tags=integration ./amfa/postgres/ ./amfacheck/"
```

These confirm the four new query methods are valid against the real `auth_event` / `auth_process` / `auth_context` schema and that one checker tick runs cleanly. They assert only that queries execute and return sane shapes (the seed data varies), so they are safe on any AMFA DB.

**3. Live end-to-end against the running stack.** This proves the fx graph assembles, the poll loop runs, and alerts land in KMT's DB. It injects fresh AMFA rows, so do it only against a local/dev AMFA database and clean up afterward.

Step 1: enable the checker with a short interval and low thresholds (under `keycloak.global` in `config.yaml`), then rebuild and restart KMT:

```yaml
    amfa_checker:
      enabled: true
      poll_interval: 30s
      checks:
        risk_rejected:      { enabled: true }
        repeated_risky:     { enabled: true, threshold: 3, window: 24h, min_risk_level: 2 }
        vpn_risky:          { enabled: true }
        login_error:        { enabled: true, threshold: 3, window: 10m }
        client_login_error: { enabled: true, threshold: 3, window: 10m }
```

```bash
docker compose -f deployments/local/docker-compose.yml build api-server
docker compose -f deployments/local/docker-compose.yml up -d --force-recreate api-server
docker logs kmt-server 2>&1 | grep -i amfacheck    # expect "amfacheck service starting ... num_checks=5"
```

Step 2: inject fresh events that trigger all five rules for a realm KMT has discovered (e.g. `AdaptiveAuth`). Insert via AMFA's superuser, not the read-only role. `auth_process` has NOT NULL + FK columns (`auth_context_hash`, `device_info_hash`, `network_location_hash`, `parameters_config_id`), so reuse values from an existing row:

```bash
# grab reusable FK-satisfying values from any existing auth_process
docker exec amfa-postgres-1 psql -U keycloak -d adaptive_mfa -x -c \
  "SELECT auth_context_hash, device_info_hash, network_location_hash, parameters_config_id FROM auth_process LIMIT 1"
```

Then insert recent (`NOW()`) `auth_process` rows with `auth_context_json='{"realm_id":"AdaptiveAuth"}'` and the desired `pre_auth_risk_decision` / `event_type`, plus matching `auth_event` rows. Trigger coverage: two `pre_auth_risk_decision=4` `LOGIN_ERROR` events (rule 1), two `is_vpn`-context `LOGIN` events at risk ≥ 3 (rule 3, set the event's `auth_context_hash` to a context where `is_vpn=true`), one user with three risk ≥ 2 events within 24h (rule 2), ≥ 3 `LOGIN_ERROR` and ≥ 3 `CLIENT_LOGIN_ERROR` in the current window (rules 4a/4b). Tag injected `auth_event` rows with a recognizable `details` marker and use an all-zeros UUID prefix so cleanup is trivial.

Step 3: wait one poll interval and confirm the alerts:

```bash
docker exec kmt-postgres psql -U monitoring -d monitoring -c \
  "SELECT check_type, severity, status, realm_name, count(*) \
   FROM configuration_alerts WHERE check_type LIKE 'amfa-%' GROUP BY 1,2,3,4 ORDER BY 1"
```

Expect `amfa-risk-rejected` (critical) plus `amfa-vpn-risky`, `amfa-repeated-risky`, `amfa-login-error`, `amfa-client-login-error` (warning).

Step 4: clean up and restore:

```bash
# remove injected AMFA rows (adjust marker/prefix to what you used)
docker exec amfa-postgres-1 psql -U keycloak -d adaptive_mfa -c \
  "DELETE FROM auth_event WHERE details='AMFACHECK_LIVE_TEST'; \
   DELETE FROM auth_process WHERE id::text LIKE '00000000-0000-0000-0000-%';"
# remove the test alerts from KMT
docker exec kmt-postgres psql -U monitoring -d monitoring -c \
  "DELETE FROM configuration_alerts WHERE check_type LIKE 'amfa-%'"
# revert config.yaml (set amfa_checker.enabled: false or remove the block) and restart
docker compose -f deployments/local/docker-compose.yml up -d --force-recreate api-server
```

> Disk note: rebuilding the server image generates a large Docker build cache. If a build or a database container starts failing with "No space left on device", reclaim it with `docker builder prune -af`.
