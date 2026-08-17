# AMFA Risk Alerts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a background service that converts high-risk AMFA login events into POINT alerts, which then flow through POINT's existing alert lifecycle and notification fan-out.

**Architecture:** A new `amfacheck/` package, modeled closely on the existing `eventscheck/` package, runs one per-tenant poll loop. Each loop executes five `Check` implementations (one per alert rule) that query a tenant's read-only AMFA Postgres database (via additive methods on `amfa.Repository`), build `*domain.Alert` rows, and hand them to the existing `alerts` repository and `notifications` service for persistence and fan-out. Wiring is done with a new Uber-fx module that builds one service per AMFA-enabled tenant.

**Tech Stack:** Go 1.25, Uber fx (DI), GORM (raw SQL over per-tenant AMFA Postgres), stdlib `testing` (no testify), `mapstructure` config tags.

---

## ⚠️ Execution notes (read before starting)

1. **No git toolchain on this host.** All Go build/test/vet commands MUST run inside Docker. The canonical incantation (run from the repo root) is:

   ```bash
   docker run --rm -v "$PWD:/app" -v go-mod-cache:/go/pkg/mod -w /app \
     golang:1.25.1-alpine sh -c "apk add --no-cache git build-base >/dev/null 2>&1 && <GO COMMAND HERE>"
   ```

   Throughout this plan, when a step says `Run (in Docker): go test -short ./amfacheck/`, substitute that command into `<GO COMMAND HERE>` above. A convenient shell function for the session:

   ```bash
   gotest() { docker run --rm -v "$PWD:/app" -v go-mod-cache:/go/pkg/mod -w /app \
     golang:1.25.1-alpine sh -c "apk add --no-cache git build-base >/dev/null 2>&1 && $*"; }
   # usage: gotest go test -short ./amfacheck/
   ```

2. **No git commits this session.** The repo owner has a standing rule: do NOT make git commits, write to git, or modify pre-existing logic unrelated to AMFA during the current session. Each task below ends with a **Commit** step for completeness. **If you are executing this plan in the current session, treat every "Commit" step as a CHECKPOINT only** — run the tests, pause for review, and do NOT actually run `git commit`. If this plan is executed later in a separate session that has commit authorization, perform the commits as written.

3. **Additive-only constraint.** Every change is either a brand-new file or an additive change to an AMFA-related file (new struct, new method, new interface member, new config field). No existing method signatures or unrelated logic change. The one place this plan touches existing AMFA test files (Task 2) is to add no-op methods to test stubs so they keep satisfying the grown `amfa.Repository` interface — this is required for the package to compile and is additive.

4. **Frontend is out of scope.** This feature surfaces in the existing Alerts page; there is no new UI. No `web/` changes, no vitest.

---

## Divergences from the design spec's illustrative code

The spec's Go snippets are illustrative. The real codebase uses slightly different names/shapes, and this plan follows the **real codebase** so the code compiles and stays idiomatic. Concretely:

| Spec snippet | This plan uses | Why |
|---|---|---|
| `Check.Type()` / `Check.Description()` | `Check.GetCheckType()` / `Check.GetDescription()` | Matches the real `eventscheck.Check` interface exactly. |
| `Notifier` interface | `NotificationService` interface | Matches the real `eventscheck.NotificationService` name. |
| `amfacheck.Service` used as `*Service` (struct) | `Service` is an **interface**; `NewService` returns `Service` | Mirrors `eventscheck.Service`, which is an interface with a private `service` struct. fx holds `[]amfacheck.Service`. |
| Checks hold `cfg *config.PerEventCheckConfig` etc. | Checks take **primitive** params (`threshold int`, `window time.Duration`, `minRisk int`) | Mirrors `eventscheck` checks and keeps `amfacheck` decoupled from the `config` package. The fx layer reads config and passes primitives. |
| `realmsFn` backed by `keycloak.Repository.GetRealms` | `realmsFn` backed by `keycloak.Service.ListRealms` | `keycloak.Service` is what's actually in the fx graph; `ListRealms(ctx, tenantID)` wraps `GetRealms`. |
| Repo methods edited directly into `repository.go` | New methods live in a new file `amfa/postgres/event_queries.go` (still methods on `*Repository`) | Keeps the edit additive and the existing `repository.go` untouched. |
| `EventRow.RiskLevel` used as `int` | It is `*int` (also `UserID *string`, `Country *string`) | Real type. Metadata marshalling handles pointers; never dereference without a nil check. |

The five rules, severities, dedup keys, config layout, error handling, and per-tenant lifecycle all follow the spec exactly.

---

## File Structure

**New files:**

```
amfacheck/
  interfaces.go              Check, AlertStore, NotificationService, Logger, RealmsFunc
  hash.go                    alertID() + windowBucket() helpers (+ hash_test.go)
  service.go                 per-tenant poll loop (Service interface + service struct)
  service_test.go
  risk_rejected_check.go     rule 1 (per-event, critical)      + risk_rejected_check_test.go
  repeated_risky_check.go    rule 2 (per-user/day, warning)    + repeated_risky_check_test.go
  vpn_risky_check.go         rule 3 (per-event, warning)       + vpn_risky_check_test.go
  login_error_check.go       rule 4a (threshold/window, warn)  + login_error_check_test.go
  client_login_error_check.go rule 4b                          + client_login_error_check_test.go
  integration_test.go        //go:build integration
amfa/postgres/event_queries.go   the 4 new query methods on *Repository
internal/fx/amfacheck.go         fx module + per-tenant wiring  + amfacheck_test.go
```

**Modified (additive only):**

```
amfa/interfaces.go            + 4 method signatures on Repository
amfa/types.go                 + UserRiskyCount struct
amfa/types_test.go            + 4 no-op methods on nopRepository stub
amfa/service_test.go          + 4 no-op methods on stubRepo stub
amfa/registry_test.go         + 4 no-op methods on fakeRepo stub
amfa/postgres/repository_integration_test.go   + integration tests for the 4 new methods
internal/config/models.go     + AmfaCheckerConfig (+ friends) + field on KeycloakInstanceConfig
internal/config/models_test.go + ApplyDefaults test
internal/fx/app.go            + AmfaCheckModule in the module list (one line)
config.yaml.example           + documented amfa_checker block
docs/amfa-integration.md      + Alerts section + troubleshooting rows
docs/architecture.md          + brief amfacheck/ note
```

---

## Task 1: Config structs for `amfa_checker`

**Files:**
- Modify: `internal/config/models.go` (add structs after `EventCheckerChecks`, ~line 343; add field to `KeycloakInstanceConfig`, ~line 232)
- Test: `internal/config/models_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/models_test.go`:

```go
func TestAmfaCheckerConfig_ApplyDefaults(t *testing.T) {
	var c AmfaCheckerConfig
	c.ApplyDefaults()

	if c.PollInterval != time.Minute {
		t.Errorf("PollInterval default = %v, want 1m", c.PollInterval)
	}
	if c.Checks.RepeatedRisky.Threshold != 3 {
		t.Errorf("RepeatedRisky.Threshold default = %d, want 3", c.Checks.RepeatedRisky.Threshold)
	}
	if c.Checks.RepeatedRisky.Window != 24*time.Hour {
		t.Errorf("RepeatedRisky.Window default = %v, want 24h", c.Checks.RepeatedRisky.Window)
	}
	if c.Checks.RepeatedRisky.MinRiskLevel != 2 {
		t.Errorf("RepeatedRisky.MinRiskLevel default = %d, want 2", c.Checks.RepeatedRisky.MinRiskLevel)
	}
	if c.Checks.LoginError.Threshold != 5 {
		t.Errorf("LoginError.Threshold default = %d, want 5", c.Checks.LoginError.Threshold)
	}
	if c.Checks.LoginError.Window != 5*time.Minute {
		t.Errorf("LoginError.Window default = %v, want 5m", c.Checks.LoginError.Window)
	}
	if c.Checks.ClientLoginError.Threshold != 5 {
		t.Errorf("ClientLoginError.Threshold default = %d, want 5", c.Checks.ClientLoginError.Threshold)
	}
	if c.Checks.ClientLoginError.Window != 5*time.Minute {
		t.Errorf("ClientLoginError.Window default = %v, want 5m", c.Checks.ClientLoginError.Window)
	}
}

func TestAmfaCheckerConfig_ApplyDefaults_PreservesExplicit(t *testing.T) {
	c := AmfaCheckerConfig{
		PollInterval: 30 * time.Second,
		Checks: AmfaCheckerChecksConfig{
			RepeatedRisky: RepeatedRiskyConfig{Threshold: 10, Window: time.Hour, MinRiskLevel: 4},
			LoginError:    ThresholdCheckConfig{Threshold: 99, Window: time.Minute},
		},
	}
	c.ApplyDefaults()

	if c.PollInterval != 30*time.Second {
		t.Errorf("PollInterval overwritten: got %v", c.PollInterval)
	}
	if c.Checks.RepeatedRisky.Threshold != 10 {
		t.Errorf("RepeatedRisky.Threshold overwritten: got %d", c.Checks.RepeatedRisky.Threshold)
	}
	if c.Checks.LoginError.Threshold != 99 {
		t.Errorf("LoginError.Threshold overwritten: got %d", c.Checks.LoginError.Threshold)
	}
}
```

Confirm `internal/config/models_test.go` already imports `time` and `testing`; if `time` is missing, add it to the import block.

- [ ] **Step 2: Run the test to verify it fails (compile error)**

Run (in Docker): `go test ./internal/config/`
Expected: FAIL — `undefined: AmfaCheckerConfig`.

- [ ] **Step 3: Add the config structs**

In `internal/config/models.go`, immediately after the `EventCheckerChecks` struct and its check-config structs (search for `type EventCheckerChecks struct`), add:

```go
// AmfaCheckerConfig configures the AMFA risk-alert checker. It mirrors the
// shape of EventCheckerConfig so operators see a familiar layout. It lives at
// keycloak.global.amfa_checker. Disabled by default — operators opt in.
type AmfaCheckerConfig struct {
	Enabled      bool                    `mapstructure:"enabled"`       // master switch; default false
	PollInterval time.Duration           `mapstructure:"poll_interval"` // default 1m
	Checks       AmfaCheckerChecksConfig `mapstructure:"checks"`
}

// AmfaCheckerChecksConfig holds the per-rule toggles and thresholds.
type AmfaCheckerChecksConfig struct {
	RiskRejected     PerEventCheckConfig  `mapstructure:"risk_rejected"`      // rule 1
	RepeatedRisky    RepeatedRiskyConfig  `mapstructure:"repeated_risky"`     // rule 2
	VPNRisky         PerEventCheckConfig  `mapstructure:"vpn_risky"`          // rule 3
	LoginError       ThresholdCheckConfig `mapstructure:"login_error"`        // rule 4a
	ClientLoginError ThresholdCheckConfig `mapstructure:"client_login_error"` // rule 4b
}

// PerEventCheckConfig is the config shape for per-event rules (1 and 3) that
// have no tunable thresholds — only an enable flag.
type PerEventCheckConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// RepeatedRiskyConfig configures rule 2 (repeated risky attempts per user/day).
type RepeatedRiskyConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	Threshold    int           `mapstructure:"threshold"`      // default 3
	Window       time.Duration `mapstructure:"window"`         // default 24h
	MinRiskLevel int           `mapstructure:"min_risk_level"` // default 2
}

// ThresholdCheckConfig configures the event-type threshold rules (4a, 4b).
type ThresholdCheckConfig struct {
	Enabled   bool          `mapstructure:"enabled"`
	Threshold int           `mapstructure:"threshold"` // default 5
	Window    time.Duration `mapstructure:"window"`    // default 5m
}

// ApplyDefaults fills in zero-valued fields with conservative defaults. Called
// by the fx layer after reading config and before constructing services.
func (c *AmfaCheckerConfig) ApplyDefaults() {
	if c.PollInterval <= 0 {
		c.PollInterval = time.Minute
	}
	if c.Checks.RepeatedRisky.Threshold <= 0 {
		c.Checks.RepeatedRisky.Threshold = 3
	}
	if c.Checks.RepeatedRisky.Window <= 0 {
		c.Checks.RepeatedRisky.Window = 24 * time.Hour
	}
	if c.Checks.RepeatedRisky.MinRiskLevel <= 0 {
		c.Checks.RepeatedRisky.MinRiskLevel = 2
	}
	if c.Checks.LoginError.Threshold <= 0 {
		c.Checks.LoginError.Threshold = 5
	}
	if c.Checks.LoginError.Window <= 0 {
		c.Checks.LoginError.Window = 5 * time.Minute
	}
	if c.Checks.ClientLoginError.Threshold <= 0 {
		c.Checks.ClientLoginError.Threshold = 5
	}
	if c.Checks.ClientLoginError.Window <= 0 {
		c.Checks.ClientLoginError.Window = 5 * time.Minute
	}
}
```

- [ ] **Step 4: Nest the config under the Keycloak global instance config**

In `internal/config/models.go`, in the `KeycloakInstanceConfig` struct (search for `type KeycloakInstanceConfig struct`), add this field right after the `VersionChecker` line:

```go
	AmfaChecker       AmfaCheckerConfig        `mapstructure:"amfa_checker"`       // AMFA risk-alert checker settings
```

- [ ] **Step 5: Run the test to verify it passes**

Run (in Docker): `go test ./internal/config/`
Expected: PASS (both new tests, plus existing config tests still green).

- [ ] **Step 6: Commit**

```bash
git add internal/config/models.go internal/config/models_test.go
git commit -m "feat(amfa): add amfa_checker config structs with defaults"
```

---

## Task 2: Grow `amfa.Repository` with 4 query methods (signatures + stub updates)

This task adds the interface members and the `UserRiskyCount` type, and updates the three in-package test stubs so the AMFA package keeps compiling. Implementation of the methods on the concrete repo comes in Task 3.

**Files:**
- Modify: `amfa/types.go` (add `UserRiskyCount`)
- Modify: `amfa/interfaces.go` (add 4 method signatures to `Repository`)
- Modify: `amfa/types_test.go` (add 4 no-op methods to `nopRepository`)
- Modify: `amfa/service_test.go` (add 4 no-op methods to `stubRepo`)
- Modify: `amfa/registry_test.go` (add 4 no-op methods to `fakeRepo`)

- [ ] **Step 1: Confirm the full set of `amfa.Repository` implementers**

Run: `grep -rln "ListEvents(_ context.Context, _ ListEventsOptions)" amfa/`
Expected output: exactly these three test stub files —
```
amfa/types_test.go
amfa/service_test.go
amfa/registry_test.go
```
Plus the concrete implementation in `amfa/postgres/repository.go` (different signature form `ctx context.Context, opts ...`). If any **other** file appears, it is an additional implementer that also needs the 4 no-op methods — add them there too following the same pattern as Step 5.

- [ ] **Step 2: Add the `UserRiskyCount` type**

In `amfa/types.go`, after the `EventRow` struct, add:

```go
// UserRiskyCount is the row shape returned by Repository.CountRepeatedRiskyByUser:
// one user and the number of risky events they accumulated in the query window.
type UserRiskyCount struct {
	UserID string
	Count  int64
}
```

- [ ] **Step 3: Add the 4 method signatures to the `Repository` interface**

In `amfa/interfaces.go`, inside the `Repository` interface (after the existing `GetGeoBuckets` line), add:

```go
	// ListRejectedEventsSince returns events whose pre_auth_risk_decision = 4
	// (rejected by the risk engine) with event_time strictly newer than `since`,
	// for the named realm, oldest first. Used by rule 1 (RiskRejectedCheck).
	ListRejectedEventsSince(ctx context.Context, realm string, since time.Time) ([]EventRow, error)

	// ListVPNRiskyEventsSince returns events with is_vpn = true AND
	// pre_auth_risk_decision >= minRisk (inclusive) and event_time strictly
	// newer than `since`, for the named realm, oldest first. Used by rule 3
	// (VPNRiskyCheck); callers pass minRisk=3 for the "risk > 2" semantics.
	ListVPNRiskyEventsSince(ctx context.Context, realm string, since time.Time, minRisk int) ([]EventRow, error)

	// CountRepeatedRiskyByUser returns, per user in the realm, the count of
	// events with pre_auth_risk_decision >= minRisk (inclusive) in [since, now),
	// keeping only users whose count >= threshold. Used by rule 2; callers pass
	// minRisk=2.
	CountRepeatedRiskyByUser(ctx context.Context, realm string, since time.Time, minRisk, threshold int) ([]UserRiskyCount, error)

	// CountByEventTypeInWindow returns the count of events of the given type in
	// [start, end] for the realm. Used by rules 4a and 4b.
	CountByEventTypeInWindow(ctx context.Context, realm, eventType string, start, end time.Time) (int64, error)
```

Confirm `amfa/interfaces.go` imports `time`; if it only imports `context`, change the import block to:

```go
import (
	"context"
	"time"
)
```

- [ ] **Step 4: Run the build to verify the stubs now fail to compile**

Run (in Docker): `go build ./amfa/...`
Expected: this succeeds (interface members don't break non-test builds), but tests will fail. Run:
Run (in Docker): `go vet ./amfa/`
Expected: FAIL — `*nopRepository does not implement Repository (missing method CountByEventTypeInWindow)` (and the other stubs).

- [ ] **Step 5: Add no-op methods to the three test stubs**

In `amfa/types_test.go`, after the existing `nopRepository` methods, add (note `nopRepository` uses value receivers):

```go
func (nopRepository) ListRejectedEventsSince(_ context.Context, _ string, _ time.Time) ([]EventRow, error) {
	return nil, nil
}
func (nopRepository) ListVPNRiskyEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]EventRow, error) {
	return nil, nil
}
func (nopRepository) CountRepeatedRiskyByUser(_ context.Context, _ string, _ time.Time, _, _ int) ([]UserRiskyCount, error) {
	return nil, nil
}
func (nopRepository) CountByEventTypeInWindow(_ context.Context, _, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}
```

In `amfa/service_test.go`, after the existing `stubRepo` methods, add (note `stubRepo` uses pointer receivers):

```go
func (s *stubRepo) ListRejectedEventsSince(_ context.Context, _ string, _ time.Time) ([]EventRow, error) {
	return nil, nil
}
func (s *stubRepo) ListVPNRiskyEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]EventRow, error) {
	return nil, nil
}
func (s *stubRepo) CountRepeatedRiskyByUser(_ context.Context, _ string, _ time.Time, _, _ int) ([]UserRiskyCount, error) {
	return nil, nil
}
func (s *stubRepo) CountByEventTypeInWindow(_ context.Context, _, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}
```

In `amfa/registry_test.go`, after the existing `fakeRepo` methods, add (note `fakeRepo` uses value receivers):

```go
func (fakeRepo) ListRejectedEventsSince(_ context.Context, _ string, _ time.Time) ([]EventRow, error) {
	return nil, nil
}
func (fakeRepo) ListVPNRiskyEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]EventRow, error) {
	return nil, nil
}
func (fakeRepo) CountRepeatedRiskyByUser(_ context.Context, _ string, _ time.Time, _, _ int) ([]UserRiskyCount, error) {
	return nil, nil
}
func (fakeRepo) CountByEventTypeInWindow(_ context.Context, _, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}
```

For each of the three test files, confirm `time` is imported (the new signatures reference `time.Time`). If a file doesn't already import `time`, add it to that file's import block.

- [ ] **Step 6: Run the AMFA tests to verify everything compiles and passes**

Run (in Docker): `go test -short ./amfa/...`
Expected: PASS (all existing AMFA tests green; stubs now satisfy the grown interface).

- [ ] **Step 7: Commit**

```bash
git add amfa/types.go amfa/interfaces.go amfa/types_test.go amfa/service_test.go amfa/registry_test.go
git commit -m "feat(amfa): extend Repository interface with risk-alert query methods"
```

---

## Task 3: Implement the 4 query methods on the concrete Postgres repository

**Files:**
- Create: `amfa/postgres/event_queries.go`
- Test: `amfa/postgres/repository_integration_test.go` (append integration tests)

The methods are defined on the existing `*Repository` (`amfa/postgres/repository.go`); placing them in a new file keeps `repository.go` untouched. They follow the exact SQL idiom of `ListEvents`/`GetStats`: GORM `Raw(...).Scan(...)`, realm filter via `p.auth_context_json->>'realm_id' = ?`, the same `LEFT JOIN` topology, and `classifyDBError` at the boundary.

- [ ] **Step 1: Write the failing integration tests**

Append to `amfa/postgres/repository_integration_test.go` (which already has the `//go:build integration` tag and the `openAmfaTestDB` helper):

```go
func TestRepository_ListRejectedEventsSince_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.ListRejectedEventsSince(context.Background(), "", time.Unix(0, 0))
	if err == nil {
		t.Error("expected error when realm is empty")
	}
}

func TestRepository_ListVPNRiskyEventsSince_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.ListVPNRiskyEventsSince(context.Background(), "", time.Unix(0, 0), 3)
	if err == nil {
		t.Error("expected error when realm is empty")
	}
}

func TestRepository_CountRepeatedRiskyByUser_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.CountRepeatedRiskyByUser(context.Background(), "", time.Unix(0, 0), 2, 3)
	if err == nil {
		t.Error("expected error when realm is empty")
	}
}

func TestRepository_CountByEventTypeInWindow_RequiresRealm(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	_, err := repo.CountByEventTypeInWindow(context.Background(), "", "LOGIN_ERROR", time.Unix(0, 0), time.Now())
	if err == nil {
		t.Error("expected error when realm is empty")
	}
}

// TestRepository_NewQueries_SmokeAgainstRealDB exercises each query against the
// seeded AMFA test DB. It asserts only that the queries execute without error
// and return sane (non-negative) shapes — the row contents depend on seed data.
func TestRepository_NewQueries_SmokeAgainstRealDB(t *testing.T) {
	db := openAmfaTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	const realm = "master" // adjust to a realm present in the test DB if needed
	since := time.Now().Add(-90 * 24 * time.Hour)

	if _, err := repo.ListRejectedEventsSince(ctx, realm, since); err != nil {
		t.Errorf("ListRejectedEventsSince: %v", err)
	}
	if _, err := repo.ListVPNRiskyEventsSince(ctx, realm, since, 3); err != nil {
		t.Errorf("ListVPNRiskyEventsSince: %v", err)
	}
	if _, err := repo.CountRepeatedRiskyByUser(ctx, realm, since, 2, 1); err != nil {
		t.Errorf("CountRepeatedRiskyByUser: %v", err)
	}
	n, err := repo.CountByEventTypeInWindow(ctx, realm, "LOGIN_ERROR", since, time.Now())
	if err != nil {
		t.Errorf("CountByEventTypeInWindow: %v", err)
	}
	if n < 0 {
		t.Errorf("CountByEventTypeInWindow returned negative count %d", n)
	}
}
```

Confirm the file imports `time`; if not, add it.

- [ ] **Step 2: Run the integration tests to verify they fail to compile**

Run (in Docker): `go test -tags=integration ./amfa/postgres/`
Expected: FAIL — `repo.ListRejectedEventsSince undefined` (compile error). (Without `AMFA_TEST_DSN` the tests would otherwise `t.Skip`, but the compile failure happens first.)

- [ ] **Step 3: Create `amfa/postgres/event_queries.go`**

```go
package postgres

import (
	"context"
	"fmt"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
)

// eventRowScan is the row shape for the full-EventRow queries below. It mirrors
// the inline scan struct used by ListEvents in repository.go.
type eventRowScan struct {
	EventID     string    `gorm:"column:event_id"`
	EventTime   time.Time `gorm:"column:event_time"`
	EventType   string    `gorm:"column:event_type"`
	UserID      *string   `gorm:"column:user_id"`
	Client      string    `gorm:"column:client"`
	IPAddress   string    `gorm:"column:ip_address"`
	Country     *string   `gorm:"column:country"`
	Lat         *float64  `gorm:"column:lat"`
	Long        *float64  `gorm:"column:long"`
	IsVPN       bool      `gorm:"column:is_vpn"`
	RiskLevel   *int      `gorm:"column:risk_level"`
	FinalStatus *string   `gorm:"column:final_status"`
}

func (s eventRowScan) toEventRow() amfa.EventRow {
	return amfa.EventRow{
		EventID:     s.EventID,
		EventTime:   s.EventTime,
		EventType:   s.EventType,
		UserID:      s.UserID,
		Client:      s.Client,
		IPAddress:   s.IPAddress,
		Country:     s.Country,
		Lat:         s.Lat,
		Long:        s.Long,
		IsVPN:       s.IsVPN,
		RiskLevel:   s.RiskLevel,
		FinalStatus: s.FinalStatus,
	}
}

// eventRowSelect is the shared SELECT/JOIN prefix used by the per-event queries.
// It is identical to the projection in ListEvents.
const eventRowSelect = `
        SELECT
            e.id              AS event_id,
            e.event_time      AS event_time,
            e.event_type      AS event_type,
            e.user_id         AS user_id,
            COALESCE(ac.client, '')      AS client,
            COALESCE(ac.ip_address, '')  AS ip_address,
            ac.country_name   AS country,
            ac.lat            AS lat,
            ac.long           AS long,
            COALESCE(ac.is_vpn, false) AS is_vpn,
            p.pre_auth_risk_decision   AS risk_level,
            p.final_status    AS final_status
        FROM auth_event e
        LEFT JOIN auth_process p   ON p.id   = e.auth_process
        LEFT JOIN auth_context ac  ON ac.hash = e.auth_context_hash
        WHERE p.auth_context_json->>'realm_id' = ?`

// ListRejectedEventsSince returns risk-rejected (pre_auth_risk_decision = 4)
// events newer than `since` for the realm, oldest first.
func (r *Repository) ListRejectedEventsSince(ctx context.Context, realm string, since time.Time) ([]amfa.EventRow, error) {
	if realm == "" {
		return nil, fmt.Errorf("amfa.ListRejectedEventsSince: realm is required")
	}
	sql := eventRowSelect + `
          AND p.pre_auth_risk_decision = 4
          AND e.event_time > ?
        ORDER BY e.event_time ASC`

	var rows []eventRowScan
	if err := r.db.WithContext(ctx).Raw(sql, realm, since).Scan(&rows).Error; err != nil {
		return nil, classifyDBError(err, "amfa list rejected events")
	}
	out := make([]amfa.EventRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toEventRow())
	}
	return out, nil
}

// ListVPNRiskyEventsSince returns VPN-flagged events with risk >= minRisk newer
// than `since` for the realm, oldest first.
func (r *Repository) ListVPNRiskyEventsSince(ctx context.Context, realm string, since time.Time, minRisk int) ([]amfa.EventRow, error) {
	if realm == "" {
		return nil, fmt.Errorf("amfa.ListVPNRiskyEventsSince: realm is required")
	}
	sql := eventRowSelect + `
          AND COALESCE(ac.is_vpn, false) = true
          AND p.pre_auth_risk_decision >= ?
          AND e.event_time > ?
        ORDER BY e.event_time ASC`

	var rows []eventRowScan
	if err := r.db.WithContext(ctx).Raw(sql, realm, minRisk, since).Scan(&rows).Error; err != nil {
		return nil, classifyDBError(err, "amfa list vpn risky events")
	}
	out := make([]amfa.EventRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toEventRow())
	}
	return out, nil
}

// CountRepeatedRiskyByUser returns users in the realm with >= threshold events
// at risk >= minRisk since `since`.
func (r *Repository) CountRepeatedRiskyByUser(ctx context.Context, realm string, since time.Time, minRisk, threshold int) ([]amfa.UserRiskyCount, error) {
	if realm == "" {
		return nil, fmt.Errorf("amfa.CountRepeatedRiskyByUser: realm is required")
	}
	sql := `
        SELECT e.user_id AS user_id, COUNT(*) AS cnt
        FROM auth_event e
        LEFT JOIN auth_process p ON p.id = e.auth_process
        WHERE p.auth_context_json->>'realm_id' = ?
          AND p.pre_auth_risk_decision >= ?
          AND e.event_time >= ?
          AND e.user_id IS NOT NULL
        GROUP BY e.user_id
        HAVING COUNT(*) >= ?`

	type scanRow struct {
		UserID string `gorm:"column:user_id"`
		Cnt    int64  `gorm:"column:cnt"`
	}
	var rows []scanRow
	if err := r.db.WithContext(ctx).Raw(sql, realm, minRisk, since, threshold).Scan(&rows).Error; err != nil {
		return nil, classifyDBError(err, "amfa count repeated risky by user")
	}
	out := make([]amfa.UserRiskyCount, 0, len(rows))
	for _, row := range rows {
		out = append(out, amfa.UserRiskyCount{UserID: row.UserID, Count: row.Cnt})
	}
	return out, nil
}

// CountByEventTypeInWindow returns the count of events of eventType in
// [start, end] for the realm.
func (r *Repository) CountByEventTypeInWindow(ctx context.Context, realm, eventType string, start, end time.Time) (int64, error) {
	if realm == "" {
		return 0, fmt.Errorf("amfa.CountByEventTypeInWindow: realm is required")
	}
	sql := `
        SELECT COUNT(*)
        FROM auth_event e
        LEFT JOIN auth_process p ON p.id = e.auth_process
        WHERE p.auth_context_json->>'realm_id' = ?
          AND e.event_type = ?
          AND e.event_time BETWEEN ? AND ?`

	var count int64
	if err := r.db.WithContext(ctx).Raw(sql, realm, eventType, start, end).Scan(&count).Error; err != nil {
		return 0, classifyDBError(err, "amfa count by event type in window")
	}
	return count, nil
}
```

- [ ] **Step 4: Verify it compiles and the `-short` suite passes**

Run (in Docker): `go build ./amfa/...`
Expected: PASS.
Run (in Docker): `go vet ./amfa/...`
Expected: PASS (no "does not implement" errors anywhere — the concrete repo now satisfies the grown interface).
Run (in Docker): `go test -short ./amfa/...`
Expected: PASS.

- [ ] **Step 5: Verify the integration tests compile (they will skip without a DSN)**

Run (in Docker): `go test -tags=integration ./amfa/postgres/`
Expected: PASS with `--- SKIP` lines (no `AMFA_TEST_DSN` on this host). The important signal is that it compiles and the require-realm assertions are reachable. (When run later with a real `AMFA_TEST_DSN`, the smoke test exercises the SQL.)

- [ ] **Step 6: Commit**

```bash
git add amfa/postgres/event_queries.go amfa/postgres/repository_integration_test.go
git commit -m "feat(amfa): implement risk-alert query methods on postgres repository"
```

---

## Task 4: `amfacheck` interfaces, logger contract, and ID helpers

**Files:**
- Create: `amfacheck/interfaces.go`
- Create: `amfacheck/hash.go`
- Test: `amfacheck/hash_test.go`

- [ ] **Step 1: Write the failing test for the ID helpers**

Create `amfacheck/hash_test.go`:

```go
package amfacheck

import (
	"testing"
	"time"
)

func TestAlertID_StableAndDistinct(t *testing.T) {
	a := alertID("amfa-risk-rejected", "tenant-1", "event-1")
	b := alertID("amfa-risk-rejected", "tenant-1", "event-1")
	if a != b {
		t.Errorf("alertID not stable: %q != %q", a, b)
	}
	if len(a) != 64 {
		t.Errorf("alertID should be 64 hex chars (sha256), got %d: %q", len(a), a)
	}

	c := alertID("amfa-risk-rejected", "tenant-1", "event-2")
	if a == c {
		t.Error("alertID should differ for different event IDs")
	}

	d := alertID("amfa-vpn-risky", "tenant-1", "event-1")
	if a == d {
		t.Error("alertID should differ for different rule prefixes")
	}
}

func TestWindowBucket_AlignsToWindow(t *testing.T) {
	window := 5 * time.Minute
	t1 := time.Date(2026, 6, 2, 12, 3, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 2, 12, 4, 59, 0, time.UTC)
	want := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	if got := windowBucket(t1, window); !got.Equal(want) {
		t.Errorf("windowBucket(%v) = %v, want %v", t1, got, want)
	}
	if got := windowBucket(t2, window); !got.Equal(want) {
		t.Errorf("windowBucket(%v) = %v, want %v", t2, got, want)
	}

	t3 := time.Date(2026, 6, 2, 12, 5, 0, 0, time.UTC)
	if got := windowBucket(t3, window); got.Equal(want) {
		t.Errorf("windowBucket at next window boundary should differ from %v", want)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run (in Docker): `go test ./amfacheck/`
Expected: FAIL — `undefined: alertID` / package has no non-test files.

- [ ] **Step 3: Create `amfacheck/hash.go`**

```go
package amfacheck

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// alertID builds a stable, unique alert ID from the given parts by joining them
// with ":" and hashing with sha256. The parts always start with a per-rule
// prefix (e.g. "amfa-risk-rejected") so IDs never collide across rules.
func alertID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(sum[:])
}

// windowBucket returns the window-aligned floor of now, so any time within
// [bucket, bucket+window) maps to the same bucket. Used by the threshold rules
// (4a/4b) to keep the AlertID stable across adjacent polls in the same window.
func windowBucket(now time.Time, window time.Duration) time.Time {
	return now.Truncate(window)
}
```

- [ ] **Step 4: Run to verify it passes**

Run (in Docker): `go test ./amfacheck/`
Expected: PASS.

- [ ] **Step 5: Create `amfacheck/interfaces.go`**

```go
// Package amfacheck converts high-risk AMFA login events into POINT alerts.
// It deliberately mirrors the shape of the eventscheck package: one per-tenant
// poll loop running a set of Check implementations whose alerts flow through
// the existing alerts repository and notification fan-out. It is a sibling of
// eventscheck rather than a refactor of it — see docs/superpowers/specs.
package amfacheck

import (
	"context"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// Check is implemented by each of the five rules. Mirrors eventscheck.Check.
type Check interface {
	// GetCheckType returns the unique identifier for this check,
	// e.g. "amfa-risk-rejected".
	GetCheckType() string

	// GetDescription returns a human-readable description of this check.
	GetDescription() string

	// Execute runs the check and returns any detected alerts.
	Execute(ctx context.Context) ([]*domain.Alert, error)
}

// AlertStore is the slice of alerts.Repository this service needs.
type AlertStore interface {
	GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
	SaveAlert(ctx context.Context, alert *domain.Alert) error
}

// NotificationService is the slice of notifications.Service this service needs.
type NotificationService interface {
	NotifyAlert(ctx context.Context, alert *domain.Alert) error
}

// Logger is the logging contract for amfacheck. Mirrors eventscheck.Logger so
// the fx adapter can be near-identical.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
	Debug(msg string, args ...any)
	WithComponent(component string) Logger
}

// RealmsFunc returns the current realm list for a tenant. Each Check holds one
// and calls it every tick, so realms added/removed in Keycloak are picked up
// without a restart.
type RealmsFunc func(ctx context.Context) ([]string, error)
```

> **Note on `AlertStore` vs the real `alerts.Repository`:** the real interface exposes `GetByAlertID(ctx, tenantID, alertID)` and `Save(ctx, alert)` (see `alerts/repository.go`), not `GetAlertByID`/`SaveAlert`. The fx layer (Task 10) provides a thin adapter that maps these names, exactly the way `eventscheck`'s `AlertStore` is satisfied. `notifications.Service.NotifyAlert` already matches the `NotificationService` signature directly.

- [ ] **Step 6: Run to verify the package still builds and tests pass**

Run (in Docker): `go test ./amfacheck/`
Expected: PASS (hash tests still green; new interfaces compile).

- [ ] **Step 7: Commit**

```bash
git add amfacheck/interfaces.go amfacheck/hash.go amfacheck/hash_test.go
git commit -m "feat(amfacheck): add Check/AlertStore/Notifier interfaces and ID helpers"
```

---

## Task 5: `amfacheck.Service` poll loop

**Files:**
- Create: `amfacheck/service.go`
- Test: `amfacheck/service_test.go`

This is a near-verbatim port of `eventscheck/service.go`, with a `tenantID` field added for log context (the spec's observable startup/per-tick log lines include `tenant_id`).

- [ ] **Step 1: Write the failing service test**

Create `amfacheck/service_test.go`:

```go
package amfacheck

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// --- test doubles ---

type fakeCheck struct {
	checkType string
	alerts    []*domain.Alert
	err       error
	calls     int
	mu        sync.Mutex
}

func (f *fakeCheck) GetCheckType() string    { return f.checkType }
func (f *fakeCheck) GetDescription() string  { return "fake check " + f.checkType }
func (f *fakeCheck) Execute(_ context.Context) ([]*domain.Alert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.alerts, f.err
}
func (f *fakeCheck) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeAlertStore struct {
	mu        sync.Mutex
	existing  map[string]*domain.Alert
	saveCount map[string]int
}

func newFakeAlertStore() *fakeAlertStore {
	return &fakeAlertStore{existing: map[string]*domain.Alert{}, saveCount: map[string]int{}}
}
func (s *fakeAlertStore) GetAlertByID(_ context.Context, _, alertID string) (*domain.Alert, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.existing[alertID]
	if !ok {
		return nil, nil
	}
	return a, nil
}
func (s *fakeAlertStore) SaveAlert(_ context.Context, alert *domain.Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCount[alert.AlertID]++
	stored := *alert
	s.existing[alert.AlertID] = &stored
	return nil
}
func (s *fakeAlertStore) saves(alertID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCount[alertID]
}

type fakeNotifier struct {
	mu      sync.Mutex
	notered map[string]int
}

func newFakeNotifier() *fakeNotifier { return &fakeNotifier{notered: map[string]int{}} }
func (n *fakeNotifier) NotifyAlert(_ context.Context, alert *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notered[alert.AlertID]++
	return nil
}
func (n *fakeNotifier) notifs(alertID string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.notered[alertID]
}

// testLogger is a no-op Logger for tests.
type testLogger struct{}

func (testLogger) Info(string, ...any)            {}
func (testLogger) Error(string, ...any)           {}
func (testLogger) Warn(string, ...any)            {}
func (testLogger) Debug(string, ...any)           {}
func (l testLogger) WithComponent(string) Logger  { return l }

func newAlert(id string) *domain.Alert {
	return &domain.Alert{
		TenantID: "tenant-1",
		AlertID:  id,
		Source:   domain.AlertSourceEvent,
		Type:     domain.AlertTypeSecurity,
		Severity: domain.AlertSeverityWarning,
		Status:   domain.AlertStatusActive,
		Title:    "test",
		Metadata: "{}",
	}
}

// --- tests ---

func TestService_RunCheckNow_NewAlertSavedAndNotifiedOnce(t *testing.T) {
	store := newFakeAlertStore()
	notifier := newFakeNotifier()
	chk := &fakeCheck{checkType: "amfa-test", alerts: []*domain.Alert{newAlert("a1")}}

	svc := NewService("tenant-1", []Check{chk}, store, notifier, testLogger{}, time.Minute)
	if err := svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}

	if store.saves("a1") != 1 {
		t.Errorf("SaveAlert calls = %d, want 1", store.saves("a1"))
	}
	if notifier.notifs("a1") != 1 {
		t.Errorf("NotifyAlert calls = %d, want 1", notifier.notifs("a1"))
	}
}

func TestService_RunCheckNow_Dedup_SecondRunSavesButDoesNotRenotify(t *testing.T) {
	store := newFakeAlertStore()
	notifier := newFakeNotifier()
	chk := &fakeCheck{checkType: "amfa-test", alerts: []*domain.Alert{newAlert("a1")}}

	svc := NewService("tenant-1", []Check{chk}, store, notifier, testLogger{}, time.Minute)
	_ = svc.RunCheckNow(context.Background())
	_ = svc.RunCheckNow(context.Background())

	if store.saves("a1") != 2 {
		t.Errorf("SaveAlert calls = %d, want 2 (LastSeen update on recurrence)", store.saves("a1"))
	}
	if notifier.notifs("a1") != 1 {
		t.Errorf("NotifyAlert calls = %d, want 1 (no re-notify within an hour)", notifier.notifs("a1"))
	}
}

func TestService_RunCheckNow_OneCheckErrorDoesNotBlockOthers(t *testing.T) {
	store := newFakeAlertStore()
	notifier := newFakeNotifier()
	bad := &fakeCheck{checkType: "amfa-bad", err: errors.New("boom")}
	good := &fakeCheck{checkType: "amfa-good", alerts: []*domain.Alert{newAlert("a2")}}

	svc := NewService("tenant-1", []Check{bad, good}, store, notifier, testLogger{}, time.Minute)
	if err := svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow returned error (should be swallowed per-check): %v", err)
	}

	if bad.callCount() != 1 {
		t.Errorf("bad check calls = %d, want 1", bad.callCount())
	}
	if good.callCount() != 1 {
		t.Errorf("good check calls = %d, want 1", good.callCount())
	}
	if store.saves("a2") != 1 {
		t.Errorf("good check alert not saved despite sibling error: saves=%d", store.saves("a2"))
	}
}

func TestService_StartStop_DoesNotHang(t *testing.T) {
	store := newFakeAlertStore()
	notifier := newFakeNotifier()
	chk := &fakeCheck{checkType: "amfa-test"}
	svc := NewService("tenant-1", []Check{chk}, store, notifier, testLogger{}, time.Hour)

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run (in Docker): `go test ./amfacheck/`
Expected: FAIL — `undefined: NewService`.

- [ ] **Step 3: Create `amfacheck/service.go`**

```go
package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// reNotifyInterval is how long an unacknowledged, still-active alert waits
// before a repeat notification is sent. Mirrors eventscheck.
const reNotifyInterval = time.Hour

// Service runs AMFA risk checks for a single tenant.
type Service interface {
	// Start begins polling. Runs an immediate check, then on a ticker.
	Start(ctx context.Context) error
	// Stop gracefully stops polling (10s drain).
	Stop() error
	// AddCheck registers an additional check.
	AddCheck(check Check)
	// RunCheckNow executes all checks once, synchronously.
	RunCheckNow(ctx context.Context) error
}

type service struct {
	tenantID     string
	checks       []Check
	alertStore   AlertStore
	notifier     NotificationService
	logger       Logger
	pollInterval time.Duration
	stopChan     chan struct{}
	doneChan     chan struct{}
}

// NewService creates a per-tenant AMFA checker service.
func NewService(
	tenantID string,
	checks []Check,
	alertStore AlertStore,
	notifier NotificationService,
	log Logger,
	pollInterval time.Duration,
) Service {
	return &service{
		tenantID:     tenantID,
		checks:       checks,
		alertStore:   alertStore,
		notifier:     notifier,
		logger:       log.WithComponent("amfa_checker"),
		pollInterval: pollInterval,
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}
}

func (s *service) AddCheck(check Check) {
	s.checks = append(s.checks, check)
	s.logger.Info("Added AMFA check", "tenant_id", s.tenantID, "check_type", check.GetCheckType())
}

func (s *service) Start(ctx context.Context) error {
	s.logger.Info("amfacheck service starting",
		"tenant_id", s.tenantID,
		"num_checks", len(s.checks),
		"poll_interval", s.pollInterval.String())

	go s.runChecks(ctx)
	go s.pollChecks(ctx)
	return nil
}

func (s *service) Stop() error {
	s.logger.Info("Stopping amfacheck service", "tenant_id", s.tenantID)
	close(s.stopChan)
	select {
	case <-s.doneChan:
		s.logger.Info("amfacheck service stopped gracefully", "tenant_id", s.tenantID)
	case <-time.After(10 * time.Second):
		s.logger.Warn("amfacheck service stop timeout", "tenant_id", s.tenantID)
	}
	return nil
}

func (s *service) pollChecks(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	defer close(s.doneChan)

	for {
		select {
		case <-ticker.C:
			s.runChecks(ctx)
		case <-s.stopChan:
			s.logger.Info("amfacheck polling stopped", "tenant_id", s.tenantID)
			return
		case <-ctx.Done():
			s.logger.Info("amfacheck polling cancelled", "tenant_id", s.tenantID)
			return
		}
	}
}

func (s *service) runChecks(ctx context.Context) {
	s.logger.Debug("amfacheck: running checks", "tenant_id", s.tenantID, "num_checks", len(s.checks))
	for _, check := range s.checks {
		if err := s.executeCheck(ctx, check); err != nil {
			s.logger.Error("amfacheck: check failed",
				"tenant_id", s.tenantID,
				"check_type", check.GetCheckType(),
				"error", err)
		}
	}
}

func (s *service) executeCheck(ctx context.Context, check Check) error {
	startTime := time.Now()
	alerts, err := check.Execute(ctx)
	if err != nil {
		return fmt.Errorf("check execution failed: %w", err)
	}

	newAlerts, updatedAlerts := 0, 0
	now := time.Now()
	for _, alert := range alerts {
		existing, gerr := s.alertStore.GetAlertByID(ctx, alert.TenantID, alert.AlertID)
		isNewAlert := gerr != nil || existing == nil

		if isNewAlert {
			alert.FirstDetected = now
			alert.LastSeen = now
			alert.Status = domain.AlertStatusActive
			newAlerts++
		} else {
			alert.FirstDetected = existing.FirstDetected
			alert.LastSeen = now
			alert.Status = existing.Status
			updatedAlerts++
		}

		shouldNotify := false
		notificationReason := ""
		if isNewAlert {
			shouldNotify = true
			notificationReason = "new alert"
		} else if existing.Status == domain.AlertStatusActive && existing.AcknowledgedAt == nil {
			lastNotifiedAt := existing.UpdatedAt
			if existing.Metadata != "" {
				var meta map[string]interface{}
				if jerr := json.Unmarshal([]byte(existing.Metadata), &meta); jerr == nil {
					if s, ok := meta["last_notified_at"].(string); ok {
						if parsed, perr := time.Parse(time.RFC3339, s); perr == nil {
							lastNotifiedAt = parsed
						}
					}
				}
			}
			if time.Since(lastNotifiedAt) > reNotifyInterval {
				shouldNotify = true
				notificationReason = "re-notification (1+ hour, not acknowledged)"
			}
		}

		if shouldNotify && alert.Metadata != "" {
			var meta map[string]interface{}
			if jerr := json.Unmarshal([]byte(alert.Metadata), &meta); jerr == nil {
				meta["last_notified_at"] = now.Format(time.RFC3339)
				if b, merr := json.Marshal(meta); merr == nil {
					alert.Metadata = string(b)
				}
			}
		}

		if serr := s.alertStore.SaveAlert(ctx, alert); serr != nil {
			s.logger.Error("amfacheck: failed to save alert",
				"tenant_id", s.tenantID, "alert_id", alert.AlertID, "error", serr)
			continue
		}

		if isNewAlert {
			s.logger.Info("amfacheck: new alert",
				"tenant_id", s.tenantID,
				"alert_id", alert.AlertID,
				"check", check.GetCheckType(),
				"severity", string(alert.Severity))
		}

		if shouldNotify && s.notifier != nil {
			if nerr := s.notifier.NotifyAlert(ctx, alert); nerr != nil {
				s.logger.Error("amfacheck: notification failed",
					"tenant_id", s.tenantID,
					"alert_id", alert.AlertID,
					"reason", notificationReason,
					"error", nerr)
			}
		}
	}

	s.logger.Debug("amfacheck: check completed",
		"tenant_id", s.tenantID,
		"check", check.GetCheckType(),
		"new_alerts", newAlerts,
		"updated_alerts", updatedAlerts,
		"duration_ms", time.Since(startTime).Milliseconds())
	return nil
}

func (s *service) RunCheckNow(ctx context.Context) error {
	s.runChecks(ctx)
	return nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run (in Docker): `go test ./amfacheck/`
Expected: PASS (all four service tests, plus hash tests).

- [ ] **Step 5: Commit**

```bash
git add amfacheck/service.go amfacheck/service_test.go
git commit -m "feat(amfacheck): add per-tenant poll-loop service"
```

---

## Task 6: Rule 1 — `RiskRejectedCheck` (per-event, critical)

**Files:**
- Create: `amfacheck/risk_rejected_check.go`
- Test: `amfacheck/risk_rejected_check_test.go`

- [ ] **Step 1: Write the failing test**

Create `amfacheck/risk_rejected_check_test.go`:

```go
package amfacheck

import (
	"context"
	"testing"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
)

// checkRepoStub is a hand-written amfa.Repository stub for check tests. Only the
// methods a given check uses return data; the rest are no-ops. It is shared by
// the rule test files in this package.
type checkRepoStub struct {
	rejected     []amfa.EventRow
	rejectedErr  error
	vpnRisky     []amfa.EventRow
	vpnRiskyErr  error
	userCounts   []amfa.UserRiskyCount
	userErr      error
	typeCount    int64
	typeCountErr error
}

func (s *checkRepoStub) ListEvents(_ context.Context, _ amfa.ListEventsOptions) (*amfa.ListEventsResult, error) {
	return &amfa.ListEventsResult{}, nil
}
func (s *checkRepoStub) GetStats(_ context.Context, _ amfa.StatsOptions) (amfa.Stats, error) {
	return amfa.Stats{}, nil
}
func (s *checkRepoStub) GetGeoBuckets(_ context.Context, _ amfa.GeoOptions) ([]amfa.GeoBucket, error) {
	return nil, nil
}
func (s *checkRepoStub) ListRejectedEventsSince(_ context.Context, _ string, _ time.Time) ([]amfa.EventRow, error) {
	return s.rejected, s.rejectedErr
}
func (s *checkRepoStub) ListVPNRiskyEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]amfa.EventRow, error) {
	return s.vpnRisky, s.vpnRiskyErr
}
func (s *checkRepoStub) CountRepeatedRiskyByUser(_ context.Context, _ string, _ time.Time, _, _ int) ([]amfa.UserRiskyCount, error) {
	return s.userCounts, s.userErr
}
func (s *checkRepoStub) CountByEventTypeInWindow(_ context.Context, _, _ string, _, _ time.Time) (int64, error) {
	return s.typeCount, s.typeCountErr
}

func singleRealm(realms ...string) RealmsFunc {
	return func(_ context.Context) ([]string, error) { return realms, nil }
}

func TestRiskRejectedCheck_ProducesCriticalPerEventAlerts(t *testing.T) {
	uid := "user-1"
	repo := &checkRepoStub{rejected: []amfa.EventRow{
		{EventID: "ev-1", EventTime: time.Now(), UserID: &uid, IPAddress: "1.2.3.4"},
		{EventID: "ev-2", EventTime: time.Now(), IPAddress: "5.6.7.8"},
	}}
	c := NewRiskRejectedCheck("tenant-1", singleRealm("master"), repo, time.Minute, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Severity != "critical" {
		t.Errorf("severity = %q, want critical", a.Severity)
	}
	if a.Source != "event" {
		t.Errorf("source = %q, want event", a.Source)
	}
	if a.CheckType != "amfa-risk-rejected" {
		t.Errorf("check_type = %q, want amfa-risk-rejected", a.CheckType)
	}
	if a.EventID == nil || *a.EventID != "ev-1" {
		t.Errorf("EventID pointer = %v, want ev-1", a.EventID)
	}
	want := alertID("amfa-risk-rejected", "tenant-1", "ev-1")
	if a.AlertID != want {
		t.Errorf("AlertID = %q, want %q", a.AlertID, want)
	}
}

func TestRiskRejectedCheck_AdvancesCheckpoint(t *testing.T) {
	// Production SQL returns only events strictly newer than the current
	// checkpoint (event_time > since), so use a recent event (newer than the
	// now-lookback floor); the monotonic After(since) guard advances it.
	newest := time.Now().Add(-10 * time.Second)
	repo := &checkRepoStub{rejected: []amfa.EventRow{
		{EventID: "ev-1", EventTime: newest.Add(-20 * time.Second)},
		{EventID: "ev-2", EventTime: newest},
	}}
	c := NewRiskRejectedCheck("tenant-1", singleRealm("master"), repo, time.Minute, testLogger{})

	if _, err := c.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := c.getLastSeen("master")
	if !got.Equal(newest) {
		t.Errorf("checkpoint = %v, want %v (newest event time)", got, newest)
	}
}

func TestRiskRejectedCheck_RepoErrorPropagates(t *testing.T) {
	repo := &checkRepoStub{rejectedErr: amfa.ErrAmfaUnavailable}
	c := NewRiskRejectedCheck("tenant-1", singleRealm("master"), repo, time.Minute, testLogger{})
	if _, err := c.Execute(context.Background()); err == nil {
		t.Error("expected error to propagate")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run (in Docker): `go test ./amfacheck/`
Expected: FAIL — `undefined: NewRiskRejectedCheck`.

- [ ] **Step 3: Create `amfacheck/risk_rejected_check.go`**

```go
package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// RiskRejectedCheck implements rule 1: one critical alert per AMFA event that
// the risk engine rejected (pre_auth_risk_decision = 4).
type RiskRejectedCheck struct {
	tenantID string
	realmsFn RealmsFunc
	repo     amfa.Repository
	lookback time.Duration // first-tick window per realm (= poll interval)
	logger   Logger

	mu       sync.Mutex
	lastSeen map[string]time.Time // per-realm checkpoint
}

// NewRiskRejectedCheck constructs rule 1. lookback bounds the first scan after
// start (per the spec: now - pollInterval).
func NewRiskRejectedCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, lookback time.Duration, log Logger) *RiskRejectedCheck {
	return &RiskRejectedCheck{
		tenantID: tenantID,
		realmsFn: realmsFn,
		repo:     repo,
		lookback: lookback,
		logger:   log.WithComponent("amfa-risk-rejected"),
		lastSeen: make(map[string]time.Time),
	}
}

func (c *RiskRejectedCheck) GetCheckType() string   { return "amfa-risk-rejected" }
func (c *RiskRejectedCheck) GetDescription() string { return "AMFA login rejected by risk engine (ACR 4)" }

func (c *RiskRejectedCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	realms, err := c.realmsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("amfa-risk-rejected: load realms: %w", err)
	}
	var alerts []*domain.Alert
	for _, realm := range realms {
		since := c.getLastSeen(realm)
		events, err := c.repo.ListRejectedEventsSince(ctx, realm, since)
		if err != nil {
			return nil, fmt.Errorf("amfa-risk-rejected: realm=%s: %w", realm, err)
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
	eventID := ev.EventID
	md, _ := json.Marshal(map[string]any{
		"amfa_event_id": ev.EventID,
		"user_id":       ev.UserID,
		"ip_address":    ev.IPAddress,
		"client":        ev.Client,
		"country":       ev.Country,
		"is_vpn":        ev.IsVPN,
		"risk_level":    ev.RiskLevel,
		"realm_name":    realm,
	})
	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID("amfa-risk-rejected", c.tenantID, ev.EventID),
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
		CheckType:      c.GetCheckType(),
		EventID:        &eventID,
		Recommendation: "Investigate the rejected login attempt. Review the user's session history and IP reputation. Consider whether the user should be temporarily blocked.",
		Metadata:       string(md),
	}
}

func (c *RiskRejectedCheck) getLastSeen(realm string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ts, ok := c.lastSeen[realm]; ok {
		return ts
	}
	return time.Now().Add(-c.lookback)
}

func (c *RiskRejectedCheck) setLastSeen(realm string, ts time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSeen[realm] = ts
}
```

- [ ] **Step 4: Run to verify it passes**

Run (in Docker): `go test ./amfacheck/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfacheck/risk_rejected_check.go amfacheck/risk_rejected_check_test.go
git commit -m "feat(amfacheck): add rule 1 RiskRejectedCheck"
```

---

## Task 7: Rule 3 — `VPNRiskyCheck` (per-event, warning)

(Rule 3 is structurally the twin of rule 1, so it comes next; rule 2 follows.)

**Files:**
- Create: `amfacheck/vpn_risky_check.go`
- Test: `amfacheck/vpn_risky_check_test.go`

- [ ] **Step 1: Write the failing test**

Create `amfacheck/vpn_risky_check_test.go`:

```go
package amfacheck

import (
	"context"
	"testing"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
)

func TestVPNRiskyCheck_ProducesWarningPerEventAlerts(t *testing.T) {
	risk := 3
	repo := &checkRepoStub{vpnRisky: []amfa.EventRow{
		{EventID: "ev-9", EventTime: time.Now(), IsVPN: true, RiskLevel: &risk, IPAddress: "9.9.9.9"},
	}}
	c := NewVPNRiskyCheck("tenant-1", singleRealm("master"), repo, time.Minute, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Severity != "warning" {
		t.Errorf("severity = %q, want warning", a.Severity)
	}
	if a.CheckType != "amfa-vpn-risky" {
		t.Errorf("check_type = %q, want amfa-vpn-risky", a.CheckType)
	}
	want := alertID("amfa-vpn-risky", "tenant-1", "ev-9")
	if a.AlertID != want {
		t.Errorf("AlertID = %q, want %q", a.AlertID, want)
	}
	if a.EventID == nil || *a.EventID != "ev-9" {
		t.Errorf("EventID = %v, want ev-9", a.EventID)
	}
}

func TestVPNRiskyCheck_UsesMinRisk3(t *testing.T) {
	repo := &checkRepoStub{}
	c := NewVPNRiskyCheck("tenant-1", singleRealm("master"), repo, time.Minute, testLogger{})
	if c.minRisk != 3 {
		t.Errorf("minRisk = %d, want 3 (hardcoded 'risk > 2')", c.minRisk)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run (in Docker): `go test ./amfacheck/`
Expected: FAIL — `undefined: NewVPNRiskyCheck`.

- [ ] **Step 3: Create `amfacheck/vpn_risky_check.go`**

```go
package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// vpnRiskyMinRisk is the inclusive lower risk bound for rule 3 (SQL
// risk_level >= 3), matching the brainstormed "risk > 2" semantics. Hardcoded;
// see the spec for how to make it configurable later.
const vpnRiskyMinRisk = 3

// VPNRiskyCheck implements rule 3: one warning alert per AMFA event that is
// VPN-flagged AND has risk >= 3.
type VPNRiskyCheck struct {
	tenantID string
	realmsFn RealmsFunc
	repo     amfa.Repository
	minRisk  int
	lookback time.Duration
	logger   Logger

	mu       sync.Mutex
	lastSeen map[string]time.Time
}

// NewVPNRiskyCheck constructs rule 3.
func NewVPNRiskyCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, lookback time.Duration, log Logger) *VPNRiskyCheck {
	return &VPNRiskyCheck{
		tenantID: tenantID,
		realmsFn: realmsFn,
		repo:     repo,
		minRisk:  vpnRiskyMinRisk,
		lookback: lookback,
		logger:   log.WithComponent("amfa-vpn-risky"),
		lastSeen: make(map[string]time.Time),
	}
}

func (c *VPNRiskyCheck) GetCheckType() string   { return "amfa-vpn-risky" }
func (c *VPNRiskyCheck) GetDescription() string { return "AMFA risky login from VPN/anonymizing IP (risk >= 3)" }

func (c *VPNRiskyCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	realms, err := c.realmsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("amfa-vpn-risky: load realms: %w", err)
	}
	var alerts []*domain.Alert
	for _, realm := range realms {
		since := c.getLastSeen(realm)
		events, err := c.repo.ListVPNRiskyEventsSince(ctx, realm, since, c.minRisk)
		if err != nil {
			return nil, fmt.Errorf("amfa-vpn-risky: realm=%s: %w", realm, err)
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

func (c *VPNRiskyCheck) toAlert(realm string, ev amfa.EventRow) *domain.Alert {
	eventID := ev.EventID
	md, _ := json.Marshal(map[string]any{
		"amfa_event_id": ev.EventID,
		"user_id":       ev.UserID,
		"ip_address":    ev.IPAddress,
		"client":        ev.Client,
		"country":       ev.Country,
		"is_vpn":        ev.IsVPN,
		"risk_level":    ev.RiskLevel,
		"realm_name":    realm,
		"min_risk":      c.minRisk,
	})
	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID("amfa-vpn-risky", c.tenantID, ev.EventID),
		Source:         domain.AlertSourceEvent,
		Type:           domain.AlertTypeSecurity,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          "AMFA: risky login from VPN/anonymizing IP",
		Description:    fmt.Sprintf("AMFA observed a risky (risk >= %d) authentication from a VPN/anonymizing IP in realm '%s' (event %s).", c.minRisk, realm, ev.EventID),
		ResourceType:   "amfa_event",
		ResourceID:     ev.EventID,
		ResourceName:   ev.EventID,
		RealmName:      realm,
		CheckType:      c.GetCheckType(),
		EventID:        &eventID,
		Recommendation: "Review the source IP and the user's recent activity. Anonymizing networks combined with elevated risk warrant a closer look at the session.",
		Metadata:       string(md),
	}
}

func (c *VPNRiskyCheck) getLastSeen(realm string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ts, ok := c.lastSeen[realm]; ok {
		return ts
	}
	return time.Now().Add(-c.lookback)
}

func (c *VPNRiskyCheck) setLastSeen(realm string, ts time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSeen[realm] = ts
}
```

- [ ] **Step 4: Run to verify it passes**

Run (in Docker): `go test ./amfacheck/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfacheck/vpn_risky_check.go amfacheck/vpn_risky_check_test.go
git commit -m "feat(amfacheck): add rule 3 VPNRiskyCheck"
```

---

## Task 8: Rule 2 — `RepeatedRiskyCheck` (per-user/day, warning)

**Files:**
- Create: `amfacheck/repeated_risky_check.go`
- Test: `amfacheck/repeated_risky_check_test.go`

- [ ] **Step 1: Write the failing test**

Create `amfacheck/repeated_risky_check_test.go`:

```go
package amfacheck

import (
	"context"
	"testing"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
)

func TestRepeatedRiskyCheck_OneAlertPerUserOverThreshold(t *testing.T) {
	repo := &checkRepoStub{userCounts: []amfa.UserRiskyCount{
		{UserID: "user-a", Count: 4},
		{UserID: "user-b", Count: 3},
	}}
	c := NewRepeatedRiskyCheck("tenant-1", singleRealm("master"), repo, 3, 24*time.Hour, 2, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Severity != "warning" {
		t.Errorf("severity = %q, want warning", a.Severity)
	}
	if a.CheckType != "amfa-repeated-risky" {
		t.Errorf("check_type = %q, want amfa-repeated-risky", a.CheckType)
	}
	if a.ResourceType != "amfa_user" {
		t.Errorf("resource_type = %q, want amfa_user", a.ResourceType)
	}
	if a.ResourceID != "user-a" {
		t.Errorf("resource_id = %q, want user-a", a.ResourceID)
	}
}

func TestRepeatedRiskyCheck_AlertIDIncludesUTCDate(t *testing.T) {
	repo := &checkRepoStub{userCounts: []amfa.UserRiskyCount{{UserID: "user-a", Count: 5}}}
	c := NewRepeatedRiskyCheck("tenant-1", singleRealm("master"), repo, 3, 24*time.Hour, 2, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	want := alertID("amfa-repeated-risky", "tenant-1", "master", "user-a", day)
	if alerts[0].AlertID != want {
		t.Errorf("AlertID = %q, want %q", alerts[0].AlertID, want)
	}
}

func TestRepeatedRiskyCheck_NoUsersNoAlerts(t *testing.T) {
	repo := &checkRepoStub{userCounts: nil}
	c := NewRepeatedRiskyCheck("tenant-1", singleRealm("master"), repo, 3, 24*time.Hour, 2, testLogger{})
	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run (in Docker): `go test ./amfacheck/`
Expected: FAIL — `undefined: NewRepeatedRiskyCheck`.

- [ ] **Step 3: Create `amfacheck/repeated_risky_check.go`**

```go
package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// RepeatedRiskyCheck implements rule 2: one warning alert per (realm, user, UTC
// day) when a user accumulates >= threshold events at risk >= minRisk within
// the rolling window. No per-realm checkpoint is needed: the count is a rolling
// window recomputed each tick, and the per-(user, day) AlertID provides dedup.
type RepeatedRiskyCheck struct {
	tenantID  string
	realmsFn  RealmsFunc
	repo      amfa.Repository
	threshold int
	window    time.Duration
	minRisk   int
	logger    Logger
}

// NewRepeatedRiskyCheck constructs rule 2.
func NewRepeatedRiskyCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, threshold int, window time.Duration, minRisk int, log Logger) *RepeatedRiskyCheck {
	return &RepeatedRiskyCheck{
		tenantID:  tenantID,
		realmsFn:  realmsFn,
		repo:      repo,
		threshold: threshold,
		window:    window,
		minRisk:   minRisk,
		logger:    log.WithComponent("amfa-repeated-risky"),
	}
}

func (c *RepeatedRiskyCheck) GetCheckType() string   { return "amfa-repeated-risky" }
func (c *RepeatedRiskyCheck) GetDescription() string { return "AMFA repeated risky attempts by one user within the window" }

func (c *RepeatedRiskyCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	realms, err := c.realmsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("amfa-repeated-risky: load realms: %w", err)
	}
	now := time.Now().UTC()
	since := now.Add(-c.window)
	day := now.Format("2006-01-02")

	var alerts []*domain.Alert
	for _, realm := range realms {
		counts, err := c.repo.CountRepeatedRiskyByUser(ctx, realm, since, c.minRisk, c.threshold)
		if err != nil {
			return nil, fmt.Errorf("amfa-repeated-risky: realm=%s: %w", realm, err)
		}
		for _, uc := range counts {
			alerts = append(alerts, c.toAlert(realm, day, uc))
		}
	}
	return alerts, nil
}

func (c *RepeatedRiskyCheck) toAlert(realm, day string, uc amfa.UserRiskyCount) *domain.Alert {
	md, _ := json.Marshal(map[string]any{
		"user_id":        uc.UserID,
		"count":          uc.Count,
		"threshold":      c.threshold,
		"min_risk_level": c.minRisk,
		"window":         c.window.String(),
		"realm_name":     realm,
		"utc_date":       day,
	})
	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID("amfa-repeated-risky", c.tenantID, realm, uc.UserID, day),
		Source:         domain.AlertSourceEvent,
		Type:           domain.AlertTypeSecurity,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          "AMFA: repeated risky login attempts by one user",
		Description:    fmt.Sprintf("User %s had %d risky (risk >= %d) authentication attempts in realm '%s' within %s (threshold: %d).", uc.UserID, uc.Count, c.minRisk, realm, c.window.String(), c.threshold),
		ResourceType:   "amfa_user",
		ResourceID:     uc.UserID,
		ResourceName:   uc.UserID,
		RealmName:      realm,
		CheckType:      c.GetCheckType(),
		Recommendation: "Review this user's recent authentication history. Repeated risky attempts may indicate credential stuffing, an account takeover attempt, or a misconfigured client.",
		Metadata:       string(md),
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run (in Docker): `go test ./amfacheck/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfacheck/repeated_risky_check.go amfacheck/repeated_risky_check_test.go
git commit -m "feat(amfacheck): add rule 2 RepeatedRiskyCheck"
```

---

## Task 9: Rule 4a — `LoginErrorCheck` (threshold/window, warning)

**Files:**
- Create: `amfacheck/login_error_check.go`
- Test: `amfacheck/login_error_check_test.go`

- [ ] **Step 1: Write the failing test**

Create `amfacheck/login_error_check_test.go`:

```go
package amfacheck

import (
	"context"
	"testing"
	"time"
)

func TestLoginErrorCheck_AlertWhenCountAtThreshold(t *testing.T) {
	repo := &checkRepoStub{typeCount: 5}
	c := NewLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Severity != "warning" {
		t.Errorf("severity = %q, want warning", a.Severity)
	}
	if a.CheckType != "amfa-login-error" {
		t.Errorf("check_type = %q, want amfa-login-error", a.CheckType)
	}
	if a.RealmName != "master" {
		t.Errorf("realm = %q, want master", a.RealmName)
	}
}

func TestLoginErrorCheck_NoAlertBelowThreshold(t *testing.T) {
	repo := &checkRepoStub{typeCount: 4}
	c := NewLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})
	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts below threshold, got %d", len(alerts))
	}
}

func TestLoginErrorCheck_AlertIDStableWithinWindow(t *testing.T) {
	repo := &checkRepoStub{typeCount: 10}
	c := NewLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

	a1, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute(1): %v", err)
	}
	a2, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute(2): %v", err)
	}
	// Two calls inside the same wall-clock 5-minute window must produce the
	// same AlertID (dedup). This is non-flaky except at a 5-minute boundary; if
	// it ever flakes, that's the boundary — re-run.
	if a1[0].AlertID != a2[0].AlertID {
		t.Errorf("AlertID not stable within window: %q != %q", a1[0].AlertID, a2[0].AlertID)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run (in Docker): `go test ./amfacheck/`
Expected: FAIL — `undefined: NewLoginErrorCheck`.

- [ ] **Step 3: Create `amfacheck/login_error_check.go`**

```go
package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// LoginErrorCheck implements rule 4a: a warning alert when the count of AMFA
// LOGIN_ERROR events in the current window-aligned bucket reaches the
// threshold, deduped per (realm, window bucket).
type LoginErrorCheck struct {
	tenantID  string
	realmsFn  RealmsFunc
	repo      amfa.Repository
	threshold int
	window    time.Duration
	logger    Logger
}

// NewLoginErrorCheck constructs rule 4a.
func NewLoginErrorCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, threshold int, window time.Duration, log Logger) *LoginErrorCheck {
	return &LoginErrorCheck{
		tenantID:  tenantID,
		realmsFn:  realmsFn,
		repo:      repo,
		threshold: threshold,
		window:    window,
		logger:    log.WithComponent("amfa-login-error"),
	}
}

func (c *LoginErrorCheck) GetCheckType() string   { return "amfa-login-error" }
func (c *LoginErrorCheck) GetDescription() string { return "AMFA excessive LOGIN_ERROR events in a window" }

func (c *LoginErrorCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	return runEventTypeThresholdCheck(ctx, eventTypeThresholdParams{
		tenantID:    c.tenantID,
		realmsFn:    c.realmsFn,
		repo:        c.repo,
		eventType:   "LOGIN_ERROR",
		threshold:   c.threshold,
		window:      c.window,
		checkType:   c.GetCheckType(),
		idPrefix:    "amfa-login-error",
		title:       "AMFA: excessive LOGIN_ERROR events",
		recommendation: "Investigate the burst of failed logins. Check for brute-force or credential-stuffing activity against this realm.",
	})
}
```

> The shared `runEventTypeThresholdCheck` helper and its params live in `amfacheck/event_type_threshold.go`, created in the next step (rule 4b reuses it — DRY).

- [ ] **Step 4: Create the shared threshold helper `amfacheck/event_type_threshold.go`**

```go
package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// eventTypeThresholdParams carries the per-rule differences for the shared
// window-threshold logic used by rules 4a and 4b.
type eventTypeThresholdParams struct {
	tenantID       string
	realmsFn       RealmsFunc
	repo           amfa.Repository
	eventType      string // AMFA event_type to count, e.g. "LOGIN_ERROR"
	threshold      int
	window         time.Duration
	checkType      string // GetCheckType() value
	idPrefix       string // AlertID prefix
	title          string
	recommendation string
}

// runEventTypeThresholdCheck counts events of a single type in the current
// window-aligned bucket per realm and emits one alert per realm whose count
// reaches the threshold. The AlertID is keyed on the window bucket so adjacent
// polls in the same window dedup to one alert row.
func runEventTypeThresholdCheck(ctx context.Context, p eventTypeThresholdParams) ([]*domain.Alert, error) {
	realms, err := p.realmsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: load realms: %w", p.checkType, err)
	}
	now := time.Now().UTC()
	bucket := windowBucket(now, p.window)
	end := bucket.Add(p.window)

	var alerts []*domain.Alert
	for _, realm := range realms {
		count, err := p.repo.CountByEventTypeInWindow(ctx, realm, p.eventType, bucket, end)
		if err != nil {
			return nil, fmt.Errorf("%s: realm=%s: %w", p.checkType, realm, err)
		}
		if count < int64(p.threshold) {
			continue
		}
		md, _ := json.Marshal(map[string]any{
			"event_type":   p.eventType,
			"count":        count,
			"threshold":    p.threshold,
			"window":       p.window.String(),
			"window_start": bucket.Format(time.RFC3339),
			"realm_name":   realm,
		})
		alerts = append(alerts, &domain.Alert{
			TenantID:       p.tenantID,
			AlertID:        alertID(p.idPrefix, p.tenantID, realm, bucket.Format(time.RFC3339)),
			Source:         domain.AlertSourceEvent,
			Type:           domain.AlertTypeSecurity,
			Severity:       domain.AlertSeverityWarning,
			Status:         domain.AlertStatusActive,
			Title:          p.title,
			Description:    fmt.Sprintf("%d %s events in realm '%s' within %s (threshold: %d).", count, p.eventType, realm, p.window.String(), p.threshold),
			ResourceType:   "realm",
			ResourceID:     realm,
			ResourceName:   realm,
			RealmName:      realm,
			CheckType:      p.checkType,
			Recommendation: p.recommendation,
			Metadata:       string(md),
		})
	}
	return alerts, nil
}
```

> Note: `login_error_check.go` imports `encoding/json` and `amfa` and `domain` only via the helper call; trim its import block to exactly what it uses — after Step 3+4 the file `login_error_check.go` only needs `context`, `time`, the `amfa` and `domain` packages are NOT directly referenced there, so its imports reduce to `context` and `time`. Run `gofmt`/`goimports` mentally: the only symbols `login_error_check.go` references are `context.Context`, `time.Duration`, `time.Minute` (none), `RealmsFunc`, `amfa.Repository`, `domain.Alert`. It DOES reference `amfa.Repository` (field type) and `domain.Alert` (return type) and `time.Duration`. So its imports are: `context`, `time`, `amfa`, `domain`. Remove `encoding/json` and `fmt` from `login_error_check.go` (they're only used by the helper). Verify with the build in Step 6.

- [ ] **Step 5: Fix `login_error_check.go` imports**

Ensure `amfacheck/login_error_check.go`'s import block is exactly:

```go
import (
	"context"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)
```

(`Execute` returns `[]*domain.Alert`; the struct holds `amfa.Repository` and `time.Duration`; `runEventTypeThresholdCheck` and `RealmsFunc` are same-package.)

- [ ] **Step 6: Run to verify it passes**

Run (in Docker): `go test ./amfacheck/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add amfacheck/login_error_check.go amfacheck/event_type_threshold.go amfacheck/login_error_check_test.go
git commit -m "feat(amfacheck): add rule 4a LoginErrorCheck + shared threshold helper"
```

---

## Task 10: Rule 4b — `ClientLoginErrorCheck`

**Files:**
- Create: `amfacheck/client_login_error_check.go`
- Test: `amfacheck/client_login_error_check_test.go`

- [ ] **Step 1: Write the failing test**

Create `amfacheck/client_login_error_check_test.go`:

```go
package amfacheck

import (
	"context"
	"testing"
	"time"
)

func TestClientLoginErrorCheck_AlertWhenAtThreshold(t *testing.T) {
	repo := &checkRepoStub{typeCount: 7}
	c := NewClientLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].CheckType != "amfa-client-login-error" {
		t.Errorf("check_type = %q, want amfa-client-login-error", alerts[0].CheckType)
	}
}

func TestClientLoginErrorCheck_DistinctAlertIDFromRule4a(t *testing.T) {
	repo := &checkRepoStub{typeCount: 9}
	a4a := NewLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})
	a4b := NewClientLoginErrorCheck("tenant-1", singleRealm("master"), repo, 5, 5*time.Minute, testLogger{})

	x, err := a4a.Execute(context.Background())
	if err != nil {
		t.Fatalf("4a Execute: %v", err)
	}
	y, err := a4b.Execute(context.Background())
	if err != nil {
		t.Fatalf("4b Execute: %v", err)
	}
	if x[0].AlertID == y[0].AlertID {
		t.Error("rule 4a and 4b must produce distinct AlertIDs for the same realm/window")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run (in Docker): `go test ./amfacheck/`
Expected: FAIL — `undefined: NewClientLoginErrorCheck`.

- [ ] **Step 3: Create `amfacheck/client_login_error_check.go`**

```go
package amfacheck

import (
	"context"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// ClientLoginErrorCheck implements rule 4b: identical to rule 4a but counts
// CLIENT_LOGIN_ERROR events and uses a distinct AlertID prefix.
type ClientLoginErrorCheck struct {
	tenantID  string
	realmsFn  RealmsFunc
	repo      amfa.Repository
	threshold int
	window    time.Duration
	logger    Logger
}

// NewClientLoginErrorCheck constructs rule 4b.
func NewClientLoginErrorCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, threshold int, window time.Duration, log Logger) *ClientLoginErrorCheck {
	return &ClientLoginErrorCheck{
		tenantID:  tenantID,
		realmsFn:  realmsFn,
		repo:      repo,
		threshold: threshold,
		window:    window,
		logger:    log.WithComponent("amfa-client-login-error"),
	}
}

func (c *ClientLoginErrorCheck) GetCheckType() string   { return "amfa-client-login-error" }
func (c *ClientLoginErrorCheck) GetDescription() string { return "AMFA excessive CLIENT_LOGIN_ERROR events in a window" }

func (c *ClientLoginErrorCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	return runEventTypeThresholdCheck(ctx, eventTypeThresholdParams{
		tenantID:       c.tenantID,
		realmsFn:       c.realmsFn,
		repo:           c.repo,
		eventType:      "CLIENT_LOGIN_ERROR",
		threshold:      c.threshold,
		window:         c.window,
		checkType:      c.GetCheckType(),
		idPrefix:       "amfa-client-login-error",
		title:          "AMFA: excessive CLIENT_LOGIN_ERROR events",
		recommendation: "Investigate the burst of client authentication failures. Check client credentials, service-account settings, and client configuration for this realm.",
	})
}
```

- [ ] **Step 4: Run to verify it passes**

Run (in Docker): `go test ./amfacheck/`
Expected: PASS (all amfacheck unit tests across all five rules + service + hash).

- [ ] **Step 5: Commit**

```bash
git add amfacheck/client_login_error_check.go amfacheck/client_login_error_check_test.go
git commit -m "feat(amfacheck): add rule 4b ClientLoginErrorCheck"
```

---

## Task 11: fx module — per-tenant wiring

**Files:**
- Create: `internal/fx/amfacheck.go`
- Create: `internal/fx/amfacheck_test.go`
- Modify: `internal/fx/app.go` (add `AmfaCheckModule` to the module list)

This module builds one `amfacheck.Service` per tenant that has `amfa.enabled == true` AND `keycloak.global.amfa_checker.enabled == true`, adapts `alerts.Repository`/`notifications.Service`/`*logger.Logger` to the `amfacheck` interfaces, builds the per-tenant `realmsFn` from `keycloak.Service.ListRealms`, and registers fx lifecycle hooks.

- [ ] **Step 1: Write the failing test for the pure builder + adapters**

Create `internal/fx/amfacheck_test.go`:

```go
package fx

import (
	"context"
	"testing"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfacheck"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// minimal AlertStore/Notifier doubles to satisfy buildAmfaCheckServices.
type acFakeAlertStore struct{}

func (acFakeAlertStore) GetByAlertID(_ context.Context, _, _ string) (*domain.Alert, error) {
	return nil, nil
}
func (acFakeAlertStore) Save(_ context.Context, _ *domain.Alert) error { return nil }

type acFakeNotifier struct{}

func (acFakeNotifier) NotifyAlert(_ context.Context, _ *domain.Alert) error { return nil }

func acRealms(_ context.Context, _ string) ([]string, error) { return []string{"master"}, nil }

func TestBuildAmfaCheckServices_SkipsWhenGloballyDisabled(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Keycloak.Global.AmfaChecker.Enabled = false
	cfg.Keycloak.Tenants = map[string]config.TenantConfig{
		"t1": {Amfa: &config.AmfaTenantConfig{Enabled: true}},
	}

	svcs := buildAmfaCheckServices(cfg, acRealmRepoStub{}, acFakeAlertStore{}, acFakeNotifier{}, nil)
	if len(svcs) != 0 {
		t.Errorf("expected 0 services when globally disabled, got %d", len(svcs))
	}
}

func TestBuildAmfaCheckServices_OnePerEnabledTenant(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Keycloak.Global.AmfaChecker.Enabled = true
	cfg.Keycloak.Global.AmfaChecker.Checks.RiskRejected.Enabled = true
	cfg.Keycloak.Tenants = map[string]config.TenantConfig{
		"t1": {Amfa: &config.AmfaTenantConfig{Enabled: true}},
		"t2": {Amfa: &config.AmfaTenantConfig{Enabled: false}},
		"t3": {Amfa: nil},
	}

	svcs := buildAmfaCheckServices(cfg, acRealmRepoStub{}, acFakeAlertStore{}, acFakeNotifier{}, nil)
	if len(svcs) != 1 {
		t.Errorf("expected 1 service (only t1), got %d", len(svcs))
	}
}

// compile-time: the adapters satisfy the amfacheck interfaces.
var (
	_ amfacheck.AlertStore          = acFakeAlertStore{}
	_ amfacheck.NotificationService = acFakeNotifier{}
)

// acRealmRepoStub satisfies the realmLister contract used by the builder.
type acRealmRepoStub struct{}

func (acRealmRepoStub) ListRealms(_ context.Context, _ string) ([]*domain.KeycloakRealmInfo, error) {
	return []*domain.KeycloakRealmInfo{{RealmName: "master"}}, nil
}
```

> **Important — discover the exact constructor inputs first.** Before writing the module, run:
> Run: `grep -n "AmfaModule\|alerts.Repository\|alerts.Service\|notifications.Service\|keycloak.Service" internal/fx/handlers.go internal/fx/alerts.go internal/fx/notifications.go internal/fx/amfa.go`
> Use the result to confirm the concrete fx-provided types for the alert store (`alerts.Repository` interface), notifier (`notifications.Service`), and realm lister (`keycloak.Service`). The provider signature in Step 3 must inject exactly those provided types. If `alerts.Repository` is the provided type, inject it directly; if only `alerts.Service` is provided, inject that and adapt (it also exposes `SaveAlert`/`GetAlert`).

- [ ] **Step 2: Run to verify it fails**

Run (in Docker): `go test ./internal/fx/`
Expected: FAIL — `undefined: buildAmfaCheckServices`.

- [ ] **Step 3: Create `internal/fx/amfacheck.go`**

```go
package fx

import (
	"context"

	"go.uber.org/fx"

	"repos.defensepoint.com/iam/oss-pmt/kmt/alerts"
	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
	"repos.defensepoint.com/iam/oss-pmt/kmt/amfacheck"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/keycloak"
	"repos.defensepoint.com/iam/oss-pmt/kmt/notifications"
)

// AmfaCheckModule wires the AMFA risk-alert checker: one per-tenant
// amfacheck.Service for every tenant that has amfa.enabled=true AND
// keycloak.global.amfa_checker.enabled=true. Each service owns one goroutine,
// started/stopped via fx lifecycle hooks. Safe to install when no tenants
// qualify: the slice is simply empty.
var AmfaCheckModule = fx.Module("amfacheck",
	fx.Provide(provideAmfaCheckServices),
	fx.Invoke(startAmfaCheckServices),
)

// realmLister is the slice of keycloak.Service that the per-tenant realmsFn
// needs. keycloak.Service satisfies it.
type realmLister interface {
	ListRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error)
}

// amfaCheckAlertStore adapts the real alerts.Repository method names
// (GetByAlertID / Save) to the amfacheck.AlertStore contract
// (GetAlertByID / SaveAlert).
type amfaCheckAlertStore struct{ repo alerts.Repository }

func (a amfaCheckAlertStore) GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	return a.repo.GetByAlertID(ctx, tenantID, alertID)
}
func (a amfaCheckAlertStore) SaveAlert(ctx context.Context, alert *domain.Alert) error {
	return a.repo.Save(ctx, alert)
}

// amfacheckLoggerAdapter adapts *logger.Logger to amfacheck.Logger, converting
// the variadic key/value args into logger.Field values so structured fields
// (tenant_id, alert_id, ...) survive. Improves on eventscheck's adapter, which
// dropped args.
type amfacheckLoggerAdapter struct{ log *logger.Logger }

func (a *amfacheckLoggerAdapter) toFields(args []any) []logger.Field {
	fields := make([]logger.Field, 0, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		fields = append(fields, logger.Any(key, args[i+1]))
	}
	return fields
}
func (a *amfacheckLoggerAdapter) Info(msg string, args ...any)  { a.log.Info(msg, a.toFields(args)...) }
func (a *amfacheckLoggerAdapter) Error(msg string, args ...any) { a.log.Error(msg, a.toFields(args)...) }
func (a *amfacheckLoggerAdapter) Warn(msg string, args ...any)  { a.log.Warn(msg, a.toFields(args)...) }
func (a *amfacheckLoggerAdapter) Debug(msg string, args ...any) { a.log.Debug(msg, a.toFields(args)...) }
func (a *amfacheckLoggerAdapter) WithComponent(name string) amfacheck.Logger {
	return &amfacheckLoggerAdapter{log: a.log.WithComponent(name)}
}

// AmfaCheckParams gathers the fx-provided dependencies.
type AmfaCheckParams struct {
	fx.In

	Config    *config.AppConfig
	Registry  *amfa.Registry
	Alerts    alerts.Repository
	Notifier  notifications.Service
	Keycloak  keycloak.Service
	Logger    *logger.Logger
}

// provideAmfaCheckServices is the fx provider; it delegates to the pure
// buildAmfaCheckServices for testability.
func provideAmfaCheckServices(p AmfaCheckParams) []amfacheck.Service {
	return buildAmfaCheckServices(p.Config, p.Keycloak, amfaCheckAlertStore{repo: p.Alerts}, p.Notifier, p.Logger)
}

// buildAmfaCheckServices is the unit-testable core. log may be nil in tests.
func buildAmfaCheckServices(
	cfg *config.AppConfig,
	realms realmLister,
	alertStore amfacheck.AlertStore,
	notifier amfacheck.NotificationService,
	log *logger.Logger,
) []amfacheck.Service {
	amfaCfg := cfg.Keycloak.Global.AmfaChecker
	if !amfaCfg.Enabled {
		logInfo(log, "AMFA checker globally disabled; no alert services constructed")
		return nil
	}
	amfaCfg.ApplyDefaults()

	adaptedLog := amfacheck.Logger(noopAmfacheckLogger{})
	if log != nil {
		adaptedLog = &amfacheckLoggerAdapter{log: log}
	}

	var services []amfacheck.Service
	for tenantID, tcfg := range cfg.Keycloak.Tenants {
		if tcfg.Amfa == nil || !tcfg.Amfa.Enabled {
			continue
		}
		tenantID := tenantID // capture

		realmsFn := func(ctx context.Context) ([]string, error) {
			rows, err := realms.ListRealms(ctx, tenantID)
			if err != nil {
				return nil, err
			}
			out := make([]string, 0, len(rows))
			for _, r := range rows {
				out = append(out, r.RealmName)
			}
			return out, nil
		}

		checks := buildAmfaChecks(tenantID, amfacheck.RealmsFunc(realmsFn), cfg, amfaCfg, adaptedLog)
		svc := amfacheck.NewService(tenantID, checks, alertStore, notifier, adaptedLog, amfaCfg.PollInterval)
		services = append(services, svc)
		logInfo(log, "amfacheck service constructed",
			logger.Str("tenant_id", tenantID),
			logger.Int("checks", len(checks)),
			logger.Str("poll_interval", amfaCfg.PollInterval.String()))
	}
	logInfo(log, "AMFA checker services constructed", logger.Int("count", len(services)))
	return services
}

// buildAmfaChecks builds the enabled checks for one tenant. The repository is
// looked up from the registry; if absent, no checks are built (logged).
func buildAmfaChecks(
	tenantID string,
	realmsFn amfacheck.RealmsFunc,
	cfg *config.AppConfig,
	amfaCfg config.AmfaCheckerConfig,
	log amfacheck.Logger,
) []amfacheck.Check {
	// NOTE: the registry lookup is intentionally done by the provider, not here,
	// to keep this function pure for testing. See provideAmfaCheckServices /
	// the registry wiring below — adjust if you prefer the lookup inline.
	return nil // replaced below in Step 4
}

func startAmfaCheckServices(lc fx.Lifecycle, services []amfacheck.Service, log *logger.Logger) {
	for _, s := range services {
		s := s
		// The poll loop must run on a long-lived context, NOT fx's OnStart
		// context: fx cancels the OnStart context once startup completes, which
		// would immediately stop the ticker goroutine (verified in live testing
		// — the loop logged "polling cancelled" ~15s after boot). We own a
		// background context here and cancel it in OnStop alongside Stop()'s
		// graceful drain so in-flight queries unwind on shutdown.
		var cancel context.CancelFunc
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				var runCtx context.Context
				runCtx, cancel = context.WithCancel(context.Background())
				return s.Start(runCtx)
			},
			OnStop: func(context.Context) error {
				if cancel != nil {
					cancel()
				}
				return s.Stop()
			},
		})
	}
	logInfo(log, "amfacheck lifecycle hooks registered", logger.Int("service_count", len(services)))
}

// noopAmfacheckLogger is used when log is nil (unit tests).
type noopAmfacheckLogger struct{}

func (noopAmfacheckLogger) Info(string, ...any)                     {}
func (noopAmfacheckLogger) Error(string, ...any)                    {}
func (noopAmfacheckLogger) Warn(string, ...any)                     {}
func (noopAmfacheckLogger) Debug(string, ...any)                    {}
func (n noopAmfacheckLogger) WithComponent(string) amfacheck.Logger { return n }
```

> The `domain` import is referenced by `realmLister` and the adapters — add `"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"` to the import block. `logInfo`/`logWarn` already exist in `internal/fx/amfa.go` (nil-tolerant helpers) — reuse them; do NOT redefine.

- [ ] **Step 4: Replace the placeholder `buildAmfaChecks` body and add the registry lookup**

The registry lookup must happen per tenant. Refactor so `buildAmfaCheckServices` takes the registry and `buildAmfaChecks` receives the resolved `amfa.Repository`. Update the two functions as follows.

Change the `buildAmfaCheckServices` signature to accept the registry, and resolve the repo inside the loop:

```go
func buildAmfaCheckServices(
	cfg *config.AppConfig,
	registry *amfa.Registry,
	realms realmLister,
	alertStore amfacheck.AlertStore,
	notifier amfacheck.NotificationService,
	log *logger.Logger,
) []amfacheck.Service {
	amfaCfg := cfg.Keycloak.Global.AmfaChecker
	if !amfaCfg.Enabled {
		logInfo(log, "AMFA checker globally disabled; no alert services constructed")
		return nil
	}
	amfaCfg.ApplyDefaults()

	adaptedLog := amfacheck.Logger(noopAmfacheckLogger{})
	if log != nil {
		adaptedLog = &amfacheckLoggerAdapter{log: log}
	}

	var services []amfacheck.Service
	for tenantID, tcfg := range cfg.Keycloak.Tenants {
		if tcfg.Amfa == nil || !tcfg.Amfa.Enabled {
			continue
		}
		tenantID := tenantID

		repo, err := registry.RepositoryFor(tenantID)
		if err != nil {
			logWarn(log, "amfacheck: skipping tenant with no AMFA repository",
				logger.Str("tenant_id", tenantID), logger.Err(err))
			continue
		}

		realmsFn := func(ctx context.Context) ([]string, error) {
			rows, lerr := realms.ListRealms(ctx, tenantID)
			if lerr != nil {
				return nil, lerr
			}
			out := make([]string, 0, len(rows))
			for _, r := range rows {
				out = append(out, r.RealmName)
			}
			return out, nil
		}

		checks := buildAmfaChecks(tenantID, amfacheck.RealmsFunc(realmsFn), repo, amfaCfg, adaptedLog)
		svc := amfacheck.NewService(tenantID, checks, alertStore, notifier, adaptedLog, amfaCfg.PollInterval)
		services = append(services, svc)
		logInfo(log, "amfacheck service constructed",
			logger.Str("tenant_id", tenantID),
			logger.Int("checks", len(checks)),
			logger.Str("poll_interval", amfaCfg.PollInterval.String()))
	}
	logInfo(log, "AMFA checker services constructed", logger.Int("count", len(services)))
	return services
}

func buildAmfaChecks(
	tenantID string,
	realmsFn amfacheck.RealmsFunc,
	repo amfa.Repository,
	amfaCfg config.AmfaCheckerConfig,
	log amfacheck.Logger,
) []amfacheck.Check {
	c := amfaCfg.Checks
	var out []amfacheck.Check
	if c.RiskRejected.Enabled {
		out = append(out, amfacheck.NewRiskRejectedCheck(tenantID, realmsFn, repo, amfaCfg.PollInterval, log))
	}
	if c.RepeatedRisky.Enabled {
		out = append(out, amfacheck.NewRepeatedRiskyCheck(tenantID, realmsFn, repo,
			c.RepeatedRisky.Threshold, c.RepeatedRisky.Window, c.RepeatedRisky.MinRiskLevel, log))
	}
	if c.VPNRisky.Enabled {
		out = append(out, amfacheck.NewVPNRiskyCheck(tenantID, realmsFn, repo, amfaCfg.PollInterval, log))
	}
	if c.LoginError.Enabled {
		out = append(out, amfacheck.NewLoginErrorCheck(tenantID, realmsFn, repo,
			c.LoginError.Threshold, c.LoginError.Window, log))
	}
	if c.ClientLoginError.Enabled {
		out = append(out, amfacheck.NewClientLoginErrorCheck(tenantID, realmsFn, repo,
			c.ClientLoginError.Threshold, c.ClientLoginError.Window, log))
	}
	return out
}
```

Update `provideAmfaCheckServices` to pass the registry:

```go
func provideAmfaCheckServices(p AmfaCheckParams) []amfacheck.Service {
	return buildAmfaCheckServices(p.Config, p.Registry, p.Keycloak,
		amfaCheckAlertStore{repo: p.Alerts}, p.Notifier, p.Logger)
}
```

Update the test from Step 1 to pass a registry. Replace the two `buildAmfaCheckServices(cfg, acRealmRepoStub{}, ...)` calls with a registry argument. Add this helper to the test file and update the calls:

```go
func acRegistry(tenantIDs ...string) *amfa.Registry {
	reg := amfa.NewRegistry()
	for _, id := range tenantIDs {
		reg.Register(id, acRepoStub{})
	}
	return reg
}

// acRepoStub is a no-op amfa.Repository for registry wiring tests.
type acRepoStub struct{}

func (acRepoStub) ListEvents(_ context.Context, _ amfa.ListEventsOptions) (*amfa.ListEventsResult, error) {
	return &amfa.ListEventsResult{}, nil
}
func (acRepoStub) GetStats(_ context.Context, _ amfa.StatsOptions) (amfa.Stats, error) {
	return amfa.Stats{}, nil
}
func (acRepoStub) GetGeoBuckets(_ context.Context, _ amfa.GeoOptions) ([]amfa.GeoBucket, error) {
	return nil, nil
}
func (acRepoStub) ListRejectedEventsSince(_ context.Context, _ string, _ time.Time) ([]amfa.EventRow, error) {
	return nil, nil
}
func (acRepoStub) ListVPNRiskyEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]amfa.EventRow, error) {
	return nil, nil
}
func (acRepoStub) CountRepeatedRiskyByUser(_ context.Context, _ string, _ time.Time, _, _ int) ([]amfa.UserRiskyCount, error) {
	return nil, nil
}
func (acRepoStub) CountByEventTypeInWindow(_ context.Context, _, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}
```

Then in both tests call:
```go
svcs := buildAmfaCheckServices(cfg, acRegistry("t1", "t2", "t3"), acRealmRepoStub{}, acFakeAlertStore{}, acFakeNotifier{}, nil)
```
(For the globally-disabled test, the registry contents don't matter; `acRegistry("t1")` is fine.) Add `"time"` and the `amfa` import to the test file's import block.

- [ ] **Step 5: Run to verify it passes**

Run (in Docker): `go test ./internal/fx/`
Expected: PASS (both builder tests; existing fx tests unaffected).

- [ ] **Step 6: Register the module in `app.go`**

In `internal/fx/app.go`, in the `baseOpts` slice, add `AmfaCheckModule,` on the line immediately after `EventsCheckModule,`:

```go
		ConfigCheckModule,
		EventsCheckModule,
		AmfaCheckModule,
		AuthModule,
```

- [ ] **Step 7: Verify the whole app graph compiles and the full suite passes**

Run (in Docker): `go build ./...`
Expected: PASS (fx graph resolves; `keycloak.Service`, `alerts.Repository`, `notifications.Service`, `*amfa.Registry`, `*config.AppConfig`, `*logger.Logger` are all already provided).
Run (in Docker): `go test -short ./...`
Expected: PASS.

> If `go build ./...` reports that `alerts.Repository` or `keycloak.Service` is "not provided" / "not in the container", check the discovery output from Task 11 Step 1 and adjust `AmfaCheckParams` to inject the type that IS provided (e.g. inject `alerts.Service` and adapt its `SaveAlert`/`GetAlert`, or inject `keycloak.Service` which is confirmed present in `internal/fx/handlers.go`). This is the one wiring risk; the fix is local to `AmfaCheckParams` and the adapter.

- [ ] **Step 8: Commit**

```bash
git add internal/fx/amfacheck.go internal/fx/amfacheck_test.go internal/fx/app.go
git commit -m "feat(amfacheck): wire per-tenant services via fx module"
```

---

## Task 12: Integration test (DB-backed, gated)

**Files:**
- Create: `amfacheck/integration_test.go`

This test is gated by `//go:build integration` and skips without `AMFA_TEST_DSN`. It exercises one full tick against a real AMFA DB and a real POINT alerts repo. On this host it will only compile + skip; it is the executable spec for a CI/staging run.

- [ ] **Step 1: Create `amfacheck/integration_test.go`**

```go
//go:build integration
// +build integration

package amfacheck_test

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfacheck"
	amfapg "repos.defensepoint.com/iam/oss-pmt/kmt/amfa/postgres"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// memAlertStore is an in-memory amfacheck.AlertStore for the integration test.
type memAlertStore struct {
	saved map[string]*domain.Alert
}

func (m *memAlertStore) GetAlertByID(_ context.Context, _, alertID string) (*domain.Alert, error) {
	return m.saved[alertID], nil
}
func (m *memAlertStore) SaveAlert(_ context.Context, a *domain.Alert) error {
	cp := *a
	m.saved[a.AlertID] = &cp
	return nil
}

type nopNotifier struct{}

func (nopNotifier) NotifyAlert(_ context.Context, _ *domain.Alert) error { return nil }

type itLogger struct{}

func (itLogger) Info(string, ...any)                   {}
func (itLogger) Error(string, ...any)                  {}
func (itLogger) Warn(string, ...any)                   {}
func (itLogger) Debug(string, ...any)                  {}
func (l itLogger) WithComponent(string) amfacheck.Logger { return l }

func openAmfaDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AMFA_TEST_DSN")
	if dsn == "" {
		t.Skip("AMFA_TEST_DSN not set; skipping amfacheck integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect AMFA DB: %v", err)
	}
	return db
}

// TestAmfaCheck_OneTick_AgainstRealDB runs one tick of the risk-rejected check
// against a seeded AMFA DB and asserts the run completes and any produced
// alerts are well-formed. Seed data is expected to contain at least one
// pre_auth_risk_decision=4 event in realm "master" within the lookback window
// for a positive assertion; otherwise the test still validates the no-error
// path. Adjust the realm to match your seed.
func TestAmfaCheck_OneTick_AgainstRealDB(t *testing.T) {
	db := openAmfaDB(t)
	repo := amfapg.NewRepository(db)

	realmsFn := amfacheck.RealmsFunc(func(_ context.Context) ([]string, error) {
		return []string{"master"}, nil
	})
	check := amfacheck.NewRiskRejectedCheck("tenant-it", realmsFn, repo, time.Hour, itLogger{})

	store := &memAlertStore{saved: map[string]*domain.Alert{}}
	svc := amfacheck.NewService("tenant-it", []amfacheck.Check{check}, store, nopNotifier{}, itLogger{}, time.Hour)

	if err := svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}
	for id, a := range store.saved {
		if a.Severity != domain.AlertSeverityCritical {
			t.Errorf("alert %s severity = %q, want critical", id, a.Severity)
		}
		if a.Source != domain.AlertSourceEvent {
			t.Errorf("alert %s source = %q, want event", id, a.Source)
		}
	}
	t.Logf("integration tick produced %d alert(s)", len(store.saved))
}
```

- [ ] **Step 2: Verify it compiles and skips**

Run (in Docker): `go test -tags=integration ./amfacheck/`
Expected: PASS with `--- SKIP` (no `AMFA_TEST_DSN`). Confirms the integration harness compiles against the real package API.

- [ ] **Step 3: Commit**

```bash
git add amfacheck/integration_test.go
git commit -m "test(amfacheck): add DB-gated integration test for one tick"
```

---

## Task 13: Config example + docs

**Files:**
- Modify: `config.yaml.example` (add `amfa_checker` block under `keycloak.global`)
- Modify: `docs/amfa-integration.md` (Alerts section + troubleshooting)
- Modify: `docs/architecture.md` (brief amfacheck/ note)

- [ ] **Step 1: Add the documented config block to `config.yaml.example`**

Under `keycloak: global:` (the same level as the existing `event_checker:` block — locate it with `grep -n "event_checker" config.yaml.example`), add:

```yaml
      # AMFA risk-alert checker. Converts high-risk AMFA login events into POINT
      # alerts that flow through the normal alert lifecycle and notification
      # fan-out. Disabled by default; opt in per check. See docs/amfa-integration.md.
      amfa_checker:
        enabled: false            # MASTER SWITCH — off by default
        poll_interval: 1m
        checks:
          risk_rejected:      { enabled: true }                                        # rule 1: risk-engine rejections (critical)
          repeated_risky:     { enabled: true, threshold: 3, window: 24h, min_risk_level: 2 }  # rule 2: repeated risky attempts per user/day
          vpn_risky:          { enabled: true }                                        # rule 3: risky login from VPN/anonymizing IP
          login_error:        { enabled: true, threshold: 5, window: 5m }              # rule 4a: LOGIN_ERROR burst
          client_login_error: { enabled: true, threshold: 5, window: 5m }              # rule 4b: CLIENT_LOGIN_ERROR burst
```

- [ ] **Step 2: Add an Alerts section to `docs/amfa-integration.md`**

Append a new section. Include: what each rule detects and its severity (rule 1 → critical; rules 2/3/4a/4b → warning); the config block above; the expected startup log lines (`AMFA checker services constructed count=N`, `amfacheck service starting tenant_id=<id> ...`, or `AMFA checker globally disabled; ...` when off); a manual smoke test (trigger one rejection via AMFA's `/decision` endpoint, wait one poll interval, see the alert on POINT's Alerts page and a notification in the configured Slack channel); a rollback note (`enabled: false` + restart); and a troubleshooting table with at least these rows:

| Symptom | Likely cause | Action |
|---|---|---|
| No `amfacheck service starting` log lines | `amfa_checker.enabled: false` (default) | Set it true and restart |
| Services start but no alerts ever | No qualifying events, or all per-check toggles disabled | Verify check toggles; confirm risky events exist in the window |
| `skipping tenant with no AMFA repository` warning | Tenant has `amfa.enabled: true` but its AMFA DB is unreachable | Check the tenant's `amfa.database` connection |
| Alert flood right after enabling | Backlog of historical risky events within the first-tick window | Expected for per-event rules; use the Alerts page silence manager to suppress noisy IPs/realms |
| Rule 2 misses a burst spanning midnight UTC | Day boundary is UTC; counts split across two days | Documented limitation; tune `window`/`threshold` if needed |

- [ ] **Step 3: Add a brief note to `docs/architecture.md`**

In the AMFA-related section (find it with `grep -n -i "amfa" docs/architecture.md`), add a short paragraph noting the `amfacheck/` package: a per-tenant poll loop, sibling to `eventscheck/`, that turns high-risk AMFA events into `domain.Alert`s via the existing alerts/notifications infrastructure; reads AMFA's DB through additive `amfa.Repository` methods; deliberately parallel to `eventscheck/` rather than a shared abstraction (see the design spec for the rationale).

- [ ] **Step 4: Verify the config example still parses (optional sanity)**

Run (in Docker): `go test -short ./internal/config/`
Expected: PASS (if the config package has an example-loading test it stays green; otherwise this just confirms no regression).

- [ ] **Step 5: Commit**

```bash
git add config.yaml.example docs/amfa-integration.md docs/architecture.md
git commit -m "docs(amfa): document amfa_checker config, alerts, and troubleshooting"
```

---

## Task 14: Full-suite verification

- [ ] **Step 1: Run the complete backend suite**

Run (in Docker): `go build ./...`
Expected: PASS.
Run (in Docker): `go vet ./...`
Expected: PASS.
Run (in Docker): `go test -short ./...`
Expected: PASS — all packages green, including `amfacheck`, `amfa`, `internal/config`, `internal/fx`.

- [ ] **Step 2: Confirm integration tests compile (and skip without DSN)**

Run (in Docker): `go test -tags=integration ./amfacheck/ ./amfa/postgres/`
Expected: PASS with `--- SKIP` lines (no `AMFA_TEST_DSN`).

- [ ] **Step 3: Final review checklist**

- [ ] No existing method signatures changed (only additive interface members + new files).
- [ ] No `web/` changes.
- [ ] Master switch defaults to off in `config.yaml.example`.
- [ ] All five rules wired in `buildAmfaChecks` with correct severities (rule 1 critical; rest warning).
- [ ] `AmfaCheckModule` present in `internal/fx/app.go`.

- [ ] **Step 4: Commit (if any cleanup was needed)**

```bash
git add -A
git commit -m "chore(amfacheck): final verification cleanup"
```

---

## Self-review notes (author)

- **Spec coverage:** rules 1–4b → Tasks 6–10; config → Task 1; repository methods → Tasks 2–3; service/dedup/lifecycle → Task 5; fx per-tenant wiring + realm discovery + lifecycle → Task 11; error handling (repo errors propagate, one check doesn't block others, save-fail doesn't notify) → Tasks 5–6 tests; in-memory checkpoint + first-tick window → Tasks 6/7; window bucketing dedup → Tasks 4/9; testing strategy (unit per check, service test, gated integration) → Tasks 4–12; deployment/config/logs/rollback/troubleshooting → Task 13.
- **Known deviations from the spec's illustrative code are listed in the "Divergences" section** and are deliberate, to match real codebase names/types and keep the change additive and decoupled. The behavior (rules, severities, dedup keys, config layout) matches the spec exactly.
- **One residual wiring risk** (Task 11 Step 7): the exact fx-provided types for the alert store and realm lister. The discovery step (Task 11 Step 1) and the fallback note make this self-correcting during execution.
