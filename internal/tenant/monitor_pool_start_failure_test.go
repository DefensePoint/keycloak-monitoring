package tenant

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// A monitor that cannot start runs no health checks, so a tenant unreachable
// at startup used to raise no health alert. The failed start must now be
// reported the way a failed health check is. The tenant record is marked
// unhealthy by startMonitorForTenant itself; that is checked here too, so the
// alert and the record agree.

type healthReport struct{ tenantID, tenantName, status, message string }

type recordingHealthAlerts struct {
	mu      sync.Mutex
	reports []healthReport
}

func (r *recordingHealthAlerts) ProcessHealthStatus(_ context.Context, tenantID, tenantName, status, message string, _ int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports = append(r.reports, healthReport{tenantID, tenantName, status, message})
	return nil
}

func (r *recordingHealthAlerts) TenantUnmonitored(_ context.Context, tenantID, tenantName, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports = append(r.reports, healthReport{tenantID, tenantName, "UNMONITORED", reason})
	return nil
}

func (r *recordingHealthAlerts) all() []healthReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]healthReport(nil), r.reports...)
}

type healthRecordingService struct {
	*retryService
	mu     sync.Mutex
	health []string
	errors []string
}

func (s *healthRecordingService) UpdateHealth(_ context.Context, _, status, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health = append(s.health, status+": "+message)
	return nil
}

func (s *healthRecordingService) UpdateError(_ context.Context, _, msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errors = append(s.errors, msg)
	return nil
}

func (s *healthRecordingService) recorded() ([]string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.health...), append([]string(nil), s.errors...)
}

func startFailureManager(svc Service, f KeycloakMonitorFactory, alerts HealthAlertManager) *MonitorPoolManager {
	m := NewMonitorPoolManager(svc, f, nil, &mockLogger{}, nil, nil, nil, nil, nil, nil, alerts)
	m.retryInitial = 10 * time.Millisecond
	m.retryMax = 40 * time.Millisecond
	return m
}

func TestStartFailure_IsReportedAsDownAndUnhealthy(t *testing.T) {
	svc := &healthRecordingService{retryService: newRetryService(retryTenant())}
	alerts := &recordingHealthAlerts{}
	m := startFailureManager(svc, &flakyFactory{}, alerts) // never starts
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopOnCleanup(t, m)

	waitFor(t, "the failed start to be reported", func() bool { return len(alerts.all()) > 0 })
	first := alerts.all()[0]
	if first.tenantID != "t1" || first.tenantName != "Tenant 1" || first.status != "DOWN" {
		t.Errorf("first report = %+v, want tenant t1 / Tenant 1 reported DOWN", first)
	}
	if !strings.Contains(first.message, "keycloak unreachable") {
		t.Errorf("report message = %q, want the start error", first.message)
	}

	health, errs := svc.recorded()
	if len(health) == 0 || !strings.HasPrefix(health[0], "unhealthy: ") || !strings.Contains(health[0], "keycloak unreachable") {
		t.Errorf("tenant health updates = %v, want unhealthy with the start error", health)
	}
	if len(errs) == 0 || !strings.Contains(errs[0], "keycloak unreachable") {
		t.Errorf("tenant error updates = %v, want the start error recorded", errs)
	}

	// Every report is DOWN: retries that keep failing never claim the tenant is up.
	waitFor(t, "a failed retry to be reported too", func() bool { return len(alerts.all()) >= 2 })
	for _, r := range alerts.all() {
		if r.status != "DOWN" {
			t.Errorf("report %+v, want DOWN while the monitor cannot start", r)
		}
	}
}

func TestStartFailure_ASuccessfulStartReportsNothing(t *testing.T) {
	svc := &healthRecordingService{retryService: newRetryService(retryTenant())}
	alerts := &recordingHealthAlerts{}
	m := startFailureManager(svc, &flakyFactory{ok: func(*MonitorConfig, int) bool { return true }}, alerts)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopOnCleanup(t, m)

	time.Sleep(50 * time.Millisecond)
	if got := alerts.all(); len(got) != 0 {
		t.Errorf("health reports for a monitor that started = %+v, want none", got)
	}
	// The pool already marks a started tenant healthy; it must never be marked
	// unhealthy on the way.
	for _, h := range func() []string { h, _ := svc.recorded(); return h }() {
		if strings.HasPrefix(h, "unhealthy") {
			t.Errorf("tenant marked %q although its monitor started", h)
		}
	}
}

// Disabling or removing a tenant must ask for its open health alerts to be
// resolved: nothing monitors it afterwards.
func TestDisableAndRemove_CloseTheTenantsHealthAlerts(t *testing.T) {
	for _, tc := range []struct {
		event Event
		how   string
	}{{EventDisabled, "disabled"}, {EventRemoved, "removed"}} {
		t.Run(tc.how, func(t *testing.T) {
			svc := &healthRecordingService{retryService: newRetryService(retryTenant())}
			alerts := &recordingHealthAlerts{}
			m := startFailureManager(svc, &flakyFactory{ok: func(*MonitorConfig, int) bool { return true }}, alerts)
			if err := m.Start(context.Background()); err != nil {
				t.Fatalf("Start: %v", err)
			}
			stopOnCleanup(t, m)

			m.handleTenantChange(tc.event, retryTenant())

			found := false
			for _, r := range alerts.all() {
				if r.status == "UNMONITORED" && r.tenantID == "t1" && r.message == tc.how {
					found = true
				}
			}
			if !found {
				t.Errorf("health alert calls = %+v, want TenantUnmonitored(t1, %q)", alerts.all(), tc.how)
			}
		})
	}
}
