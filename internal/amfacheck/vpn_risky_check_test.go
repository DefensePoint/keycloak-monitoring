package amfacheck

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
)

func TestVPNRiskyCheck_ProducesWarningPerEventAlerts(t *testing.T) {
	risk := 3
	repo := &checkRepoStub{vpnRisky: []amfa.EventRow{
		{EventID: "ev-9", EventTime: time.Now(), IsVPN: true, RiskLevel: &risk, IPAddress: "9.9.9.9"},
	}}
	c := NewVPNRiskyCheck("tenant-1", singleRealm("master"), repo, time.Minute, testLogger{})

	alerts, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Severity != "warning" {
		t.Errorf("severity = %q, want warning", a.Severity)
	}
	if a.CheckType != "amfa-vpn-risky" {
		t.Errorf("check_type = %q, want amfa-vpn-risky", a.CheckType)
	}
	want := alertID("amfa-vpn-risky", "tenant-1", "ev-9")
	if a.AlertID != want {
		t.Errorf("AlertID = %q, want %q", a.AlertID, want)
	}
	if a.EventID == nil || *a.EventID != "ev-9" {
		t.Errorf("EventID = %v, want ev-9", a.EventID)
	}
}

func TestVPNRiskyCheck_UsesMinRisk3(t *testing.T) {
	repo := &checkRepoStub{}
	c := NewVPNRiskyCheck("tenant-1", singleRealm("master"), repo, time.Minute, testLogger{})
	if c.minRisk != 3 {
		t.Errorf("minRisk = %d, want 3 (hardcoded 'risk > 2')", c.minRisk)
	}
}
