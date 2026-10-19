package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// RiskRejectedCheck implements rule 1: one critical alert per AMFA event that
// the risk engine rejected (pre_auth_risk_decision = 4).
type RiskRejectedCheck struct {
	tenantID string
	realmsFn RealmsFunc
	repo     amfa.Repository
	logger   Logger
	events   *eventCursor
}

// NewRiskRejectedCheck constructs rule 1. lookback is how far back a realm's
// first scan reaches when WithResume finds no earlier alert.
func NewRiskRejectedCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, lookback time.Duration, log Logger, opts ...EventCheckOption) *RiskRejectedCheck {
	c := &RiskRejectedCheck{
		tenantID: tenantID,
		realmsFn: realmsFn,
		repo:     repo,
		logger:   log.WithComponent("amfa-risk-rejected"),
	}
	c.events = newEventCursor(c.GetCheckType(), lookback, c.logger, opts)
	return c
}

func (c *RiskRejectedCheck) GetCheckType() string { return "amfa-risk-rejected" }
func (c *RiskRejectedCheck) GetDescription() string {
	return "AMFA login rejected by risk engine (ACR 4)"
}

func (c *RiskRejectedCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	realms, err := c.realmsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("amfa-risk-rejected: load realms: %w", err)
	}
	alerts, err := c.events.read(ctx, realms,
		func(ctx context.Context, realm string, since time.Time) ([]amfa.EventRow, error) {
			return c.repo.ListRejectedEventsSince(ctx, realm, since)
		}, c.toAlert)
	if err != nil {
		err = fmt.Errorf("amfa-risk-rejected: %w", err)
	}
	return alerts, err
}

// Commit implements EventCheck.
func (c *RiskRejectedCheck) Commit(stored map[string]bool) { c.events.Commit(stored) }

func (c *RiskRejectedCheck) toAlert(realm string, ev amfa.EventRow) *domain.Alert {
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
	})
	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID("amfa-risk-rejected", c.tenantID, ev.EventID),
		Source:         domain.AlertSourceEvent,
		Type:           domain.AlertTypeSecurity,
		Severity:       domain.AlertSeverityCritical,
		Status:         domain.AlertStatusActive,
		Title:          "AMFA: login rejected by risk engine",
		Description:    fmt.Sprintf("AMFA rejected an authentication attempt in realm '%s' (event %s).", realm, ev.EventID),
		ResourceType:   "amfa_event",
		ResourceID:     ev.EventID,
		ResourceName:   ev.EventID,
		RealmName:      realm,
		CheckType:      c.GetCheckType(),
		EventID:        &eventID,
		Recommendation: "Investigate the rejected login attempt. Review the user's session history and IP reputation. Consider whether the user should be temporarily blocked.",
		Metadata:       string(md),
	}
}
