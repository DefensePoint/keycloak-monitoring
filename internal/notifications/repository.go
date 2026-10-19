package notifications

import (
	"context"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Repository defines the interface for notification log persistence operations.
type Repository interface {
	Reader
	Writer
}

// AlertGroup is the resource whose open alerts share one tracked issue. The
// match is exact, so okta and okta-eu, or one alias in two realms, stay apart.
type AlertGroup struct {
	TenantID     string
	RealmName    string
	ResourceType string
	ResourceName string
}

// GroupOf returns the group alert belongs to.
func GroupOf(alert *domain.Alert) AlertGroup {
	return AlertGroup{
		TenantID:     alert.TenantID,
		RealmName:    alert.RealmName,
		ResourceType: alert.ResourceType,
		ResourceName: alert.ResourceName,
	}
}

// Reader defines read operations for notification logs.
type Reader interface {
	// CheckAlreadySent reports whether the channel was already notified during
	// the alert's current occurrence (since it last went from resolved to
	// active, or since it was first raised).
	CheckAlreadySent(ctx context.Context, tenantID, alertID, channel string) (bool, error)

	// FindSharedExternalID returns the external ID an open alert of the group
	// already holds on this channel, from that alert's current occurrence.
	// Empty when no open alert of the group holds one.
	FindSharedExternalID(ctx context.Context, group AlertGroup, channel string) (string, error)

	// FindHeldExternalID returns the external ID one of these alerts holds on
	// this channel from its current occurrence, whatever its status.
	FindHeldExternalID(ctx context.Context, tenantID, channel string, alertIDs []string) (externalID, holderTitle string, err error)

	// CountSharingAlerts counts the open alerts, other than excludeAlertID,
	// whose current occurrence holds externalID on this channel.
	CountSharingAlerts(ctx context.Context, tenantID, channel, externalID, excludeAlertID string) (int, error)

	// CountIgnoredSharingAlerts counts the ignored alerts whose current
	// occurrence holds externalID on this channel.
	CountIgnoredSharingAlerts(ctx context.Context, tenantID, channel, externalID string) (int, error)

	// GetByAlertAndChannel retrieves a notification log by alert ID, channel, and status.
	GetByAlertAndChannel(ctx context.Context, alertID, channel, status string) (*domain.NotificationLog, error)

	// GetLogsByAlertID retrieves all notification logs for an alert.
	GetLogsByAlertID(ctx context.Context, alertID string) ([]*domain.NotificationLog, error)

	// GetFailedByAlertID retrieves failed notification logs for an alert.
	GetFailedByAlertID(ctx context.Context, alertID string) ([]*domain.NotificationLog, error)

	// UndeliveredChannels lists the channels whose send failed during the
	// alert's current occurrence and has not succeeded since.
	UndeliveredChannels(ctx context.Context, tenantID, alertID string) ([]string, error)

	// ListUndelivered lists the active, unacknowledged alerts of the given
	// check types that have an undelivered channel.
	ListUndelivered(ctx context.Context, checkTypes []string) ([]AlertKey, error)
}

// AlertKey identifies an alert.
type AlertKey struct {
	TenantID string
	AlertID  string
}

// Writer defines write operations for notification logs.
type Writer interface {
	// LogNotification logs a notification attempt.
	LogNotification(ctx context.Context, log *domain.NotificationLog) error
}
