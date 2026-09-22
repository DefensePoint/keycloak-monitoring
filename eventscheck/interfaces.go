package eventscheck

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// KeycloakEventReader defines the minimal interface for reading Keycloak events
// used by all event checkers (LoginErrorCheck, ClientErrorCheck, etc.).
type KeycloakEventReader interface {
	CountEventsByType(ctx context.Context, tenantID, realm, eventType string, start, end time.Time) (int64, error)
}

// Check defines the interface for event-based checks.
type Check interface {
	// GetCheckType returns the type identifier for this check.
	GetCheckType() string

	// Execute runs the check and returns any detected alerts.
	Execute(ctx context.Context) ([]*domain.Alert, error)

	// GetDescription returns a human-readable description of this check.
	GetDescription() string
}

// AlertStore defines the interface for storing and retrieving alerts.
type AlertStore interface {
	// SaveAlert saves or updates an alert.
	SaveAlert(ctx context.Context, alert *domain.Alert) error

	// GetAlertByID retrieves an alert by its ID.
	GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
}

// NotificationService defines the interface for sending alert notifications.
type NotificationService interface {
	// NotifyAlert sends a notification for an alert.
	NotifyAlert(ctx context.Context, alert *domain.Alert) error
}

// Logger defines the interface for logging in the eventscheck domain.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
	Debug(msg string, args ...any)
	WithComponent(component string) Logger
}
