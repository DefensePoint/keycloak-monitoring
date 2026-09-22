package alerts

import (
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// SanitizeAlert returns a copy of the alert with the internal fields stripped:
// Metadata, CheckType, RuleID and EventID expose check internals and rule
// wiring, so they are reserved for privileged consumers.
func SanitizeAlert(alert *domain.Alert) *domain.Alert {
	if alert == nil {
		return nil
	}
	sanitized := *alert
	sanitized.Metadata = ""
	sanitized.CheckType = ""
	sanitized.RuleID = nil
	sanitized.EventID = nil
	return &sanitized
}

// SanitizeAlerts applies SanitizeAlert to every alert in the list.
func SanitizeAlerts(alertList []*domain.Alert) []*domain.Alert {
	sanitized := make([]*domain.Alert, len(alertList))
	for i, alert := range alertList {
		sanitized[i] = SanitizeAlert(alert)
	}
	return sanitized
}
