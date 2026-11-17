package fx

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// cwService is a tenant's check service that records whether it is running.
type cwService struct {
	tenantID      string
	mu            sync.Mutex
	starts, stops int
}

func (s *cwService) Start(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starts++
	return nil
}

func (s *cwService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stops++
	return nil
}

func (s *cwService) running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.starts == 1 && s.stops == 0
}

// cwLister is the tenant service at startup. Like the real one, it lists
// only the tenants loaded from the database so far. When block is set, the
// load waits on it, so a test can deliver a change mid-load.
type cwLister struct {
	tenants []*domain.KeycloakTenant
	err     error
	block   chan struct{}
	called  chan struct{}
	loaded  bool
	loads   int
	// unloaded makes the tenants listable only after LoadTenants.
	unloaded bool
}

func (l *cwLister) LoadTenants(context.Context) error {
	if l.called != nil {
		close(l.called)
	}
	if l.block != nil {
		<-l.block
	}
	l.loaded = true
	l.loads++
	return l.err
}

func (l *cwLister) ListEnabledTenants(context.Context) ([]*domain.KeycloakTenant, error) {
	if l.unloaded && !l.loaded {
		return nil, nil
	}
	return l.tenants, nil
}

type cwHarness struct {
	workers *tenantCheckWorkers[*cwService]
	mu      sync.Mutex
	built   []*cwService
	// noChecks are tenants the build reports nothing to run for.
	noChecks map[string]bool
}

func newCWHarness() *cwHarness {
	h := &cwHarness{noChecks: map[string]bool{}}
	h.workers = &tenantCheckWorkers[*cwService]{
		component: "test",
		build: func(tenantID string) (*cwService, bool) {
			if h.noChecks[tenantID] {
				return nil, false
			}
			svc := &cwService{tenantID: tenantID}
			h.mu.Lock()
			h.built = append(h.built, svc)
			h.mu.Unlock()
			return svc, true
		},
	}
	return h
}

func (h *cwHarness) start(t *testing.T, lister tenantSource) {
	t.Helper()
	if err := h.workers.start(context.Background(), lister); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = h.workers.stop() })
}

// running returns the IDs of the tenants with a running service, one entry
// per running service, so a tenant running twice shows up twice.
func (h *cwHarness) running() map[string]int {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[string]int{}
	for _, s := range h.built {
		if s.running() {
			out[s.tenantID]++
		}
	}
	return out
}

func (h *cwHarness) wantRunning(t *testing.T, tenantIDs ...string) {
	t.Helper()
	got := h.running()
	if len(got) != len(tenantIDs) {
		t.Fatalf("running tenants = %v, want %v", got, tenantIDs)
	}
	for _, id := range tenantIDs {
		if got[id] != 1 {
			t.Fatalf("tenant %s has %d running services, want 1 (running: %v)", id, got[id], got)
		}
	}
	if n := h.workers.pool.Len(); n != len(tenantIDs) {
		t.Fatalf("pool has %d services, want %d", n, len(tenantIDs))
	}
}

func enabledTenant(id string) *domain.KeycloakTenant {
	return &domain.KeycloakTenant{TenantID: id, Enabled: true}
}

func disabledTenant(id string) *domain.KeycloakTenant {
	return &domain.KeycloakTenant{TenantID: id}
}

func TestTenantCheckWorkers_StartsEveryEnabledTenantAtStartup(t *testing.T) {
	h := newCWHarness()
	h.start(t, &cwLister{tenants: []*domain.KeycloakTenant{enabledTenant("a"), enabledTenant("b")}})

	h.wantRunning(t, "a", "b")
}

// The check modules may start before the monitoring module has loaded the
// tenant cache, which would leave a tenant created in an earlier run, and so
// never announced as a change, unchecked.
func TestTenantCheckWorkers_LoadsTheTenantsBeforeListingThem(t *testing.T) {
	h := newCWHarness()
	h.workers.loadTenants = true
	h.start(t, &cwLister{tenants: []*domain.KeycloakTenant{enabledTenant("from-db")}, unloaded: true})

	h.wantRunning(t, "from-db")
}

// Loading writes the database rows over the cache without the lock tenant
// edits take, so workers that start once the HTTP server is serving must not
// load: a tenant saved meanwhile would be cached stale.
func TestTenantCheckWorkers_DoesNotReloadTheCacheUnlessAsked(t *testing.T) {
	h := newCWHarness()
	lister := &cwLister{tenants: []*domain.KeycloakTenant{enabledTenant("a")}}
	h.start(t, lister)

	if lister.loads != 0 {
		t.Fatalf("LoadTenants called %d times, want 0", lister.loads)
	}
	h.wantRunning(t, "a")
}

func TestTenantCheckWorkers_StartupLoadFailureFailsStart(t *testing.T) {
	h := newCWHarness()
	h.workers.loadTenants = true
	boom := errors.New("database down")

	err := h.workers.start(context.Background(), &cwLister{err: boom})
	if !errors.Is(err, boom) {
		t.Fatalf("start error = %v, want %v", err, boom)
	}
}

func TestTenantCheckWorkers_TenantWithNothingToRunGetsNoService(t *testing.T) {
	h := newCWHarness()
	h.noChecks["a"] = true
	h.start(t, &cwLister{tenants: []*domain.KeycloakTenant{enabledTenant("a"), enabledTenant("b")}})

	h.wantRunning(t, "b")
}

// The bug: a tenant created in the interface got no checks until a restart.
func TestTenantCheckWorkers_TenantAddedAtRuntimeGetsChecksAtOnce(t *testing.T) {
	h := newCWHarness()
	h.start(t, &cwLister{})

	h.workers.tenantChanged(tenant.EventAdded, enabledTenant("new"))

	h.wantRunning(t, "new")
}

func TestTenantCheckWorkers_DisabledTenantAddedGetsNoChecks(t *testing.T) {
	h := newCWHarness()
	h.start(t, &cwLister{})

	h.workers.tenantChanged(tenant.EventAdded, disabledTenant("new"))

	h.wantRunning(t)
}

// The other half of the bug: a disabled tenant was checked until a restart.
func TestTenantCheckWorkers_DisablingATenantStopsItsChecks(t *testing.T) {
	h := newCWHarness()
	h.start(t, &cwLister{tenants: []*domain.KeycloakTenant{enabledTenant("a"), enabledTenant("b")}})

	h.workers.tenantChanged(tenant.EventDisabled, disabledTenant("a"))

	h.wantRunning(t, "b")
}

func TestTenantCheckWorkers_DeletingATenantStopsItsChecks(t *testing.T) {
	h := newCWHarness()
	h.start(t, &cwLister{tenants: []*domain.KeycloakTenant{enabledTenant("a")}})

	h.workers.tenantChanged(tenant.EventRemoved, enabledTenant("a"))

	h.wantRunning(t)
}

func TestTenantCheckWorkers_ReEnabledTenantGetsChecksAgain(t *testing.T) {
	h := newCWHarness()
	h.start(t, &cwLister{tenants: []*domain.KeycloakTenant{enabledTenant("a")}})

	h.workers.tenantChanged(tenant.EventDisabled, disabledTenant("a"))
	h.workers.tenantChanged(tenant.EventEnabled, enabledTenant("a"))

	h.wantRunning(t, "a")
}

// An edit (name, URL, credentials) keeps the running service: the checks look
// the tenant's Keycloak client up on every call, and a fresh service would
// forget how many runs in a row have missed each open alert, delaying its
// resolution.
func TestTenantCheckWorkers_EditKeepsTheRunningService(t *testing.T) {
	h := newCWHarness()
	h.start(t, &cwLister{tenants: []*domain.KeycloakTenant{enabledTenant("a")}})
	first := h.built[0]

	h.workers.tenantChanged(tenant.EventUpdated, enabledTenant("a"))

	h.wantRunning(t, "a")
	if len(h.built) != 1 || !first.running() {
		t.Fatalf("an edit replaced the running service (built %d)", len(h.built))
	}
}

func TestTenantCheckWorkers_EditOfAnEnabledTenantWithoutServiceStartsIt(t *testing.T) {
	h := newCWHarness()
	h.start(t, &cwLister{})

	h.workers.tenantChanged(tenant.EventUpdated, enabledTenant("a"))

	h.wantRunning(t, "a")
}

func TestTenantCheckWorkers_EditOfADisabledTenantStartsNothing(t *testing.T) {
	h := newCWHarness()
	h.start(t, &cwLister{})

	h.workers.tenantChanged(tenant.EventUpdated, disabledTenant("a"))

	h.wantRunning(t)
}

// A tenant disabled while the startup list is being read must not be started
// from that list's stale copy afterwards.
func TestTenantCheckWorkers_ChangeDuringStartupLoadWinsOverTheList(t *testing.T) {
	h := newCWHarness()
	h.workers.loadTenants = true
	lister := &cwLister{
		tenants: []*domain.KeycloakTenant{enabledTenant("a"), enabledTenant("b")},
		block:   make(chan struct{}),
		called:  make(chan struct{}),
	}
	done := make(chan error, 1)
	go func() { done <- h.workers.start(context.Background(), lister) }()
	t.Cleanup(func() { _ = h.workers.stop() })

	<-lister.called
	h.workers.tenantChanged(tenant.EventDisabled, disabledTenant("a"))
	close(lister.block)
	if err := <-done; err != nil {
		t.Fatalf("start: %v", err)
	}

	h.wantRunning(t, "b")
}

func TestTenantCheckWorkers_StopStopsEveryServiceAndIgnoresLaterChanges(t *testing.T) {
	h := newCWHarness()
	if err := h.workers.start(context.Background(), &cwLister{tenants: []*domain.KeycloakTenant{enabledTenant("a")}}); err != nil {
		t.Fatalf("start: %v", err)
	}

	if err := h.workers.stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	h.workers.tenantChanged(tenant.EventAdded, enabledTenant("late"))

	if got := h.running(); len(got) != 0 {
		t.Fatalf("running after stop: %v", got)
	}
}
