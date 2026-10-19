package fx

import (
	"context"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Acknowledging a health alert says someone is on it, not that it is over. A
// new occurrence starts only from a resolved alert, so a recovery that left an
// acknowledged alert as it was made the tenant's next outage reopen it as the
// same, already announced occurrence: the second outage reached no one.

func acknowledged(id string) *domain.Alert {
	a := activeAlert(id)
	a.Status = domain.AlertStatusAcknowled
	return a
}

func TestHealthAlerts_AnAcknowledgedAlertResolvesOnRecovery(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		alertID string
		run     func(t *testing.T, m *healthAlertManagerImpl)
	}{
		{"down, then up", downID, func(t *testing.T, m *healthAlertManagerImpl) {
			m.previousStatus["tenant-a"] = "DOWN"
			if err := m.ProcessHealthStatus(ctx, "tenant-a", "Tenant A", "UP", "", 0); err != nil {
				t.Fatalf("ProcessHealthStatus: %v", err)
			}
		}},
		{"degraded, then up", degradedID, func(t *testing.T, m *healthAlertManagerImpl) {
			m.previousStatus["tenant-a"] = "DEGRADED"
			if err := m.ProcessHealthStatus(ctx, "tenant-a", "Tenant A", "UP", "", 0); err != nil {
				t.Fatalf("ProcessHealthStatus: %v", err)
			}
		}},
		{"up at the first check after a restart", downID, func(t *testing.T, m *healthAlertManagerImpl) {
			firstCheck(t, m, "UP")
		}},
		{"tenant no longer monitored", downID, func(t *testing.T, m *healthAlertManagerImpl) {
			if err := m.TenantUnmonitored(ctx, "tenant-a", "Tenant A", "disabled"); err != nil {
				t.Fatalf("TenantUnmonitored: %v", err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, repo, notifier := freshManager(acknowledged(tc.alertID))
			tc.run(t, m)
			if got := repo.status(tc.alertID); got != domain.AlertStatusResolved {
				t.Errorf("status = %q, want resolved", got)
			}
			if len(notifier.resolved) != 1 || notifier.resolved[0] != tc.alertID {
				t.Errorf("resolution notifications = %v, want one for %s", notifier.resolved, tc.alertID)
			}
		})
	}
}

func TestHealthAlerts_AnIgnoredAlertIsLeftAlone(t *testing.T) {
	ignored := activeAlert(downID)
	ignored.Status = domain.AlertStatusIgnored
	m, repo, notifier := freshManager(ignored)
	m.previousStatus["tenant-a"] = "DOWN"

	if err := m.ProcessHealthStatus(context.Background(), "tenant-a", "Tenant A", "UP", "", 0); err != nil {
		t.Fatalf("ProcessHealthStatus: %v", err)
	}
	if got := repo.status(downID); got != domain.AlertStatusIgnored || len(notifier.resolved) != 0 {
		t.Errorf("status = %q, resolutions %v; want still ignored, none", got, notifier.resolved)
	}
}
