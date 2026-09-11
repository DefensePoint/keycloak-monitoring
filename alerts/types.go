// Package alerts provides the alerts domain model and services.
package alerts

import (
	"errors"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// ErrNotFound is returned when the requested alert does not exist.
var ErrNotFound = errors.New("alert not found")

// Statistics provides aggregated statistics about alerts.
type Statistics struct {
	TotalActive int            `json:"total_active"`
	BySeverity  map[string]int `json:"by_severity"`
	ByType      map[string]int `json:"by_type"`
}

// ListOptions defines filtering and pagination options for alert queries.
type ListOptions struct {
	Limit     int
	Offset    int
	Severity  domain.AlertSeverity
	Status    domain.AlertStatus
	Type      domain.AlertType
	RealmName string
	// RealmNames restricts the query to a set of realms. An empty set applies no
	// filter at all, which reads as every realm.
	RealmNames   []string
	ResourceType string
}

// UpdateStatusRequest represents a request to update alert status.
type UpdateStatusRequest struct {
	Status         domain.AlertStatus `json:"status"`
	AcknowledgedBy string             `json:"acknowledged_by"`
}
