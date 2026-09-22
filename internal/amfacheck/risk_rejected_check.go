package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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
	lookback time.Duration // first-tick window per realm (= poll interval)
	logger   Logger

	mu       sync.Mutex
	lastSeen map[string]time.Time // per-realm checkpoint
}

// NewRiskRejectedCheck constructs rule 1. lookback bounds the first scan after
// start (per the spec: now - pollInterval).
func NewRiskRejectedCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, lookback time.Duration, log Logger) *RiskRejectedCheck {
	return &RiskRejectedCheck{
		tenantID: tenantID,
		realmsFn: realmsFn,
		repo:     repo,
		lookback: lookback,
		logger:   log.WithComponent("amfa-risk-rejected"),
		lastSeen: make(map[string]time.Time),
	}
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
	var alerts []*domain.Alert
	for _, realm := range realms {
		since := c.getLastSeen(realm)
		events, err := c.repo.ListRejectedEventsSince(ctx, realm, since)
		if err != nil {
			return nil, fmt.Errorf("amfa-risk-rejected: realm=%s: %w", realm, err)
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

func (c *RiskRejectedCheck) getLastSeen(realm string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ts, ok := c.lastSeen[realm]; ok {
		return ts
	}
	return time.Now().Add(-c.lookback)
}

func (c *RiskRejectedCheck) setLastSeen(realm string, ts time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSeen[realm] = ts
}
