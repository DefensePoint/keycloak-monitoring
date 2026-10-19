package eventscheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts/clearing"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// CodeToTokenErrorCheck monitors CODE_TO_TOKEN_ERROR events.
type CodeToTokenErrorCheck struct {
	eventsRepo KeycloakEventReader
	logger     Logger
	tenantID   string
	realmsFn   RealmsFunc
	threshold  int
	timeWindow time.Duration
}

// NewCodeToTokenErrorCheck creates a new code-to-token error checker.
func NewCodeToTokenErrorCheck(
	eventsRepo KeycloakEventReader,
	log Logger,
	tenantID string,
	realmsFn RealmsFunc,
	threshold int,
	timeWindow time.Duration,
) *CodeToTokenErrorCheck {
	if threshold == 0 {
		threshold = 1
	}
	if timeWindow == 0 {
		timeWindow = 5 * time.Minute
	}

	return &CodeToTokenErrorCheck{
		eventsRepo: eventsRepo,
		logger:     log.WithComponent("code_to_token_error_check"),
		tenantID:   tenantID,
		realmsFn:   realmsFn,
		threshold:  threshold,
		timeWindow: timeWindow,
	}
}

// GetCheckType returns the unique identifier for this check.
func (c *CodeToTokenErrorCheck) GetCheckType() string {
	return "code_to_token_error_events"
}

// GetDescription returns a human-readable description of this check.
func (c *CodeToTokenErrorCheck) GetDescription() string {
	return "Monitors CODE_TO_TOKEN_ERROR events (OAuth code exchange failures)"
}

// Execute performs the code-to-token error check.
func (c *CodeToTokenErrorCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	ev, err := c.Evaluate(ctx)
	return ev.Alerts, err
}

// Evaluate runs the check and reports, besides the alerts, every realm whose
// count it read, so a condition that cleared there can resolve. A realm whose
// count failed is left out: its condition was not evaluated.
func (c *CodeToTokenErrorCheck) Evaluate(ctx context.Context) (clearing.Evaluation, error) {
	c.logger.Debug("Checking CODE_TO_TOKEN_ERROR events")

	start := time.Now().Add(-c.timeWindow)
	end := time.Now()

	realms, err := c.realmsFn(ctx)
	if err != nil {
		return clearing.Evaluation{}, fmt.Errorf("list realms: %w", err)
	}

	ev := clearing.Evaluation{TenantID: c.tenantID}

	for _, realmName := range realms {
		count, err := c.eventsRepo.CountEventsByType(ctx, c.tenantID, realmName, "CODE_TO_TOKEN_ERROR", start, end)
		if err != nil {
			c.logger.Error("Failed to count CODE_TO_TOKEN_ERROR events",
				"realm", realmName,
				"error", err)
			continue
		}

		ev.MarkEvaluated(c.generateAlertID(realmName))
		if count >= int64(c.threshold) {
			ev.Alerts = append(ev.Alerts, c.createAlert(realmName, int(count)))
		}
	}

	return ev, nil
}

// createAlert creates an alert.
func (c *CodeToTokenErrorCheck) createAlert(realmName string, count int) *domain.Alert {
	alertID := c.generateAlertID(realmName)

	metadata := map[string]interface{}{
		"tenant_id":   c.tenantID,
		"realm_name":  realmName,
		"event_type":  "CODE_TO_TOKEN_ERROR",
		"count":       count,
		"threshold":   c.threshold,
		"time_window": c.timeWindow.String(),
	}
	metadataJSON, _ := json.Marshal(metadata)

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID,
		Type:           domain.AlertTypeSecurity,
		Severity:       domain.AlertSeverityCritical,
		Status:         domain.AlertStatusActive,
		Title:          "OAuth Code Exchange Failures",
		Description:    fmt.Sprintf("%d CODE_TO_TOKEN_ERROR events detected in realm '%s' within the last %s. OAuth authorization code exchange is failing.", count, realmName, c.timeWindow.String()),
		ResourceType:   "realm",
		ResourceName:   realmName,
		RealmName:      realmName,
		CheckType:      c.GetCheckType(),
		Recommendation: "Investigate OAuth/OIDC configuration. Check client secrets, redirect URIs, and token endpoint settings. Verify network connectivity to authorization server.",
		Metadata:       string(metadataJSON),
	}
}

// generateAlertID generates a stable, unique alert ID for deduplication.
func (c *CodeToTokenErrorCheck) generateAlertID(realmName string) string {
	input := fmt.Sprintf("%s:%s:%s:CODE_TO_TOKEN_ERROR", c.GetCheckType(), c.tenantID, realmName)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("events-code-token-%x", hash[:16])
}
