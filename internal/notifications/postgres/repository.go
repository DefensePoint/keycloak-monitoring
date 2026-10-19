// Package postgres provides the PostgreSQL implementation of the notifications repository.
package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
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

// CheckAlreadySent reports whether this channel was already notified during
// the alert's current occurrence. Alert IDs are stable per problem, so counting
// every send the alert ever had meant a problem that resolved and came back was
// never notified again. Only sends at or after the occurrence start count; the
// start is read from the alert row itself, so it holds however the alert was
// raised or resolved. With no alert row every send counts, as before.
func (r *Repository) CheckAlreadySent(ctx context.Context, tenantID, alertID, channel string) (bool, error) {
	var count int64
	result := r.db.DB().WithContext(ctx).
		Model(&database.NotificationLog{}).
		Where("alert_id = ? AND channel = ? AND status = ?", alertID, channel, "sent").
		Where(`created_at >= COALESCE((
			SELECT COALESCE(a.occurrence_started_at, a.first_detected)
			FROM configuration_alerts a
			WHERE a.tenant_id = ? AND a.alert_id = ? AND a.deleted_at IS NULL
			LIMIT 1), '-infinity'::timestamptz)`, tenantID, alertID).
		Count(&count)

	if result.Error != nil {
		return false, fmt.Errorf("failed to check notification status: %w", result.Error)
	}

	return count > 0, nil
}

// openAlertStatuses are the statuses of an alert whose problem still stands,
// and which therefore still holds its issue open.
var openAlertStatuses = []string{string(domain.AlertStatusActive), string(domain.AlertStatusAcknowled)}

// heldIssues selects the channel's sends that an alert of the tenant holds:
// those from its current occurrence. An issue an earlier occurrence opened was
// closed when that occurrence resolved, so it is neither reused nor counted.
func (r *Repository) heldIssues(ctx context.Context, tenantID, channel string) *gorm.DB {
	return r.db.DB().WithContext(ctx).
		Model(&database.NotificationLog{}).
		Joins("JOIN configuration_alerts a ON a.alert_id = notification_logs.alert_id AND a.tenant_id = ? AND a.deleted_at IS NULL", tenantID).
		Where("notification_logs.channel = ? AND notification_logs.status = ?", channel, "sent").
		Where("notification_logs.external_id <> ''").
		Where("notification_logs.created_at >= COALESCE(a.occurrence_started_at, a.first_detected)")
}

// sharedIssues narrows heldIssues to the alerts whose problem still stands,
// and which therefore still hold their issue open.
func (r *Repository) sharedIssues(ctx context.Context, tenantID, channel string) *gorm.DB {
	return r.heldIssues(ctx, tenantID, channel).Where("a.status IN ?", openAlertStatuses)
}

// latestExternalID returns the most recently sent external ID the query holds.
func latestExternalID(query *gorm.DB, channel string) (string, error) {
	var externalIDs []string
	result := query.
		Order("notification_logs.created_at DESC").Order("notification_logs.id DESC").
		Limit(1).
		Pluck("notification_logs.external_id", &externalIDs)

	if result.Error != nil {
		return "", fmt.Errorf("failed to find a shared %s issue: %w", channel, result.Error)
	}
	if len(externalIDs) == 0 {
		return "", nil
	}
	return externalIDs[0], nil
}

// FindSharedExternalID returns the external ID an open alert of the group
// already holds on this channel.
func (r *Repository) FindSharedExternalID(ctx context.Context, group notifications.AlertGroup, channel string) (string, error) {
	return latestExternalID(r.sharedIssues(ctx, group.TenantID, channel).
		Where("a.realm_name = ? AND a.resource_type = ? AND a.resource_name = ?",
			group.RealmName, group.ResourceType, group.ResourceName), channel)
}

// FindHeldExternalID returns the external ID one of these alerts holds on this
// channel.
func (r *Repository) FindHeldExternalID(ctx context.Context, tenantID, channel string, alertIDs []string) (string, string, error) {
	if len(alertIDs) == 0 {
		return "", "", nil
	}
	var held []struct {
		ExternalID string
		Title      string
	}
	result := r.heldIssues(ctx, tenantID, channel).
		Where("notification_logs.alert_id IN ?", alertIDs).
		Order("notification_logs.created_at DESC").Order("notification_logs.id DESC").
		Limit(1).
		Select("notification_logs.external_id, a.title").
		Scan(&held)
	if result.Error != nil {
		return "", "", fmt.Errorf("failed to find a held %s issue: %w", channel, result.Error)
	}
	if len(held) == 0 {
		return "", "", nil
	}
	return held[0].ExternalID, held[0].Title, nil
}

// CountSharingAlerts counts the open alerts, other than excludeAlertID, whose
// current occurrence holds externalID on this channel.
func (r *Repository) CountSharingAlerts(ctx context.Context, tenantID, channel, externalID, excludeAlertID string) (int, error) {
	var count int64
	result := r.sharedIssues(ctx, tenantID, channel).
		Where("notification_logs.external_id = ? AND notification_logs.alert_id <> ?", externalID, excludeAlertID).
		Distinct("notification_logs.alert_id").
		Count(&count)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to count the alerts sharing %s issue %s: %w", channel, externalID, result.Error)
	}
	return int(count), nil
}

// CountIgnoredSharingAlerts counts the ignored alerts whose current
// occurrence holds externalID on this channel.
func (r *Repository) CountIgnoredSharingAlerts(ctx context.Context, tenantID, channel, externalID string) (int, error) {
	var count int64
	result := r.heldIssues(ctx, tenantID, channel).
		Where("a.status = ? AND notification_logs.external_id = ?", string(domain.AlertStatusIgnored), externalID).
		Distinct("notification_logs.alert_id").
		Count(&count)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to count the ignored alerts sharing %s issue %s: %w", channel, externalID, result.Error)
	}
	return int(count), nil
}

// GetByAlertAndChannel retrieves the most recent notification log for an alert,
// channel and status. Most recent, because an alert is notified once per
// occurrence: resolving it must act on the current occurrence's send (its
// GitLab issue, say), not on the first one it ever had.
func (r *Repository) GetByAlertAndChannel(ctx context.Context, alertID, channel, status string) (*domain.NotificationLog, error) {
	var dbLog database.NotificationLog
	result := r.db.DB().WithContext(ctx).
		Where("alert_id = ? AND channel = ? AND status = ?", alertID, channel, status).
		Order("created_at DESC").Order("id DESC").
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

// undeliveredSince matches failed sends in notification_logs f, for alert a,
// made during the alert's current occurrence on a channel that has had no
// successful send since the occurrence began.
const undeliveredSince = `f.status = 'failed'
	AND f.created_at >= COALESCE(a.occurrence_started_at, a.first_detected)
	AND NOT EXISTS (
		SELECT 1 FROM notification_logs s
		WHERE s.alert_id = f.alert_id AND s.channel = f.channel AND s.status = 'sent'
		AND s.created_at >= COALESCE(a.occurrence_started_at, a.first_detected))`

// UndeliveredChannels lists the channels whose send failed during the alert's
// current occurrence and has not succeeded since.
func (r *Repository) UndeliveredChannels(ctx context.Context, tenantID, alertID string) ([]string, error) {
	var channels []string
	err := r.db.DB().WithContext(ctx).Raw(`SELECT DISTINCT f.channel
		FROM notification_logs f
		JOIN configuration_alerts a ON a.alert_id = f.alert_id AND a.tenant_id = ? AND a.deleted_at IS NULL
		WHERE f.alert_id = ? AND `+undeliveredSince+`
		ORDER BY f.channel`, tenantID, alertID).Scan(&channels).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list undelivered channels: %w", err)
	}
	return channels, nil
}

// ListUndelivered lists the active, unacknowledged alerts of the given check
// types that have an undelivered channel.
func (r *Repository) ListUndelivered(ctx context.Context, checkTypes []string) ([]notifications.AlertKey, error) {
	var keys []notifications.AlertKey
	err := r.db.DB().WithContext(ctx).Raw(`SELECT DISTINCT a.tenant_id, a.alert_id
		FROM configuration_alerts a
		JOIN notification_logs f ON f.alert_id = a.alert_id
		WHERE a.deleted_at IS NULL AND a.status = 'active' AND a.acknowledged_at IS NULL
		AND a.check_type IN ? AND `+undeliveredSince+`
		ORDER BY a.tenant_id, a.alert_id`, checkTypes).Scan(&keys).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list alerts with undelivered notifications: %w", err)
	}
	return keys, nil
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
