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
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		logs:          make([]*domain.NotificationLog, 0),
		alreadySent:   make(map[string]bool),
		loggedEntries: make([]*domain.NotificationLog, 0),
	}
}

func (m *mockRepository) CheckAlreadySent(ctx context.Context, alertID, channel string) (bool, error) {
	key := alertID + ":" + channel
	return m.alreadySent[key], nil
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
	sent, err := svc.CheckAlreadySent(context.Background(), "alert-001", "slack")
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
	sent, err = svc.CheckAlreadySent(context.Background(), "alert-001", "slack")
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
