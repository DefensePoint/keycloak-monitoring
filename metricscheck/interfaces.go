package metricscheck

import (
	"context"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// ============================================================================
// EXTERNAL DEPENDENCY INTERFACES (Local interfaces for dependency injection)
// ============================================================================

// AlertStore defines the minimal interface for alert persistence
// used by the metricscheck Service.
type AlertStore interface {
	GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
	SaveAlert(ctx context.Context, alert *domain.Alert) error
}

// NotificationService defines the interface for sending notifications.
type NotificationService interface {
	NotifyAlert(ctx context.Context, alert *domain.Alert) error
}

// Logger defines the interface for logging in the metricscheck domain.
type Logger interface {
	Debug(msg string, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	WithComponent(component string) Logger
}

// ============================================================================
// HEALTH READER INTERFACE
// ============================================================================

// HealthReader defines the minimal interface for reading health metrics.
// This is the local interface - implementations come from other domains.
type HealthReader interface {
	GetLatestHealth(ctx context.Context, tenantID string) (*domain.KeycloakHealth, error)
}

// ============================================================================
// CHECK INTERFACE
// ============================================================================

// Checker defines the interface for a health/metrics check.
type Checker interface {
	GetCheckType() string
	GetDescription() string
	Execute(ctx context.Context) ([]*domain.Alert, error)
}
