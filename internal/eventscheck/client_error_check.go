package eventscheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// ClientErrorCheck monitors CLIENT_LOGIN_ERROR events.
type ClientErrorCheck struct {
	eventsRepo KeycloakEventReader
	logger     Logger
	tenantID   string
	realms     []string
	threshold  int
	timeWindow time.Duration
}

// NewClientErrorCheck creates a new client login error checker.
func NewClientErrorCheck(
	eventsRepo KeycloakEventReader,
	log Logger,
	tenantID string,
	realms []string,
	threshold int,
	timeWindow time.Duration,
) *ClientErrorCheck {
	// Set defaults - CLIENT_LOGIN_ERROR is medium priority
	if threshold == 0 {
		threshold = 10
	}
	if timeWindow == 0 {
		timeWindow = 5 * time.Minute
	}

	return &ClientErrorCheck{
		eventsRepo: eventsRepo,
		logger:     log.WithComponent("client_error_check"),
		tenantID:   tenantID,
		realms:     realms,
		threshold:  threshold,
		timeWindow: timeWindow,
	}
}

// GetCheckType returns the unique identifier for this check.
func (c *ClientErrorCheck) GetCheckType() string {
	return "client_login_error_events"
}

// GetDescription returns a human-readable description of this check.
func (c *ClientErrorCheck) GetDescription() string {
	return "Monitors CLIENT_LOGIN_ERROR events (client authentication failures)"
}

// Execute performs the client login error check.
func (c *ClientErrorCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	c.logger.Debug("Checking CLIENT_LOGIN_ERROR events")

	start := time.Now().Add(-c.timeWindow)
	end := time.Now()

	var alerts []*domain.Alert

	for _, realmName := range c.realms {
		count, err := c.eventsRepo.CountEventsByType(ctx, c.tenantID, realmName, "CLIENT_LOGIN_ERROR", start, end)
		if err != nil {
			c.logger.Error("Failed to count CLIENT_LOGIN_ERROR events",
				"realm", realmName,
				"error", err)
			continue
		}

		if count >= int64(c.threshold) {
			alerts = append(alerts, c.createAlert(realmName, int(count)))
		}
	}

	return alerts, nil
}

// createAlert creates an alert.
func (c *ClientErrorCheck) createAlert(realmName string, count int) *domain.Alert {
	alertID := c.generateAlertID(realmName)

	metadata := map[string]interface{}{
		"tenant_id":   c.tenantID,
		"realm_name":  realmName,
		"event_type":  "CLIENT_LOGIN_ERROR",
		"count":       count,
		"threshold":   c.threshold,
		"time_window": c.timeWindow.String(),
	}
	metadataJSON, _ := json.Marshal(metadata)

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID,
		Type:           domain.AlertTypeSecurity,
		Severity:       domain.AlertSeverityWarning,
		Status:         domain.AlertStatusActive,
		Title:          "Excessive Client Authentication Failures",
		Description:    fmt.Sprintf("%d client authentication failures detected in realm '%s' within the last %s (threshold: %d)", count, realmName, c.timeWindow.String(), c.threshold),
		ResourceType:   "realm",
		ResourceName:   realmName,
		RealmName:      realmName,
		CheckType:      c.GetCheckType(),
		Recommendation: "Investigate client authentication issues. Check client credentials, service account settings, and client configurations. Review client authentication logs for specific error details.",
		Metadata:       string(metadataJSON),
	}
}

// generateAlertID generates a stable, unique alert ID for deduplication.
func (c *ClientErrorCheck) generateAlertID(realmName string) string {
	input := fmt.Sprintf("%s:%s:%s:CLIENT_LOGIN_ERROR", c.GetCheckType(), c.tenantID, realmName)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("events-client-%x", hash[:16])
}
