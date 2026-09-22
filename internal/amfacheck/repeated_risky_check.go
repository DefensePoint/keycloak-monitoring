package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// RepeatedRiskyCheck implements rule 2: one warning alert per (realm, user, UTC
// day) when a user accumulates >= threshold events at risk >= minRisk within
// the rolling window. No per-realm checkpoint is needed: the count is a rolling
// window recomputed each tick, and the per-(user, day) AlertID provides dedup.
type RepeatedRiskyCheck struct {
	tenantID  string
	realmsFn  RealmsFunc
	repo      amfa.Repository
	threshold int
	window    time.Duration
	minRisk   int
	logger    Logger
}

// NewRepeatedRiskyCheck constructs rule 2.
func NewRepeatedRiskyCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, threshold int, window time.Duration, minRisk int, log Logger) *RepeatedRiskyCheck {
	return &RepeatedRiskyCheck{
		tenantID:  tenantID,
		realmsFn:  realmsFn,
		repo:      repo,
		threshold: threshold,
		window:    window,
		minRisk:   minRisk,
		logger:    log.WithComponent("amfa-repeated-risky"),
	}
}

func (c *RepeatedRiskyCheck) GetCheckType() string { return "amfa-repeated-risky" }
func (c *RepeatedRiskyCheck) GetDescription() string {
	return "AMFA repeated risky attempts by one user within the window"
}

func (c *RepeatedRiskyCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	realms, err := c.realmsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("amfa-repeated-risky: load realms: %w", err)
	}
	now := time.Now().UTC()
	// Count over a rolling window (since = now - window), but dedup the alert
	// per calendar UTC day. These two notions of "day" differ by design: a user
	// crossing the threshold produces one alert per UTC date.
	since := now.Add(-c.window)
	day := now.Format("2006-01-02")

	var alerts []*domain.Alert
	for _, realm := range realms {
		counts, err := c.repo.CountRepeatedRiskyByUser(ctx, realm, since, c.minRisk, c.threshold)
		if err != nil {
			return nil, fmt.Errorf("amfa-repeated-risky: realm=%s: %w", realm, err)
		}
		for _, uc := range counts {
			alerts = append(alerts, c.toAlert(realm, day, uc))
		}
	}
	return alerts, nil
}

func (c *RepeatedRiskyCheck) toAlert(realm, day string, uc amfa.UserRiskyCount) *domain.Alert {
	md, _ := json.Marshal(map[string]any{
		"user_id":        uc.UserID,
		"count":          uc.Count,
		"threshold":      c.threshold,
		"min_risk_level": c.minRisk,
		"window":         c.window.String(),
		"realm_name":     realm,
		"utc_date":       day,
	})
	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID("amfa-repeated-risky", c.tenantID, realm, uc.UserID, day),
		Source:         domain.AlertSourceEvent,
		Type:           domain.AlertTypeSecurity,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          "AMFA: repeated risky login attempts by one user",
		Description:    fmt.Sprintf("User %s had %d risky (risk >= %d) authentication attempts in realm '%s' within %s (threshold: %d).", uc.UserID, uc.Count, c.minRisk, realm, c.window.String(), c.threshold),
		ResourceType:   "amfa_user",
		ResourceID:     uc.UserID,
		ResourceName:   uc.UserID,
		RealmName:      realm,
		CheckType:      c.GetCheckType(),
		Recommendation: "Review this user's recent authentication history. Repeated risky attempts may indicate credential stuffing, an account takeover attempt, or a misconfigured client.",
		Metadata:       string(md),
	}
}
