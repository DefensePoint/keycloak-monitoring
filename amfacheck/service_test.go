package amfacheck

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// --- test doubles ---

type fakeCheck struct {
	checkType string
	alerts    []*domain.Alert
	err       error
	calls     int
	mu        sync.Mutex
}

func (f *fakeCheck) GetCheckType() string   { return f.checkType }
func (f *fakeCheck) GetDescription() string { return "fake check " + f.checkType }
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

func (testLogger) Info(string, ...any)           {}
func (testLogger) Error(string, ...any)          {}
func (testLogger) Warn(string, ...any)           {}
func (testLogger) Debug(string, ...any)          {}
func (l testLogger) WithComponent(string) Logger { return l }

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
	// A failing check must NOT block the others (best-effort), but RunCheckNow
	// must SURFACE the failure to the caller so a manual trigger can report it.
	err := svc.RunCheckNow(context.Background())
	if err == nil {
		t.Fatal("RunCheckNow should surface the failed check's error, got nil")
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

func TestService_RunCheckNow_SurfacesAmfaUnavailable(t *testing.T) {
	store := newFakeAlertStore()
	notifier := newFakeNotifier()
	// A check whose Execute fails with an ErrAmfaUnavailable-wrapped error.
	bad := &fakeCheck{checkType: "amfa-risk-rejected", err: fmt.Errorf("realm=master: %w", amfa.ErrAmfaUnavailable)}

	svc := NewService("tenant-1", []Check{bad}, store, notifier, testLogger{}, time.Minute)
	err := svc.RunCheckNow(context.Background())
	if !errors.Is(err, amfa.ErrAmfaUnavailable) {
		t.Fatalf("RunCheckNow error = %v, want it to wrap amfa.ErrAmfaUnavailable", err)
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
