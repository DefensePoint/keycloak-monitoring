package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
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
	logger   Logger
	events   *eventCursor
}

// NewVPNRiskyCheck constructs rule 3.
func NewVPNRiskyCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, lookback time.Duration, log Logger, opts ...EventCheckOption) *VPNRiskyCheck {
	c := &VPNRiskyCheck{
		tenantID: tenantID,
		realmsFn: realmsFn,
		repo:     repo,
		minRisk:  vpnRiskyMinRisk,
		logger:   log.WithComponent("amfa-vpn-risky"),
	}
	c.events = newEventCursor(c.GetCheckType(), lookback, c.logger, opts)
	return c
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
	alerts, err := c.events.read(ctx, realms,
		func(ctx context.Context, realm string, since time.Time) ([]amfa.EventRow, error) {
			return c.repo.ListVPNRiskyEventsSince(ctx, realm, since, c.minRisk)
		}, c.toAlert)
	if err != nil {
		err = fmt.Errorf("amfa-vpn-risky: %w", err)
	}
	return alerts, err
}

// Commit implements EventCheck.
func (c *VPNRiskyCheck) Commit(stored map[string]bool) { c.events.Commit(stored) }

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
