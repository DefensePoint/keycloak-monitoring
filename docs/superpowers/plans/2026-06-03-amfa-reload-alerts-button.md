# AMFA "Reload Alerts" Button Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Reload AMFA alerts" button to the Alerts page that triggers the AMFA risk-alert checker on demand for the current tenant (then refetches), shown only when the checker is enabled for that tenant.

**Architecture:** A small `amfacheck.Runner` (tenant→Service map) exposes `Enabled`/`RunNow`. The existing `AmfaHandlers` gains two routes - `GET /amfa-checker` (capability) and `POST /amfa-checker/run` (trigger) - under the same auth + `amfa:read` group it already uses. `RunNow` calls the checker's existing `RunCheckNow`, so no check/alert logic changes. The frontend adds a capability query + a button next to Refresh that POSTs then refetches.

**Tech Stack:** Go 1.25 (Uber fx, chi router, GORM), React + TypeScript + MUI + React Query + vitest. Builds on the AMFA Risk Alerts feature (`amfacheck/`).

---

## ⚠️ Execution notes (read before starting)

1. **No Go toolchain on this host - run Go in Docker.** Convenience function (run from repo root `/Users/user/work-dir/point-monitoring-platform`):
   ```bash
   gotest() { docker run --rm -v "$PWD:/app" -v go-mod-cache:/go/pkg/mod -w /app \
     golang:1.25.1-alpine sh -c "apk add --no-cache git build-base >/dev/null 2>&1 && $*"; }
   # usage: gotest go test ./amfacheck/
   ```
2. **Frontend tests run with vitest.** If the host's rollup binary errors (`@rollup/rollup-darwin-arm64`), run inside Docker:
   ```bash
   fetest() { docker run --rm -v "$PWD/web:/web" -w /web node:20-alpine \
     sh -c "npm ci >/dev/null 2>&1 || npm install >/dev/null 2>&1; npx vitest run $*"; }
   ```
   Otherwise `cd web && npx vitest run <path>` works.
3. **No git commits this session.** Each "Commit" step is a CHECKPOINT ONLY: run the tests, confirm green, do NOT run `git commit`. (If this plan is executed later with commit authorization, perform the commits as written.)
4. **Disk note:** rebuilding the server image generates a large Docker build cache. If a build or a DB container fails with "No space left on device", reclaim it with `docker builder prune -af`.
5. **No em-dashes** in any code/comment/doc you write; use a hyphen.

---

## File Structure

**New:**
```
amfacheck/runner.go                          Runner (tenant->Service lookup) + ErrCheckerNotEnabled
amfacheck/runner_test.go                     unit tests
```

**Modified (additive):**
```
internal/fx/amfacheck.go                     build map + provide *amfacheck.Runner; start hooks from Runner
internal/fx/amfacheck_test.go                builder tests now expect a map
internal/fx/amfa.go                          inject *amfacheck.Runner into AmfaHandlers
internal/http/chi/amfa_handlers.go           + Runner field, + 2 handler methods, + 2 routes
internal/http/chi/amfa_handlers_test.go      handler tests for the 2 new methods
web/src/features/alerts/services/alertService.ts        + getAmfaCheckerStatus, reloadAmfaAlerts
web/src/features/alerts/components/AlertsFilters.tsx     + "Reload AMFA alerts" button
web/src/features/alerts/components/AlertsFilters.test.tsx  button tests
web/src/features/alerts/pages/AlertsPage.tsx            capability query + reload handler + wiring
docs/amfa-integration.md                     document the button + endpoints
```

**Not modified:** the `alerts/` package, the alert lifecycle, `configuration_alerts` schema, the `GET /alerts` read path, the existing Refresh button, and the `amfacheck` check/service logic (only the fx provider shape changes to expose the Runner).

---

## Task 1: `amfacheck.Runner`

**Files:**
- Create: `amfacheck/runner.go`
- Test: `amfacheck/runner_test.go`

- [ ] **Step 1: Write the failing test**

Create `amfacheck/runner_test.go`:

```go
package amfacheck

import (
	"context"
	"errors"
	"testing"
)

// runnerFakeService is a minimal Service for Runner tests; only RunCheckNow is exercised.
type runnerFakeService struct {
	calls int
	err   error
}

func (f *runnerFakeService) Start(context.Context) error { return nil }
func (f *runnerFakeService) Stop() error                 { return nil }
func (f *runnerFakeService) AddCheck(Check)              {}
func (f *runnerFakeService) RunCheckNow(context.Context) error {
	f.calls++
	return f.err
}

func TestRunner_Enabled(t *testing.T) {
	r := NewRunner(map[string]Service{"t1": &runnerFakeService{}})
	if !r.Enabled("t1") {
		t.Error("expected t1 enabled")
	}
	if r.Enabled("t2") {
		t.Error("expected t2 not enabled")
	}
}

func TestRunner_RunNow_DispatchesToTenant(t *testing.T) {
	svc := &runnerFakeService{}
	r := NewRunner(map[string]Service{"t1": svc})
	if err := r.RunNow(context.Background(), "t1"); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if svc.calls != 1 {
		t.Errorf("RunCheckNow calls = %d, want 1", svc.calls)
	}
}

func TestRunner_RunNow_UnknownTenant(t *testing.T) {
	r := NewRunner(nil) // nil map tolerated
	err := r.RunNow(context.Background(), "nope")
	if !errors.Is(err, ErrCheckerNotEnabled) {
		t.Errorf("expected ErrCheckerNotEnabled, got %v", err)
	}
}

func TestRunner_Services(t *testing.T) {
	r := NewRunner(map[string]Service{"a": &runnerFakeService{}, "b": &runnerFakeService{}})
	if len(r.Services()) != 2 {
		t.Errorf("Services() len = %d, want 2", len(r.Services()))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `gotest go test ./amfacheck/`
Expected: FAIL - `undefined: NewRunner` / `undefined: ErrCheckerNotEnabled`.

- [ ] **Step 3: Create `amfacheck/runner.go`**

```go
package amfacheck

import (
	"context"
	"errors"
)

// ErrCheckerNotEnabled is returned when no AMFA checker is running for a tenant
// (the tenant has amfa.enabled=false, or the global amfa_checker switch is off).
var ErrCheckerNotEnabled = errors.New("amfacheck: checker not enabled for tenant")

// Runner exposes the per-tenant AMFA checker services for on-demand triggering.
// It is constructed once at startup with the same Service instances the fx
// lifecycle starts, so RunNow drives the exact same checks the ticker runs.
type Runner struct {
	services map[string]Service // tenantID -> Service
}

// NewRunner wraps the per-tenant service map. A nil map is tolerated (no tenant
// has a checker), so the Runner is always safe to construct and inject.
func NewRunner(services map[string]Service) *Runner {
	if services == nil {
		services = map[string]Service{}
	}
	return &Runner{services: services}
}

// Enabled reports whether a checker is running for the tenant.
func (r *Runner) Enabled(tenantID string) bool {
	_, ok := r.services[tenantID]
	return ok
}

// RunNow runs all of the tenant's checks once, synchronously, returning
// ErrCheckerNotEnabled if the tenant has no checker. It delegates to the
// tenant's Service.RunCheckNow - the same path the ticker uses.
func (r *Runner) RunNow(ctx context.Context, tenantID string) error {
	svc, ok := r.services[tenantID]
	if !ok {
		return ErrCheckerNotEnabled
	}
	return svc.RunCheckNow(ctx)
}

// Services returns the underlying services (used by the fx lifecycle starter).
func (r *Runner) Services() []Service {
	out := make([]Service, 0, len(r.services))
	for _, s := range r.services {
		out = append(out, s)
	}
	return out
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `gotest go test ./amfacheck/`
Expected: PASS (the 4 new tests plus all existing amfacheck tests).

- [ ] **Step 5: Commit**

```bash
git add amfacheck/runner.go amfacheck/runner_test.go
git commit -m "feat(amfacheck): add Runner for on-demand per-tenant checker triggering"
```

---

## Task 2: fx wiring - build the map, provide the Runner

**Files:**
- Modify: `internal/fx/amfacheck.go`
- Modify: `internal/fx/amfacheck_test.go`

- [ ] **Step 1: Change `buildAmfaCheckServices` to return a tenant→Service map**

In `internal/fx/amfacheck.go`, change the function signature and body. Replace the existing `buildAmfaCheckServices` (the version returning `[]amfacheck.Service`) with:

```go
// buildAmfaCheckServices is the unit-testable core. log may be nil in tests.
// It returns one Service per AMFA-enabled tenant, keyed by tenant ID.
func buildAmfaCheckServices(
	cfg *config.AppConfig,
	registry *amfa.Registry,
	realms realmLister,
	alertStore amfacheck.AlertStore,
	notifier amfacheck.NotificationService,
	log *logger.Logger,
) map[string]amfacheck.Service {
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

	services := make(map[string]amfacheck.Service)
	for tenantID, tcfg := range cfg.Keycloak.Tenants {
		if tcfg.Amfa == nil || !tcfg.Amfa.Enabled {
			continue
		}
		tenantID := tenantID // capture

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
		services[tenantID] = amfacheck.NewService(tenantID, checks, alertStore, notifier, adaptedLog, amfaCfg.PollInterval)
		logInfo(log, "amfacheck service constructed",
			logger.Str("tenant_id", tenantID),
			logger.Int("checks", len(checks)),
			logger.Str("poll_interval", amfaCfg.PollInterval.String()))
	}
	logInfo(log, "AMFA checker services constructed", logger.Int("count", len(services)))
	return services
}
```

(Only the return type, the `services` container, and the assignment line changed; the per-tenant logic is identical.)

- [ ] **Step 2: Replace the provider and lifecycle starter**

In `internal/fx/amfacheck.go`, change the module to provide a `*amfacheck.Runner`, and update the provider + starter. Replace `provideAmfaCheckServices` and `startAmfaCheckServices` with:

```go
var AmfaCheckModule = fx.Module("amfacheck",
	fx.Provide(provideAmfaCheckRunner),
	fx.Invoke(startAmfaCheckServices),
)

// provideAmfaCheckRunner builds the per-tenant services once and wraps them in a
// Runner. The Runner is injected into the AMFA HTTP handlers for on-demand runs;
// the lifecycle starter (below) starts/stops the same Service instances.
func provideAmfaCheckRunner(p AmfaCheckParams) *amfacheck.Runner {
	services := buildAmfaCheckServices(p.Config, p.Registry, p.Keycloak,
		amfaCheckAlertStore{repo: p.Alerts}, p.Notifier, p.Logger)
	return amfacheck.NewRunner(services)
}

func startAmfaCheckServices(lc fx.Lifecycle, runner *amfacheck.Runner, log *logger.Logger) {
	for _, s := range runner.Services() {
		s := s
		// The poll loop must run on a long-lived context, NOT fx's OnStart
		// context: fx cancels the OnStart context once startup completes, which
		// would immediately stop the ticker goroutine. We own a background
		// context here and cancel it in OnStop (alongside Stop()'s graceful
		// drain) so in-flight queries unwind on shutdown.
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
	logInfo(log, "amfacheck lifecycle hooks registered", logger.Int("service_count", len(runner.Services())))
}
```

The `AmfaCheckParams`, adapters, `noopAmfacheckLogger`, and `buildAmfaChecks` are unchanged.

- [ ] **Step 3: Update the builder tests to expect a map**

In `internal/fx/amfacheck_test.go`, the two tests call `buildAmfaCheckServices(...)` and assert `len(svcs)`. `len` works on a map unchanged, so the assertions stay valid. Confirm the variable is used only via `len(svcs)`; no code change is needed. If either test indexes `svcs[0]` (slice access), change it to inspect the map instead. (As written they only check `len`, so they compile and pass as-is.)

- [ ] **Step 4: Build and run fx + amfacheck tests**

Run: `gotest go build ./...`
Expected: PASS (the whole graph still compiles; `provideAmfaCheckRunner` returns `*amfacheck.Runner`, consumed in Task 3).
Run: `gotest go test ./internal/fx/ ./amfacheck/`
Expected: PASS.

> Note: until Task 3 injects the Runner into a consumer, fx provides `*amfacheck.Runner` and `startAmfaCheckServices` consumes it, so the graph is valid. `go build ./...` confirms it.

- [ ] **Step 5: Commit**

```bash
git add internal/fx/amfacheck.go internal/fx/amfacheck_test.go
git commit -m "refactor(amfacheck): provide a Runner; key services by tenant"
```

---

## Task 3: Backend endpoints on `AmfaHandlers`

**Files:**
- Modify: `internal/http/chi/amfa_handlers.go`
- Modify: `internal/fx/amfa.go` (inject the Runner)
- Test: `internal/http/chi/amfa_handlers_test.go`

- [ ] **Step 1: Write the failing handler tests**

Append to `internal/http/chi/amfa_handlers_test.go`. First, look at the top of that file to reuse its existing test logger constructor and imports (run `grep -n "logger\.\|func New\|chi\." internal/http/chi/amfa_handlers_test.go | head`); use the same `*logger.Logger` construction the existing tests use in place of `testLogger()` below if the name differs.

```go
// fakeCheckerService implements amfacheck.Service for handler tests.
type fakeCheckerService struct{ runErr error }

func (fakeCheckerService) Start(context.Context) error { return nil }
func (fakeCheckerService) Stop() error                 { return nil }
func (fakeCheckerService) AddCheck(amfacheck.Check)    {}
func (f fakeCheckerService) RunCheckNow(context.Context) error { return f.runErr }

func newAmfaCheckerTestHandler(services map[string]amfacheck.Service) *AmfaHandlers {
	return &AmfaHandlers{
		checkerRunner: amfacheck.NewRunner(services),
		log:           testLogger(), // reuse the existing test logger helper in this file
	}
}

func doReq(t *testing.T, h http.HandlerFunc, method, tenantID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("tenantID", tenantID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestAmfaCheckerStatus_EnabledTrue(t *testing.T) {
	h := newAmfaCheckerTestHandler(map[string]amfacheck.Service{"t1": fakeCheckerService{}})
	rec := doReq(t, h.handleAmfaCheckerStatus, http.MethodGet, "t1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"enabled":true`) {
		t.Errorf("body = %s, want enabled:true", rec.Body.String())
	}
}

func TestAmfaCheckerStatus_EnabledFalse(t *testing.T) {
	h := newAmfaCheckerTestHandler(map[string]amfacheck.Service{"t1": fakeCheckerService{}})
	rec := doReq(t, h.handleAmfaCheckerStatus, http.MethodGet, "other")
	if !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Errorf("body = %s, want enabled:false", rec.Body.String())
	}
}

func TestAmfaCheckerRun_OK(t *testing.T) {
	h := newAmfaCheckerTestHandler(map[string]amfacheck.Service{"t1": fakeCheckerService{}})
	rec := doReq(t, h.handleAmfaCheckerRun, http.MethodPost, "t1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %s, want status:ok", rec.Body.String())
	}
}

func TestAmfaCheckerRun_NotEnabled(t *testing.T) {
	h := newAmfaCheckerTestHandler(map[string]amfacheck.Service{"t1": fakeCheckerService{}})
	rec := doReq(t, h.handleAmfaCheckerRun, http.MethodPost, "other")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}
```

Ensure the test file imports: `context`, `net/http`, `net/http/httptest`, `strings`, `testing`, the chi package (`github.com/go-chi/chi/v5`), and `repos.defensepoint.com/iam/oss-pmt/kmt/amfacheck`.

- [ ] **Step 2: Run to verify it fails**

Run: `gotest go test ./internal/http/chi/`
Expected: FAIL - `h.checkerRunner undefined` / `h.handleAmfaCheckerStatus undefined`.

- [ ] **Step 3: Add the Runner field + constructor param**

In `internal/http/chi/amfa_handlers.go`:

Add the import (with the other module imports):
```go
	"repos.defensepoint.com/iam/oss-pmt/kmt/amfacheck"
```
Confirm `context`, `errors`, `net/http`, and `time` are imported (add any missing).

Add a field to the struct:
```go
type AmfaHandlers struct {
	service        amfa.Service
	tenantService  tenant.Service
	rbacService    RBACService
	log            *logger.Logger
	authMiddleware func(http.Handler) http.Handler
	rbacMiddleware *RBACMiddleware
	checkerRunner  *amfacheck.Runner
}
```

Update the constructor signature and body to accept and set it (add `checkerRunner *amfacheck.Runner` as the last parameter):
```go
func NewAmfaHandlers(
	svc amfa.Service,
	tenantSvc tenant.Service,
	rbacSvc RBACService,
	log *logger.Logger,
	authMW func(http.Handler) http.Handler,
	rbacMW *RBACMiddleware,
	checkerRunner *amfacheck.Runner,
) *AmfaHandlers {
	return &AmfaHandlers{
		service:        svc,
		tenantService:  tenantSvc,
		rbacService:    rbacSvc,
		log:            log,
		authMiddleware: authMW,
		rbacMiddleware: rbacMW,
		checkerRunner:  checkerRunner,
	}
}
```

- [ ] **Step 4: Add the two handler methods**

In `internal/http/chi/amfa_handlers.go`, add:

```go
// handleAmfaCheckerStatus reports whether the AMFA risk-alert checker is running
// for this tenant, so the UI can show/hide the "Reload AMFA alerts" button.
func (h *AmfaHandlers) handleAmfaCheckerStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	enabled := h.checkerRunner != nil && h.checkerRunner.Enabled(tenantID)
	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]any{"enabled": enabled})
}

// handleAmfaCheckerRun triggers an immediate AMFA checker run for this tenant.
// It runs the same checks the background ticker runs, synchronously, bounded by
// a timeout so a slow/unreachable AMFA DB cannot hang the request.
func (h *AmfaHandlers) handleAmfaCheckerRun(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if h.checkerRunner == nil {
		httputil.RespondError(w, h.log, http.StatusConflict, "amfa_checker_not_enabled")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := h.checkerRunner.RunNow(ctx, tenantID); err != nil {
		switch {
		case errors.Is(err, amfacheck.ErrCheckerNotEnabled):
			httputil.RespondError(w, h.log, http.StatusConflict, "amfa_checker_not_enabled")
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			httputil.RespondError(w, h.log, http.StatusServiceUnavailable, "amfa_unavailable")
		default:
			httputil.RespondError(w, h.log, http.StatusInternalServerError, "amfa_checker_run_failed")
		}
		return
	}
	httputil.RespondJSON(w, h.log, http.StatusOK, map[string]any{"status": "ok"})
}
```

- [ ] **Step 5: Register the two routes**

In `internal/http/chi/amfa_handlers.go`, in `RegisterTenantRoutes`, add the two routes inside the existing `r.Group(...)` block (which already applies auth, `RequireTenantAccess()`, and `RequirePermission("amfa:read")`), next to the existing `/amfa/...` routes:

```go
		r.Get("/amfa/events", h.handleAmfaEvents)
		r.Get("/amfa/stats", h.handleAmfaStats)
		r.Get("/amfa/geo", h.handleAmfaGeo)
		r.Get("/amfa-checker", h.handleAmfaCheckerStatus)
		r.Post("/amfa-checker/run", h.handleAmfaCheckerRun)
```

- [ ] **Step 6: Inject the Runner in fx**

In `internal/fx/amfa.go`, add the import:
```go
	"repos.defensepoint.com/iam/oss-pmt/kmt/amfacheck"
```
Add the field to `AmfaHandlersParams` and pass it through `provideAmfaHandlers`:

```go
type AmfaHandlersParams struct {
	fx.In

	Service        amfa.Service
	TenantService  tenant.Service
	RBACService    rbac.Service
	Logger         *logger.Logger
	AuthMiddleware *chihttp.AuthMiddleware
	RBACMiddleware *chihttp.RBACMiddleware
	CheckerRunner  *amfacheck.Runner
}

func provideAmfaHandlers(p AmfaHandlersParams) *chihttp.AmfaHandlers {
	return chihttp.NewAmfaHandlers(
		p.Service,
		p.TenantService,
		&rbacCheckerAdapter{p.RBACService},
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
		p.CheckerRunner,
	)
}
```

`*amfacheck.Runner` is provided by `AmfaCheckModule` (Task 2), so fx resolves it across modules. When the checker is globally disabled, the Runner is non-nil but empty, so `Enabled` returns false and the button stays hidden.

- [ ] **Step 7: Run handler tests + full build**

Run: `gotest go test ./internal/http/chi/`
Expected: PASS (4 new tests + existing).
Run: `gotest go build ./...`
Expected: PASS (fx graph resolves the Runner into the handler).

- [ ] **Step 8: Commit**

```bash
git add internal/http/chi/amfa_handlers.go internal/http/chi/amfa_handlers_test.go internal/fx/amfa.go
git commit -m "feat(amfa): add amfa-checker status + run endpoints"
```

---

## Task 4: Frontend service methods

**Files:**
- Modify: `web/src/features/alerts/services/alertService.ts`

- [ ] **Step 1: Add the two methods**

In `web/src/features/alerts/services/alertService.ts`, inside the `AlertService` class (after `getAlertStats`), add:

```typescript
  async getAmfaCheckerStatus(
    tenantId: string,
  ): Promise<{ enabled: boolean }> {
    return apiClient.get<{ enabled: boolean }>(
      `/tenants/${tenantId}/amfa-checker`,
    );
  }

  async reloadAmfaAlerts(tenantId: string): Promise<{ status: string }> {
    return apiClient.post<{ status: string }>(
      `/tenants/${tenantId}/amfa-checker/run`,
      {},
    );
  }
```

- [ ] **Step 2: Verify it compiles (type-check)**

Run: `cd web && npx tsc --noEmit` (or `fetest` is not needed for tsc; use `docker run --rm -v "$PWD/web:/web" -w /web node:20-alpine sh -c "npm ci >/dev/null 2>&1 || npm i >/dev/null 2>&1; npx tsc --noEmit"` if the host lacks deps).
Expected: PASS (no type errors).

- [ ] **Step 3: Commit**

```bash
git add web/src/features/alerts/services/alertService.ts
git commit -m "feat(web): alertService methods for amfa-checker status + run"
```

---

## Task 5: Frontend button + capability gating

**Files:**
- Modify: `web/src/features/alerts/components/AlertsFilters.tsx`
- Modify: `web/src/features/alerts/components/AlertsFilters.test.tsx`
- Modify: `web/src/features/alerts/pages/AlertsPage.tsx`

- [ ] **Step 1: Write the failing button tests**

In `web/src/features/alerts/components/AlertsFilters.test.tsx`, add to the existing `defaultProps` the new optional props and add tests:

```typescript
  // add to defaultProps:
  //   showReloadAmfa: false,
  //   onReloadAmfa: vi.fn(),
  //   reloadingAmfa: false,

  it("hides the Reload AMFA button when showReloadAmfa is false", () => {
    render(<AlertsFilters {...defaultProps} showReloadAmfa={false} />);
    expect(
      screen.queryByRole("button", { name: /reload amfa/i }),
    ).not.toBeInTheDocument();
  });

  it("shows and fires the Reload AMFA button when enabled", async () => {
    const user = userEvent.setup();
    const onReloadAmfa = vi.fn();
    render(
      <AlertsFilters
        {...defaultProps}
        showReloadAmfa={true}
        onReloadAmfa={onReloadAmfa}
      />,
    );
    const btn = screen.getByRole("button", { name: /reload amfa/i });
    await user.click(btn);
    expect(onReloadAmfa).toHaveBeenCalledTimes(1);
  });

  it("disables the Reload AMFA button while reloading", () => {
    render(
      <AlertsFilters
        {...defaultProps}
        showReloadAmfa={true}
        reloadingAmfa={true}
      />,
    );
    expect(screen.getByRole("button", { name: /reload amfa/i })).toBeDisabled();
  });
```

Add the three new fields to the `defaultProps` object in that test file:
```typescript
    showReloadAmfa: false,
    onReloadAmfa: vi.fn(),
    reloadingAmfa: false,
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && npx vitest run src/features/alerts/components/AlertsFilters.test.tsx` (or `fetest src/features/alerts/components/AlertsFilters.test.tsx`).
Expected: FAIL - the button isn't rendered / prop not supported yet.

- [ ] **Step 3: Add the button to `AlertsFilters.tsx`**

In `web/src/features/alerts/components/AlertsFilters.tsx`:

Add `Sync` to the icon import:
```typescript
import { FilterAltOff, Refresh, Sync } from "@mui/icons-material";
```

Add the three optional props to `AlertsFiltersProps`:
```typescript
  onRefresh: () => void;
  showReloadAmfa?: boolean;
  onReloadAmfa?: () => void;
  reloadingAmfa?: boolean;
```

Destructure them in the component params (with defaults):
```typescript
  onRefresh,
  showReloadAmfa = false,
  onReloadAmfa,
  reloadingAmfa = false,
```

Render the button immediately before the existing Refresh button (so it sits next to it). Find the `<Button ... startIcon={<Refresh />} ...>Refresh</Button>` block and insert before it:
```tsx
        {showReloadAmfa && (
          <Button
            variant="outlined"
            size="small"
            startIcon={<Sync />}
            onClick={onReloadAmfa}
            disabled={reloadingAmfa}
          >
            {reloadingAmfa ? "Reload AMFA alerts..." : "Reload AMFA alerts"}
          </Button>
        )}
```

The grid auto-flows; if the `lg` `gridTemplateColumns` enumerates fixed `auto` columns, add one more `auto` so the extra button gets its own column. Locate `lg: "1fr 1fr 1fr 1fr auto auto"` and change it to `lg: "1fr 1fr 1fr 1fr auto auto auto"`.

- [ ] **Step 4: Run the button tests**

Run: `cd web && npx vitest run src/features/alerts/components/AlertsFilters.test.tsx`
Expected: PASS (new tests + existing AlertsFilters tests).

- [ ] **Step 5: Wire capability query + reload handler into `AlertsPage.tsx`**

In `web/src/features/alerts/pages/AlertsPage.tsx`:

Add imports:
```typescript
import { useQuery } from "@tanstack/react-query";
import { alertService } from "../services/alertService";
```

Capture the stats refetch (change the existing `useAlertStats` line):
```typescript
  const { data: stats, refetch: refetchStats } = useAlertStats(
    selectedTenant?.tenant_id,
  );
```

Add the capability query and reload state (near the other hooks):
```typescript
  const { data: amfaCheckerStatus } = useQuery({
    queryKey: ["amfa-checker-status", selectedTenant?.tenant_id],
    queryFn: () => alertService.getAmfaCheckerStatus(selectedTenant!.tenant_id),
    enabled: !!selectedTenant?.tenant_id,
  });
  const [reloadingAmfa, setReloadingAmfa] = useState(false);

  const handleReloadAmfa = useCallback(async () => {
    if (!selectedTenant) return;
    setReloadingAmfa(true);
    try {
      await alertService.reloadAmfaAlerts(selectedTenant.tenant_id);
      await Promise.all([refetchAlerts(), refetchStats()]);
      setToast({ message: "AMFA alerts reloaded", type: "success" });
    } catch {
      setToast({ message: "Couldn't reload AMFA alerts, try again", type: "error" });
    } finally {
      setReloadingAmfa(false);
    }
  }, [selectedTenant, refetchAlerts, refetchStats]);
```

Pass the new props to `<AlertsFilters ... />` (add alongside `onRefresh`):
```tsx
        onRefresh={() => refetchAlerts()}
        showReloadAmfa={!!amfaCheckerStatus?.enabled}
        onReloadAmfa={handleReloadAmfa}
        reloadingAmfa={reloadingAmfa}
```

- [ ] **Step 6: Type-check + run the alerts page tests**

Run: `cd web && npx tsc --noEmit`
Expected: PASS.
Run: `cd web && npx vitest run src/features/alerts/`
Expected: PASS. If `AlertsPage.test.tsx` mocks `useAlertStats` and now destructures `refetch`, ensure its mock returns a `refetch: vi.fn()` (it already returns `refetch` for `useAlerts`; mirror that for the stats mock). Update the `useAlertStats` mock in `AlertsPage.test.tsx` to include `refetch: vi.fn()` if the test fails on a missing refetch.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/alerts/components/AlertsFilters.tsx web/src/features/alerts/components/AlertsFilters.test.tsx web/src/features/alerts/pages/AlertsPage.tsx
git commit -m "feat(web): Reload AMFA alerts button on the Alerts page"
```

---

## Task 6: Documentation

**Files:**
- Modify: `docs/amfa-integration.md`

- [ ] **Step 1: Document the button and endpoints**

In `docs/amfa-integration.md`, in the **Alerts** section (after "Manual smoke test" or near the troubleshooting table), add a short subsection. No em-dashes.

```markdown
### Reload AMFA alerts (on demand)

The Alerts page shows a **Reload AMFA alerts** button next to Refresh, but only
for tenants where the AMFA checker is enabled (both the tenant's `amfa.enabled`
and the global `amfa_checker.enabled`). Clicking it runs the AMFA checker for
that tenant immediately - the same checks the background poller runs - then
reloads the list, so new AMFA alerts appear without waiting for the next poll
interval. It does not touch the other checkers, and it never removes alerts
(the checker is additive); notifications fire exactly as they do when polled.

Endpoints (both require `amfa:read`, under `/api/tenants/{tenantId}`):

- `GET  /amfa-checker`      -> `{ "enabled": true|false }` (drives button visibility)
- `POST /amfa-checker/run`  -> `{ "status": "ok" }`; `409 amfa_checker_not_enabled`
  if the checker is off for the tenant; `503 amfa_unavailable` on timeout.
```

- [ ] **Step 2: Commit**

```bash
git add docs/amfa-integration.md
git commit -m "docs(amfa): document the Reload AMFA alerts button and endpoints"
```

---

## Task 7: Full verification

- [ ] **Step 1: Backend suite**

Run: `gotest go build ./...`
Expected: PASS.
Run: `gotest go vet ./...`
Expected: PASS.
Run: `gotest go test -short ./...`
Expected: PASS (all packages, including `amfacheck`, `internal/fx`, `internal/http/chi`).

- [ ] **Step 2: Frontend suite**

Run: `cd web && npx tsc --noEmit && npx vitest run src/features/alerts/`
Expected: PASS. (If rollup errors on the host, use the `fetest` Docker variant from the execution notes.)

- [ ] **Step 3: Final review checklist**

- [ ] Both endpoints sit under the existing `amfa:read` group (no new RBAC permission added).
- [ ] `Runner` is provided once and both the lifecycle starter and the handler consume it (services constructed once, not twice).
- [ ] Button hidden when capability returns `enabled:false`.
- [ ] Refresh button unchanged; no change to the alerts engine/lifecycle/schema.
- [ ] No em-dashes in new code/docs.

- [ ] **Step 4: Optional live check (manual)**

With a tenant that has the AMFA checker enabled, load the Alerts page, confirm the "Reload AMFA alerts" button appears, click it, and confirm new AMFA alerts appear immediately and non-AMFA rows are unchanged. (See `docs/amfa-integration.md` "Testing the checker" for seeding fresh events.)

---

## Self-review notes (author)

- **Spec coverage:** Runner + RunNow/Enabled (Task 1); fx provides Runner, services constructed once (Task 2); `GET /amfa-checker` capability + `POST /amfa-checker/run` trigger with timeout, `amfa:read`, 200/409/503 (Task 3); frontend service methods (Task 4); capability-gated button next to Refresh + refetch + snackbar (Task 5); docs (Task 6); behavior-on-existing-rows and notifications unchanged because the same `RunCheckNow` path is reused (Tasks 1-3). Reliability: bounded timeout + idempotent upsert (Task 3); proven `runChecks` path (no logic change).
- **Type consistency:** `buildAmfaCheckServices` returns `map[string]amfacheck.Service` (Task 2) consumed by `NewRunner` (Task 1) and `Runner.Services()` by `startAmfaCheckServices` (Task 2); `NewAmfaHandlers` gains a trailing `*amfacheck.Runner` param used by both new handlers (Task 3); frontend `getAmfaCheckerStatus`/`reloadAmfaAlerts` names match across service (Task 4) and page (Task 5); `showReloadAmfa`/`onReloadAmfa`/`reloadingAmfa` prop names match across `AlertsFilters` (Task 5) and its tests.
- **No new permission:** trigger reuses `amfa:read` per the approved spec; tightening is a documented follow-up.
- **Discovery flagged:** the exact test-logger constructor in `amfa_handlers_test.go` and the `useAlertStats` mock shape in `AlertsPage.test.tsx` are confirmed by reading those files at implementation time (Tasks 3 and 5 call this out).
