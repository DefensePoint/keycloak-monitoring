package tenant

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// retryService is a concurrency-safe Service for the retry tests: the retry
// loop reads tenants from its own goroutine while the test changes them.
type retryService struct {
	mockService
	mu      sync.Mutex
	tenants map[string]*domain.KeycloakTenant
}

func newRetryService(tenants ...*domain.KeycloakTenant) *retryService {
	s := &retryService{tenants: map[string]*domain.KeycloakTenant{}}
	for _, t := range tenants {
		s.tenants[t.TenantID] = t
	}
	return s
}

func (s *retryService) GetTenant(_ context.Context, tenantID string) (*domain.KeycloakTenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tenants[tenantID]
	if !ok {
		return nil, nil
	}
	copied := *t
	return &copied, nil
}

func (s *retryService) ListEnabledTenants(_ context.Context) ([]*domain.KeycloakTenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*domain.KeycloakTenant
	for _, t := range s.tenants {
		if t.Enabled {
			copied := *t
			out = append(out, &copied)
		}
	}
	return out, nil
}

func (s *retryService) set(mutate func(map[string]*domain.KeycloakTenant)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mutate(s.tenants)
}

// flakyFactory fails CreateClient until ok reports true for the tenant config,
// the way an unreachable Keycloak fails authentication.
type flakyFactory struct {
	mockMonitorFactory
	mu    sync.Mutex
	calls int
	last  *MonitorConfig
	ok    func(cfg *MonitorConfig, call int) bool
}

func (f *flakyFactory) CreateClient(_ context.Context, cfg *MonitorConfig, _ any) (KeycloakClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	copied := *cfg
	f.last = &copied
	if f.ok != nil && f.ok(cfg, f.calls) {
		return &mockKeycloakClient{}, nil
	}
	return nil, errors.New("keycloak unreachable")
}

func (f *flakyFactory) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newRetryManager(s Service, f KeycloakMonitorFactory) *MonitorPoolManager {
	m := NewMonitorPoolManager(s, f, nil, &mockLogger{}, nil, nil, nil, nil, nil, nil, nil)
	m.retryInitial = 10 * time.Millisecond
	m.retryMax = 40 * time.Millisecond
	return m
}

// stopOnCleanup stops the pool when the test ends and fails the test if
// stopping reports an error.
func stopOnCleanup(t *testing.T, m *MonitorPoolManager) {
	t.Helper()
	t.Cleanup(func() {
		if err := m.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
}

func retryTenant() *domain.KeycloakTenant {
	return &domain.KeycloakTenant{TenantID: "t1", Name: "Tenant 1", ServerURL: "https://kc.example.com",
		AdminRealm: "master", ClientID: "kmt", ClientSecret: "secret", Enabled: true}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The reported bug: a tenant whose Keycloak is down at startup was never
// monitored, even after Keycloak came back.
func TestRetry_StartsMonitorOnceKeycloakRecovers(t *testing.T) {
	svc := newRetryService(retryTenant())
	f := &flakyFactory{ok: func(_ *MonitorConfig, call int) bool { return call >= 3 }}
	m := newRetryManager(svc, f)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopOnCleanup(t, m)

	if got := m.GetActiveMonitorCount(); got != 0 {
		t.Fatalf("active monitors right after a failed start = %d, want 0", got)
	}
	waitFor(t, "the monitor to start after Keycloak recovers", func() bool { return m.GetActiveMonitorCount() == 1 })
	if got := f.callCount(); got != 3 {
		t.Errorf("CreateClient calls = %d, want 3 (the failed start, one failed retry, the successful retry)", got)
	}
	// Once running, the tenant is no longer retried.
	time.Sleep(100 * time.Millisecond)
	if got := f.callCount(); got != 3 {
		t.Errorf("CreateClient calls after recovery = %d, want still 3", got)
	}
}

func TestRetry_BackoffDoublesAndCaps(t *testing.T) {
	initial, max := 15*time.Second, 5*time.Minute
	for attempt, want := range map[int]time.Duration{
		1: 15 * time.Second, 2: 30 * time.Second, 3: time.Minute, 4: 2 * time.Minute,
		5: 4 * time.Minute, 6: 5 * time.Minute, 50: 5 * time.Minute,
	} {
		if got := retryBackoff(initial, max, attempt); got != want {
			t.Errorf("retryBackoff(attempt %d) = %v, want %v", attempt, got, want)
		}
	}
}

func TestRetry_DoesNotReviveDisabledOrDeletedTenant(t *testing.T) {
	for name, change := range map[string]func(map[string]*domain.KeycloakTenant){
		"disabled": func(ts map[string]*domain.KeycloakTenant) { ts["t1"].Enabled = false },
		"deleted":  func(ts map[string]*domain.KeycloakTenant) { delete(ts, "t1") },
	} {
		t.Run(name, func(t *testing.T) {
			svc := newRetryService(retryTenant())
			var recovered sync.Mutex
			allow := false
			f := &flakyFactory{ok: func(_ *MonitorConfig, _ int) bool { recovered.Lock(); defer recovered.Unlock(); return allow }}
			m := newRetryManager(svc, f)
			if err := m.Start(context.Background()); err != nil {
				t.Fatalf("Start: %v", err)
			}
			stopOnCleanup(t, m)

			svc.set(change)
			recovered.Lock()
			allow = true // Keycloak is back, but the tenant must stay unmonitored.
			recovered.Unlock()
			time.Sleep(200 * time.Millisecond)
			if got := m.GetActiveMonitorCount(); got != 0 {
				t.Errorf("active monitors for a %s tenant = %d, want 0", name, got)
			}
		})
	}
}

func TestRetry_UsesCurrentTenantSettings(t *testing.T) {
	svc := newRetryService(retryTenant())
	f := &flakyFactory{ok: func(cfg *MonitorConfig, _ int) bool { return cfg.ClientSecret == "corrected" }}
	m := newRetryManager(svc, f)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopOnCleanup(t, m)

	svc.set(func(ts map[string]*domain.KeycloakTenant) { ts["t1"].ClientSecret = "corrected" })
	waitFor(t, "the monitor to start with the corrected secret", func() bool { return m.GetActiveMonitorCount() == 1 })
}

// An admin enabling the tenant while a retry is pending must not end in two
// monitors, and the pending retry must not keep firing afterwards.
func TestRetry_NoDuplicateWhenStartedWhilePending(t *testing.T) {
	svc := newRetryService(retryTenant())
	var mu sync.Mutex
	allow := false
	f := &flakyFactory{ok: func(_ *MonitorConfig, _ int) bool { mu.Lock(); defer mu.Unlock(); return allow }}
	m := newRetryManager(svc, f)
	m.retryInitial, m.retryMax = time.Hour, time.Hour // keep the retry pending
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopOnCleanup(t, m)

	mu.Lock()
	allow = true
	mu.Unlock()
	tenant, _ := svc.GetTenant(context.Background(), "t1")
	m.handleTenantChange(EventEnabled, tenant)
	if got := m.GetActiveMonitorCount(); got != 1 {
		t.Fatalf("active monitors after enable = %d, want 1", got)
	}
	if m.retryPending("t1") {
		t.Error("tenant still pending a retry after an admin action started its monitor")
	}
}

func TestRetry_DisableEventCancelsPendingRetry(t *testing.T) {
	svc := newRetryService(retryTenant())
	f := &flakyFactory{}
	m := newRetryManager(svc, f)
	m.retryInitial, m.retryMax = time.Hour, time.Hour
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopOnCleanup(t, m)

	if !m.retryPending("t1") {
		t.Fatal("a tenant that failed to start is not pending a retry")
	}
	tenant, _ := svc.GetTenant(context.Background(), "t1")
	tenant.Enabled = false
	m.handleTenantChange(EventDisabled, tenant)
	if m.retryPending("t1") {
		t.Error("disabling the tenant left its retry pending")
	}
}

func TestRetry_StopEndsRetries(t *testing.T) {
	svc := newRetryService(retryTenant())
	f := &flakyFactory{}
	m := newRetryManager(svc, f)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, "at least one retry", func() bool { return f.callCount() >= 2 })
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	after := f.callCount()
	time.Sleep(150 * time.Millisecond)
	if got := f.callCount(); got != after {
		t.Errorf("CreateClient calls after Stop grew from %d to %d", after, got)
	}
}
