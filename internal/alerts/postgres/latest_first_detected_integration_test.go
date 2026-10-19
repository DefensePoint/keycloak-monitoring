package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// A restarted per-event AMFA check resumes from LatestFirstDetected, so it must
// see only this tenant's, check's and realm's alerts, and count deleted ones:
// a deleted alert's event was still handled.
func TestLatestFirstDetected(t *testing.T) {
	r, tenant := newOpenByCheckRepo(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)

	seed := func(tenantID, alertID, checkType, realm string, at time.Time) {
		t.Helper()
		if err := r.Save(ctx, &domain.Alert{
			TenantID: tenantID, AlertID: alertID, CheckType: checkType, Status: domain.AlertStatusActive,
			Source: domain.AlertSourceEvent, Type: domain.AlertTypeSecurity, Severity: domain.AlertSeverityCritical,
			Title: alertID, ResourceType: "amfa_event", ResourceID: alertID, ResourceName: alertID, RealmName: realm,
			FirstDetected: at, LastSeen: at,
		}); err != nil {
			t.Fatalf("seed %s: %v", alertID, err)
		}
	}
	seed(tenant, "older", "amfa-risk-rejected", "r1", base)
	seed(tenant, "newest-deleted", "amfa-risk-rejected", "r1", base.Add(20*time.Minute))
	seed(tenant, "other-realm", "amfa-risk-rejected", "r2", base.Add(40*time.Minute))
	seed(tenant, "other-check", "amfa-vpn-risky", "r1", base.Add(40*time.Minute))
	seed(tenant+"-other", "other-tenant", "amfa-risk-rejected", "r1", base.Add(40*time.Minute))
	if err := r.Delete(ctx, tenant, "newest-deleted"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	at, ok, err := r.LatestFirstDetected(ctx, tenant, "amfa-risk-rejected", "r1")
	if err != nil || !ok {
		t.Fatalf("LatestFirstDetected: %v, ok=%v", err, ok)
	}
	if want := base.Add(20 * time.Minute); !at.Equal(want) {
		t.Errorf("latest = %v, want %v (the deleted alert counts)", at, want)
	}

	if _, ok, err := r.LatestFirstDetected(ctx, tenant, "amfa-risk-rejected", "r-none"); err != nil || ok {
		t.Errorf("realm with no alerts: ok=%v err=%v, want ok=false", ok, err)
	}
}
