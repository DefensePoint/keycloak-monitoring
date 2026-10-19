package alerts

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Repository defines the interface for alert persistence.
type Repository interface {
	Reader
	Writer
}

// Reader defines read operations for alerts.
type Reader interface {
	// GetByID retrieves an alert by its database ID.
	GetByID(ctx context.Context, id uint) (*domain.Alert, error)

	// GetByAlertID retrieves an alert by its unique alert_id.
	GetByAlertID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)

	// GetByAlertIDGlobal retrieves an alert by its alert_id across all tenants.
	GetByAlertIDGlobal(ctx context.Context, alertID string) (*domain.Alert, error)

	// List retrieves alerts with optional filtering.
	List(ctx context.Context, tenantID string, opts *ListOptions) ([]*domain.Alert, error)

	// ListByRealm retrieves alerts for a specific realm.
	ListByRealm(ctx context.Context, tenantID, realmName string, opts *ListOptions) ([]*domain.Alert, error)

	// ListOpenByCheck returns the tenant's alerts raised by one check that are
	// still open, active or acknowledged, so the check can resolve those whose
	// condition has cleared.
	ListOpenByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error)

	// LatestFirstDetected returns when the newest alert raised by checkType in
	// realmName was first detected, counting deleted alerts. ok is false when
	// there is none.
	LatestFirstDetected(ctx context.Context, tenantID, checkType, realmName string) (at time.Time, ok bool, err error)

	// ExistsIncludingDeleted reports whether the tenant ever had an alert with
	// alertID, counting deleted alerts.
	ExistsIncludingDeleted(ctx context.Context, tenantID, alertID string) (bool, error)

	// ListUnresolved returns every alert of the tenant that is not resolved:
	// active, acknowledged or ignored.
	ListUnresolved(ctx context.Context, tenantID string) ([]*domain.Alert, error)

	// CountActive returns the total number of active alerts.
	CountActive(ctx context.Context, tenantID string) (int64, error)

	// Count returns the number of alerts matching the given filter options.
	Count(ctx context.Context, tenantID string, opts *ListOptions) (int64, error)

	// GetStatistics returns aggregated alert statistics, optionally scoped to a
	// set of realms.
	GetStatistics(ctx context.Context, tenantID string, realmName ...string) (*Statistics, error)
}

// Writer defines write operations for alerts.
type Writer interface {
	// Save inserts or updates an alert.
	Save(ctx context.Context, alert *domain.Alert) error

	// UpdateStatus updates the status of an alert.
	UpdateStatus(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) error

	// Resolve marks an alert as resolved.
	Resolve(ctx context.Context, tenantID, alertID string) error

	// Delete removes an alert.
	Delete(ctx context.Context, tenantID, alertID string) error
}
