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

	// CheckAlreadySent reports whether the channel was already notified during
	// the alert's current occurrence.
	CheckAlreadySent(ctx context.Context, tenantID, alertID, channel string) (bool, error)

	// LogNotification logs a notification attempt.
	LogNotification(ctx context.Context, alertID, channel, externalID, status, errorMessage string, metadata []byte) error

	// GetNotificationLogs retrieves all notification logs for an alert.
	GetNotificationLogs(ctx context.Context, alertID string) ([]*domain.NotificationLog, error)

	// GetFailedNotifications retrieves failed notification logs for an alert.
	GetFailedNotifications(ctx context.Context, alertID string) ([]*domain.NotificationLog, error)

	// GetByAlertAndChannel retrieves a notification log by alert ID, channel, and status.
	GetByAlertAndChannel(ctx context.Context, alertID, channel, status string) (*domain.NotificationLog, error)

	// RetryFailedNotifications sends the alert again to each channel whose
	// send failed during its current occurrence and has not succeeded since.
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

// resolutionNotifier is implemented by notifiers that can announce a
// resolution (Slack). issueResolver is implemented by notifiers that track an
// alert as an issue they can share and close (GitLab).
type resolutionNotifier interface {
	NotifyResolution(ctx context.Context, alert *domain.Alert) error
}

type issueResolver interface {
	ShareIssue(ctx context.Context, issueIID string, alert *domain.Alert, replaces string) error
	ResolveIssue(ctx context.Context, issueIID string, alert *domain.Alert, othersOpen, ignored int) error
}

const resourceTypeIdentityProvider = "identity_provider"

// sharesIssue reports whether the alert belongs on the issue the other open
// alerts of its resource already have. One identity provider's problems (no
// metadata URL, a metadata fetch that fails, a certificate per expiry) are one
// piece of work for whoever fixes the provider, so they share one issue, which
// closes when the last of them resolves. Every other alert keeps its own.
func sharesIssue(alert *domain.Alert) bool {
	return alert.ResourceType == resourceTypeIdentityProvider
}

// sendResolutionToSlack announces the resolution on Slack, when this occurrence
// of the alert was announced there.
func (s *service) sendResolutionToSlack(ctx context.Context, alert *domain.Alert) error {
	channel := ChannelSlack.String()
	notified, err := s.repo.CheckAlreadySent(ctx, alert.TenantID, alert.AlertID, channel)
	if err != nil {
		return err
	}
	if !notified {
		s.logger.Info("Slack resolution skipped: this occurrence of the alert was not sent to Slack",
			logger.Str("alert_id", alert.AlertID))
		return nil
	}

	resolver, ok := s.slack.(resolutionNotifier)
	if !ok {
		return fmt.Errorf("slack notifier cannot send resolutions")
	}
	if err := resolver.NotifyResolution(ctx, alert); err != nil {
		s.recorder().IncNotification(channel, "failure")
		return err
	}
	s.recorder().IncNotification(channel, "success")
	s.logger.Info("Slack resolution notification sent",
		logger.Str("alert_id", alert.AlertID))
	return nil
}

// sendResolutionToGitLab comments on and closes the issue this occurrence of
// the alert opened.
func (s *service) sendResolutionToGitLab(ctx context.Context, alert *domain.Alert) error {
	channel := ChannelGitLab.String()
	notified, err := s.repo.CheckAlreadySent(ctx, alert.TenantID, alert.AlertID, channel)
	if err != nil {
		return err
	}
	if !notified {
		s.logger.Info("GitLab resolution skipped: this occurrence of the alert opened no GitLab issue",
			logger.Str("alert_id", alert.AlertID))
		return nil
	}

	// The most recent send is this occurrence's; its issue IID is the ExternalID.
	notifLog, err := s.repo.GetByAlertAndChannel(ctx, alert.AlertID, channel, StatusSent.String())
	if err != nil {
		return err
	}
	if notifLog == nil || notifLog.ExternalID == "" {
		s.logger.Warn("GitLab issue ID not found in notification log",
			logger.Str("alert_id", alert.AlertID))
		return nil
	}

	resolver, ok := s.gitlab.(issueResolver)
	if !ok {
		return fmt.Errorf("gitlab notifier cannot resolve issues")
	}
	othersOpen, err := s.repo.CountSharingAlerts(ctx, alert.TenantID, channel, notifLog.ExternalID, alert.AlertID)
	if err != nil {
		return err
	}
	ignored := 0
	if othersOpen == 0 {
		if ignored, err = s.repo.CountIgnoredSharingAlerts(ctx, alert.TenantID, channel, notifLog.ExternalID); err != nil {
			return err
		}
	}
	if err := resolver.ResolveIssue(ctx, notifLog.ExternalID, alert, othersOpen, ignored); err != nil {
		s.recorder().IncNotification(channel, "failure")
		return err
	}
	s.recorder().IncNotification(channel, "success")
	if othersOpen > 0 {
		s.logger.Info("Reported the resolution on a GitLab issue other open alerts still hold",
			logger.Str("alert_id", alert.AlertID),
			logger.Str("issue_id", notifLog.ExternalID),
			logger.Int("others_open", othersOpen))
		return nil
	}
	s.logger.Info("Closed GitLab issue as resolved",
		logger.Str("alert_id", alert.AlertID),
		logger.Str("issue_id", notifLog.ExternalID))
	return nil
}

// sendToChannel sends an alert to a specific notification channel.
func (s *service) sendToChannel(ctx context.Context, alert *domain.Alert, notifier Notifier) error {
	channel := notifier.Channel().String()

	// Once per occurrence: a send from an earlier occurrence of this alert,
	// before it was resolved and came back, does not count.
	alreadySent, err := s.repo.CheckAlreadySent(ctx, alert.TenantID, alert.AlertID, channel)
	if err != nil {
		s.logger.Warn("Failed to check notification status",
			logger.Str("alert_id", alert.AlertID),
			logger.Str("channel", channel),
			logger.Err(err))
		// Continue anyway - better to send duplicate than miss an alert
	} else if alreadySent {
		// Info, not Debug: a skipped send is the answer to "why did nobody
		// hear about this", and it must be visible at the default level.
		s.logger.Info("Notification skipped: already sent during this occurrence of the alert",
			logger.Str("alert_id", alert.AlertID),
			logger.Str("channel", channel))
		return nil
	}

	// Send notification
	externalID, err := s.send(ctx, alert, notifier)
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
	s.logger.Info("Notification sent",
		logger.Str("alert_id", alert.AlertID),
		logger.Str("channel", channel))

	// Log successful notification
	if err := s.logNotificationAttempt(ctx, alert, channel, externalID, StatusSent.String(), ""); err != nil {
		s.logger.Warn("Failed to log notification",
			logger.Str("alert_id", alert.AlertID),
			logger.Str("channel", channel),
			logger.Err(err))
	}

	return nil
}

// send gives the alert to the channel and returns the external ID to record
// for it, which is the issue an open alert on the same resource already holds
// when the alert shares one.
func (s *service) send(ctx context.Context, alert *domain.Alert, notifier Notifier) (string, error) {
	channel := notifier.Channel().String()
	resolver, ok := notifier.(issueResolver)
	if !ok || !sharesIssue(alert) {
		return notifier.Notify(ctx, alert)
	}

	shared, replaces, err := s.sharedIssue(ctx, alert, channel)
	if err != nil {
		s.logger.Warn("Failed to look for an issue to share; opening a new one",
			logger.Str("alert_id", alert.AlertID),
			logger.Str("channel", channel),
			logger.Err(err))
		return notifier.Notify(ctx, alert)
	}
	if shared == "" {
		return notifier.Notify(ctx, alert)
	}

	if err := resolver.ShareIssue(ctx, shared, alert, replaces); err != nil {
		return "", err
	}
	s.logger.Info("Added the alert to the issue its resource already has",
		logger.Str("alert_id", alert.AlertID),
		logger.Str("channel", channel),
		logger.Str("issue_id", shared))
	return shared, nil
}

// sharedIssue returns the issue the alert belongs on: the one the other open
// alerts of its resource hold or, failing that, the one held by an alert it
// supersedes, which is resolved without a notice of its own precisely because
// this alert carries the news, and whose issue would otherwise be left behind.
// replaces is the title of a superseded alert that holds the same issue.
func (s *service) sharedIssue(ctx context.Context, alert *domain.Alert, channel string) (issue, replaces string, err error) {
	shared, err := s.repo.FindSharedExternalID(ctx, GroupOf(alert), channel)
	if err != nil || len(alert.Supersedes) == 0 {
		return shared, "", err
	}
	held, title, err := s.repo.FindHeldExternalID(ctx, alert.TenantID, channel, alert.Supersedes)
	if err != nil {
		return "", "", err
	}
	if shared == "" {
		return held, title, nil
	}
	if held == shared {
		return shared, title, nil
	}
	return shared, "", nil
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

// RetryFailedNotifications sends the alert again to each channel whose send
// failed during its current occurrence and has not succeeded since, if that
// channel is still enabled and the alert still meets its minimum severity.
func (s *service) RetryFailedNotifications(ctx context.Context, alert *domain.Alert) error {
	if !s.enabled {
		return nil
	}

	channels, err := s.repo.UndeliveredChannels(ctx, alert.TenantID, alert.AlertID)
	if err != nil {
		return fmt.Errorf("failed to find undelivered channels: %w", err)
	}

	for _, channel := range channels {
		notifier, minSeverity := s.channelNotifier(Channel(channel))
		if notifier == nil || !notifier.IsEnabled() || !MeetsMinimumSeverity(alert.Severity, minSeverity) {
			continue
		}
		s.logger.Info("Retrying a failed notification",
			logger.Str("alert_id", alert.AlertID),
			logger.Str("channel", channel))
		if err := s.sendToChannel(ctx, alert, notifier); err != nil {
			s.logger.Error("Retry failed",
				logger.Str("alert_id", alert.AlertID),
				logger.Str("channel", channel),
				logger.Err(err))
		}
	}

	return nil
}

// channelNotifier returns the notifier for a channel and its minimum severity.
func (s *service) channelNotifier(channel Channel) (Notifier, string) {
	switch channel {
	case ChannelGitLab:
		return s.gitlab, s.config.GitLab.MinSeverity
	case ChannelSlack:
		return s.slack, s.config.Slack.MinSeverity
	case ChannelEmail:
		return s.email, s.config.Email.MinSeverity
	}
	return nil, ""
}

// CheckAlreadySent reports whether the channel was already notified during the
// alert's current occurrence.
func (s *service) CheckAlreadySent(ctx context.Context, tenantID, alertID, channel string) (bool, error) {
	return s.repo.CheckAlreadySent(ctx, tenantID, alertID, channel)
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
