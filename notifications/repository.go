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

// Reader defines read operations for notification logs.
type Reader interface {
	// CheckAlreadySent checks if a notification has already been sent for this alert to this channel.
	CheckAlreadySent(ctx context.Context, alertID, channel string) (bool, error)

	// GetByAlertAndChannel retrieves a notification log by alert ID, channel, and status.
	GetByAlertAndChannel(ctx context.Context, alertID, channel, status string) (*domain.NotificationLog, error)

	// GetLogsByAlertID retrieves all notification logs for an alert.
	GetLogsByAlertID(ctx context.Context, alertID string) ([]*domain.NotificationLog, error)

	// GetFailedByAlertID retrieves failed notification logs for an alert.
	GetFailedByAlertID(ctx context.Context, alertID string) ([]*domain.NotificationLog, error)
}

// Writer defines write operations for notification logs.
type Writer interface {
	// LogNotification logs a notification attempt.
	LogNotification(ctx context.Context, log *domain.NotificationLog) error
}
