package configcheck

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// mockConfigurationCheck implements Check for testing
type mockConfigurationCheck struct {
	checkType   string
	description string
	alerts      []*domain.Alert
	err         error
	// executeCalls is atomic because the service increments it from its own
	// goroutines (Start launches an immediate runChecks plus a poller) while
	// tests read it, which the race detector flags intermittently.
	executeCalls atomic.Int64
}

func (m *mockConfigurationCheck) GetCheckType() string {
	return m.checkType
}

func (m *mockConfigurationCheck) GetDescription() string {
	return m.description
}

func (m *mockConfigurationCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	m.executeCalls.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	return m.alerts, nil
}

// mockAlertStore implements AlertStore for testing
type mockAlertStore struct {
	alerts    map[string]*domain.Alert
	saveError error
	getError  error
}

func newMockAlertStore() *mockAlertStore {
	return &mockAlertStore{
		alerts: make(map[string]*domain.Alert),
	}
}

func (m *mockAlertStore) SaveAlert(ctx context.Context, alert *domain.Alert) error {
	if m.saveError != nil {
		return m.saveError
	}
	m.alerts[alert.AlertID] = alert
	return nil
}

func (m *mockAlertStore) GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	if alert, ok := m.alerts[alertID]; ok {
		return alert, nil
	}
	return nil, errors.New("alert not found")
}

func TestNewService(t *testing.T) {
	alertStore := newMockAlertStore()
	log := logger.NewNoop()
	pollInterval := 5 * time.Minute

	check := &mockConfigurationCheck{
		checkType:   "test-check",
		description: "Test check",
	}
	checks := []Check{check}

	service := NewService(checks, alertStore, nil, log, pollInterval)

	if service == nil {
		t.Fatal("NewService returned nil")
	}
}

func TestService_AddCheck(t *testing.T) {
	alertStore := newMockAlertStore()
	log := logger.NewNoop()
	svc := NewService([]Check{}, alertStore, nil, log, 1*time.Minute)

	// Access the underlying service to check internal state
	s := svc.(*service)

	check1 := &mockConfigurationCheck{checkType: "check-1"}
	check2 := &mockConfigurationCheck{checkType: "check-2"}

	s.AddCheck(check1)
	if len(s.checks) != 1 {
		t.Errorf("expected 1 check after first add, got %d", len(s.checks))
	}

	s.AddCheck(check2)
	if len(s.checks) != 2 {
		t.Errorf("expected 2 checks after second add, got %d", len(s.checks))
	}
}

func TestService_ExecuteCheck(t *testing.T) {
	tests := []struct {
		name          string
		check         *mockConfigurationCheck
		existingAlert *domain.Alert
		expectedSaves int
		expectError   bool
	}{
		{
			name: "New alert - creates with active status",
			check: &mockConfigurationCheck{
				checkType: "test-check",
				alerts: []*domain.Alert{
					{
						TenantID:     "test-tenant",
						AlertID:      "test-alert-1",
						Type:         domain.AlertTypeConfiguration,
						Severity:     domain.AlertSeverityWarning,
						Title:        "Test Alert",
						Description:  "Test Description",
						ResourceType: "test",
						ResourceID:   "test-id",
						ResourceName: "test-name",
						RealmName:    "test-realm",
						CheckType:    "test-check",
					},
				},
			},
			existingAlert: nil,
			expectedSaves: 1,
			expectError:   false,
		},
		{
			name: "Existing alert - updates last seen",
			check: &mockConfigurationCheck{
				checkType: "test-check",
				alerts: []*domain.Alert{
					{
						TenantID:     "test-tenant",
						AlertID:      "test-alert-1",
						Type:         domain.AlertTypeConfiguration,
						Severity:     domain.AlertSeverityWarning,
						Title:        "Test Alert",
						Description:  "Test Description",
						ResourceType: "test",
						ResourceID:   "test-id",
						ResourceName: "test-name",
						RealmName:    "test-realm",
						CheckType:    "test-check",
					},
				},
			},
			existingAlert: &domain.Alert{
				TenantID:      "test-tenant",
				AlertID:       "test-alert-1",
				FirstDetected: time.Now().Add(-1 * time.Hour),
				LastSeen:      time.Now().Add(-30 * time.Minute),
				Status:        domain.AlertStatusActive,
			},
			expectedSaves: 1,
			expectError:   false,
		},
		{
			name: "Check returns error - handles gracefully",
			check: &mockConfigurationCheck{
				checkType: "test-check",
				err:       errors.New("check failed"),
			},
			existingAlert: nil,
			expectedSaves: 0,
			expectError:   true,
		},
		{
			name: "No alerts detected",
			check: &mockConfigurationCheck{
				checkType: "test-check",
				alerts:    []*domain.Alert{},
			},
			existingAlert: nil,
			expectedSaves: 0,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alertStore := newMockAlertStore()
			if tt.existingAlert != nil {
				alertStore.alerts[tt.existingAlert.AlertID] = tt.existingAlert
			}

			log := logger.NewNoop()
			svc := NewService([]Check{}, alertStore, nil, log, 1*time.Minute)
			s := svc.(*service)

			ctx := context.Background()
			err := s.executeCheck(ctx, tt.check)

			if (err != nil) != tt.expectError {
				t.Errorf("executeCheck() error = %v, expectError %v", err, tt.expectError)
			}

			if len(alertStore.alerts) != tt.expectedSaves {
				t.Errorf("expected %d saved alerts, got %d", tt.expectedSaves, len(alertStore.alerts))
			}

			// Validate alert timestamps and status
			if tt.expectedSaves > 0 && !tt.expectError {
				for _, alert := range alertStore.alerts {
					if alert.FirstDetected.IsZero() {
						t.Error("FirstDetected not set")
					}
					if alert.LastSeen.IsZero() {
						t.Error("LastSeen not set")
					}
					if alert.Status == "" {
						t.Error("Status not set")
					}

					// If updating existing alert, FirstDetected should be preserved
					if tt.existingAlert != nil {
						timeDiff := alert.FirstDetected.Sub(tt.existingAlert.FirstDetected)
						if timeDiff.Abs() > time.Second {
							t.Error("FirstDetected should be preserved for existing alerts")
						}
					}
				}
			}
		})
	}
}

func TestService_RunChecks(t *testing.T) {
	alertStore := newMockAlertStore()
	log := logger.NewNoop()

	check1 := &mockConfigurationCheck{
		checkType: "check-1",
		alerts: []*domain.Alert{
			{
				TenantID:     "test-tenant",
				AlertID:      "alert-1",
				Type:         domain.AlertTypeConfiguration,
				Severity:     domain.AlertSeverityWarning,
				Title:        "Alert 1",
				ResourceType: "test",
				ResourceID:   "id-1",
				ResourceName: "name-1",
				RealmName:    "realm-1",
				CheckType:    "check-1",
			},
		},
	}

	check2 := &mockConfigurationCheck{
		checkType: "check-2",
		alerts: []*domain.Alert{
			{
				TenantID:     "test-tenant",
				AlertID:      "alert-2",
				Type:         domain.AlertTypeConfiguration,
				Severity:     domain.AlertSeverityCritical,
				Title:        "Alert 2",
				ResourceType: "test",
				ResourceID:   "id-2",
				ResourceName: "name-2",
				RealmName:    "realm-2",
				CheckType:    "check-2",
			},
		},
	}

	svc := NewService([]Check{check1, check2}, alertStore, nil, log, 1*time.Minute)
	s := svc.(*service)

	ctx := context.Background()
	s.runChecks(ctx)

	// Both checks should have been executed
	if check1.executeCalls.Load() != 1 {
		t.Errorf("check1 executed %d times, expected 1", check1.executeCalls.Load())
	}
	if check2.executeCalls.Load() != 1 {
		t.Errorf("check2 executed %d times, expected 1", check2.executeCalls.Load())
	}

	// Both alerts should be saved
	if len(alertStore.alerts) != 2 {
		t.Errorf("expected 2 alerts, got %d", len(alertStore.alerts))
	}
}

func TestService_RunChecks_ErrorDoesNotStopOtherChecks(t *testing.T) {
	alertStore := newMockAlertStore()
	log := logger.NewNoop()

	failingCheck := &mockConfigurationCheck{
		checkType: "failing-check",
		err:       errors.New("check failed"),
	}

	successCheck := &mockConfigurationCheck{
		checkType: "success-check",
		alerts: []*domain.Alert{
			{
				TenantID:     "test-tenant",
				AlertID:      "success-alert",
				Type:         domain.AlertTypeConfiguration,
				Severity:     domain.AlertSeverityInfo,
				Title:        "Success",
				ResourceType: "test",
				ResourceID:   "id",
				ResourceName: "name",
				RealmName:    "realm",
				CheckType:    "success-check",
			},
		},
	}

	svc := NewService([]Check{failingCheck, successCheck}, alertStore, nil, log, 1*time.Minute)
	s := svc.(*service)

	ctx := context.Background()
	s.runChecks(ctx)

	// Both checks should have been attempted
	if failingCheck.executeCalls.Load() != 1 {
		t.Error("failing check was not executed")
	}
	if successCheck.executeCalls.Load() != 1 {
		t.Error("success check was not executed")
	}

	// Only successful check's alert should be saved
	if len(alertStore.alerts) != 1 {
		t.Errorf("expected 1 alert, got %d", len(alertStore.alerts))
	}
}

func TestService_Start_Stop(t *testing.T) {
	alertStore := newMockAlertStore()
	log := logger.NewNoop()

	check := &mockConfigurationCheck{
		checkType: "test-check",
		alerts:    []*domain.Alert{},
	}

	// Use very short interval for testing
	svc := NewService([]Check{check}, alertStore, nil, log, 10*time.Millisecond)

	ctx := context.Background()
	err := svc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Wait for at least one poll cycle
	time.Sleep(50 * time.Millisecond)

	// Stop the service
	err = svc.Stop()
	if err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}

	// Check should have been executed at least once
	if check.executeCalls.Load() == 0 {
		t.Error("check was never executed")
	}

	executedTimes := check.executeCalls.Load()

	// Wait a bit more to ensure polling has stopped
	time.Sleep(50 * time.Millisecond)

	// Execution count should not increase after stop
	if check.executeCalls.Load() != executedTimes {
		t.Error("check continued executing after Stop()")
	}
}

func TestService_RunCheckNow(t *testing.T) {
	alertStore := newMockAlertStore()
	log := logger.NewNoop()

	check := &mockConfigurationCheck{
		checkType: "test-check",
		alerts:    []*domain.Alert{},
	}

	svc := NewService([]Check{check}, alertStore, nil, log, 1*time.Hour)

	ctx := context.Background()
	err := svc.RunCheckNow(ctx)
	if err != nil {
		t.Errorf("RunCheckNow() returned error: %v", err)
	}

	// Check should have been executed once
	if check.executeCalls.Load() != 1 {
		t.Errorf("expected check to execute 1 time, executed %d times", check.executeCalls.Load())
	}

	// Run again
	err = svc.RunCheckNow(ctx)
	if err != nil {
		t.Errorf("RunCheckNow() second call returned error: %v", err)
	}

	// Should have executed twice total
	if check.executeCalls.Load() != 2 {
		t.Errorf("expected check to execute 2 times, executed %d times", check.executeCalls.Load())
	}
}

// Stop must not return while the immediate first run launched by Start is
// still in flight.
//
// Start fires runChecks in its own goroutine and then starts the poller. Only
// the poller signalled completion, so Stop drained the ticker and returned
// while that first run was still executing checks and writing alerts, letting
// a caller tear down the alert store underneath it.
//
// The check is gated on channels rather than a sleep so the assertion cannot
// go soft under load: the run is provably in flight before Stop is called, and
// provably still in flight while Stop is blocked.
func TestService_Stop_WaitsForInitialRun(t *testing.T) {
	check := newGatedCheck()
	// A poll interval far longer than the test: the only run in flight is the
	// immediate one Start launches.
	svc := NewService([]Check{check}, newMockAlertStore(), nil, logger.NewNoop(), time.Hour)

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Wait for the run to actually reach the check.
	select {
	case <-check.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the initial run never reached the check")
	}

	stopped := make(chan struct{})
	go func() {
		_ = svc.Stop()
		close(stopped)
	}()

	// Stop must still be blocked, because the check has not been released.
	select {
	case <-stopped:
		t.Fatal("Stop() returned while the initial run was still in flight")
	case <-time.After(100 * time.Millisecond):
	}

	close(check.release)

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return after the initial run finished")
	}

	if got := check.completedCalls.Load(); got != 1 {
		t.Errorf("completed %d checks, want 1", got)
	}
}

// gatedCheck blocks inside Execute until the test releases it, so a test can
// hold a run in flight deterministically rather than racing a sleep.
type gatedCheck struct {
	entered        chan struct{}
	release        chan struct{}
	once           sync.Once
	completedCalls atomic.Int64
}

func newGatedCheck() *gatedCheck {
	return &gatedCheck{entered: make(chan struct{}), release: make(chan struct{})}
}

func (c *gatedCheck) GetCheckType() string   { return "gated-check" }
func (c *gatedCheck) GetDescription() string { return "blocks until the test releases it" }

func (c *gatedCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	c.once.Do(func() { close(c.entered) })
	<-c.release
	c.completedCalls.Add(1)
	return nil, nil
}
