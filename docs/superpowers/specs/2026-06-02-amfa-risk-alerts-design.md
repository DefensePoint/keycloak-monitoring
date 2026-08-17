# AMFA Risk Alerts — Design Spec

| | |
|---|---|
| **Date** | 2026-06-02 |
| **Author** | Jonas Figu (with Claude) |
| **Status** | Draft — awaiting user review |
| **Scope** | Add automatic POINT alerts triggered by high-risk AMFA events. Builds on the existing AMFA Events feature (2026-05-26) and POINT's existing `alerts/` + `notifications/` infrastructure. |
| **Touches** | POINT Monitoring Platform only. AMFA codebase is **not** modified. Pre-existing POINT logic is **not** changed; only additive code. |

---

## 1. Goal

POINT Monitoring already surfaces AMFA login activity in a dashboard page (risky KPI, events table, geolocation map). Operators currently have to *look* at the page to notice problems. This spec adds a background service that **automatically converts high-risk AMFA events into POINT alerts**, which then flow through the existing alert lifecycle (active → acknowledged → resolved → ignored) and notification fan-out (Slack / Email / GitLab) — making AMFA risk signals first-class alerting citizens alongside Keycloak configuration alerts.

## 2. Non-goals

- **Modifying AMFA.** AMFA's codebase is read-only from POINT's perspective.
- **Activating the dormant `eventscheck/` package.** It contains `LoginErrorCheck`, `ClientErrorCheck`, etc. but is not wired into runtime today. Activating it is a separate decision tracked elsewhere; this spec deliberately mirrors its shape in a sibling `amfacheck/` package rather than touching it.
- **Persistent checkpoint state.** First MVP keeps the "last-seen event time" per-Check in memory only. Lost on restart, recovered by re-scanning the most recent poll window; per-event AlertID dedup absorbs the replay.
- **Per-tenant config of thresholds.** Thresholds are global (under `keycloak.global.amfa_checker`), matching the existing `event_checker` shape. Per-tenant overrides are a future enhancement.
- **Real-time/push alerting** from AMFA. The service polls.
- **A new UI page.** Alerts surface in the existing Alerts page; the AMFA Events page is unchanged.

## 3. Decisions made during brainstorming

| Decision | Choice | Rationale |
|---|---|---|
| Alert triggers (the rules) | 4 rules: (1) `risk_level=4` per-event, (2) `≥3 attempts/day with risk≥2` per user, (3) `is_vpn=true AND risk>2` per-event, (4) threshold checks on AMFA's `LOGIN_ERROR` and `CLIENT_LOGIN_ERROR` event types | Covers high-confidence signals (rule 1), pattern-detection (rule 2), suspicious-context (rule 3), and bursts (rule 4). Decided by user during brainstorming. |
| Per-event dedup for rules 1, 3 | One alert row per AMFA event | User chose this over per-user-per-day grouping — wants full visibility into every risky event |
| Per-(user, day) dedup for rule 2 | One alert per (rule, tenant, realm, user, UTC date) | Already a "this user had a pattern today" signal; per-event would be too noisy |
| Per-realm dedup for rules 4a, 4b | One alert per (rule, tenant, realm), counted over a rolling `[now-window, now]` window | Stable per-realm AlertID dedups adjacent polls and manual triggers; matches `eventscheck`. (Originally a window-aligned bucket; changed 2026-06-11 - see "Window handling for rules 4a/4b".) |
| Severity mapping | Rule 1 → critical, rules 2/3/4a/4b → warning | Rule 1 is the only "auth was actually denied" signal; everything else is suspicion. Aligns with typical channel thresholds (Slack: warning+, Email: error+, GitLab: critical only). |
| Config location | Global under `keycloak.global.amfa_checker` | Mirrors the existing `event_checker` pattern; operators already know that layout |
| Rule 4 implementation scope | Build sibling `amfacheck/` package | The dormant `eventscheck/` checks are not running in production; reusing them would require activating dormant code, which carries unknown risk |
| Integration model | Per-tenant `amfacheck.Service` instance, one Check goroutine, mirrors `eventscheck.Service` shape | Matches the existing per-tenant pattern AMFA already uses (per-tenant Repository in `amfa.Registry`, per-tenant Keycloak admin client) |

## 4. Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│  POINT API server (cmd/server, Go)                                │
│                                                                  │
│  Existing infrastructure used as-is:                             │
│    amfa.Registry          — tenantID → *amfa/postgres.Repository │
│    alerts.Repository      — save / load / lifecycle              │
│    notifications.Service  — Slack / Email / GitLab fan-out       │
│    domain.Alert           — unified alert struct (already         │
│                             supports AlertSource = "event")      │
│                                                                  │
│  NEW (this design):                                              │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  amfacheck/                                                │  │
│  │   service.go             per-tenant poll loop              │  │
│  │   interfaces.go          Check, AlertStore, Notifier       │  │
│  │   risk_rejected_check.go     (rule 1, per-event)           │  │
│  │   repeated_risky_check.go    (rule 2, per-user/day)        │  │
│  │   vpn_risky_check.go         (rule 3, per-event)           │  │
│  │   login_error_check.go       (rule 4a, threshold/window)   │  │
│  │   client_login_error_check.go (rule 4b)                    │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  amfa/postgres/repository.go: + 4 new query methods (additive)  │
│  internal/config/models.go : + AmfaCheckerConfig structs         │
│  internal/fx/amfacheck.go  : + module + per-tenant Service wiring│
│  internal/fx/app.go        : + AmfaCheckModule import line       │
└──────────────────────────────────────────────────────────────────┘
                              │
                              │ Reads per-tenant
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│  AMFA's PostgreSQL (existing, read-only via point_monitoring_ro) │
└──────────────────────────────────────────────────────────────────┘
                              │
                              │ Writes alerts (via alerts.Repository)
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│  POINT's PostgreSQL — alerts table (existing schema, unchanged)  │
└──────────────────────────────────────────────────────────────────┘
                              │
                              │ Notifications via existing fan-out
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│  Slack / Email / GitLab (existing per-channel severity gates)    │
└──────────────────────────────────────────────────────────────────┘
```

**Key boundaries**

- `amfacheck/` depends only on: `amfa.Registry` (for repositories), `amfacheck.AlertStore` interface (satisfied by `alerts.Repository`), `amfacheck.Notifier` interface (satisfied by `notifications.Service`), `domain.Alert`. No direct UI or HTTP code.
- Per-tenant lifecycle: one `amfacheck.Service` instance per tenant whose `amfa.enabled == true` AND `keycloak.global.amfa_checker.enabled == true`. Each owns one goroutine.
- New repository methods are additive to `amfa/postgres/repository.go` and the `amfa.Repository` interface; no existing method signatures change.

## 5. Backend design

### 5.1 Configuration (`internal/config/models.go`)

New structs nested under `keycloak.global.amfa_checker`. Mirrors the existing `EventCheckerConfig` shape so operators see a familiar layout.

```go
type AmfaCheckerConfig struct {
    Enabled      bool                    `mapstructure:"enabled"`        // master switch; default false
    PollInterval time.Duration           `mapstructure:"poll_interval"`  // default 1m
    Checks       AmfaCheckerChecksConfig `mapstructure:"checks"`
}

type AmfaCheckerChecksConfig struct {
    RiskRejected     PerEventCheckConfig  `mapstructure:"risk_rejected"`        // rule 1
    RepeatedRisky    RepeatedRiskyConfig  `mapstructure:"repeated_risky"`       // rule 2
    VPNRisky         PerEventCheckConfig  `mapstructure:"vpn_risky"`            // rule 3
    LoginError       ThresholdCheckConfig `mapstructure:"login_error"`          // rule 4a
    ClientLoginError ThresholdCheckConfig `mapstructure:"client_login_error"`   // rule 4b
}

type PerEventCheckConfig struct {
    Enabled bool `mapstructure:"enabled"`
}

type RepeatedRiskyConfig struct {
    Enabled      bool          `mapstructure:"enabled"`
    Threshold    int           `mapstructure:"threshold"`      // default 3
    Window       time.Duration `mapstructure:"window"`         // default 24h
    MinRiskLevel int           `mapstructure:"min_risk_level"` // default 2
}

type ThresholdCheckConfig struct {
    Enabled   bool          `mapstructure:"enabled"`
    Threshold int           `mapstructure:"threshold"`  // default 5
    Window    time.Duration `mapstructure:"window"`     // default 5m
}

func (c *AmfaCheckerConfig) ApplyDefaults() {
    if c.PollInterval <= 0 {
        c.PollInterval = time.Minute
    }
    if c.Checks.RepeatedRisky.Threshold <= 0     { c.Checks.RepeatedRisky.Threshold = 3 }
    if c.Checks.RepeatedRisky.Window    <= 0     { c.Checks.RepeatedRisky.Window = 24 * time.Hour }
    if c.Checks.RepeatedRisky.MinRiskLevel <= 0  { c.Checks.RepeatedRisky.MinRiskLevel = 2 }
    if c.Checks.LoginError.Threshold       <= 0  { c.Checks.LoginError.Threshold = 5 }
    if c.Checks.LoginError.Window          <= 0  { c.Checks.LoginError.Window = 5 * time.Minute }
    if c.Checks.ClientLoginError.Threshold <= 0  { c.Checks.ClientLoginError.Threshold = 5 }
    if c.Checks.ClientLoginError.Window    <= 0  { c.Checks.ClientLoginError.Window = 5 * time.Minute }
}
```

`AmfaCheckerConfig` is added as a new field on the existing `KeycloakGlobalConfig` struct (or wherever `EventChecker` currently sits). Disabled by default — operators opt in explicitly.

Example `config.yaml`:

```yaml
keycloak:
  global:
    amfa_checker:
      enabled: true
      poll_interval: 1m
      checks:
        risk_rejected:      { enabled: true }
        repeated_risky:     { enabled: true, threshold: 3, window: 24h, min_risk_level: 2 }
        vpn_risky:          { enabled: true }
        login_error:        { enabled: true, threshold: 5, window: 5m }
        client_login_error: { enabled: true, threshold: 5, window: 5m }
```

### 5.2 New repository methods (`amfa/interfaces.go` + `amfa/postgres/repository.go`)

Additive to the existing `amfa.Repository` interface — existing method signatures are not changed.

```go
type Repository interface {
    // ... existing methods unchanged ...

    // ListRejectedEventsSince returns events with pre_auth_risk_decision=4 newer
    // than `since`, for the named realm. Used by rule 1 (RiskRejectedCheck).
    ListRejectedEventsSince(ctx context.Context, realm string, since time.Time) ([]EventRow, error)

    // ListVPNRiskyEventsSince returns events with is_vpn=true AND
    // risk_level >= minRisk (inclusive bound) newer than `since`, for the
    // named realm. Used by rule 3 (VPNRiskyCheck). Callers pass minRisk=3
    // for the brainstormed "risk > 2" semantics.
    ListVPNRiskyEventsSince(ctx context.Context, realm string, since time.Time, minRisk int) ([]EventRow, error)

    // CountRepeatedRiskyByUser returns users (within realm) whose event_count
    // satisfies risk_level >= minRisk (inclusive bound) in [since, now),
    // grouped by user_id. Only rows where count >= threshold are returned.
    // Used by rule 2. Callers pass minRisk=2 for the brainstormed
    // "risk_level >= 2" semantics.
    CountRepeatedRiskyByUser(ctx context.Context, realm string, since time.Time, minRisk, threshold int) ([]UserRiskyCount, error)

    // CountByEventTypeInWindow returns the count of events of the given type
    // in [start, end] for the realm. Used by rules 4a, 4b.
    CountByEventTypeInWindow(ctx context.Context, realm, eventType string, start, end time.Time) (int64, error)
}

// UserRiskyCount is the row shape returned by CountRepeatedRiskyByUser.
type UserRiskyCount struct {
    UserID string
    Count  int64
}
```

All four methods follow the same SQL patterns as the existing `ListEvents` / `GetStats` queries — same realm-filter via `auth_context_json->>'realm_id'`, same LEFT JOIN topology (acknowledging the documented INNER-equivalent behavior from the prior code review), same `classifyDBError` wrapping at the boundary.

### 5.3 amfacheck package

#### `amfacheck/interfaces.go`

```go
// Check is implemented by each of the 5 rules. Mirrors eventscheck.Check
// exactly so the Service can be a near-copy.
type Check interface {
    Type() string                                     // unique identifier, e.g. "amfa-risk-rejected"
    Description() string
    Execute(ctx context.Context) ([]*domain.Alert, error)
}

// AlertStore is the slice of alerts.Repository this service needs.
type AlertStore interface {
    GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
    SaveAlert(ctx context.Context, alert *domain.Alert) error
}

// Notifier is the slice of notifications.Service this service needs.
type Notifier interface {
    NotifyAlert(ctx context.Context, alert *domain.Alert) error
}
```

#### `amfacheck/service.go`

Near-verbatim copy of `eventscheck.Service` — the poll loop, dedup, save, notify orchestration logic is identical:

- `NewService(tenantID string, checks []Check, alertStore AlertStore, notifier Notifier, log Logger, pollInterval time.Duration) Service`
- `Start(ctx) error` — initial tick + ticker goroutine
- `Stop() error` — close stopChan, 10s graceful drain
- Internal `executeCheck(ctx, check)` — runs check, dedupes via `GetAlertByID`, calls `SaveAlert` + `NotifyAlert` per the rules in Section 3 of the lifecycle design

We deliberately copy rather than abstract a shared base because the only differences are imports and check construction; sharing would require a third package neither owns.

#### Check implementations

Each check file is small (~70 LoC) and follows the same shape:

```go
// risk_rejected_check.go (rule 1)
type RiskRejectedCheck struct {
    tenantID string
    // realmsFn returns the current realm list for this tenant. Called
    // every tick, so realms added/removed in Keycloak are picked up
    // automatically without restart. Backed by keycloak.Repository.GetRealms.
    realmsFn func(ctx context.Context) ([]string, error)
    repo     amfa.Repository
    cfg      *config.PerEventCheckConfig
    log      Logger
    mu       sync.Mutex
    lastSeen map[string]time.Time // per-realm checkpoint
}

func (c *RiskRejectedCheck) Type() string        { return "amfa-risk-rejected" }
func (c *RiskRejectedCheck) Description() string { return "AMFA login rejected by risk engine (ACR 4)" }

func (c *RiskRejectedCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
    if !c.cfg.Enabled {
        return nil, nil
    }
    realms, err := c.realmsFn(ctx)
    if err != nil {
        return nil, fmt.Errorf("risk_rejected: load realms: %w", err)
    }
    var alerts []*domain.Alert
    for _, realm := range realms {
        since := c.getLastSeen(realm)
        events, err := c.repo.ListRejectedEventsSince(ctx, realm, since)
        if err != nil {
            // Repository already wraps connection errors as ErrAmfaUnavailable.
            return nil, fmt.Errorf("risk_rejected: realm=%s: %w", realm, err)
        }
        for _, ev := range events {
            alerts = append(alerts, c.toAlert(realm, ev))
            if ev.EventTime.After(since) {
                since = ev.EventTime
            }
        }
        c.setLastSeen(realm, since)
    }
    return alerts, nil
}

func (c *RiskRejectedCheck) toAlert(realm string, ev amfa.EventRow) *domain.Alert {
    aid := sha256Hex(fmt.Sprintf("amfa-risk-rejected:%s:%s", c.tenantID, ev.EventID))
    md, _ := json.Marshal(map[string]any{
        "amfa_event_id": ev.EventID,
        "user_id":       ev.UserID,
        "ip_address":    ev.IPAddress,
        "client":        ev.Client,
        "country":       ev.Country,
        "is_vpn":        ev.IsVPN,
        "risk_level":    ev.RiskLevel,
    })
    return &domain.Alert{
        TenantID:       c.tenantID,
        AlertID:        aid,
        Source:         domain.AlertSourceEvent,
        Type:           domain.AlertTypeSecurity,
        Severity:       domain.AlertSeverityCritical,
        Status:         domain.AlertStatusActive,
        Title:          "AMFA: login rejected by risk engine",
        Description:    fmt.Sprintf("AMFA rejected an authentication attempt in realm '%s' (event %s).", realm, ev.EventID),
        ResourceType:   "amfa_event",
        ResourceID:     ev.EventID,
        ResourceName:   ev.EventID,
        RealmName:      realm,
        CheckType:      c.Type(),
        EventID:        &ev.EventID,
        Recommendation: "Investigate the rejected login attempt. Review the user's session history and IP reputation. Consider whether the user should be temporarily blocked.",
        Metadata:       string(md),
    }
}
```

The other four checks follow the same skeleton:

- **`repeated_risky_check.go`** (rule 2): calls `CountRepeatedRiskyByUser`, produces one alert per user that crossed the threshold today; `AlertID = sha256("amfa-repeated-risky:" + tenantID + ":" + realm + ":" + userID + ":" + utcDate)`. Severity = warning. Metadata includes count and threshold.
- **`vpn_risky_check.go`** (rule 3): calls `ListVPNRiskyEventsSince(realm, since, minRisk=3)`. The `minRisk` parameter is the inclusive lower bound (SQL: `risk_level >= 3`), matching the brainstorming rule "risk > 2". Per-event alert keyed by event_id. Severity = warning. The bound is currently hardcoded; if it becomes useful to tune separately from rule 2's `min_risk_level`, add a `MinRiskLevel int` field to `PerEventCheckConfig` (or split rule 3 into its own config struct).
- **`login_error_check.go`** (rule 4a): calls `CountByEventTypeInWindow(LOGIN_ERROR, now-window, now)`. Alert when count >= threshold. `AlertID = sha256("amfa-login-error:" + tenantID + ":" + realm)`. Severity = warning. Title: `"AMFA: excessive LOGIN_ERROR events"`.
- **`client_login_error_check.go`** (rule 4b): identical to 4a but `eventType = "CLIENT_LOGIN_ERROR"`. Different AlertID prefix.

#### Window handling for rules 4a/4b (updated 2026-06-11)

Rules 4a/4b count their event type over a **rolling** `[now - window, now]` window (the same rolling style rule 2 uses), and the `AlertID` is **stable per `(realm, eventType)`** (no time component). Repeated polls and manual runs therefore dedup to one alert row (the Service updates `LastSeen` on recurrence rather than creating a new row), which is exactly how the existing `eventscheck` error-threshold alerts behave.

> **Superseded design.** The original spec used a fixed window-aligned bucket (`windowBucket(now) = now.Truncate(window)`) folded into the `AlertID`, to dedup adjacent polls. That was changed during testing because it made the on-demand "Reload AMFA alerts" trigger unreliable: a burst that landed just before a bucket boundary was missed if the click occurred after the boundary (and a burst straddling a boundary was split across two sub-threshold buckets). A rolling window always covers the last `window` regardless of clock alignment, so manual triggers and polls reliably catch recent bursts; the stable `AlertID` provides the dedup the bucket used to provide.

### 5.4 fx wiring (`internal/fx/amfacheck.go`)

```go
package fx

var AmfaCheckModule = fx.Module("amfacheck",
    fx.Provide(provideAmfaCheckServices),
    fx.Invoke(startAmfaCheckServices),
)

// provideAmfaCheckServices builds one *amfacheck.Service per AMFA-enabled tenant.
// Returns a slice; downstream fx.Invoke starts/stops each per lifecycle.
func provideAmfaCheckServices(
    cfg *config.AppConfig,
    reg *amfa.Registry,
    alertStore alerts.Repository,
    notifier notifications.Service,
    realmRepo keycloak.Repository,  // exposes GetRealms(ctx, tenantID) — already in the fx graph
    log *logger.Logger,
) []*amfacheck.Service {
    amfaCfg := cfg.Keycloak.Global.AmfaChecker
    if !amfaCfg.Enabled {
        log.Info("AMFA checker globally disabled; no alert services constructed")
        return nil
    }
    amfaCfg.ApplyDefaults()

    var services []*amfacheck.Service
    for tenantID, tcfg := range cfg.Keycloak.Tenants {
        if tcfg.Amfa == nil || !tcfg.Amfa.Enabled {
            continue
        }
        repo, err := reg.RepositoryFor(tenantID)
        if err != nil {
            log.Warn("amfacheck: skipping tenant with no Repository",
                logger.Str("tenant_id", tenantID), logger.Err(err))
            continue
        }
        // realmsFn closes over the tenantID + realmRepo so each tick fetches
        // the current realm list. Picks up realms added/removed in Keycloak
        // without restart.
        tenantID := tenantID // capture
        realmsFn := func(ctx context.Context) ([]string, error) {
            rows, err := realmRepo.GetRealms(ctx, tenantID)
            if err != nil {
                return nil, err
            }
            out := make([]string, 0, len(rows))
            for _, r := range rows {
                out = append(out, r.RealmName)
            }
            return out, nil
        }
        checks := buildChecks(tenantID, realmsFn, repo, amfaCfg.Checks, log)
        svc := amfacheck.NewService(tenantID, checks, alertStore, notifier, log, amfaCfg.PollInterval)
        services = append(services, svc)
    }
    log.Info("AMFA checker services constructed", logger.Int("count", len(services)))
    return services
}

func buildChecks(tenantID string, realmsFn func(context.Context) ([]string, error),
                 repo amfa.Repository, cfg config.AmfaCheckerChecksConfig,
                 log *logger.Logger) []amfacheck.Check {
    var out []amfacheck.Check
    if cfg.RiskRejected.Enabled {
        out = append(out, amfacheck.NewRiskRejectedCheck(tenantID, realmsFn, repo, &cfg.RiskRejected, log))
    }
    if cfg.RepeatedRisky.Enabled {
        out = append(out, amfacheck.NewRepeatedRiskyCheck(tenantID, realmsFn, repo, &cfg.RepeatedRisky, log))
    }
    if cfg.VPNRisky.Enabled {
        out = append(out, amfacheck.NewVPNRiskyCheck(tenantID, realmsFn, repo, &cfg.VPNRisky, log))
    }
    if cfg.LoginError.Enabled {
        out = append(out, amfacheck.NewLoginErrorCheck(tenantID, realmsFn, repo, &cfg.LoginError, log))
    }
    if cfg.ClientLoginError.Enabled {
        out = append(out, amfacheck.NewClientLoginErrorCheck(tenantID, realmsFn, repo, &cfg.ClientLoginError, log))
    }
    return out
}

func startAmfaCheckServices(lc fx.Lifecycle, services []*amfacheck.Service, log *logger.Logger) {
    for _, s := range services {
        s := s  // capture
        lc.Append(fx.Hook{
            OnStart: func(ctx context.Context) error { return s.Start(ctx) },
            OnStop:  func(ctx context.Context) error { return s.Stop() },
        })
    }
}
```

`AmfaCheckModule` is added to the module list in `internal/fx/app.go` — one new line.

## 6. Lifecycle, dedup, alert ID generation

Covered in detail in Section 3 of the brainstorming. Reproduced here for completeness:

**Per-tick flow** (mirrors `eventscheck.Service`):

```
on tick:
  for each enabled Check c:
    alerts, err := c.Execute(ctx)
    for each alert:
      existing := alertStore.GetAlertByID(ctx, tenant, alert.AlertID)
      if existing == nil:                  // new
        alert.FirstDetected = now; alert.LastSeen = now; alert.Status = Active
        alertStore.SaveAlert(alert)
        notifier.NotifyAlert(alert)
      else:                                // recurrence
        existing.LastSeen = now
        alertStore.SaveAlert(existing)
        // re-notify only if it had been Resolved+now active again, per existing
        // notifications.Service noise-suppression
```

**AlertID by rule:**

| Rule | AlertID formula |
|---|---|
| 1 | `sha256("amfa-risk-rejected:" + tenantID + ":" + eventID)` |
| 2 | `sha256("amfa-repeated-risky:" + tenantID + ":" + realm + ":" + userID + ":" + yyyy-mm-dd UTC)` |
| 3 | `sha256("amfa-vpn-risky:" + tenantID + ":" + eventID)` |
| 4a | `sha256("amfa-login-error:" + tenantID + ":" + realm)` (rolling window; stable per realm; see "Window handling for rules 4a/4b") |
| 4b | `sha256("amfa-client-login-error:" + tenantID + ":" + realm)` (rolling window; stable per realm) |

**Per-Check checkpoints** for rules 1 and 3 are in-memory (`map[realm]time.Time`) and reset on restart; first tick after restart defaults to `now - pollInterval`. The per-event AlertID dedup absorbs the small replay window that this implies.

## 7. Error handling

| Failure | Behavior |
|---|---|
| Repository returns `amfa.ErrAmfaUnavailable` | Log WARN, skip this Check this tick, retry next tick. No partial alerts. |
| Repository returns other error | Log ERROR with context, skip this Check this tick. |
| `alertStore.SaveAlert` fails | Log ERROR; do NOT notify; next tick retries via Check re-execution. |
| `notifier.NotifyAlert` fails | Log ERROR; alert still saved in POINT's DB and visible in UI; future recurrences trigger fresh notification attempts. |
| Tenant's Repository missing from registry at startup | Service for that tenant not constructed; logged WARN; other tenants unaffected. |
| Shutdown deadline exceeded | 10s timeout; in-flight queries respect ctx cancellation. |

Errors from one Check never block other Checks on the same tick or other tenants' services.

## 8. Edge cases

| Edge case | Behavior |
|---|---|
| First tick after server start (empty checkpoint) | Default to `now - pollInterval` so the first scan is bounded |
| Realm list changes mid-runtime | Each Check holds a `realmsFn` closure that calls `keycloak.Repository.GetRealms(ctx, tenantID)` on every tick — new/removed realms are picked up without restart |
| Tenant removed at runtime | Out of scope; config is read at boot only. Documented limitation. |
| 1000+ risky events in one tick | Rule 1 produces 1000+ alerts (per-event dedup means no consolidation). Operators see the flood; this is the documented intent. Operators can use the existing `alerts.SilenceManager` to suppress noisy IPs/realms. |
| AMFA / POINT DB clock skew | Both use UTC; minute-scale skew is absorbed by per-event AlertID dedup |
| Rule 2 at UTC midnight | Day boundary is UTC. 2 events at 23:50 + 1 event at 00:10 won't cross the 3-event threshold (split across two days). Acceptable; documented. |
| Check threshold = 0 in config | Treated as "disabled" (matches eventscheck behavior) |
| Empty realms list for a tenant | Service starts; every tick is no-op until realms appear |

## 9. Testing strategy

**Unit tests (per check, no DB):**

- `risk_rejected_check_test.go` — stub `Repository` returns N events; verify N `*domain.Alert` rows with correct severity (critical), AlertSource (event), AlertID shape.
- `repeated_risky_check_test.go` — stub returns user-count groupings; verify only users at/above threshold produce alerts; per-(user, day) AlertID shape.
- `vpn_risky_check_test.go` — symmetric to rule 1.
- `login_error_check_test.go`, `client_login_error_check_test.go` — given count above/below threshold, verify alert created or not; verify the rolling-window count and the stable per-realm AlertID.

**Service test (`service_test.go`):**

- Two Checks, one returns one alert, the other returns empty. Verify both run.
- Verify dedup: same AlertID twice in a row — `SaveAlert` called twice (LastSeen updated), `NotifyAlert` called once on first occurrence.
- Verify errors from one Check don't block the next.

**Integration test (`//go:build integration`):**

- Seed AMFA's test DB with known events covering all 5 rules' triggers.
- Construct `amfacheck.Service` against the real AMFA DB and POINT's test alerts repo.
- Run one tick; assert correct alert rows in POINT's DB.
- Skips with `t.Skip` when `AMFA_TEST_DSN` is missing (same pattern as existing AMFA integration tests).

**Manual smoke** (documented in `docs/amfa-integration.md`):

- After deploy, trigger one rejection via AMFA's `/decision` endpoint.
- Wait one poll interval (default 1m).
- See the alert appear on POINT's Alerts page.
- Verify notification arrived in the configured Slack channel.

## 10. Deployment

### 10.1 New env vars / config

```yaml
keycloak:
  global:
    amfa_checker:
      enabled: true              # MASTER SWITCH — off by default
      poll_interval: 1m
      checks:
        risk_rejected:      { enabled: true }
        repeated_risky:     { enabled: true, threshold: 3, window: 24h, min_risk_level: 2 }
        vpn_risky:          { enabled: true }
        login_error:        { enabled: true, threshold: 5, window: 5m }
        client_login_error: { enabled: true, threshold: 5, window: 5m }
```

### 10.2 Expected startup log lines

```
[INFO]  AMFA checker services constructed   count=N
[INFO]  amfacheck service starting           tenant_id=<id> checks=5 poll_interval=1m0s
[INFO]  amfacheck service starting           tenant_id=<id> checks=5 poll_interval=1m0s
```

If `enabled: false` (default), only one log line:

```
[INFO]  AMFA checker globally disabled; no alert services constructed
```

### 10.3 Per-tick log lines

DEBUG (high-volume; off at default INFO level):
```
[DEBUG] amfacheck: running checks     tenant_id=<id>  num_checks=5
[DEBUG] amfacheck: check completed    tenant_id=<id>  check=amfa-risk-rejected  alerts=0  duration=12ms
```

INFO when alerts are produced:
```
[INFO]  amfacheck: new alert          tenant_id=<id>  alert_id=<sha>  check=amfa-risk-rejected  severity=critical
```

### 10.4 Rollback

Set `keycloak.global.amfa_checker.enabled: false` and restart POINT. All `amfacheck.Service` instances stop; no new alerts produced. Existing alerts in POINT's DB remain visible in the UI for the operator to ack/resolve/ignore.

## 11. Files changed (summary)

**New:**
```
amfacheck/
  service.go
  service_test.go
  interfaces.go
  risk_rejected_check.go              + _test.go
  repeated_risky_check.go             + _test.go
  vpn_risky_check.go                  + _test.go
  login_error_check.go                + _test.go
  client_login_error_check.go         + _test.go
  integration_test.go                 //go:build integration
internal/fx/amfacheck.go              + _test.go
```

**Modified (additive only):**
```
amfa/interfaces.go                    + 4 new method signatures on Repository
amfa/postgres/repository.go           + 4 new query implementations
amfa/postgres/repository_integration_test.go   + tests for the new methods
internal/config/models.go             + AmfaCheckerConfig + 4 friends + nest on global
internal/fx/app.go                    + AmfaCheckModule import line
config.yaml.example                   + documented amfa_checker block
docs/amfa-integration.md              + Alerts section + troubleshooting rows
docs/architecture.md                  + brief amfacheck/ note in section 22
```

**Not modified:** existing `alerts/`, `notifications/`, `eventscheck/`, AMFA codebase.

## 12. Known follow-ups (out of scope here)

- **Per-tenant config overrides** for thresholds — operators with very different traffic profiles per tenant will want to tune independently. Today thresholds are global.
- **Persistent checkpoint** — currently in-memory, lost on restart. A small `amfacheck_checkpoint` table per (tenant, check_type, realm) would eliminate the replay window.
- **Activating the dormant `eventscheck/`** — orthogonal decision. AMFA event-type threshold checks (rules 4a, 4b) are reimplemented locally here precisely so we don't have to take that decision now.
- **Hot config reload** — config is read at boot. Changing thresholds requires a restart.
- **Per-tenant silence rule UI** — relies on the existing `alerts.SilenceManager`; no new UI work in this spec.
- **AMFA-specific alert columns** in the Alerts page UI — the existing columns already work (severity, realm, title, last seen); deeper fields (risk_level, is_vpn) are in `Metadata` JSON. If operators want columns for those, separate UI spec.

## 13. Risks acknowledged

| Risk | Mitigation |
|---|---|
| Alert flood from a misbehaving rule on first deploy | Master switch is off by default; operators opt in. Per-check enable flags allow gradual rollout. |
| In-memory checkpoint loses events on restart | First-tick window of `now - pollInterval` re-scans; per-event AlertID dedup absorbs replay. Documented limitation. |
| Threshold defaults don't match a customer's traffic profile | Defaults conservative (3/day for rule 2, 5/5min for rule 4); operators tune via config. |
| Adding load to AMFA's primary Postgres | Same read-replica path as the AMFA Events feature — `read_replica_host` config option supported. New queries are O(events in window), well-indexed if the recommended `idx_auth_process_realm_id` is in place. |
| `eventscheck/` and `amfacheck/` drift over time | Documented in code that they are deliberately parallel; a future refactor could extract the shared poll-loop into a third package once activation decisions on eventscheck are settled. |
