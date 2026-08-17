# AMFA "Reload Alerts" Button - Design Spec

| | |
|---|---|
| **Date** | 2026-06-03 |
| **Author** | Jonas Figu (with Claude) |
| **Status** | Draft - awaiting user review |
| **Scope** | Add an on-demand way to run the AMFA risk-alert checker from the Alerts page, via a dedicated "Reload AMFA alerts" button. Builds on the AMFA Risk Alerts feature (2026-06-02). |
| **Touches** | Keycloak Monitoring Tool only. AMFA codebase is **not** modified. No pre-existing alerts logic is changed; only additive code. |

---

## 1. Goal

The AMFA risk-alert checker (`amfacheck/`) runs on a per-tenant timer (`poll_interval`, default 1m). Operators currently must wait up to a full interval for new AMFA alerts to appear. This spec adds a button on the Alerts page that triggers the AMFA checker **immediately** for the current tenant, then refreshes the list - collapsing "wait for the next poll tick, then refresh" into one on-demand action.

The button reuses the checker's existing `RunCheckNow` path, so it produces exactly the row changes a scheduled tick would produce, evaluated at click time.

## 2. Non-goals (YAGNI)

- **Running other checkers.** The button triggers the AMFA checker only, not the config/event/metric checkers.
- **Overloading the existing Refresh button.** Refresh stays a read-only refetch. This is a distinct, explicitly-labeled button.
- **Auto-resolving stale AMFA alerts.** The checker is additive (create/update only); it never resolves. Alerts that no longer apply remain `active` until handled through the normal lifecycle. Auto-resolution is a separate, larger feature.
- **Asynchronous job/queue.** The run is synchronous (AMFA queries are millisecond-scale); no background job, progress stream, or polling-for-completion.
- **Per-check result counts in the response.** The refetched alert list is the feedback. Returning per-rule counts is a possible later enhancement.
- **Changing notification behavior.** Manual runs notify exactly like polled runs.

## 3. Decisions made during brainstorming

| Decision | Choice | Rationale |
|---|---|---|
| UX model | A **separate** "Reload AMFA alerts" button, not overloading Refresh | Refresh is also auto-triggered (React Query focus/interval refetches); making it run checks would cause unintended AMFA queries, alert writes, and notifications. A distinct button keeps viewing cheap and makes running an explicit, permission-gated action. |
| Scope | AMFA checker only, current tenant | That is what operators want here; AMFA queries are fast, so synchronous is fine. "Run all checkers" is a bigger surface (Keycloak round-trips) and out of scope. |
| Notifications | Same as polling | The button runs the identical `runChecks`/`executeCheck` path, so new alerts notify and recurrences dedupe (no re-notify within an hour, status preserved). |
| Stale alerts | Not cleared | Checker is additive, consistent with every other checker. |
| Button visibility | Hidden unless the AMFA checker is enabled for the tenant | A button that errors on click is poor UX. Visibility is driven by a capability endpoint that reflects the true state (both `amfa.enabled` for the tenant **and** the global `amfa_checker.enabled` switch). |
| Execution model | Synchronous, then refetch | AMFA checks complete in milliseconds; a synchronous request bounded by a timeout is simpler than async. |

## 4. Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│  Web (Alerts page)                                                 │
│   AlertsPage.tsx                                                   │
│     - GET  /tenants/{id}/amfa-checker   -> { enabled }  (capability)│
│     - render "Reload AMFA alerts" button only when enabled         │
│     - onClick: POST /tenants/{id}/amfa-checker/run                 │
│                then refetchAlerts() + refetch stats                │
│                snackbar on success / error                         │
└───────────────────────────────┬────────────────────────────────────┘
                                 │ HTTP (chi)
                                 ▼
┌──────────────────────────────────────────────────────────────────┐
│  KMT API server                                                  │
│   NEW endpoints (AMFA checker handlers):                           │
│     GET  /tenants/{id}/amfa-checker      -> { enabled: bool }      │
│     POST /tenants/{id}/amfa-checker/run  -> { status: "ok" }       │
│        (RBAC-gated; tenant-resolved; bounded by a context timeout) │
│                                                                    │
│   NEW: amfacheck.Runner                                            │
│     map[tenantID]amfacheck.Service                                 │
│       Enabled(tenantID) bool                                       │
│       RunNow(ctx, tenantID) error  // ErrCheckerNotEnabled if none │
│                                                                    │
│   amfacheck.Service.RunCheckNow(ctx)   (EXISTING, unchanged)       │
│     -> runChecks -> executeCheck -> alerts.Repository.Save         │
│                                   -> notifications.Service.Notify  │
└──────────────────────────────────────────────────────────────────┘
```

**Key boundary:** the only new behavior is *triggering* the existing checker on demand and *exposing its enabled-state*. Persisting alerts, deduping, notifying, the alert lifecycle, the `GET /alerts` read path, and the Refresh button are all unchanged.

## 5. Backend design

### 5.1 `amfacheck.Runner` (new)

A small lookup so a handler can reach a specific tenant's checker:

```go
// Runner exposes the per-tenant AMFA checker services for on-demand triggering.
type Runner struct {
    services map[string]Service // tenantID -> Service
}

// ErrCheckerNotEnabled is returned when no AMFA checker is running for a tenant
// (the tenant has amfa.enabled=false, or the global amfa_checker switch is off).
var ErrCheckerNotEnabled = errors.New("amfacheck: checker not enabled for tenant")

func NewRunner(services map[string]Service) *Runner { ... }

// Enabled reports whether a checker is running for the tenant.
func (r *Runner) Enabled(tenantID string) bool { ... }

// RunNow runs all of the tenant's checks once, synchronously, returning
// ErrCheckerNotEnabled if the tenant has no checker.
func (r *Runner) RunNow(ctx context.Context, tenantID string) error { ... }
```

`RunNow` delegates to the tenant's `Service.RunCheckNow(ctx)` - the **same** method and code path the ticker uses. No new check logic.

### 5.2 fx wiring (`internal/fx/amfacheck.go`)

`buildAmfaCheckServices` already iterates AMFA-enabled tenants and constructs one `Service` each. Change it to also collect them into a `map[string]Service` and build a `*amfacheck.Runner`, which is provided to the graph and injected into the handler. The lifecycle hooks (`startAmfaCheckServices`) continue to start/stop each service exactly as today. When the checker is globally disabled, the map is empty and the Runner reports `Enabled == false` for every tenant.

This is additive to the feature's own module; it does not change polling.

### 5.3 Endpoints

Registered alongside the existing AMFA routes (same auth + tenant-resolution middleware the AMFA Events endpoints use).

**`GET /tenants/{tenantId}/amfa-checker`** (capability)
- RBAC: `amfa:read` (same as viewing AMFA data - it only reveals a boolean).
- Resolves tenant; returns `200 { "enabled": true|false }` from `Runner.Enabled(tenantID)`.

**`POST /tenants/{tenantId}/amfa-checker/run`** (trigger)
- RBAC: `amfa:read` for now. (The action does cause alert writes + notifications, so a write/operator-level permission would be more correct; tightening it is a deliberate future enhancement, kept out of scope here to avoid adding a new permission + seeding migration.)
- Resolves tenant; wraps the call in a context timeout (e.g. 30s) so a slow/unreachable AMFA DB cannot hang the request.
- Calls `Runner.RunNow(ctx, tenantID)`:
  - success → `200 { "status": "ok" }`
  - `ErrCheckerNotEnabled` → `409 { "error": "amfa_checker_not_enabled" }`
  - context deadline/cancel → `503 { "error": "amfa_unavailable" }`

### 5.4 Error handling and semantics

- `RunCheckNow` is best-effort, mirroring the ticker: a per-check query failure is logged server-side and the check is skipped that run (it does not fail the whole request). This matches the "works the same as polling" decision. Consequently a successful `200` means "the run completed", not "every check succeeded"; if AMFA was unreachable, the logs show it and no new alerts appear. (Surfacing per-check errors/counts in the response is a documented future enhancement.)
- Idempotency: alerts are keyed by deterministic `AlertID` and saved with `ON CONFLICT` upsert, so a manual run overlapping a scheduled tick cannot create duplicate rows.
- Concurrency (optional tidiness): a `sync.Mutex` in the `Service` may serialize `runChecks` so a manual run and a tick don't execute simultaneously. Not required for correctness (dedup + upsert already make overlap safe); included only to avoid wasted duplicate work.

## 6. Frontend design

### 6.1 Capability query + button visibility

On the Alerts page, add a query for `GET /tenants/{tenantId}/amfa-checker` (cached via React Query, keyed by tenant). Render the **"Reload AMFA alerts"** button only when `enabled === true`. For tenants without the AMFA checker, the button is not shown at all.

### 6.2 Button behavior

- Placement: directly next to the existing **Refresh** button in the alerts toolbar.
- `onClick`:
  1. set a local `running` state (button shows a spinner and is disabled),
  2. `await alertService.reloadAmfaAlerts(tenantId)` (`POST .../amfa-checker/run`),
  3. on success: call the existing `refetchAlerts()` and the stats refetch, then show a success snackbar ("AMFA alerts reloaded"),
  4. on error: show an error snackbar (`409` → "AMFA checker isn't enabled for this tenant"; otherwise "Couldn't reload AMFA alerts, try again"),
  5. clear `running` in a `finally`.
- The existing **Refresh** button and the alerts list/query are unchanged.

### 6.3 Service method (`alertService.ts`)

Add `reloadAmfaAlerts(tenantId)` → `apiClient.post(\`/tenants/${tenantId}/amfa-checker/run\`)` and `getAmfaCheckerStatus(tenantId)` → `apiClient.get(\`/tenants/${tenantId}/amfa-checker\`)`.

## 7. Behavior on existing rows (clarified during brainstorming)

- **Non-AMFA alerts** (config/event/metric): untouched by the trigger; after the refetch they re-render exactly as before.
- **Existing AMFA alerts**: re-detected ones update `LastSeen` (same `AlertID`, dedup), keep their current status, and do not re-notify within the hour. They are not removed.
- **New AMFA alerts**: created and shown after the refetch; notify per normal rules.
- **Filters/pagination/sort**: preserved (refetch reuses current query params).

## 8. Reliability

- The triggered work is the **same `runChecks` path already validated end-to-end** against the live stack (all five rules created/updated/persisted, dedup and notifications correct). The button rides proven code.
- New surface is thin: the `Runner` lookup, two endpoints, one button + two service methods.
- Bounded by a request timeout; concurrency is safe via deterministic `AlertID` + upsert.

## 9. Testing strategy

**Backend (stdlib `testing`, no testify):**
- `amfacheck.Runner` unit test: `RunNow` dispatches to the correct tenant's service; returns `ErrCheckerNotEnabled` for an unknown/disabled tenant; `Enabled` reflects presence.
- Handler tests: `GET` returns the enabled boolean; `POST` calls `RunNow` and returns `200`; `409` when not enabled; RBAC enforced; tenant resolution.

**Frontend (vitest):**
- `AlertsPage`: button renders only when the capability query returns `enabled: true`; hidden otherwise; click calls `reloadAmfaAlerts` then `refetchAlerts`; disabled/spinner while running; error snackbar on `409`.
- `alertService`: the two new methods hit the right paths.

**Manual smoke:** on a tenant with the AMFA checker enabled, click "Reload AMFA alerts" and confirm new AMFA alerts appear immediately (vs waiting for the poll interval), non-AMFA rows are unchanged, and repeated clicks don't create duplicates.

## 10. Files changed (summary)

**New:**
```
amfacheck/runner.go                         + runner_test.go
internal/http/chi/amfacheck_handlers.go     + amfacheck_handlers_test.go   (or added to amfa_handlers.go)
web/src/features/alerts/...                  capability hook + button wiring (+ tests)
```

**Modified (additive):**
```
internal/fx/amfacheck.go        build + provide *amfacheck.Runner; inject into handler
internal/fx/handlers.go (or amfa handler params)   wire the new handler/route
web/src/features/alerts/pages/AlertsPage.tsx       render the button + onClick
web/src/features/alerts/components/AlertsFilters.tsx (if button lives there)
web/src/features/alerts/services/alertService.ts   + reloadAmfaAlerts, getAmfaCheckerStatus
docs/amfa-integration.md        document the button + capability endpoint
```

(No RBAC changes: both endpoints reuse the existing `amfa:read` permission.)

**Not modified:** the `alerts/` package, the alert lifecycle, the `configuration_alerts` schema, the `GET /alerts` read path, the existing Refresh button, the `amfacheck` check/service logic (only the fx provider is extended to expose the Runner).

## 11. Risks and edge cases

| Risk / edge case | Handling |
|---|---|
| AMFA DB slow/unreachable on click | Request bounded by a context timeout → `503`; per-check errors logged like polling; no partial UI breakage. |
| Manual run overlaps a scheduled tick | Safe via deterministic `AlertID` + upsert; optional mutex to avoid duplicate work. |
| Global `amfa_checker` disabled | Runner empty → `Enabled=false` everywhere → button hidden; `POST` returns `409`. |
| Tenant has `amfa.enabled` but global switch off | No service for tenant → `Enabled=false` → button hidden (correct). |
| User double-clicks | Button disabled while `running`; idempotent anyway. |
| Success but AMFA was down (no new alerts) | `200` means "run completed" (best-effort, mirrors polling); documented. Future enhancement: return per-check status/counts. |

## 12. Future enhancements (out of scope)

- Return per-rule counts (created/updated/errored) and surface them in the snackbar.
- A "Run all checkers now" action covering config/event/metric checkers.
- Auto-resolution of AMFA alerts whose conditions no longer hold.
- Tighten the trigger's RBAC from `amfa:read` to a write/operator-level permission (the action causes alert writes + notifications).
