package eventscheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// LoginErrorCheck monitors LOGIN_ERROR events.
type LoginErrorCheck struct {
	eventsRepo KeycloakEventReader
	logger     Logger
	tenantID   string
	realms     []string
	threshold  int
	timeWindow time.Duration
}

// NewLoginErrorCheck creates a new login error checker.
func NewLoginErrorCheck(
	eventsRepo KeycloakEventReader,
	log Logger,
	tenantID string,
	realms []string,
	threshold int,
	timeWindow time.Duration,
) *LoginErrorCheck {
	// Set defaults
	if threshold == 0 {
		threshold = 10
	}
	if timeWindow == 0 {
		timeWindow = 5 * time.Minute
	}

	return &LoginErrorCheck{
		eventsRepo: eventsRepo,
		logger:     log.WithComponent("login_error_check"),
		tenantID:   tenantID,
		realms:     realms,
		threshold:  threshold,
		timeWindow: timeWindow,
	}
}

// GetCheckType returns the unique identifier for this check.
func (c *LoginErrorCheck) GetCheckType() string {
	return "login_error_events"
}

// GetDescription returns a human-readable description of this check.
func (c *LoginErrorCheck) GetDescription() string {
	return "Monitors LOGIN_ERROR events and alerts when threshold is exceeded"
}

// Execute performs the login error check.
func (c *LoginErrorCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	c.logger.Debug("Checking LOGIN_ERROR events")

	// Query events in time window
	start := time.Now().Add(-c.timeWindow)
	end := time.Now()

	var alerts []*domain.Alert

	// Count LOGIN_ERROR events per realm
	for _, realmName := range c.realms {
		count, err := c.eventsRepo.CountEventsByType(ctx, c.tenantID, realmName, "LOGIN_ERROR", start, end)
		if err != nil {
			c.logger.Error("Failed to count LOGIN_ERROR events",
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
func (c *LoginErrorCheck) createAlert(realmName string, count int) *domain.Alert {
	alertID := c.generateAlertID(realmName)

	metadata := map[string]interface{}{
		"tenant_id":   c.tenantID,
		"realm_name":  realmName,
		"event_type":  "LOGIN_ERROR",
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
		Title:          "Excessive Login Failures",
		Description:    fmt.Sprintf("%d failed login attempts detected in realm '%s' within the last %s (threshold: %d)", count, realmName, c.timeWindow.String(), c.threshold),
		ResourceType:   "realm",
		ResourceName:   realmName,
		RealmName:      realmName,
		CheckType:      c.GetCheckType(),
		Recommendation: "Investigate failed login attempts. Check for brute force attacks or configuration issues. Consider reviewing authentication logs and enabling additional security measures.",
		Metadata:       string(metadataJSON),
	}
}

// generateAlertID generates a stable, unique alert ID for deduplication.
func (c *LoginErrorCheck) generateAlertID(realmName string) string {
	// Same realm + event type = same alert ID = deduplication
	input := fmt.Sprintf("%s:%s:%s:LOGIN_ERROR", c.GetCheckType(), c.tenantID, realmName)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("events-login-%x", hash[:16])
}
