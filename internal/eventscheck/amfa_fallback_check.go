package eventscheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts/clearing"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
)

// AMFAFallbackCounter counts the logins the AMFA plugin handled without the
// engine's decision.
type AMFAFallbackCounter interface {
	CountAMFAFallbackLogins(ctx context.Context, tenantID, realmName string, from, to time.Time) (keycloak.AMFAFallbackCounts, error)
}

// AMFAFallbackCheck alerts when logins in a realm did not get the AMFA engine's
// decision. When the plugin cannot reach the engine, or it errors or times
// out, a login continues at the realm's fallback risk level, or is refused in
// mandatory failure mode, and nothing else says so: with the fallback level at
// 1, an engine outage lets every login through without a step-up. The plugin
// marks such a login with the decision_source detail on its events, and this
// check counts those logins in the Keycloak events KMT already collects, so it
// works while the engine itself cannot be read.
type AMFAFallbackCheck struct {
	counter    AMFAFallbackCounter
	logger     Logger
	tenantID   string
	realmsFn   RealmsFunc
	threshold  int
	timeWindow time.Duration
}

// NewAMFAFallbackCheck creates the check. A threshold of 0 means 1: every login
// without the engine's decision matters. A time window of 0 means 5m.
func NewAMFAFallbackCheck(
	counter AMFAFallbackCounter,
	log Logger,
	tenantID string,
	realmsFn RealmsFunc,
	threshold int,
	timeWindow time.Duration,
) *AMFAFallbackCheck {
	if threshold <= 0 {
		threshold = 1
	}
	if timeWindow <= 0 {
		timeWindow = 5 * time.Minute
	}
	return &AMFAFallbackCheck{
		counter:    counter,
		logger:     log.WithComponent("amfa_fallback_check"),
		tenantID:   tenantID,
		realmsFn:   realmsFn,
		threshold:  threshold,
		timeWindow: timeWindow,
	}
}

// GetCheckType returns the unique identifier for this check.
func (c *AMFAFallbackCheck) GetCheckType() string {
	return "amfa_fallback_events"
}

// GetDescription returns a human-readable description of this check.
func (c *AMFAFallbackCheck) GetDescription() string {
	return "Alerts when logins did not get the AMFA engine's decision and ran at the fallback risk level or were refused"
}

// Execute performs the check.
func (c *AMFAFallbackCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	ev, err := c.Evaluate(ctx)
	return ev.Alerts, err
}

// Evaluate runs the check and reports, besides the alerts, every realm whose
// count it read, so an alert resolves once the engine answers again. A realm
// whose count failed is left out.
func (c *AMFAFallbackCheck) Evaluate(ctx context.Context) (clearing.Evaluation, error) {
	end := time.Now()
	start := end.Add(-c.timeWindow)

	realms, err := c.realmsFn(ctx)
	if err != nil {
		return clearing.Evaluation{}, fmt.Errorf("list realms: %w", err)
	}

	ev := clearing.Evaluation{TenantID: c.tenantID}
	for _, realmName := range realms {
		counts, err := c.counter.CountAMFAFallbackLogins(ctx, c.tenantID, realmName, start, end)
		if err != nil {
			c.logger.Error("Failed to count logins without the AMFA engine's decision",
				"realm", realmName,
				"error", err)
			continue
		}

		ev.MarkEvaluated(c.generateAlertID(realmName))
		if counts.Fallback+counts.Refused >= int64(c.threshold) {
			ev.Alerts = append(ev.Alerts, c.createAlert(realmName, counts))
		}
	}
	return ev, nil
}

func (c *AMFAFallbackCheck) createAlert(realmName string, counts keycloak.AMFAFallbackCounts) *domain.Alert {
	metadata := map[string]interface{}{
		"tenant_id":       c.tenantID,
		"realm_name":      realmName,
		"fallback_logins": counts.Fallback,
		"refused_logins":  counts.Refused,
		"threshold":       c.threshold,
		"time_window":     c.timeWindow.String(),
	}
	metadataJSON, _ := json.Marshal(metadata)

	return &domain.Alert{
		TenantID: c.tenantID,
		AlertID:  c.generateAlertID(realmName),
		Type:     domain.AlertTypeSecurity,
		Severity: domain.AlertSeverityError,
		Status:   domain.AlertStatusActive,
		Title:    "Logins without the AMFA engine's decision",
		Description: fmt.Sprintf("In realm '%s' within the last %s, %s ran at the fallback risk level "+
			"and %s refused, because the AMFA plugin could not get the engine's decision (threshold: %d)",
			realmName, c.timeWindow.String(), logins(counts.Fallback), loginsWere(counts.Refused), c.threshold),
		ResourceType: "realm",
		ResourceName: realmName,
		RealmName:    realmName,
		CheckType:    c.GetCheckType(),
		Recommendation: "Check that the AMFA engine is up and reachable from Keycloak: its /health endpoint, the realm's " +
			"engine endpoint, and the Keycloak log lines starting with ADAPTIVE_AUTH_ENGINE_UNREACHABLE. Until it answers, " +
			"logins get the realm's fallback risk level, which with a level of 1 means no step-up.",
		Metadata: string(metadataJSON),
	}
}

// generateAlertID generates a stable alert ID per realm, so a repeat updates
// the open alert instead of raising another.
func (c *AMFAFallbackCheck) generateAlertID(realmName string) string {
	input := fmt.Sprintf("%s:%s:%s", c.GetCheckType(), c.tenantID, realmName)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("events-amfa-fallback-%x", hash[:16])
}

func logins(n int64) string {
	if n == 1 {
		return "1 login"
	}
	return fmt.Sprintf("%d logins", n)
}

func loginsWere(n int64) string {
	if n == 1 {
		return "1 login was"
	}
	return fmt.Sprintf("%d logins were", n)
}
