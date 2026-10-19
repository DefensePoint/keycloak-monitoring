package amfacheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts/clearing"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// RealmRejectBurstCheck implements the realm-wide reject-burst rule: a
// critical alert when a realm accumulates >= threshold distinct users
// rejected by the risk engine (rule 1's condition) within a rolling window.
// It runs alongside rule 1 (RiskRejectedCheck), not instead of it: rule 1
// still fires one alert per rejected event; this rule adds the "many
// different accounts at once" signal that a flood of per-event alerts
// doesn't surface on its own. One alert per realm that resolves once the
// burst ages out of the window and reopens on the next one, like rule 2.
type RealmRejectBurstCheck struct {
	tenantID  string
	realmsFn  RealmsFunc
	repo      amfa.Repository
	threshold int
	window    time.Duration
	logger    Logger
}

// NewRealmRejectBurstCheck constructs the realm-wide reject-burst rule.
func NewRealmRejectBurstCheck(tenantID string, realmsFn RealmsFunc, repo amfa.Repository, threshold int, window time.Duration, log Logger) *RealmRejectBurstCheck {
	return &RealmRejectBurstCheck{
		tenantID:  tenantID,
		realmsFn:  realmsFn,
		repo:      repo,
		threshold: threshold,
		window:    window,
		logger:    log.WithComponent("amfa-realm-reject-burst"),
	}
}

func (c *RealmRejectBurstCheck) GetCheckType() string { return "amfa-realm-reject-burst" }
func (c *RealmRejectBurstCheck) GetDescription() string {
	return "AMFA many distinct users rejected by the risk engine in a realm within a window"
}

func (c *RealmRejectBurstCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	ev, err := c.Evaluate(ctx)
	return ev.Alerts, err
}

// Evaluate runs the check and reports every realm it counted, so a burst that
// ended there resolves and a later one reopens the alert.
func (c *RealmRejectBurstCheck) Evaluate(ctx context.Context) (clearing.Evaluation, error) {
	realms, err := c.realmsFn(ctx)
	if err != nil {
		return clearing.Evaluation{}, fmt.Errorf("amfa-realm-reject-burst: load realms: %w", err)
	}
	since := time.Now().UTC().Add(-c.window)

	ev := clearing.Evaluation{TenantID: c.tenantID}
	for _, realm := range realms {
		count, err := c.repo.CountDistinctRejectedUsersSince(ctx, realm, since)
		if err != nil {
			return clearing.Evaluation{}, fmt.Errorf("amfa-realm-reject-burst: realm=%s: %w", realm, err)
		}
		// Marked by realm rather than by alert ID, so an alert raised under
		// the per-day ID this check used before resolves too.
		ev.MarkRealmEvaluated(realm)
		if count < int64(c.threshold) {
			continue
		}
		ev.Alerts = append(ev.Alerts, c.toAlert(realm, count))
	}
	return ev, nil
}

func (c *RealmRejectBurstCheck) toAlert(realm string, count int64) *domain.Alert {
	md, _ := json.Marshal(map[string]any{
		"distinct_rejected_users": count,
		"threshold":               c.threshold,
		"window":                  c.window.String(),
		"realm_name":              realm,
	})
	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID("amfa-realm-reject-burst", c.tenantID, realm),
		Source:         domain.AlertSourceEvent,
		Type:           domain.AlertTypeSecurity,
		Severity:       domain.AlertSeverityCritical,
		Status:         domain.AlertStatusActive,
		Title:          "AMFA: many distinct users rejected by the risk engine",
		Description:    fmt.Sprintf("%d distinct users were rejected by the risk engine in realm '%s' within %s (threshold: %d).", count, realm, c.window.String(), c.threshold),
		ResourceType:   "realm",
		ResourceID:     realm,
		ResourceName:   realm,
		RealmName:      realm,
		CheckType:      c.GetCheckType(),
		Recommendation: "This looks like a coordinated attack (e.g. credential stuffing) rather than isolated failed logins. Review the affected accounts and consider blocking the source IPs.",
		Metadata:       string(md),
	}
}
