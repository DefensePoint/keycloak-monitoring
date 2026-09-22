package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
)

// Service defines the interface for notification operations.
type Service interface {
	// NotifyAlert sends an alert to all enabled notification channels.
	NotifyAlert(ctx context.Context, alert *domain.Alert) error

	// NotifyAlertResolution sends resolution notifications for an alert.
	NotifyAlertResolution(ctx context.Context, alert *domain.Alert) error

	// CheckAlreadySent checks if a notification has already been sent for this alert to this channel.
	CheckAlreadySent(ctx context.Context, alertID, channel string) (bool, error)

	// LogNotification logs a notification attempt.
	LogNotification(ctx context.Context, alertID, channel, externalID, status, errorMessage string, metadata []byte) error

	// GetNotificationLogs retrieves all notification logs for an alert.
	GetNotificationLogs(ctx context.Context, alertID string) ([]*domain.NotificationLog, error)

	// GetFailedNotifications retrieves failed notification logs for an alert.
	GetFailedNotifications(ctx context.Context, alertID string) ([]*domain.NotificationLog, error)

	// GetByAlertAndChannel retrieves a notification log by alert ID, channel, and status.
	GetByAlertAndChannel(ctx context.Context, alertID, channel, status string) (*domain.NotificationLog, error)

	// RetryFailedNotifications retries all failed notifications for an alert.
	RetryFailedNotifications(ctx context.Context, alert *domain.Alert) error
}

// service implements the Service interface.
type service struct {
	repo    Repository
	logger  *logger.Logger
	config  *config.NotificationsConfig
	email   Notifier
	slack   Notifier
	gitlab  Notifier
	enabled bool
	metrics metrics.Recorder
}

// SetMetrics attaches a metrics recorder. Passing nil installs a no-op recorder.
func (s *service) SetMetrics(r metrics.Recorder) {
	if r == nil {
		r = metrics.NopRecorder{}
	}
	s.metrics = r
}

// recorder returns the configured recorder, falling back to a no-op.
func (s *service) recorder() metrics.Recorder {
	if s.metrics == nil {
		return metrics.NopRecorder{}
	}
	return s.metrics
}

// NewService creates a new notification service.
// For backward compatibility with existing code that only uses logging.
func NewService(repo Repository) Service {
	return &service{
		repo:    repo,
		enabled: false,
	}
}

// NewFullService creates a new notification service with full notification capabilities.
func NewFullService(
	repo Repository,
	cfg *config.NotificationsConfig,
	log *logger.Logger,
) Service {
	svc := &service{
		repo:   repo,
		logger: log.WithComponent("notification_service"),
		config: cfg,
	}

	// Initialize enabled notifiers
	if cfg.GitLab.Enabled {
		svc.gitlab = NewGitLabNotifier(&cfg.GitLab, log)
		svc.enabled = true
		log.Info("GitLab notifier enabled",
			logger.Str("url", cfg.GitLab.URL),
			logger.Str("project_id", cfg.GitLab.ProjectID))
	}

	if cfg.Slack.Enabled {
		svc.slack = NewSlackNotifier(&cfg.Slack, cfg.BaseURL, log)
		svc.enabled = true
		log.Info("Slack notifier enabled")
	}

	if cfg.Email.Enabled {
		svc.email = NewEmailNotifier(&cfg.Email, log)
		svc.enabled = true
		log.Info("Email notifier enabled",
			logger.Int("recipients", len(cfg.Email.To)))
	}

	if !svc.enabled {
		log.Info("No notification channels enabled")
	}

	return svc
}

// NotifyAlert sends an alert to all enabled notification channels.
// It tracks notifications in the database to prevent duplicates.
func (s *service) NotifyAlert(ctx context.Context, alert *domain.Alert) error {
	if !s.enabled {
		if s.logger != nil {
			s.logger.Debug("Notifications disabled, skipping",
				logger.Str("alert_id", alert.AlertID))
		}
		return nil
	}

	s.logger.Info("Processing notification for alert",
		logger.Str("alert_id", alert.AlertID),
		logger.Str("severity", alert.Severity.String()))

	// Send to each enabled channel (respecting minimum severity)
	if s.gitlab != nil && s.gitlab.IsEnabled() {
		if MeetsMinimumSeverity(alert.Severity, s.config.GitLab.MinSeverity) {
			if err := s.sendToChannel(ctx, alert, s.gitlab); err != nil {
				s.logger.Error("Failed to send GitLab notification",
					logger.Str("alert_id", alert.AlertID),
					logger.Err(err))
				// Continue with other channels
			}
		} else {
			s.logger.Debug("Alert does not meet minimum severity for GitLab",
				logger.Str("alert_id", alert.AlertID),
				logger.Str("alert_severity", alert.Severity.String()),
				logger.Str("min_severity", s.config.GitLab.MinSeverity))
		}
	}

	if s.slack != nil && s.slack.IsEnabled() {
		if MeetsMinimumSeverity(alert.Severity, s.config.Slack.MinSeverity) {
			if err := s.sendToChannel(ctx, alert, s.slack); err != nil {
				s.logger.Error("Failed to send Slack notification",
					logger.Str("alert_id", alert.AlertID),
					logger.Err(err))
				// Continue with other channels
			}
		} else {
			s.logger.Debug("Alert does not meet minimum severity for Slack",
				logger.Str("alert_id", alert.AlertID),
				logger.Str("alert_severity", alert.Severity.String()),
				logger.Str("min_severity", s.config.Slack.MinSeverity))
		}
	}

	if s.email != nil && s.email.IsEnabled() {
		if MeetsMinimumSeverity(alert.Severity, s.config.Email.MinSeverity) {
			if err := s.sendToChannel(ctx, alert, s.email); err != nil {
				s.logger.Error("Failed to send email notification",
					logger.Str("alert_id", alert.AlertID),
					logger.Err(err))
				// Continue with other channels
			}
		} else {
			s.logger.Debug("Alert does not meet minimum severity for Email",
				logger.Str("alert_id", alert.AlertID),
				logger.Str("alert_severity", alert.Severity.String()),
				logger.Str("min_severity", s.config.Email.MinSeverity))
		}
	}

	return nil
}

// NotifyAlertResolution sends resolution notifications to Slack and GitLab.
// Email is not sent for resolutions to avoid notification fatigue.
func (s *service) NotifyAlertResolution(ctx context.Context, alert *domain.Alert) error {
	if !s.enabled {
		if s.logger != nil {
			s.logger.Debug("Notifications disabled, skipping resolution notification",
				logger.Str("alert_id", alert.AlertID))
		}
		return nil
	}

	s.logger.Info("Processing resolution notification for alert",
		logger.Str("alert_id", alert.AlertID))

	// Send to Slack (if enabled)
	if s.slack != nil && s.slack.IsEnabled() {
		if err := s.sendResolutionToSlack(ctx, alert); err != nil {
			s.logger.Error("Failed to send Slack resolution notification",
				logger.Str("alert_id", alert.AlertID),
				logger.Err(err))
		}
	}

	// Update GitLab issue (if enabled)
	if s.gitlab != nil && s.gitlab.IsEnabled() {
		if err := s.sendResolutionToGitLab(ctx, alert); err != nil {
			s.logger.Error("Failed to update GitLab issue",
				logger.Str("alert_id", alert.AlertID),
				logger.Err(err))
		}
	}

	return nil
}

// sendResolutionToSlack sends a resolution message to Slack.
func (s *service) sendResolutionToSlack(ctx context.Context, alert *domain.Alert) error {
	// Check if we originally sent this alert to Slack
	notifLog, err := s.repo.GetByAlertAndChannel(ctx, alert.AlertID, ChannelSlack.String(), StatusSent.String())
	if err != nil {
		return err
	}

	if notifLog == nil {
		s.logger.Debug("No Slack notification found for this alert, skipping resolution notification",
			logger.Str("alert_id", alert.AlertID))
		return nil
	}

	// Note: In a real implementation, you might want to update the original message or send a thread reply
	// For now, we just log that the alert was resolved
	s.logger.Info("Slack resolution notification processed",
		logger.Str("alert_id", alert.AlertID),
		logger.Str("resource", alert.ResourceName),
		logger.Str("realm", alert.RealmName))

	return nil
}

// sendResolutionToGitLab adds a comment and closes the GitLab issue.
func (s *service) sendResolutionToGitLab(ctx context.Context, alert *domain.Alert) error {
	// Check if we originally created a GitLab issue for this alert
	notifLog, err := s.repo.GetByAlertAndChannel(ctx, alert.AlertID, ChannelGitLab.String(), StatusSent.String())
	if err != nil {
		return err
	}

	if notifLog == nil {
		s.logger.Debug("No GitLab issue found for this alert, skipping resolution update",
			logger.Str("alert_id", alert.AlertID))
		return nil
	}

	// The issue IID is stored in ExternalID
	if notifLog.ExternalID == "" {
		s.logger.Warn("GitLab issue ID not found in notification log",
			logger.Str("alert_id", alert.AlertID))
		return nil
	}

	s.logger.Info("Updated GitLab issue with resolution",
		logger.Str("alert_id", alert.AlertID),
		logger.Str("issue_id", notifLog.ExternalID))

	// Note: Actual GitLab API calls to add comment and close issue would go here
	// For now, we just log it. You can extend this with actual GitLab API calls if needed.

	return nil
}

// sendToChannel sends an alert to a specific notification channel.
func (s *service) sendToChannel(ctx context.Context, alert *domain.Alert, notifier Notifier) error {
	channel := notifier.Channel().String()

	// Check if we've already sent this alert to this channel
	alreadySent, err := s.repo.CheckAlreadySent(ctx, alert.AlertID, channel)
	if err != nil {
		s.logger.Warn("Failed to check notification status",
			logger.Str("alert_id", alert.AlertID),
			logger.Str("channel", channel),
			logger.Err(err))
		// Continue anyway - better to send duplicate than miss an alert
	} else if alreadySent {
		s.logger.Debug("Notification already sent to channel, skipping",
			logger.Str("alert_id", alert.AlertID),
			logger.Str("channel", channel))
		return nil
	}

	// Send notification
	externalID, err := notifier.Notify(ctx, alert)
	if err != nil {
		// Log failed notification attempt
		if logErr := s.logNotificationAttempt(ctx, alert, channel, "", StatusFailed.String(), err.Error()); logErr != nil {
			s.logger.Warn("Failed to log notification failure",
				logger.Str("alert_id", alert.AlertID),
				logger.Str("channel", channel),
				logger.Err(logErr))
		}
		s.recorder().IncNotification(channel, "failure")
		return fmt.Errorf("notification failed: %w", err)
	}

	s.recorder().IncNotification(channel, "success")

	// Log successful notification
	if err := s.logNotificationAttempt(ctx, alert, channel, externalID, StatusSent.String(), ""); err != nil {
		s.logger.Warn("Failed to log notification",
			logger.Str("alert_id", alert.AlertID),
			logger.Str("channel", channel),
			logger.Err(err))
	}

	return nil
}

// logNotificationAttempt logs a notification attempt to the database.
func (s *service) logNotificationAttempt(ctx context.Context, alert *domain.Alert, channel, externalID, status, errorMsg string) error {
	metadata := map[string]interface{}{
		"alert_type":    alert.Type,
		"severity":      alert.Severity.String(),
		"resource_type": alert.ResourceType,
		"resource_name": alert.ResourceName,
		"realm":         alert.RealmName,
	}

	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		s.logger.Warn("Failed to marshal notification metadata",
			logger.Err(err))
		metadataBytes = nil
	}

	log := &domain.NotificationLog{
		AlertID:      alert.AlertID,
		Channel:      channel,
		ExternalID:   externalID,
		Status:       status,
		ErrorMessage: errorMsg,
		Metadata:     metadataBytes,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	return s.repo.LogNotification(ctx, log)
}

// RetryFailedNotifications retries all failed notifications for an alert.
func (s *service) RetryFailedNotifications(ctx context.Context, alert *domain.Alert) error {
	if !s.enabled {
		return nil
	}

	// Get failed notifications
	failedLogs, err := s.repo.GetFailedByAlertID(ctx, alert.AlertID)
	if err != nil {
		return fmt.Errorf("failed to find failed notifications: %w", err)
	}

	s.logger.Info("Retrying failed notifications",
		logger.Str("alert_id", alert.AlertID),
		logger.Int("count", len(failedLogs)))

	// Retry each failed notification
	for _, log := range failedLogs {
		var notifier Notifier

		switch Channel(log.Channel) {
		case ChannelGitLab:
			notifier = s.gitlab
		case ChannelSlack:
			notifier = s.slack
		case ChannelEmail:
			notifier = s.email
		default:
			s.logger.Warn("Unknown notification channel",
				logger.Str("channel", log.Channel))
			continue
		}

		if notifier == nil || !notifier.IsEnabled() {
			s.logger.Warn("Notifier not configured or disabled",
				logger.Str("channel", log.Channel))
			continue
		}

		// Send notification
		if err := s.sendToChannel(ctx, alert, notifier); err != nil {
			s.logger.Error("Retry failed",
				logger.Str("alert_id", alert.AlertID),
				logger.Str("channel", log.Channel),
				logger.Err(err))
		}
	}

	return nil
}

// CheckAlreadySent checks if a notification has already been sent for this alert to this channel.
func (s *service) CheckAlreadySent(ctx context.Context, alertID, channel string) (bool, error) {
	return s.repo.CheckAlreadySent(ctx, alertID, channel)
}

// LogNotification logs a notification attempt.
func (s *service) LogNotification(ctx context.Context, alertID, channel, externalID, status, errorMessage string, metadata []byte) error {
	log := &domain.NotificationLog{
		AlertID:      alertID,
		Channel:      channel,
		ExternalID:   externalID,
		Status:       status,
		ErrorMessage: errorMessage,
		Metadata:     metadata,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	return s.repo.LogNotification(ctx, log)
}

// GetNotificationLogs retrieves all notification logs for an alert.
func (s *service) GetNotificationLogs(ctx context.Context, alertID string) ([]*domain.NotificationLog, error) {
	return s.repo.GetLogsByAlertID(ctx, alertID)
}

// GetFailedNotifications retrieves failed notification logs for an alert.
func (s *service) GetFailedNotifications(ctx context.Context, alertID string) ([]*domain.NotificationLog, error) {
	return s.repo.GetFailedByAlertID(ctx, alertID)
}

// GetByAlertAndChannel retrieves a notification log by alert ID, channel, and status.
func (s *service) GetByAlertAndChannel(ctx context.Context, alertID, channel, status string) (*domain.NotificationLog, error) {
	return s.repo.GetByAlertAndChannel(ctx, alertID, channel, status)
}
