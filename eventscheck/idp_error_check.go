package eventscheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// IdpErrorCheck monitors identity provider error events.
type IdpErrorCheck struct {
	eventsRepo KeycloakEventReader
	logger     Logger
	tenantID   string
	realms     []string
	threshold  int
	timeWindow time.Duration
	eventTypes []string
}

// NewIdpErrorCheck creates a new IDP error checker.
func NewIdpErrorCheck(
	eventsRepo KeycloakEventReader,
	log Logger,
	tenantID string,
	realms []string,
	threshold int,
	timeWindow time.Duration,
	eventTypes []string,
) *IdpErrorCheck {
	// Set defaults
	if threshold == 0 {
		threshold = 1 // IDP errors are critical, alert immediately
	}
	if timeWindow == 0 {
		timeWindow = 5 * time.Minute
	}
	if len(eventTypes) == 0 {
		eventTypes = []string{
			"IDENTITY_PROVIDER_LOGIN_ERROR",
			"IDENTITY_PROVIDER_POST_LOGIN_ERROR",
			"IDENTITY_PROVIDER_RESPONSE_ERROR",
			"IDENTITY_PROVIDER_RETRIEVE_TOKEN_ERROR",
		}
	}

	return &IdpErrorCheck{
		eventsRepo: eventsRepo,
		logger:     log.WithComponent("idp_error_check"),
		tenantID:   tenantID,
		realms:     realms,
		threshold:  threshold,
		timeWindow: timeWindow,
		eventTypes: eventTypes,
	}
}

// GetCheckType returns the unique identifier for this check.
func (c *IdpErrorCheck) GetCheckType() string {
	return "idp_error_events"
}

// GetDescription returns a human-readable description of this check.
func (c *IdpErrorCheck) GetDescription() string {
	return "Monitors identity provider error events (LOGIN, POST_LOGIN, RESPONSE, RETRIEVE_TOKEN)"
}

// Execute performs the IDP error check.
func (c *IdpErrorCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	c.logger.Debug("Checking IDP error events")

	start := time.Now().Add(-c.timeWindow)
	end := time.Now()

	var alerts []*domain.Alert

	// Check each realm for each IDP error type
	for _, realmName := range c.realms {
		for _, eventType := range c.eventTypes {
			count, err := c.eventsRepo.CountEventsByType(ctx, c.tenantID, realmName, eventType, start, end)
			if err != nil {
				c.logger.Error("Failed to count IDP error events",
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

// createAlert creates an alert for IDP errors.
func (c *IdpErrorCheck) createAlert(realmName string, eventType string, count int) *domain.Alert {
	alertID := c.generateAlertID(realmName, eventType)

	// Determine severity based on error type
	severity := domain.AlertSeverityError
	if eventType == "IDENTITY_PROVIDER_LOGIN_ERROR" {
		severity = domain.AlertSeverityCritical
	}

	metadata := map[string]interface{}{
		"tenant_id":   c.tenantID,
		"realm_name":  realmName,
		"event_type":  eventType,
		"count":       count,
		"threshold":   c.threshold,
		"time_window": c.timeWindow.String(),
	}
	metadataJSON, _ := json.Marshal(metadata)

	title := fmt.Sprintf("Identity Provider Error: %s", formatEventType(eventType))
	description := fmt.Sprintf("%d %s events detected in realm '%s' within the last %s",
		count, eventType, realmName, c.timeWindow.String())

	recommendation := "Investigate identity provider configuration. Check IDP metadata URLs, certificates, and connection settings. " +
		"Verify IDP is accessible and properly configured in Keycloak."

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID,
		Type:           domain.AlertTypeIdentityProvider,
		Severity:       severity,
		Status:         domain.AlertStatusActive,
		Title:          title,
		Description:    description,
		ResourceType:   "identity_provider",
		ResourceName:   realmName,
		RealmName:      realmName,
		CheckType:      c.GetCheckType(),
		Recommendation: recommendation,
		Metadata:       string(metadataJSON),
	}
}

// generateAlertID generates a stable, unique alert ID for deduplication.
func (c *IdpErrorCheck) generateAlertID(realmName string, eventType string) string {
	// Same realm + event type = same alert ID = deduplication
	input := fmt.Sprintf("%s:%s:%s:%s", c.GetCheckType(), c.tenantID, realmName, eventType)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("events-idp-%x", hash[:16])
}

// formatEventType converts event type to human-readable format.
func formatEventType(eventType string) string {
	switch eventType {
	case "IDENTITY_PROVIDER_LOGIN_ERROR":
		return "Login Error"
	case "IDENTITY_PROVIDER_POST_LOGIN_ERROR":
		return "Post-Login Error"
	case "IDENTITY_PROVIDER_RESPONSE_ERROR":
		return "Response Error"
	case "IDENTITY_PROVIDER_RETRIEVE_TOKEN_ERROR":
		return "Token Retrieval Error"
	default:
		return eventType
	}
}
