package metricscheck

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	alertspkg "github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// A read error says nothing about whether the alert exists, so a failed
// lookup must leave the stored alert as it was and request no notification,
// while an alert that is really missing is still raised.

type mapAlertStore struct {
	alerts map[string]*domain.Alert
	getErr error
}

func (m *mapAlertStore) GetAlertByID(_ context.Context, _, id string) (*domain.Alert, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if a, ok := m.alerts[id]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("%w: %s", alertspkg.ErrNotFound, id)
}

func (m *mapAlertStore) SaveAlert(_ context.Context, a *domain.Alert) error {
	m.alerts[a.AlertID] = a
	return nil
}

type countingNotifier struct{ raised int }

func (n *countingNotifier) NotifyAlert(context.Context, *domain.Alert) error {
	n.raised++
	return nil
}

type oneAlertChecker struct{}

func (oneAlertChecker) GetCheckType() string   { return "test-metrics" }
func (oneAlertChecker) GetDescription() string { return "test" }
func (oneAlertChecker) Execute(context.Context) ([]*domain.Alert, error) {
	return []*domain.Alert{{TenantID: "t", AlertID: "metrics-1", Title: "High load", CheckType: "test-metrics"}}, nil
}

func TestExecuteCheck_AFailedLookupLeavesTheStoredAlertAlone(t *testing.T) {
	ackAt := time.Now().Add(-10 * time.Minute)
	stored := &domain.Alert{TenantID: "t", AlertID: "metrics-1", Status: domain.AlertStatusAcknowled,
		AcknowledgedAt: &ackAt, AcknowledgedBy: "operator@example.com", FirstDetected: time.Now().Add(-time.Hour)}
	store := &mapAlertStore{alerts: map[string]*domain.Alert{"metrics-1": stored},
		getErr: errors.New("failed to get alert: driver: bad connection")}
	notifier := &countingNotifier{}

	svc := NewService([]Checker{oneAlertChecker{}}, store, notifier, discardLogger{}, time.Minute).(*service)
	if err := svc.executeCheck(context.Background(), oneAlertChecker{}); err != nil {
		t.Fatalf("executeCheck: %v", err)
	}

	if store.alerts["metrics-1"] != stored {
		t.Fatalf("the stored alert was replaced after a failed lookup")
	}
	if stored.Status != domain.AlertStatusAcknowled || stored.AcknowledgedBy != "operator@example.com" {
		t.Errorf("stored alert = %s by %q, want acknowledged by operator@example.com", stored.Status, stored.AcknowledgedBy)
	}
	if notifier.raised != 0 {
		t.Errorf("notifications = %d, want 0 for a failed lookup", notifier.raised)
	}
}

func TestExecuteCheck_AMissingAlertIsStillRaised(t *testing.T) {
	store := &mapAlertStore{alerts: map[string]*domain.Alert{}}
	notifier := &countingNotifier{}

	svc := NewService([]Checker{oneAlertChecker{}}, store, notifier, discardLogger{}, time.Minute).(*service)
	if err := svc.executeCheck(context.Background(), oneAlertChecker{}); err != nil {
		t.Fatalf("executeCheck: %v", err)
	}

	if got, ok := store.alerts["metrics-1"]; !ok || got.Status != domain.AlertStatusActive {
		t.Errorf("stored alert = %+v, want a new active alert", got)
	}
	if notifier.raised != 1 {
		t.Errorf("notifications = %d, want 1 for a new alert", notifier.raised)
	}
}
