package fx

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfacheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfamirror"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// twService is a per-tenant service that records the repository it was built
// with and whether it is running.
type twService struct {
	repo          amfa.Repository
	mu            sync.Mutex
	starts, stops int
}

func (s *twService) Start(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starts++
	return nil
}

func (s *twService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stops++
	return nil
}

func (s *twService) running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.starts == 1 && s.stops == 0
}

// twHarness drives tenant changes through the real AMFA tenant sync and
// registry into an amfaTenantWorkers built on twService.
type twHarness struct {
	sync     *amfaTenantSync
	registry *amfa.Registry
	workers  *amfaTenantWorkers[*twService]
	built    []*twService
}

func newTWHarness(t *testing.T, cfg *config.AppConfig) *twHarness {
	t.Helper()
	s, reg := newSync()
	s.cfg = cfg
	h := &twHarness{sync: s, registry: reg}
	h.workers = &amfaTenantWorkers[*twService]{
		component:     "test",
		configTenants: cfg.Keycloak.Tenants,
		build: func(_ string, repo amfa.Repository) *twService {
			svc := &twService{repo: repo}
			h.built = append(h.built, svc)
			return svc
		},
	}
	h.workers.follow(reg)
	if err := h.workers.pool.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = h.workers.pool.Stop() })
	return h
}

func (h *twHarness) change(event tenant.Event, tn *domain.KeycloakTenant) {
	h.sync.handleTenantChange(event, tn)
}

// runningFor returns the tenant's running services; the tests expect at most one.
func (h *twHarness) runningFor(t *testing.T) []*twService {
	t.Helper()
	var out []*twService
	for _, s := range h.built {
		if s.running() {
			out = append(out, s)
		}
	}
	return out
}

func (h *twHarness) wantOneRunningWithCurrentRepo(t *testing.T, tenantID string) *twService {
	t.Helper()
	running := h.runningFor(t)
	if len(running) != 1 {
		t.Fatalf("running services = %d, want 1", len(running))
	}
	repo, err := h.registry.RepositoryFor(tenantID)
	if err != nil {
		t.Fatalf("RepositoryFor(%s): %v", tenantID, err)
	}
	if running[0].repo != repo {
		t.Error("running service does not use the tenant's current AMFA repository")
	}
	if h.workers.pool.Len() != 1 {
		t.Errorf("pool has %d services, want 1", h.workers.pool.Len())
	}
	return running[0]
}

func (h *twHarness) wantNoneRunning(t *testing.T) {
	t.Helper()
	if n := len(h.runningFor(t)); n != 0 {
		t.Errorf("running services = %d, want 0", n)
	}
	if h.workers.pool.Len() != 0 {
		t.Errorf("pool has %d services, want 0", h.workers.pool.Len())
	}
}

func TestAmfaTenantWorkers_TenantCreatedWithAmfaStartsAService(t *testing.T) {
	h := newTWHarness(t, &config.AppConfig{})
	h.change(tenant.EventAdded, dbTenant("t1", true, "https://amfa.internal"))
	h.wantOneRunningWithCurrentRepo(t, "t1")
}

func TestAmfaTenantWorkers_TenantCreatedWithoutAmfaStartsNothing(t *testing.T) {
	h := newTWHarness(t, &config.AppConfig{})
	h.change(tenant.EventAdded, dbTenant("t1", false, ""))
	h.change(tenant.EventAdded, dbTenant("t2", true, "")) // AMFA on, but no endpoint
	h.wantNoneRunning(t)
}

func TestAmfaTenantWorkers_AmfaTurnedOnLaterStartsAService(t *testing.T) {
	h := newTWHarness(t, &config.AppConfig{})
	h.change(tenant.EventAdded, dbTenant("t1", false, "https://amfa.internal"))
	h.wantNoneRunning(t)
	h.change(tenant.EventUpdated, dbTenant("t1", true, "https://amfa.internal"))
	h.wantOneRunningWithCurrentRepo(t, "t1")
}

func TestAmfaTenantWorkers_EndpointChangeRestartsWithTheNewEndpoint(t *testing.T) {
	h := newTWHarness(t, &config.AppConfig{})
	h.change(tenant.EventAdded, dbTenant("t1", true, "https://amfa-old.internal"))
	old := h.wantOneRunningWithCurrentRepo(t, "t1")

	h.change(tenant.EventUpdated, dbTenant("t1", true, "https://amfa-new.internal"))
	replacement := h.wantOneRunningWithCurrentRepo(t, "t1")
	if replacement == old {
		t.Fatal("service was not replaced")
	}
	if old.running() {
		t.Error("old service still running")
	}
}

func TestAmfaTenantWorkers_AmfaTurnedOffStopsTheService(t *testing.T) {
	h := newTWHarness(t, &config.AppConfig{})
	h.change(tenant.EventAdded, dbTenant("t1", true, "https://amfa.internal"))
	h.change(tenant.EventUpdated, dbTenant("t1", false, "https://amfa.internal"))
	h.wantNoneRunning(t)
}

func TestAmfaTenantWorkers_TenantDisabledStopsTheService(t *testing.T) {
	h := newTWHarness(t, &config.AppConfig{})
	h.change(tenant.EventAdded, dbTenant("t1", true, "https://amfa.internal"))
	disabled := dbTenant("t1", true, "https://amfa.internal")
	disabled.Enabled = false
	h.change(tenant.EventDisabled, disabled)
	h.wantNoneRunning(t)

	h.change(tenant.EventEnabled, dbTenant("t1", true, "https://amfa.internal"))
	h.wantOneRunningWithCurrentRepo(t, "t1")
}

func TestAmfaTenantWorkers_TenantDeletedStopsTheService(t *testing.T) {
	h := newTWHarness(t, &config.AppConfig{})
	h.change(tenant.EventAdded, dbTenant("t1", true, "https://amfa.internal"))
	h.change(tenant.EventRemoved, dbTenant("t1", true, "https://amfa.internal"))
	h.wantNoneRunning(t)
}

func TestAmfaTenantWorkers_RepeatedEditsKeepOneService(t *testing.T) {
	h := newTWHarness(t, &config.AppConfig{})
	h.change(tenant.EventAdded, dbTenant("t1", true, "https://amfa.internal"))
	for i := 0; i < 5; i++ {
		h.change(tenant.EventUpdated, dbTenant("t1", true, "https://amfa.internal"))
	}
	h.wantOneRunningWithCurrentRepo(t, "t1")
	for _, s := range h.built {
		s.mu.Lock()
		if s.starts != 1 || (s.stops != 0 && s.stops != 1) {
			t.Errorf("service starts=%d stops=%d", s.starts, s.stops)
		}
		s.mu.Unlock()
	}
}

// config.yaml tenants keep the services built for them at startup: a registry
// change for one of their IDs is ignored.
func TestAmfaTenantWorkers_IgnoresConfigTenants(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Keycloak.Tenants = map[string]config.TenantConfig{
		"cfg1": {Amfa: &config.AmfaTenantConfig{Enabled: true}},
	}
	h := newTWHarness(t, cfg)
	boot := &twService{}
	_ = h.workers.pool.Put("cfg1", boot)

	h.registry.Register("cfg1", acRepoStub{})
	h.registry.Unregister("cfg1")

	if !boot.running() {
		t.Error("the service built at startup for a config.yaml tenant was stopped")
	}
	if len(h.built) != 0 {
		t.Errorf("built %d services for a config.yaml tenant, want 0", len(h.built))
	}
}

// A database tenant loaded at startup gets its service once the app starts.
func TestAmfaTenantWorkers_TenantRegisteredBeforeStartStartsWithTheApp(t *testing.T) {
	reg := amfa.NewRegistry()
	var built []*twService
	w := &amfaTenantWorkers[*twService]{
		component: "test",
		build: func(_ string, repo amfa.Repository) *twService {
			svc := &twService{repo: repo}
			built = append(built, svc)
			return svc
		},
	}
	w.follow(reg)
	reg.Register("t1", acRepoStub{})
	if len(built) != 1 || built[0].running() {
		t.Fatal("expected one service, not started before the app starts")
	}
	_ = w.pool.Start()
	defer func() { _ = w.pool.Stop() }()
	if !built[0].running() {
		t.Error("service not started with the app")
	}
}

// ---------------------------------------------------------------------------
// The checker and the mirror, through their real constructors. Their pools are
// not started: a started service would call the test tenants' AMFA endpoints.
// ---------------------------------------------------------------------------

func checkerConfig(enabled bool) *config.AppConfig {
	cfg := &config.AppConfig{}
	cfg.Keycloak.Global.AmfaChecker.Enabled = enabled
	cfg.Keycloak.Global.AmfaChecker.Checks.RiskRejected.Enabled = true
	cfg.Keycloak.Global.AmfaChecker.PollInterval = time.Hour
	return cfg
}

func newCheckerUnderTest(t *testing.T, cfg *config.AppConfig, reg *amfa.Registry) (*amfacheck.Runner, *amfaTenantWorkers[amfacheck.Service]) {
	t.Helper()
	runner, workers := newAmfaCheckWorkers(cfg, reg, acRealmRepoStub{}, acFakeAlertStore{}, acFakeNotifier{}, nil, nil)
	t.Cleanup(func() { _ = workers.pool.Stop() })
	return runner, workers
}

func TestAmfaChecker_FollowsInterfaceTenants(t *testing.T) {
	cfg := checkerConfig(true)
	s, reg := newSync()
	runner, workers := newCheckerUnderTest(t, cfg, reg)

	s.handleTenantChange(tenant.EventAdded, dbTenant("t1", true, "https://amfa.internal"))
	if !runner.Enabled("t1") || workers.pool.Len() != 1 {
		t.Fatalf("after create: enabled=%v services=%d, want true and 1", runner.Enabled("t1"), workers.pool.Len())
	}

	for i := 0; i < 3; i++ {
		s.handleTenantChange(tenant.EventUpdated, dbTenant("t1", true, "https://amfa-new.internal"))
	}
	if !runner.Enabled("t1") || workers.pool.Len() != 1 {
		t.Errorf("after edits: enabled=%v services=%d, want true and 1", runner.Enabled("t1"), workers.pool.Len())
	}

	s.handleTenantChange(tenant.EventUpdated, dbTenant("t1", false, "https://amfa-new.internal"))
	if runner.Enabled("t1") || workers.pool.Len() != 0 {
		t.Errorf("after AMFA off: enabled=%v services=%d, want false and 0", runner.Enabled("t1"), workers.pool.Len())
	}

	s.handleTenantChange(tenant.EventUpdated, dbTenant("t1", true, "https://amfa-new.internal"))
	s.handleTenantChange(tenant.EventRemoved, dbTenant("t1", true, "https://amfa-new.internal"))
	if runner.Enabled("t1") || workers.pool.Len() != 0 {
		t.Errorf("after delete: enabled=%v services=%d, want false and 0", runner.Enabled("t1"), workers.pool.Len())
	}
}

func TestAmfaChecker_GlobalOffWins(t *testing.T) {
	s, reg := newSync()
	runner, workers := newCheckerUnderTest(t, checkerConfig(false), reg)

	s.handleTenantChange(tenant.EventAdded, dbTenant("t1", true, "https://amfa.internal"))
	if runner.Enabled("t1") || workers.pool.Len() != 0 {
		t.Error("checker started for a tenant while amfa_checker is globally off")
	}
}

func TestAmfaChecker_ConfigTenantsStillStartAtBoot(t *testing.T) {
	cfg := checkerConfig(true)
	cfg.Keycloak.Tenants = map[string]config.TenantConfig{
		"cfg1": {Amfa: &config.AmfaTenantConfig{Enabled: true}},
	}
	runner, workers := newCheckerUnderTest(t, cfg, acRegistry("cfg1"))
	if !runner.Enabled("cfg1") || workers.pool.Len() != 1 {
		t.Errorf("config tenant: enabled=%v services=%d, want true and 1", runner.Enabled("cfg1"), workers.pool.Len())
	}
}

// twEventStore is an amfamirror.EventStore that stores nothing.
type twEventStore struct{}

func (twEventStore) MergeAmfaEvent(context.Context, *domain.Event) error { return nil }
func (twEventStore) AmfaMirrorWatermark(context.Context, string, string) (time.Time, error) {
	return time.Time{}, nil
}
func (twEventStore) SaveAmfaMirrorWatermark(context.Context, string, string, time.Time) error {
	return nil
}
func (twEventStore) ReconcileAMFAMerges(context.Context) (int64, error) { return 0, nil }

var _ amfamirror.EventStore = twEventStore{}

func mirrorConfig(enabled bool) *config.AppConfig {
	cfg := &config.AppConfig{}
	cfg.Keycloak.Global.AmfaMirror.Enabled = enabled
	cfg.Keycloak.Global.AmfaMirror.PollInterval = time.Hour
	return cfg
}

func newMirrorUnderTest(t *testing.T, cfg *config.AppConfig, reg *amfa.Registry) *amfaTenantWorkers[amfamirror.Service] {
	t.Helper()
	workers := newAmfaMirrorWorkers(cfg, reg, twEventStore{}, nil, acRealmRepoStub{}, nil)
	t.Cleanup(func() { _ = workers.pool.Stop() })
	return workers
}

func TestAmfaMirror_FollowsInterfaceTenants(t *testing.T) {
	s, reg := newSync()
	workers := newMirrorUnderTest(t, mirrorConfig(true), reg)

	s.handleTenantChange(tenant.EventAdded, dbTenant("t1", true, "https://amfa.internal"))
	if workers.pool.Len() != 1 {
		t.Fatalf("mirrors after create = %d, want 1", workers.pool.Len())
	}
	for i := 0; i < 3; i++ {
		s.handleTenantChange(tenant.EventUpdated, dbTenant("t1", true, "https://amfa-new.internal"))
	}
	if workers.pool.Len() != 1 {
		t.Errorf("mirrors after edits = %d, want 1", workers.pool.Len())
	}
	disabled := dbTenant("t1", true, "https://amfa-new.internal")
	disabled.Enabled = false
	s.handleTenantChange(tenant.EventDisabled, disabled)
	if workers.pool.Len() != 0 {
		t.Errorf("mirrors after disable = %d, want 0", workers.pool.Len())
	}
	s.handleTenantChange(tenant.EventEnabled, dbTenant("t1", true, "https://amfa-new.internal"))
	s.handleTenantChange(tenant.EventRemoved, dbTenant("t1", true, "https://amfa-new.internal"))
	if workers.pool.Len() != 0 {
		t.Errorf("mirrors after delete = %d, want 0", workers.pool.Len())
	}
}

func TestAmfaMirror_GlobalOffWins(t *testing.T) {
	s, reg := newSync()
	workers := newMirrorUnderTest(t, mirrorConfig(false), reg)
	s.handleTenantChange(tenant.EventAdded, dbTenant("t1", true, "https://amfa.internal"))
	if workers.pool.Len() != 0 {
		t.Error("mirror started for a tenant while amfa_mirror is globally off")
	}
}

func TestAmfaMirror_ConfigTenantsStillStartAtBoot(t *testing.T) {
	cfg := mirrorConfig(true)
	cfg.Keycloak.Tenants = map[string]config.TenantConfig{
		"cfg1": {Amfa: &config.AmfaTenantConfig{Enabled: true}},
	}
	workers := newMirrorUnderTest(t, cfg, acRegistry("cfg1"))
	if workers.pool.Len() != 1 {
		t.Errorf("config tenant mirrors = %d, want 1", workers.pool.Len())
	}
}
