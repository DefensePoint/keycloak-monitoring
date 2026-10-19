package fx

import (
	"context"
	"sync"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
)

// The first health check after startup used to set a baseline and nothing
// else. An alert left active by a server that stopped mid-outage then stayed
// active forever if the tenant recovered while it was down, and a tenant that
// went down while the server was down was never alerted.

const (
	downID     = "tenant-health-down-tenant-a"
	degradedID = "tenant-health-degraded-tenant-a"
)

type reconcileAlertRepo struct {
	alerts.Repository
	mu     sync.Mutex
	stored map[string]*domain.Alert
}

func (r *reconcileAlertRepo) Save(_ context.Context, a *domain.Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := *a
	r.stored[a.AlertID] = &copied
	return nil
}

func (r *reconcileAlertRepo) GetByAlertID(_ context.Context, _, alertID string) (*domain.Alert, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.stored[alertID]
	if !ok {
		return nil, alerts.ErrNotFound
	}
	copied := *a
	return &copied, nil
}

func (r *reconcileAlertRepo) Resolve(_ context.Context, _, alertID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.stored[alertID]; ok {
		a.Status = domain.AlertStatusResolved
	}
	return nil
}

func (r *reconcileAlertRepo) status(alertID string) domain.AlertStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.stored[alertID]; ok {
		return a.Status
	}
	return ""
}

type reconcileNotifier struct {
	notifications.Service
	mu       sync.Mutex
	alerted  []string
	resolved []string
}

func (n *reconcileNotifier) NotifyAlert(_ context.Context, a *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.alerted = append(n.alerted, a.AlertID)
	return nil
}

func (n *reconcileNotifier) NotifyAlertResolution(_ context.Context, a *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.resolved = append(n.resolved, a.AlertID)
	return nil
}

// freshManager is a health alert manager that has just started: no tenant has
// a previous status, whatever the stored alerts say.
func freshManager(stored ...*domain.Alert) (*healthAlertManagerImpl, *reconcileAlertRepo, *reconcileNotifier) {
	repo := &reconcileAlertRepo{stored: map[string]*domain.Alert{}}
	for _, a := range stored {
		repo.stored[a.AlertID] = a
	}
	notifier := &reconcileNotifier{}
	return &healthAlertManagerImpl{
		alertRepo:      repo,
		notifier:       notifier,
		log:            logger.NewNoop(),
		previousStatus: map[string]string{},
	}, repo, notifier
}

func activeAlert(id string) *domain.Alert {
	return &domain.Alert{TenantID: "tenant-a", AlertID: id, Status: domain.AlertStatusActive, Title: id}
}

func firstCheck(t *testing.T, m *healthAlertManagerImpl, status string) {
	t.Helper()
	if err := m.ProcessHealthStatus(context.Background(), "tenant-a", "Tenant A", status, "connection refused", 0); err != nil {
		t.Fatalf("ProcessHealthStatus(%s): %v", status, err)
	}
}

func TestHealthReconcile_RecoveredWhileStoppedResolvesTheStoredAlert(t *testing.T) {
	m, repo, notifier := freshManager(activeAlert(downID))
	firstCheck(t, m, "UP")

	if got := repo.status(downID); got != domain.AlertStatusResolved {
		t.Errorf("down alert status = %q, want resolved", got)
	}
	if len(notifier.resolved) != 1 || notifier.resolved[0] != downID {
		t.Errorf("resolution notices = %v, want one for the down alert", notifier.resolved)
	}
	if len(notifier.alerted) != 0 {
		t.Errorf("alerts sent = %v, want none for a healthy tenant", notifier.alerted)
	}
}

func TestHealthReconcile_RecoveredWhileStoppedResolvesADegradedAlertToo(t *testing.T) {
	m, repo, _ := freshManager(activeAlert(degradedID))
	firstCheck(t, m, "UP")

	if got := repo.status(degradedID); got != domain.AlertStatusResolved {
		t.Errorf("degraded alert status = %q, want resolved", got)
	}
}

func TestHealthReconcile_WentDownWhileStoppedIsAlerted(t *testing.T) {
	m, repo, notifier := freshManager()
	firstCheck(t, m, "DOWN")

	if got := repo.status(downID); got != domain.AlertStatusActive {
		t.Errorf("down alert status = %q, want active", got)
	}
	if len(notifier.alerted) != 1 || notifier.alerted[0] != downID {
		t.Errorf("alerts sent = %v, want one for the down alert", notifier.alerted)
	}
}

func TestHealthReconcile_DegradedResolvesDownAndRaisesDegraded(t *testing.T) {
	m, repo, _ := freshManager(activeAlert(downID))
	firstCheck(t, m, "DEGRADED")

	if got := repo.status(downID); got != domain.AlertStatusResolved {
		t.Errorf("down alert status = %q, want resolved", got)
	}
	if got := repo.status(degradedID); got != domain.AlertStatusActive {
		t.Errorf("degraded alert status = %q, want active", got)
	}
}

func TestHealthReconcile_HealthyWithNothingStoredDoesNothing(t *testing.T) {
	m, repo, notifier := freshManager()
	firstCheck(t, m, "UP")

	if len(repo.stored) != 0 || len(notifier.alerted) != 0 || len(notifier.resolved) != 0 {
		t.Errorf("stored %d alert(s), alerted %v, resolved %v; want nothing", len(repo.stored), notifier.alerted, notifier.resolved)
	}
}
