package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// vpnRiskyMinRisk is the inclusive lower risk bound for rule 3 (SQL
// risk_level >= 3), matching the brainstormed "risk > 2" semantics. Hardcoded;
// see the spec for how to make it configurable later.
const vpnRiskyMinRisk = 3

// VPNRiskyCheck implements rule 3: one warning alert per AMFA event that is
// VPN-flagged AND has risk >= 3.
type VPNRiskyCheck struct {
	tenantID string
	realmsFn RealmsFunc
	repo     amfa.Repository
	minRisk  int
	lookback time.Duration
	logger   Logger

	mu       sync.Mutex
	lastSeen map[string]time.Time
}

// NewVPNRiskyCheck constructs rule 3.
func NewVPNRiskyCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, lookback time.Duration, log Logger) *VPNRiskyCheck {
	return &VPNRiskyCheck{
		tenantID: tenantID,
		realmsFn: realmsFn,
		repo:     repo,
		minRisk:  vpnRiskyMinRisk,
		lookback: lookback,
		logger:   log.WithComponent("amfa-vpn-risky"),
		lastSeen: make(map[string]time.Time),
	}
}

func (c *VPNRiskyCheck) GetCheckType() string { return "amfa-vpn-risky" }
func (c *VPNRiskyCheck) GetDescription() string {
	return "AMFA risky login from VPN/anonymizing IP (risk >= 3)"
}

func (c *VPNRiskyCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	realms, err := c.realmsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("amfa-vpn-risky: load realms: %w", err)
	}
	var alerts []*domain.Alert
	for _, realm := range realms {
		since := c.getLastSeen(realm)
		events, err := c.repo.ListVPNRiskyEventsSince(ctx, realm, since, c.minRisk)
		if err != nil {
			return nil, fmt.Errorf("amfa-vpn-risky: realm=%s: %w", realm, err)
		}
		for _, ev := range events {
			alerts = append(alerts, c.toAlert(realm, ev))
			if ev.EventTime.After(since) {
				since = ev.EventTime
			}
		}
		c.setLastSeen(realm, since)
	}
	return alerts, nil
}

func (c *VPNRiskyCheck) toAlert(realm string, ev amfa.EventRow) *domain.Alert {
	eventID := ev.EventID
	md, _ := json.Marshal(map[string]any{
		"amfa_event_id": ev.EventID,
		"user_id":       ev.UserID,
		"ip_address":    ev.IPAddress,
		"client":        ev.Client,
		"country":       ev.Country,
		"is_vpn":        ev.IsVPN,
		"risk_level":    ev.RiskLevel,
		"realm_name":    realm,
		"min_risk":      c.minRisk,
	})
	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID("amfa-vpn-risky", c.tenantID, ev.EventID),
		Source:         domain.AlertSourceEvent,
		Type:           domain.AlertTypeSecurity,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          "AMFA: risky login from VPN/anonymizing IP",
		Description:    fmt.Sprintf("AMFA observed a risky (risk >= %d) authentication from a VPN/anonymizing IP in realm '%s' (event %s).", c.minRisk, realm, ev.EventID),
		ResourceType:   "amfa_event",
		ResourceID:     ev.EventID,
		ResourceName:   ev.EventID,
		RealmName:      realm,
		CheckType:      c.GetCheckType(),
		EventID:        &eventID,
		Recommendation: "Review the source IP and the user's recent activity. Anonymizing networks combined with elevated risk warrant a closer look at the session.",
		Metadata:       string(md),
	}
}

func (c *VPNRiskyCheck) getLastSeen(realm string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ts, ok := c.lastSeen[realm]; ok {
		return ts
	}
	return time.Now().Add(-c.lookback)
}

func (c *VPNRiskyCheck) setLastSeen(realm string, ts time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSeen[realm] = ts
}
