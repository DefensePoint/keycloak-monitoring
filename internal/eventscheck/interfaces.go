package eventscheck

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts/clearing"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// KeycloakEventReader defines the minimal interface for reading Keycloak events
// used by all event checkers (LoginErrorCheck, ClientErrorCheck, etc.).
type KeycloakEventReader interface {
	CountEventsByType(ctx context.Context, tenantID, realm, eventType string, start, end time.Time) (int64, error)
}

// RealmsFunc returns the current realm list for a tenant. Each Check holds one
// and calls it every tick, so realms added or removed in Keycloak are picked
// up without a restart.
type RealmsFunc func(ctx context.Context) ([]string, error)

// Check defines the interface for event-based checks.
type Check interface {
	// GetCheckType returns the type identifier for this check.
	GetCheckType() string

	// Execute runs the check and returns any detected alerts.
	Execute(ctx context.Context) ([]*domain.Alert, error)

	// GetDescription returns a human-readable description of this check.
	GetDescription() string
}

// Evaluator is a Check that also reports which conditions it evaluated, so
// the service can resolve alerts whose condition has cleared. Every threshold
// check implements it; Execute remains for callers that want only the alerts.
type Evaluator interface {
	Check
	Evaluate(ctx context.Context) (clearing.Evaluation, error)
}

// AlertStore defines the interface for storing and retrieving alerts.
type AlertStore interface {
	// SaveAlert saves or updates an alert.
	SaveAlert(ctx context.Context, alert *domain.Alert) error

	// GetAlertByID retrieves an alert by its ID.
	GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)

	// ListOpenAlertsByCheck returns a check's alerts that are still open.
	ListOpenAlertsByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error)

	// ResolveAlert marks an alert resolved.
	ResolveAlert(ctx context.Context, tenantID, alertID string) error
}

// Silencer reports whether a configured silence rule covers an alert, and
// which one. alerts.SilenceManager implements it.
type Silencer interface {
	ShouldSilence(alert *domain.Alert) (bool, string)
}

// NotificationService defines the interface for sending alert notifications.
type NotificationService interface {
	// NotifyAlert sends a notification for an alert.
	NotifyAlert(ctx context.Context, alert *domain.Alert) error

	// NotifyAlertResolution sends the notification that an alert resolved.
	NotifyAlertResolution(ctx context.Context, alert *domain.Alert) error
}

// Logger defines the interface for logging in the eventscheck domain.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
	Debug(msg string, args ...any)
	WithComponent(component string) Logger
}

// Every production check evaluates, so its alerts can resolve.
var (
	_ Evaluator = (*LoginErrorCheck)(nil)
	_ Evaluator = (*ClientErrorCheck)(nil)
	_ Evaluator = (*CodeToTokenErrorCheck)(nil)
	_ Evaluator = (*IdpErrorCheck)(nil)
	_ Evaluator = (*CommonErrorCheck)(nil)
)
