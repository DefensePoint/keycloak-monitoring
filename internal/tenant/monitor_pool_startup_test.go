package tenant

import (
	"context"
	"sync"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// A tenant created, changed or removed while the pool is starting must end up
// in the state its last change asked for. The pool used to list the tenants,
// start their monitors, and only then follow tenant changes, so a change made
// meanwhile was never applied: a tenant created then stayed unmonitored until
// the next restart. Starting the monitors can take seconds, more while a
// tenant's Keycloak does not answer, and the API takes changes meanwhile.

// startupService keeps its tenants in order and delivers changes the way the
// real service does: one worker per callback, in order, after the change.
// Changes made before a callback is registered reach nobody.
type startupService struct {
	mockService
	mu         sync.Mutex
	order      []string
	tenants    map[string]*domain.KeycloakTenant
	callbacks  []*registeredCallback
	beforeList func()
	inFlight   sync.WaitGroup // announced changes not yet handled
}

func newStartupService(tenants ...*domain.KeycloakTenant) *startupService {
	s := &startupService{tenants: map[string]*domain.KeycloakTenant{}}
	for _, t := range tenants {
		s.order = append(s.order, t.TenantID)
		s.tenants[t.TenantID] = t
	}
	return s
}

func (s *startupService) RegisterCallback(cb ChangeCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = append(s.callbacks, newRegisteredCallback(func(e Event, t *domain.KeycloakTenant) {
		defer s.inFlight.Done()
		cb(e, t)
	}))
}

func (s *startupService) GetTenant(_ context.Context, tenantID string) (*domain.KeycloakTenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tenants[tenantID]
	if !ok {
		return nil, nil
	}
	copied := *t
	return &copied, nil
}

func (s *startupService) ListEnabledTenants(_ context.Context) ([]*domain.KeycloakTenant, error) {
	if s.beforeList != nil {
		s.beforeList()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*domain.KeycloakTenant
	for _, id := range s.order {
		if t, ok := s.tenants[id]; ok && t.Enabled {
			copied := *t
			out = append(out, &copied)
		}
	}
	return out, nil
}

// change stores a tenant (nil removes it) and announces the change.
func (s *startupService) change(event Event, t *domain.KeycloakTenant) {
	s.mu.Lock()
	if event == EventRemoved {
		delete(s.tenants, t.TenantID)
	} else {
		if _, ok := s.tenants[t.TenantID]; !ok {
			s.order = append(s.order, t.TenantID)
		}
		stored := *t
		s.tenants[t.TenantID] = &stored
	}
	callbacks := append([]*registeredCallback(nil), s.callbacks...)
	s.inFlight.Add(len(callbacks))
	s.mu.Unlock()
	for _, rc := range callbacks {
		copied := *t
		rc.queue <- callbackJob{event: event, tenant: &copied}
	}
}

// gatedFactory holds the "slow" tenant's client creation until release, the
// way a Keycloak that does not answer holds the token request, and records
// every client created.
type gatedFactory struct {
	mockMonitorFactory
	entered  chan struct{}
	gate     chan struct{}
	mu       sync.Mutex
	creates  map[string]int
	secretOf map[string]string
}

func newGatedFactory() *gatedFactory {
	return &gatedFactory{entered: make(chan struct{}, 1), gate: make(chan struct{}),
		creates: map[string]int{}, secretOf: map[string]string{}}
}

func (f *gatedFactory) CreateClient(_ context.Context, cfg *MonitorConfig, _ any) (KeycloakClient, error) {
	if cfg.TenantID == "slow" {
		select {
		case f.entered <- struct{}{}:
		default:
		}
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates[cfg.TenantID]++
	f.secretOf[cfg.TenantID] = cfg.ClientSecret
	return &mockKeycloakClient{}, nil
}

func (f *gatedFactory) CreateMonitor(string, string, KeycloakClient, any, EventRepository, GeneralEventRepository,
	MetricsRepository, HealthRepository, RealmRepository, EventAlertConverter, HealthAlertManager) KeycloakMonitor {
	return &mockKeycloakMonitor{}
}

func (f *gatedFactory) createdFor(tenantID string) (int, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.creates[tenantID], f.secretOf[tenantID]
}

func startupTenant(id string) *domain.KeycloakTenant {
	return &domain.KeycloakTenant{TenantID: id, Name: "Tenant " + id, ServerURL: "https://kc.example.com",
		AdminRealm: "master", ClientID: "kmt", ClientSecret: "secret", Enabled: true}
}

func running(m *MonitorPoolManager, tenantID string) bool {
	_, ok := m.monitors.Load(tenantID)
	return ok
}

// startHeld starts the pool in the background and returns once it is held on
// the slow tenant; release lets it finish, and waits until it has.
func startHeld(t *testing.T, m *MonitorPoolManager, f *gatedFactory) (release func()) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- m.Start(context.Background()) }()
	<-f.entered
	stopOnCleanup(t, m)
	return func() {
		close(f.gate)
		if err := <-done; err != nil {
			t.Fatalf("Start: %v", err)
		}
	}
}

func TestStartup_TenantAddedWhileStartingIsMonitored(t *testing.T) {
	svc := newStartupService(startupTenant("slow"))
	f := newGatedFactory()
	m := startFailureManager(svc, f, &recordingHealthAlerts{})
	release := startHeld(t, m, f)

	svc.change(EventAdded, startupTenant("new"))
	release()

	waitFor(t, "the tenant added during startup to be monitored", func() bool { return running(m, "new") })
}

func TestStartup_TenantDisabledOrRemovedWhileStartingIsNotMonitored(t *testing.T) {
	for _, event := range []Event{EventDisabled, EventRemoved} {
		t.Run(string(event), func(t *testing.T) {
			svc := newStartupService(startupTenant("slow"), startupTenant("t2"))
			f := newGatedFactory()
			alerts := &recordingHealthAlerts{}
			m := startFailureManager(svc, f, alerts)
			release := startHeld(t, m, f)

			changed := startupTenant("t2")
			changed.Enabled = event != EventDisabled
			svc.change(event, changed)
			release()

			waitFor(t, "the change to be applied", func() bool {
				for _, r := range alerts.all() {
					if r.tenantID == "t2" && r.status == "UNMONITORED" {
						return true
					}
				}
				return false
			})
			if running(m, "t2") {
				t.Errorf("tenant %s during startup is still monitored", event)
			}
		})
	}
}

func TestStartup_TenantUpdatedWhileStartingRunsWithTheNewSettings(t *testing.T) {
	svc := newStartupService(startupTenant("slow"), startupTenant("t2"))
	f := newGatedFactory()
	m := startFailureManager(svc, f, &recordingHealthAlerts{})
	release := startHeld(t, m, f)

	updated := startupTenant("t2")
	updated.ClientSecret = "corrected"
	svc.change(EventUpdated, updated)
	release()

	waitFor(t, "the monitor to run with the updated secret", func() bool {
		_, secret := f.createdFor("t2")
		return running(m, "t2") && secret == "corrected"
	})
}

// A tenant created after the pool subscribes but before it lists is both
// listed and announced; it gets one monitor, and the second start is not a
// failure: no DOWN alert, no retry.
func TestStartup_TenantListedAndAnnouncedGetsOneMonitor(t *testing.T) {
	svc := newStartupService(startupTenant("t1"))
	f := newGatedFactory()
	alerts := &recordingHealthAlerts{}
	m := startFailureManager(svc, f, alerts)
	announced := make(chan struct{})
	svc.beforeList = func() {
		svc.beforeList = nil
		svc.change(EventAdded, startupTenant("t2"))
		close(announced)
	}
	close(f.gate)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopOnCleanup(t, m)
	<-announced
	// The announcement for a tenant already running may still be on its way.
	svc.change(EventEnabled, startupTenant("t2"))
	waitFor(t, "both tenants to be monitored", func() bool { return running(m, "t1") && running(m, "t2") })
	waitForQueue(t, svc)

	assertStartedOnceWithoutAlarm(t, m, f, alerts, "t2")
}

// The notice of a tenant's creation can arrive after startup has already
// started its monitor from the list; it is not a failed start.
func TestStartup_LateNoticeOfARunningTenantIsNotAFailure(t *testing.T) {
	svc := newStartupService(startupTenant("t1"))
	f := newGatedFactory()
	close(f.gate)
	alerts := &recordingHealthAlerts{}
	m := startFailureManager(svc, f, alerts)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopOnCleanup(t, m)

	svc.change(EventAdded, startupTenant("t1"))
	waitForQueue(t, svc)

	assertStartedOnceWithoutAlarm(t, m, f, alerts, "t1")
}

// waitForQueue waits until every change announced so far has been handled.
func waitForQueue(t *testing.T, svc *startupService) {
	t.Helper()
	svc.inFlight.Wait()
}

func assertStartedOnceWithoutAlarm(t *testing.T, m *MonitorPoolManager, f *gatedFactory,
	alerts *recordingHealthAlerts, tenantID string) {
	t.Helper()
	if n, _ := f.createdFor(tenantID); n != 1 {
		t.Errorf("clients created for %s = %d, want 1", tenantID, n)
	}
	if m.retryPending(tenantID) {
		t.Errorf("a retry is pending for %s, which is running", tenantID)
	}
	for _, r := range alerts.all() {
		if r.tenantID == tenantID {
			t.Errorf("health report for %s, which is running: %+v", tenantID, r)
		}
	}
}
