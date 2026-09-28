package notifications

import (
	"context"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Notifier defines the interface for notification channels.
// Each channel (email, slack, gitlab) implements this interface.
type Notifier interface {
	// Notify sends a notification for the given alert.
	// Returns the external ID (e.g., issue number, message ID) on success.
	Notify(ctx context.Context, alert *domain.Alert) (externalID string, err error)

	// Channel returns the channel type this notifier handles.
	Channel() Channel

	// IsEnabled returns whether this notifier is enabled.
	IsEnabled() bool
}

// Logger defines the interface for logging in the notifications domain.
// This allows the domain to be independent of concrete logger implementations.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// MeetsMinimumSeverity checks if the alert severity meets or exceeds the minimum severity.
func MeetsMinimumSeverity(alertSeverity domain.AlertSeverity, minSeverity string) bool {
	if minSeverity == "" {
		return true // No minimum configured, allow all
	}
	return SeverityLevel(alertSeverity) >= SeverityLevel(domain.AlertSeverity(minSeverity))
}
