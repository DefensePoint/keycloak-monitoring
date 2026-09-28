package metricscheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// HealthCheck monitors Keycloak server health metrics.
type HealthCheck struct {
	healthRepo            HealthReader
	logger                *logger.Logger
	tenantID              string
	memoryWarningPercent  int
	memoryCriticalPercent int
}

// NewHealthCheck creates a new health checker.
func NewHealthCheck(
	healthRepo HealthReader,
	log *logger.Logger,
	tenantID string,
	memoryWarningPercent int,
	memoryCriticalPercent int,
) *HealthCheck {
	// Set defaults
	if memoryWarningPercent == 0 {
		memoryWarningPercent = 80
	}
	if memoryCriticalPercent == 0 {
		memoryCriticalPercent = 90
	}

	return &HealthCheck{
		healthRepo:            healthRepo,
		logger:                log.WithComponent("health_check"),
		tenantID:              tenantID,
		memoryWarningPercent:  memoryWarningPercent,
		memoryCriticalPercent: memoryCriticalPercent,
	}
}

// GetCheckType returns the unique identifier for this check.
func (c *HealthCheck) GetCheckType() string {
	return "health"
}

// GetDescription returns a human-readable description of this check.
func (c *HealthCheck) GetDescription() string {
	return "Monitors Keycloak server health metrics including memory usage and server status"
}

// Execute performs the health check.
func (c *HealthCheck) Execute(ctx context.Context) ([]*domain.Alert, error) {
	// Get latest health data (already collected by Monitor every 1 minute)
	health, err := c.healthRepo.GetLatestHealth(ctx, c.tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest health: %w", err)
	}

	if health == nil {
		c.logger.Debug("No health data available for tenant")
		return nil, nil
	}

	var alerts []*domain.Alert

	// Check server status
	if health.Status == "DOWN" {
		alerts = append(alerts, c.createAlert(
			"server_down",
			"Keycloak Server Down",
			domain.AlertSeverityCritical,
			fmt.Sprintf("Keycloak server is not responding. Error: %s", health.ErrorMessage),
			"Check server logs and restart Keycloak. Verify network connectivity.",
		))
		return alerts, nil // If server is down, no point checking memory
	}

	// Check memory usage
	if health.MemoryMax > 0 {
		memoryPercent := float64(health.MemoryUsed) / float64(health.MemoryMax) * 100

		if int(memoryPercent) >= c.memoryCriticalPercent {
			alerts = append(alerts, c.createAlert(
				"memory_critical",
				"Critical Memory Usage",
				domain.AlertSeverityCritical,
				fmt.Sprintf("Memory usage at %.1f%% (%s / %s). Immediate action required.",
					memoryPercent, formatBytes(health.MemoryUsed), formatBytes(health.MemoryMax)),
				fmt.Sprintf("Increase JVM heap size or restart server. Threshold: %d%%", c.memoryCriticalPercent),
			))
		} else if int(memoryPercent) >= c.memoryWarningPercent {
			alerts = append(alerts, c.createAlert(
				"memory_warning",
				"High Memory Usage",
				domain.AlertSeverityWarning,
				fmt.Sprintf("Memory usage at %.1f%% (%s / %s).",
					memoryPercent, formatBytes(health.MemoryUsed), formatBytes(health.MemoryMax)),
				fmt.Sprintf("Monitor closely. Consider increasing heap size. Threshold: %d%%", c.memoryWarningPercent),
			))
		}
	}

	return alerts, nil
}

// createAlert creates a configuration alert.
func (c *HealthCheck) createAlert(
	alertType string,
	title string,
	severity domain.AlertSeverity,
	description string,
	recommendation string,
) *domain.Alert {
	alertID := c.generateAlertID(alertType)

	metadata := map[string]interface{}{
		"tenant_id":  c.tenantID,
		"alert_type": alertType,
	}
	metadataJSON, _ := json.Marshal(metadata)

	return &domain.Alert{
		TenantID:       c.tenantID,
		AlertID:        alertID,
		Type:           domain.AlertTypeConfiguration,
		Severity:       severity,
		Status:         domain.AlertStatusActive,
		Title:          title,
		Description:    description,
		ResourceType:   "server",
		ResourceName:   "keycloak-server",
		CheckType:      c.GetCheckType(),
		Recommendation: recommendation,
		Metadata:       string(metadataJSON),
	}
}

// generateAlertID generates a stable, unique alert ID for deduplication.
func (c *HealthCheck) generateAlertID(alertType string) string {
	// Same alert type = same alert ID = deduplication
	input := fmt.Sprintf("%s:%s:%s", c.GetCheckType(), c.tenantID, alertType)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("health-%x", hash[:16])
}

// formatBytes formats bytes to human-readable format.
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
