// Package amfacheck converts high-risk AMFA login events into KMT alerts.
// It deliberately mirrors the shape of the eventscheck package: one per-tenant
// poll loop running a set of Check implementations whose alerts flow through
// the existing alerts repository and notification fan-out. It is a sibling of
// eventscheck rather than a refactor of it.
package amfacheck

import (
	"context"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts/clearing"
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

// Evaluator is a Check that also reports which conditions it evaluated, so
// the service can resolve alerts whose condition has cleared. Every
// rolling-window threshold rule implements it, since its count drops once the
// burst ages out of the window. The per-event rules do not: each of their
// alerts records one security event someone should review, which does not
// clear.
type Evaluator interface {
	Check
	Evaluate(ctx context.Context) (clearing.Evaluation, error)
}

// AlertStore is the slice of alerts.Repository this service needs.
type AlertStore interface {
	GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
	SaveAlert(ctx context.Context, alert *domain.Alert) error
	ListOpenAlertsByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error)
	ResolveAlert(ctx context.Context, tenantID, alertID string) error
	// AlertExists reports whether alertID was ever raised for the tenant,
	// counting deleted alerts: a per-event check rereads recent events, and
	// an alert an operator deleted must not come back.
	AlertExists(ctx context.Context, tenantID, alertID string) (bool, error)
}

// Silencer reports whether a configured silence rule covers an alert, and
// which one. alerts.SilenceManager implements it.
type Silencer interface {
	ShouldSilence(alert *domain.Alert) (bool, string)
}

// NotificationService is the slice of notifications.Service this service needs.
type NotificationService interface {
	NotifyAlert(ctx context.Context, alert *domain.Alert) error
	NotifyAlertResolution(ctx context.Context, alert *domain.Alert) error
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

// The rolling-window threshold rules evaluate, so their alerts can resolve.
var (
	_ Evaluator = (*LoginErrorCheck)(nil)
	_ Evaluator = (*ClientLoginErrorCheck)(nil)
)
