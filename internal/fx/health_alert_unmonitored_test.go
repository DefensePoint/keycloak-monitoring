package fx

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// A disabled or removed tenant runs no health checks, so an open health alert
// for it would stay open, with its GitLab issue, forever.

type noteNotifier struct {
	reconcileNotifier
	mu    sync.Mutex
	notes map[string]string
}

func (n *noteNotifier) NotifyAlertResolution(ctx context.Context, a *domain.Alert) error {
	n.mu.Lock()
	n.notes[a.AlertID] = a.ResolutionNote
	n.mu.Unlock()
	return n.reconcileNotifier.NotifyAlertResolution(ctx, a)
}

func TestTenantUnmonitored_ResolvesOpenAlertsAndSaysWhy(t *testing.T) {
	m, repo, _ := freshManager(activeAlert(downID), activeAlert(degradedID))
	notifier := &noteNotifier{notes: map[string]string{}}
	m.notifier = notifier
	m.previousStatus["tenant-a"] = "DOWN"

	if err := m.TenantUnmonitored(context.Background(), "tenant-a", "Tenant A", "disabled"); err != nil {
		t.Fatalf("TenantUnmonitored: %v", err)
	}
	for _, id := range []string{downID, degradedID} {
		if got := repo.status(id); got != domain.AlertStatusResolved {
			t.Errorf("%s status = %q, want resolved", id, got)
		}
		note := notifier.notes[id]
		if !strings.Contains(note, "disabled") || !strings.Contains(note, "without a recovery") {
			t.Errorf("%s resolution note = %q, want it to say the tenant was disabled and it was not a recovery", id, note)
		}
	}
	if _, tracked := m.previousStatus["tenant-a"]; tracked {
		t.Error("the tenant's tracked status was kept; re-enabling it would not reconcile afresh")
	}
}

func TestTenantUnmonitored_LeavesResolvedAlertsAlone(t *testing.T) {
	resolved := activeAlert(downID)
	resolved.Status = domain.AlertStatusResolved
	m, _, _ := freshManager(resolved)
	notifier := &noteNotifier{notes: map[string]string{}}
	m.notifier = notifier

	if err := m.TenantUnmonitored(context.Background(), "tenant-a", "Tenant A", "removed"); err != nil {
		t.Fatalf("TenantUnmonitored: %v", err)
	}
	if len(notifier.notes) != 0 {
		t.Errorf("resolution notices = %v, want none for an alert that was already resolved", notifier.notes)
	}
}
