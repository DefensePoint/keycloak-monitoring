// Package alerts provides the alerts domain model and services.
package alerts

import (
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// Statistics provides aggregated statistics about alerts.
type Statistics struct {
	TotalActive int            `json:"total_active"`
	BySeverity  map[string]int `json:"by_severity"`
	ByType      map[string]int `json:"by_type"`
}

// ListOptions defines filtering and pagination options for alert queries.
type ListOptions struct {
	Limit        int
	Offset       int
	Severity     domain.AlertSeverity
	Status       domain.AlertStatus
	Type         domain.AlertType
	RealmName    string
	ResourceType string
}

// UpdateStatusRequest represents a request to update alert status.
type UpdateStatusRequest struct {
	Status         domain.AlertStatus `json:"status"`
	AcknowledgedBy string             `json:"acknowledged_by"`
}
