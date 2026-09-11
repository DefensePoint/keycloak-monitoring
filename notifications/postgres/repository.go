// Package postgres provides the PostgreSQL implementation of the notifications repository.
package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/notifications"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// Repository implements notifications.Repository using PostgreSQL via GORM.
type Repository struct {
	db *database.Client
}

// NewRepository creates a new PostgreSQL notifications repository.
func NewRepository(db *database.Client) notifications.Repository {
	return &Repository{db: db}
}

// CheckAlreadySent checks if a notification has already been sent for this alert to this channel.
func (r *Repository) CheckAlreadySent(ctx context.Context, alertID, channel string) (bool, error) {
	var count int64
	result := r.db.DB().WithContext(ctx).
		Model(&database.NotificationLog{}).
		Where("alert_id = ? AND channel = ? AND status = ?", alertID, channel, "sent").
		Count(&count)

	if result.Error != nil {
		return false, fmt.Errorf("failed to check notification status: %w", result.Error)
	}

	return count > 0, nil
}

// GetByAlertAndChannel retrieves a notification log by alert ID, channel, and status.
func (r *Repository) GetByAlertAndChannel(ctx context.Context, alertID, channel, status string) (*domain.NotificationLog, error) {
	var dbLog database.NotificationLog
	result := r.db.DB().WithContext(ctx).
		Where("alert_id = ? AND channel = ? AND status = ?", alertID, channel, status).
		First(&dbLog)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get notification log: %w", result.Error)
	}

	return toNotificationsLog(&dbLog), nil
}

// GetLogsByAlertID retrieves all notification logs for an alert.
func (r *Repository) GetLogsByAlertID(ctx context.Context, alertID string) ([]*domain.NotificationLog, error) {
	var dbLogs []*database.NotificationLog
	result := r.db.DB().WithContext(ctx).
		Where("alert_id = ?", alertID).
		Order("created_at DESC").
		Find(&dbLogs)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get notification logs: %w", result.Error)
	}

	return toNotificationsLogs(dbLogs), nil
}

// GetFailedByAlertID retrieves failed notification logs for an alert.
func (r *Repository) GetFailedByAlertID(ctx context.Context, alertID string) ([]*domain.NotificationLog, error) {
	var dbLogs []*database.NotificationLog
	result := r.db.DB().WithContext(ctx).
		Where("alert_id = ? AND status = ?", alertID, "failed").
		Order("created_at DESC").
		Find(&dbLogs)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get failed notification logs: %w", result.Error)
	}

	return toNotificationsLogs(dbLogs), nil
}

// LogNotification logs a notification attempt.
func (r *Repository) LogNotification(ctx context.Context, log *domain.NotificationLog) error {
	dbLog := toDBLog(log)
	result := r.db.DB().WithContext(ctx).Create(dbLog)
	if result.Error != nil {
		return fmt.Errorf("failed to log notification: %w", result.Error)
	}

	log.ID = dbLog.ID
	return nil
}

// toNotificationsLog converts a database NotificationLog to domain.NotificationLog.
func toNotificationsLog(db *database.NotificationLog) *domain.NotificationLog {
	if db == nil {
		return nil
	}
	return &domain.NotificationLog{
		ID:           db.ID,
		AlertID:      db.AlertID,
		Channel:      db.Channel,
		ExternalID:   db.ExternalID,
		Status:       db.Status,
		ErrorMessage: db.ErrorMessage,
		Metadata:     db.Metadata,
		CreatedAt:    db.CreatedAt,
		UpdatedAt:    db.UpdatedAt,
	}
}

// toNotificationsLogs converts a slice of database NotificationLog to domain.NotificationLog.
func toNotificationsLogs(dbLogs []*database.NotificationLog) []*domain.NotificationLog {
	logs := make([]*domain.NotificationLog, len(dbLogs))
	for i, db := range dbLogs {
		logs[i] = toNotificationsLog(db)
	}
	return logs
}

// toDBLog converts a domain.NotificationLog to database NotificationLog.
func toDBLog(n *domain.NotificationLog) *database.NotificationLog {
	if n == nil {
		return nil
	}
	return &database.NotificationLog{
		ID:           n.ID,
		AlertID:      n.AlertID,
		Channel:      n.Channel,
		ExternalID:   n.ExternalID,
		Status:       n.Status,
		ErrorMessage: n.ErrorMessage,
		Metadata:     n.Metadata,
		CreatedAt:    n.CreatedAt,
		UpdatedAt:    n.UpdatedAt,
	}
}

// Ensure Repository implements notifications.Repository
var _ notifications.Repository = (*Repository)(nil)
