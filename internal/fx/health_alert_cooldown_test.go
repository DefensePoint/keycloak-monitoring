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

// memoryAlertRepo keeps alerts in a map. Only the methods the health alert
// manager calls are implemented; the embedded interface panics on any other.
type memoryAlertRepo struct {
	alerts.Repository
	mu     sync.Mutex
	alerts map[string]*domain.Alert
}

func (r *memoryAlertRepo) Save(_ context.Context, a *domain.Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := *a
	r.alerts[a.AlertID] = &copied
	return nil
}

func (r *memoryAlertRepo) GetByAlertID(_ context.Context, _, alertID string) (*domain.Alert, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.alerts[alertID]
	if !ok {
		return nil, alerts.ErrNotFound
	}
	copied := *a
	return &copied, nil
}

func (r *memoryAlertRepo) Resolve(_ context.Context, _, alertID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.alerts[alertID]; ok {
		a.Status = domain.AlertStatusResolved
	}
	return nil
}

// countingNotifier records the down notifications it is asked to send.
type countingNotifier struct {
	notifications.Service
	mu   sync.Mutex
	sent []string
}

func (n *countingNotifier) NotifyAlert(_ context.Context, a *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, a.AlertID)
	return nil
}

func (n *countingNotifier) NotifyAlertResolution(context.Context, *domain.Alert) error { return nil }

// A tenant that went down, recovered and went down again within five minutes
// used to be notified only once: an in-memory cooldown dropped the second
// outage. Each outage is its own occurrence and must be announced.
func TestHealthAlerts_ASecondOutageAfterRecoveryIsNotified(t *testing.T) {
	notifier := &countingNotifier{}
	m := &healthAlertManagerImpl{
		alertRepo:      &memoryAlertRepo{alerts: map[string]*domain.Alert{}},
		notifier:       notifier,
		log:            logger.NewNoop(),
		previousStatus: map[string]string{},
	}
	ctx := context.Background()

	for _, status := range []string{"UP", "DOWN", "UP", "DOWN"} {
		if err := m.ProcessHealthStatus(ctx, "tenant-a", "Tenant A", status, "connection refused", 0); err != nil {
			t.Fatalf("ProcessHealthStatus(%s): %v", status, err)
		}
	}

	if len(notifier.sent) != 2 {
		t.Fatalf("down notifications = %d %v, want 2: one per outage", len(notifier.sent), notifier.sent)
	}
	for _, id := range notifier.sent {
		if id != "tenant-health-down-tenant-a" {
			t.Errorf("notified %q, want the tenant down alert", id)
		}
	}
}

// Staying down is one outage: no transition, no second notification.
func TestHealthAlerts_StayingDownNotifiesOnce(t *testing.T) {
	notifier := &countingNotifier{}
	m := &healthAlertManagerImpl{
		alertRepo:      &memoryAlertRepo{alerts: map[string]*domain.Alert{}},
		notifier:       notifier,
		log:            logger.NewNoop(),
		previousStatus: map[string]string{},
	}
	for _, status := range []string{"UP", "DOWN", "DOWN", "DOWN"} {
		if err := m.ProcessHealthStatus(context.Background(), "tenant-a", "Tenant A", status, "timeout", 0); err != nil {
			t.Fatalf("ProcessHealthStatus(%s): %v", status, err)
		}
	}
	if len(notifier.sent) != 1 {
		t.Errorf("down notifications = %d, want 1 while the tenant stays down", len(notifier.sent))
	}
}
