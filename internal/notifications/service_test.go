package notifications_test

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
)

// mockRepository implements notifications.Repository for testing.
type mockRepository struct {
	logs          []*domain.NotificationLog
	alreadySent   map[string]bool
	loggedEntries []*domain.NotificationLog
	alerts        map[string]*domain.Alert
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		logs:          make([]*domain.NotificationLog, 0),
		alreadySent:   make(map[string]bool),
		loggedEntries: make([]*domain.NotificationLog, 0),
		alerts:        make(map[string]*domain.Alert),
	}
}

// track stores the alerts the sharing queries read, as the alerts table does
// for the real repository.
func (m *mockRepository) track(alerts ...*domain.Alert) {
	for _, alert := range alerts {
		m.alerts[alert.AlertID] = alert
	}
}

// open returns the tracked alert when its problem still stands.
func (m *mockRepository) open(alertID string) *domain.Alert {
	alert := m.alerts[alertID]
	if alert == nil || alert.Status == domain.AlertStatusResolved || alert.Status == domain.AlertStatusIgnored {
		return nil
	}
	return alert
}

func (m *mockRepository) CheckAlreadySent(ctx context.Context, tenantID, alertID, channel string) (bool, error) {
	key := alertID + ":" + channel
	return m.alreadySent[key], nil
}

func (m *mockRepository) FindSharedExternalID(ctx context.Context, group notifications.AlertGroup, channel string) (string, error) {
	for i := len(m.logs) - 1; i >= 0; i-- {
		log := m.logs[i]
		if log.Channel != channel || log.Status != "sent" || log.ExternalID == "" {
			continue
		}
		if alert := m.open(log.AlertID); alert != nil && notifications.GroupOf(alert) == group {
			return log.ExternalID, nil
		}
	}
	return "", nil
}

func (m *mockRepository) FindHeldExternalID(ctx context.Context, tenantID, channel string, alertIDs []string) (string, string, error) {
	held := make(map[string]bool, len(alertIDs))
	for _, id := range alertIDs {
		held[id] = true
	}
	for i := len(m.logs) - 1; i >= 0; i-- {
		log := m.logs[i]
		if log.Channel != channel || log.Status != "sent" || log.ExternalID == "" || !held[log.AlertID] {
			continue
		}
		if alert := m.alerts[log.AlertID]; alert != nil && alert.TenantID == tenantID {
			return log.ExternalID, alert.Title, nil
		}
	}
	return "", "", nil
}

func (m *mockRepository) CountIgnoredSharingAlerts(ctx context.Context, tenantID, channel, externalID string) (int, error) {
	ignored := make(map[string]bool)
	for _, log := range m.logs {
		if log.Channel != channel || log.Status != "sent" || log.ExternalID != externalID {
			continue
		}
		if alert := m.alerts[log.AlertID]; alert != nil && alert.TenantID == tenantID && alert.Status == domain.AlertStatusIgnored {
			ignored[log.AlertID] = true
		}
	}
	return len(ignored), nil
}

func (m *mockRepository) CountSharingAlerts(ctx context.Context, tenantID, channel, externalID, excludeAlertID string) (int, error) {
	sharing := make(map[string]bool)
	for _, log := range m.logs {
		if log.Channel != channel || log.Status != "sent" || log.ExternalID != externalID || log.AlertID == excludeAlertID {
			continue
		}
		if alert := m.open(log.AlertID); alert != nil && alert.TenantID == tenantID {
			sharing[log.AlertID] = true
		}
	}
	return len(sharing), nil
}

func (m *mockRepository) GetByAlertAndChannel(ctx context.Context, alertID, channel, status string) (*domain.NotificationLog, error) {
	for _, log := range m.logs {
		if log.AlertID == alertID && log.Channel == channel && log.Status == status {
			return log, nil
		}
	}
	return nil, nil
}

func (m *mockRepository) GetLogsByAlertID(ctx context.Context, alertID string) ([]*domain.NotificationLog, error) {
	var result []*domain.NotificationLog
	for _, log := range m.logs {
		if log.AlertID == alertID {
			result = append(result, log)
		}
	}
	return result, nil
}

// UndeliveredChannels follows the repository: a channel is undelivered while
// the last send logged for the alert on it failed. It returned nothing, so
// retries through the mock never resent anything.
func (m *mockRepository) UndeliveredChannels(_ context.Context, _ string, alertID string) ([]string, error) {
	last := map[string]string{}
	var order []string
	for _, log := range m.logs {
		if log.AlertID != alertID {
			continue
		}
		if _, seen := last[log.Channel]; !seen {
			order = append(order, log.Channel)
		}
		last[log.Channel] = log.Status
	}
	var channels []string
	for _, channel := range order {
		if last[channel] == "failed" {
			channels = append(channels, channel)
		}
	}
	return channels, nil
}

func (m *mockRepository) ListUndelivered(context.Context, []string) ([]notifications.AlertKey, error) {
	return nil, nil
}

func (m *mockRepository) GetFailedByAlertID(ctx context.Context, alertID string) ([]*domain.NotificationLog, error) {
	var result []*domain.NotificationLog
	for _, log := range m.logs {
		if log.AlertID == alertID && log.Status == "failed" {
			result = append(result, log)
		}
	}
	return result, nil
}

func (m *mockRepository) LogNotification(ctx context.Context, log *domain.NotificationLog) error {
	m.logs = append(m.logs, log)
	m.loggedEntries = append(m.loggedEntries, log)
	if log.Status == "sent" {
		key := log.AlertID + ":" + log.Channel
		m.alreadySent[key] = true
	}
	return nil
}

func TestNewService(t *testing.T) {
	repo := newMockRepository()
	svc := notifications.NewService(repo)

	if svc == nil {
		t.Fatal("expected service to be created")
	}
}

func TestNewFullService(t *testing.T) {
	repo := newMockRepository()
	log := logger.NewNoop()
	cfg := &config.NotificationsConfig{
		BaseURL: "https://monitoring.example.com",
		Slack: config.SlackConfig{
			Enabled:    false,
			WebhookURL: "https://hooks.slack.com/test",
		},
		Email: config.EmailConfig{
			Enabled:  false,
			SMTPHost: "smtp.example.com",
			SMTPPort: 587,
		},
		GitLab: config.GitLabConfig{
			Enabled:   false,
			URL:       "https://gitlab.com",
			ProjectID: "123",
		},
	}

	svc := notifications.NewFullService(repo, cfg, log)

	if svc == nil {
		t.Fatal("expected full service to be created")
	}
}

func TestNotifyAlert_DisabledChannels(t *testing.T) {
	repo := newMockRepository()
	log := logger.NewNoop()
	cfg := &config.NotificationsConfig{
		// All channels disabled
		Slack:  config.SlackConfig{Enabled: false},
		Email:  config.EmailConfig{Enabled: false},
		GitLab: config.GitLabConfig{Enabled: false},
	}

	svc := notifications.NewFullService(repo, cfg, log)

	alert := &domain.Alert{
		AlertID:       "test-alert-001",
		TenantID:      "tenant-001",
		Type:          "SECURITY_ISSUE",
		Severity:      domain.AlertSeverityWarning,
		Status:        "active",
		Title:         "Test Alert",
		Description:   "This is a test alert",
		ResourceType:  "realm",
		ResourceID:    "realm-001",
		ResourceName:  "master",
		RealmName:     "master",
		FirstDetected: time.Now(),
		LastSeen:      time.Now(),
	}

	err := svc.NotifyAlert(context.Background(), alert)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// No notifications should be logged since all channels are disabled
	if len(repo.loggedEntries) != 0 {
		t.Errorf("expected 0 logged entries, got %d", len(repo.loggedEntries))
	}
}

func TestCheckAlreadySent(t *testing.T) {
	repo := newMockRepository()
	svc := notifications.NewService(repo)

	// Initially not sent
	sent, err := svc.CheckAlreadySent(context.Background(), "tenant-1", "alert-001", "slack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sent {
		t.Error("expected not sent initially")
	}

	// Log a notification
	err = svc.LogNotification(context.Background(), "alert-001", "slack", "ext-123", "sent", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Now should be marked as sent
	sent, err = svc.CheckAlreadySent(context.Background(), "tenant-1", "alert-001", "slack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sent {
		t.Error("expected sent after logging")
	}
}

func TestGetNotificationLogs(t *testing.T) {
	repo := newMockRepository()
	svc := notifications.NewService(repo)

	// Log some notifications
	_ = svc.LogNotification(context.Background(), "alert-001", "slack", "ext-1", "sent", "", nil)
	_ = svc.LogNotification(context.Background(), "alert-001", "email", "ext-2", "sent", "", nil)
	_ = svc.LogNotification(context.Background(), "alert-002", "slack", "ext-3", "sent", "", nil)

	logs, err := svc.GetNotificationLogs(context.Background(), "alert-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(logs) != 2 {
		t.Errorf("expected 2 logs for alert-001, got %d", len(logs))
	}
}

func TestGetFailedNotifications(t *testing.T) {
	repo := newMockRepository()
	svc := notifications.NewService(repo)

	// Log some notifications
	_ = svc.LogNotification(context.Background(), "alert-001", "slack", "", "failed", "connection error", nil)
	_ = svc.LogNotification(context.Background(), "alert-001", "email", "ext-2", "sent", "", nil)

	failed, err := svc.GetFailedNotifications(context.Background(), "alert-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(failed) != 1 {
		t.Errorf("expected 1 failed notification, got %d", len(failed))
	}

	if failed[0].Channel != "slack" {
		t.Errorf("expected failed channel to be slack, got %s", failed[0].Channel)
	}
}

func TestMeetsMinimumSeverity(t *testing.T) {
	tests := []struct {
		alertSeverity domain.AlertSeverity
		minSeverity   string
		expected      bool
	}{
		{domain.AlertSeverityCritical, "", true},     // No minimum = allow all
		{domain.AlertSeverityCritical, "info", true}, // Critical >= Info
		{domain.AlertSeverityCritical, "warning", true},
		{domain.AlertSeverityCritical, "error", true},
		{domain.AlertSeverityCritical, "critical", true},
		{domain.AlertSeverityError, "critical", false}, // Error < Critical
		{domain.AlertSeverityWarning, "error", false},  // Warning < Error
		{domain.AlertSeverityInfo, "warning", false},   // Info < Warning
		{domain.AlertSeverityInfo, "info", true},       // Info == Info
	}

	for _, tt := range tests {
		result := notifications.MeetsMinimumSeverity(tt.alertSeverity, tt.minSeverity)
		if result != tt.expected {
			t.Errorf("MeetsMinimumSeverity(%s, %s) = %v, expected %v",
				tt.alertSeverity, tt.minSeverity, result, tt.expected)
		}
	}
}

func TestSeverityLevel(t *testing.T) {
	tests := []struct {
		severity domain.AlertSeverity
		expected int
	}{
		{domain.AlertSeverityInfo, 0},
		{domain.AlertSeverityWarning, 1},
		{domain.AlertSeverityError, 2},
		{domain.AlertSeverityCritical, 3},
	}

	for _, tt := range tests {
		result := notifications.SeverityLevel(tt.severity)
		if result != tt.expected {
			t.Errorf("SeverityLevel(%s) = %d, expected %d",
				tt.severity, result, tt.expected)
		}
	}
}
