package fx

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
)

// A tenant whose events keep failing to be stored used to show it only in the
// server log. These pin when the alert is raised, when it resolves, and that
// each is announced once.

const storageID = "tenant-event-storage-tenant-a"

type storageNotifier struct {
	notifications.Service
	mu       sync.Mutex
	raised   int
	resolved int
}

func (n *storageNotifier) NotifyAlert(context.Context, *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.raised++
	return nil
}

func (n *storageNotifier) NotifyAlertResolution(context.Context, *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.resolved++
	return nil
}

func newStorageManager(stored ...*domain.Alert) (*eventStorageAlertManager, *memoryAlertRepo, *storageNotifier) {
	repo := &memoryAlertRepo{alerts: map[string]*domain.Alert{}}
	for _, a := range stored {
		repo.alerts[a.AlertID] = a
	}
	notifier := &storageNotifier{}
	return newEventStorageAlertManager(repo, notifier, nil, logger.NewNoop()), repo, notifier
}

var (
	failing = keycloak.EventStorageOutcome{Attempted: 4, Failed: 4, FirstError: errors.New("duplicate key value violates unique constraint")}
	stored  = keycloak.EventStorageOutcome{Attempted: 4}
	nothing = keycloak.EventStorageOutcome{}
)

func report(m *eventStorageAlertManager, outcomes ...keycloak.EventStorageOutcome) {
	for _, o := range outcomes {
		m.ReportEventStorage(context.Background(), "tenant-a", "Tenant A", o)
	}
}

func storageStatus(repo *memoryAlertRepo) domain.AlertStatus {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if a, ok := repo.alerts[storageID]; ok {
		return a.Status
	}
	return ""
}

func TestEventStorage_ThreeFailingCyclesRaiseOneAlert(t *testing.T) {
	m, repo, notifier := newStorageManager()

	report(m, failing, failing)
	if got := storageStatus(repo); got != "" {
		t.Fatalf("alert after two failing cycles = %q, want none yet", got)
	}

	report(m, failing)
	if got := storageStatus(repo); got != domain.AlertStatusActive {
		t.Fatalf("alert after three failing cycles = %q, want active", got)
	}
	a := repo.alerts[storageID]
	if a.Severity != domain.AlertSeverityCritical || a.CheckType != "event_storage" || a.ResourceID != "tenant-a" {
		t.Errorf("alert = severity %s, check %s, resource %s; want a critical event_storage alert for tenant-a", a.Severity, a.CheckType, a.ResourceID)
	}
	if !strings.Contains(a.Description, "duplicate key value") || !strings.Contains(a.Description, "4 of 4") {
		t.Errorf("description %q does not carry the error and the counts", a.Description)
	}

	report(m, failing, failing, failing, failing)
	if notifier.raised != 1 {
		t.Errorf("raise notifications = %d over seven failing cycles, want 1", notifier.raised)
	}
}

func TestEventStorage_ACycleThatStoresEverythingResolves(t *testing.T) {
	m, repo, notifier := newStorageManager()
	report(m, failing, failing, failing)

	report(m, stored)
	if got := storageStatus(repo); got != domain.AlertStatusResolved {
		t.Fatalf("status after a cycle that stored everything = %q, want resolved", got)
	}
	report(m, stored, stored)
	if notifier.resolved != 1 {
		t.Errorf("resolution notifications = %d, want 1", notifier.resolved)
	}
}

// Nothing to store says nothing about storage: it neither counts as failing
// nor resolves.
func TestEventStorage_ACycleWithNothingToStoreChangesNothing(t *testing.T) {
	m, repo, _ := newStorageManager()
	report(m, failing, failing, nothing, failing)
	if got := storageStatus(repo); got != domain.AlertStatusActive {
		t.Fatalf("status = %q, want active: an empty cycle must not break the run of failures", got)
	}
	report(m, nothing, nothing)
	if got := storageStatus(repo); got != domain.AlertStatusActive {
		t.Errorf("status after empty cycles = %q, want still active", got)
	}
}

func TestEventStorage_AnInterruptedRunStartsOver(t *testing.T) {
	m, repo, notifier := newStorageManager()
	report(m, failing, failing, stored, failing, failing)
	if got := storageStatus(repo); got != "" || notifier.raised != 0 {
		t.Errorf("status = %q, raised %d: two failing cycles after a good one must not alert", got, notifier.raised)
	}
}

func TestEventStorage_AfterARestart(t *testing.T) {
	left := func() *domain.Alert {
		return &domain.Alert{TenantID: "tenant-a", AlertID: storageID, Status: domain.AlertStatusActive, Description: "left"}
	}

	t.Run("storing again resolves the alert left from before", func(t *testing.T) {
		m, repo, notifier := newStorageManager(left())
		report(m, stored)
		if got := storageStatus(repo); got != domain.AlertStatusResolved || notifier.resolved != 1 {
			t.Errorf("status = %q, resolutions %d; want resolved once", got, notifier.resolved)
		}
	})

	t.Run("still failing keeps it active", func(t *testing.T) {
		m, repo, notifier := newStorageManager(left())
		report(m, failing, failing, failing, failing)
		if got := storageStatus(repo); got != domain.AlertStatusActive || notifier.resolved != 0 {
			t.Errorf("status = %q, resolutions %d; want still active, none", got, notifier.resolved)
		}
	})
}

// Acknowledging the alert says someone is on it, not that it is over. It must
// still resolve when events are stored again: a new occurrence starts only
// from a resolved alert, so left acknowledged, the next outage would reopen it
// unannounced.
func TestEventStorage_AnAcknowledgedAlertResolvesToo(t *testing.T) {
	m, repo, notifier := newStorageManager()
	report(m, failing, failing, failing)
	repo.alerts[storageID].Status = domain.AlertStatusAcknowled

	report(m, stored)
	if got := storageStatus(repo); got != domain.AlertStatusResolved || notifier.resolved != 1 {
		t.Errorf("status = %q, resolutions %d; want resolved once", got, notifier.resolved)
	}
}

func TestEventStorage_AnIgnoredAlertIsLeftAlone(t *testing.T) {
	m, repo, notifier := newStorageManager()
	report(m, failing, failing, failing)
	repo.alerts[storageID].Status = domain.AlertStatusIgnored

	report(m, stored)
	if got := storageStatus(repo); got != domain.AlertStatusIgnored || notifier.resolved != 0 {
		t.Errorf("status = %q, resolutions %d; want still ignored, none", got, notifier.resolved)
	}
}

func TestTenantUnmonitored_ResolvesTheEventStorageAlert(t *testing.T) {
	repo := &memoryAlertRepo{alerts: map[string]*domain.Alert{
		storageID: {TenantID: "tenant-a", AlertID: storageID, Status: domain.AlertStatusActive},
	}}
	m := &healthAlertManagerImpl{alertRepo: repo, notifier: &storageNotifier{}, log: logger.NewNoop(), previousStatus: map[string]string{}}

	if err := m.TenantUnmonitored(context.Background(), "tenant-a", "Tenant A", "removed"); err != nil {
		t.Fatalf("TenantUnmonitored: %v", err)
	}
	if got := storageStatus(repo); got != domain.AlertStatusResolved {
		t.Errorf("event storage alert status = %q, want resolved", got)
	}
}
