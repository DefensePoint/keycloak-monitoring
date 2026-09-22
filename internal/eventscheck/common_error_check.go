package eventscheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// CommonErrorCheck monitors low-priority error events (REGISTER_ERROR, RESET_PASSWORD_ERROR, LOGOUT_ERROR).
type CommonErrorCheck struct {
	eventsRepo KeycloakEventReader
	logger     Logger
	tenantID   string
	realms     []string
	threshold  int
	timeWindow time.Duration
	eventTypes []string
}

// NewCommonErrorCheck creates a new common error checker.
func NewCommonErrorCheck(
	eventsRepo KeycloakEventReader,
	log Logger,
	tenantID string,
	realms []string,
	threshold int,
	timeWindow time.Duration,
	eventTypes []string,
) *CommonErrorCheck {
	// Set defaults - low priority errors need higher threshold
	if threshold == 0 {
		threshold = 50
	}
	if timeWindow == 0 {
		timeWindow = 5 * time.Minute
	}
	if len(eventTypes) == 0 {
		eventTypes = []string{
			"REGISTER_ERROR",
			"RESET_PASSWORD_ERROR",
			"LOGOUT_ERROR",
		}
	}

	return &CommonErrorCheck{
		eventsRepo: eventsRepo,
		logger:     log.WithComponent("common_error_check"),
		tenantID:   tenantID,
		realms:     realms,
		threshold:  threshold,
		timeWindow: timeWindow,
		eventTypes: eventTypes,
	}
}

// GetCheckType returns the unique identifier for this check.
func (c *CommonErrorCheck) GetCheckType() string {
	return "common_error_events"
}

// GetDescription returns a human-readable description of this check.
func (c *CommonErrorCheck) GetDescription() string {
	return "Monitors common error events (REGISTER, RESET_PASSWORD, LOGOUT)"
}

// Execute performs the common error check.
func (c *CommonErrorCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	c.logger.Debug("Checking common error events")

	start := time.Now().Add(-c.timeWindow)
	end := time.Now()

	var alerts []*domain.Alert

	// Check each realm for each error type
	for _, realmName := range c.realms {
		for _, eventType := range c.eventTypes {
			count, err := c.eventsRepo.CountEventsByType(ctx, c.tenantID, realmName, eventType, start, end)
			if err != nil {
				c.logger.Error("Failed to count common error events",
					"realm", realmName,
					"event_type", eventType,
					"error", err)
				continue
			}

			if count >= int64(c.threshold) {
				alerts = append(alerts, c.createAlert(realmName, eventType, int(count)))
			}
		}
	}

	return alerts, nil
}

// createAlert creates an alert for common errors.
func (c *CommonErrorCheck) createAlert(realmName string, eventType string, count int) *domain.Alert {
	alertID := c.generateAlertID(realmName, eventType)

	// All common errors are informational severity (often user error)
	severity := domain.AlertSeverityInfo

	metadata := map[string]interface{}{
		"tenant_id":   c.tenantID,
		"realm_name":  realmName,
		"event_type":  eventType,
		"count":       count,
		"threshold":   c.threshold,
		"time_window": c.timeWindow.String(),
	}
	metadataJSON, _ := json.Marshal(metadata)

	title := fmt.Sprintf("High Volume of %s Events", formatCommonEventType(eventType))
	description := fmt.Sprintf("%d %s events detected in realm '%s' within the last %s (threshold: %d)",
		count, eventType, realmName, c.timeWindow.String(), c.threshold)

	recommendation := getRecommendationForEventType(eventType)

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID,
		Type:           domain.AlertTypePerformance,
		Severity:       severity,
		Status:         domain.AlertStatusActive,
		Title:          title,
		Description:    description,
		ResourceType:   "realm",
		ResourceName:   realmName,
		RealmName:      realmName,
		CheckType:      c.GetCheckType(),
		Recommendation: recommendation,
		Metadata:       string(metadataJSON),
	}
}

// generateAlertID generates a stable, unique alert ID for deduplication.
func (c *CommonErrorCheck) generateAlertID(realmName string, eventType string) string {
	input := fmt.Sprintf("%s:%s:%s:%s", c.GetCheckType(), c.tenantID, realmName, eventType)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("events-common-%x", hash[:16])
}

// formatCommonEventType converts event type to human-readable format.
func formatCommonEventType(eventType string) string {
	switch eventType {
	case "REGISTER_ERROR":
		return "Registration Errors"
	case "RESET_PASSWORD_ERROR":
		return "Password Reset Errors"
	case "LOGOUT_ERROR":
		return "Logout Errors"
	default:
		return eventType
	}
}

// getRecommendationForEventType returns specific recommendations based on error type.
func getRecommendationForEventType(eventType string) string {
	switch eventType {
	case "REGISTER_ERROR":
		return "Review user registration flow. Check for misconfigured required actions, email verification settings, or password policies that may be preventing successful registration."
	case "RESET_PASSWORD_ERROR":
		return "Investigate password reset process. Check SMTP configuration for email delivery, verify reset password actions are properly configured, and review password policies."
	case "LOGOUT_ERROR":
		return "Check logout configuration and session management. Verify redirect URIs and post-logout settings. High volume may indicate client-side issues or misconfiguration."
	default:
		return "Investigate error patterns and review logs for specific error details."
	}
}
