package configcheck

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// A read error says nothing about whether the alert exists, so a failed
// lookup must leave the stored alert as it was and request no notification.

type countingNotifier struct{ raised atomic.Int32 }

func (n *countingNotifier) NotifyAlert(context.Context, *domain.Alert) error {
	n.raised.Add(1)
	return nil
}

func (n *countingNotifier) NotifyAlertResolution(context.Context, *domain.Alert) error {
	return nil
}

func TestExecuteCheck_AFailedLookupLeavesTheStoredAlertAlone(t *testing.T) {
	ackAt := time.Now().Add(-10 * time.Minute)
	stored := &domain.Alert{
		TenantID:       "test-tenant",
		AlertID:        "test-alert-1",
		FirstDetected:  time.Now().Add(-1 * time.Hour),
		LastSeen:       time.Now().Add(-5 * time.Minute),
		Status:         domain.AlertStatusAcknowled,
		AcknowledgedAt: &ackAt,
		AcknowledgedBy: "operator@example.com",
	}
	store := newMockAlertStore()
	store.alerts[stored.AlertID] = stored
	store.getError = errors.New("failed to get alert: driver: bad connection")
	notifier := &countingNotifier{}

	check := &mockConfigurationCheck{
		checkType: "test-check",
		alerts: []*domain.Alert{{
			TenantID:  "test-tenant",
			AlertID:   "test-alert-1",
			Title:     "Test Alert",
			CheckType: "test-check",
		}},
	}
	svc := NewService([]Check{check}, store, notifier, logger.NewNoop(), time.Minute).(*service)
	if err := svc.executeCheck(context.Background(), check); err != nil {
		t.Fatalf("executeCheck: %v", err)
	}

	got := store.alerts["test-alert-1"]
	if got != stored {
		t.Fatalf("the stored alert was replaced after a failed lookup")
	}
	if got.Status != domain.AlertStatusAcknowled || got.AcknowledgedBy != "operator@example.com" {
		t.Errorf("stored alert = %s by %q, want acknowledged by operator@example.com", got.Status, got.AcknowledgedBy)
	}
	if n := notifier.raised.Load(); n != 0 {
		t.Errorf("notifications = %d, want 0 for a failed lookup", n)
	}
}
