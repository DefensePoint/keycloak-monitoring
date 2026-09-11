// Package amfacheck converts high-risk AMFA login events into KMT alerts.
// It deliberately mirrors the shape of the eventscheck package: one per-tenant
// poll loop running a set of Check implementations whose alerts flow through
// the existing alerts repository and notification fan-out. It is a sibling of
// eventscheck rather than a refactor of it.
package amfacheck

import (
	"context"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Check is implemented by each of the five rules. Mirrors eventscheck.Check.
type Check interface {
	// GetCheckType returns the unique identifier for this check,
	// e.g. "amfa-risk-rejected".
	GetCheckType() string

	// GetDescription returns a human-readable description of this check.
	GetDescription() string

	// Execute runs the check and returns any detected alerts.
	Execute(ctx context.Context) ([]*domain.Alert, error)
}

// AlertStore is the slice of alerts.Repository this service needs.
type AlertStore interface {
	GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
	SaveAlert(ctx context.Context, alert *domain.Alert) error
}

// NotificationService is the slice of notifications.Service this service needs.
type NotificationService interface {
	NotifyAlert(ctx context.Context, alert *domain.Alert) error
}

// Logger is the logging contract for amfacheck. Mirrors eventscheck.Logger so
// the fx adapter can be near-identical.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
	Debug(msg string, args ...any)
	WithComponent(component string) Logger
}

// RealmsFunc returns the current realm list for a tenant. Each Check holds one
// and calls it every tick, so realms added/removed in Keycloak are picked up
// without a restart.
type RealmsFunc func(ctx context.Context) ([]string, error)
