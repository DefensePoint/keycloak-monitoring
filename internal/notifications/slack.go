package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httplib "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// SlackNotifier handles sending notifications to Slack via webhooks.
type SlackNotifier struct {
	config     *config.SlackConfig
	baseURL    string
	httpClient *http.Client
	logger     *logger.Logger
}

// NewSlackNotifier creates a new Slack notifier.
func NewSlackNotifier(cfg *config.SlackConfig, baseURL string, log *logger.Logger) *SlackNotifier {
	return &SlackNotifier{
		config:  cfg,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: log.WithComponent("slack_notifier"),
	}
}

// Channel returns the channel type.
func (s *SlackNotifier) Channel() Channel {
	return ChannelSlack
}

// IsEnabled returns whether Slack notifications are enabled.
func (s *SlackNotifier) IsEnabled() bool {
	return s.config.Enabled
}

// slackMessage represents a Slack webhook message structure.
type slackMessage struct {
	Text        string            `json:"text,omitempty"`
	Channel     string            `json:"channel,omitempty"`
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	Attachments []slackAttachment `json:"attachments,omitempty"`
}

// slackAttachment represents a Slack message attachment.
type slackAttachment struct {
	Color      string       `json:"color,omitempty"`
	Title      string       `json:"title,omitempty"`
	TitleLink  string       `json:"title_link,omitempty"`
	Text       string       `json:"text,omitempty"`
	Fields     []slackField `json:"fields,omitempty"`
	Footer     string       `json:"footer,omitempty"`
	FooterIcon string       `json:"footer_icon,omitempty"`
	Timestamp  int64        `json:"ts,omitempty"`
}

// slackField represents a field in a Slack attachment.
type slackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// Notify sends an alert to Slack as a formatted message.
func (s *SlackNotifier) Notify(ctx context.Context, alert *domain.Alert) (string, error) {
	if !s.config.Enabled {
		return "", fmt.Errorf("slack integration is not enabled")
	}

	s.logger.Info("Sending Slack notification",
		logger.Str("alert_id", alert.AlertID),
		logger.Str("severity", alert.Severity.String()))

	// Build Slack message
	msg := s.buildSlackMessage(alert)

	// Marshal to JSON
	bodyBytes, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal message: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", s.config.WebhookURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set(httplib.HeaderContentType, httplib.ContentTypeJSON)

	// Send request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("slack API returned status %d: %s", resp.StatusCode, string(body))
	}

	s.logger.Info("Slack notification sent successfully",
		logger.Str("alert_id", alert.AlertID))

	// Return a message identifier (we don't get one from webhook, so use alert ID + timestamp)
	return fmt.Sprintf("%s-%d", alert.AlertID, time.Now().Unix()), nil
}

// buildSlackMessage builds a formatted Slack message from an alert.
func (s *SlackNotifier) buildSlackMessage(alert *domain.Alert) *slackMessage {
	// Get severity emoji
	severityEmoji := s.getSeverityEmoji(alert.Severity)

	// Build summary text with emoji
	summaryText := fmt.Sprintf("%s *%s Alert*: %s",
		severityEmoji,
		strings.ToUpper(alert.Severity.String()),
		alert.Title)

	// Build title link if base URL is configured
	titleLink := ""
	if s.baseURL != "" {
		titleLink = fmt.Sprintf("%s/%s/alerts/%s",
			strings.TrimSuffix(s.baseURL, "/"),
			alert.TenantID,
			alert.AlertID)
	}

	// Build attachment
	attachment := slackAttachment{
		Color:     s.getSeverityColor(alert.Severity),
		Title:     alert.Title,
		TitleLink: titleLink,
		Text:      alert.Description,
		Timestamp: alert.FirstDetected.Unix(),
		Footer:    "Keycloak Monitoring Tool",
		Fields: []slackField{
			{
				Title: "Severity",
				Value: fmt.Sprintf("%s *%s*", severityEmoji, strings.ToUpper(alert.Severity.String())),
				Short: true,
			},
			{
				Title: "Source",
				Value: string(alert.Source),
				Short: true,
			},
			{
				Title: "Tenant",
				Value: alert.TenantID,
				Short: true,
			},
			{
				Title: "Realm",
				Value: alert.RealmName,
				Short: true,
			},
			{
				Title: "Resource",
				Value: fmt.Sprintf("%s: %s", alert.ResourceType, alert.ResourceName),
				Short: false,
			},
		},
	}

	// Add recommendation if present
	if alert.Recommendation != "" {
		attachment.Fields = append(attachment.Fields, slackField{
			Title: "Recommendation",
			Value: alert.Recommendation,
			Short: false,
		})
	}

	// Add resource ID
	attachment.Fields = append(attachment.Fields, slackField{
		Title: "Resource ID",
		Value: fmt.Sprintf("`%s`", alert.ResourceID),
		Short: true,
	})

	// Add alert ID
	attachment.Fields = append(attachment.Fields, slackField{
		Title: "Alert ID",
		Value: fmt.Sprintf("`%s`", alert.AlertID),
		Short: true,
	})

	// Add quick link button if base URL is configured
	if s.baseURL != "" {
		quickLinkText := fmt.Sprintf("<%s/%s/alerts/%s|View Alert in Keycloak Monitoring Tool>",
			strings.TrimSuffix(s.baseURL, "/"),
			alert.TenantID,
			alert.AlertID)
		attachment.Fields = append(attachment.Fields, slackField{
			Title: "Quick Actions",
			Value: quickLinkText,
			Short: false,
		})
	}

	msg := &slackMessage{
		Text:        summaryText,
		Attachments: []slackAttachment{attachment},
	}

	// Add optional overrides from config
	if s.config.Channel != "" {
		msg.Channel = s.config.Channel
	}
	if s.config.Username != "" {
		msg.Username = s.config.Username
	}
	if s.config.IconEmoji != "" {
		msg.IconEmoji = s.config.IconEmoji
	}

	return msg
}

// getSeverityColor returns the Slack color for the given severity level.
func (s *SlackNotifier) getSeverityColor(severity domain.AlertSeverity) string {
	switch severity {
	case domain.AlertSeverityCritical:
		return "#dc3545" // Red
	case domain.AlertSeverityError:
		return "#fd7e14" // Orange
	case domain.AlertSeverityWarning:
		return "#ffc107" // Yellow
	case domain.AlertSeverityInfo:
		return "#17a2b8" // Blue
	default:
		return "#6c757d" // Gray
	}
}

// getSeverityEmoji returns an emoji for the given severity level.
func (s *SlackNotifier) getSeverityEmoji(severity domain.AlertSeverity) string {
	switch severity {
	case domain.AlertSeverityCritical:
		return "🚨" // Siren for critical
	case domain.AlertSeverityError:
		return "🔴" // Red circle for error
	case domain.AlertSeverityWarning:
		return "🟠" // Orange circle for warning
	case domain.AlertSeverityInfo:
		return "🔵" // Blue circle for info
	default:
		return "❓" // Question mark for unknown
	}
}

// Ensure SlackNotifier implements Notifier
var _ Notifier = (*SlackNotifier)(nil)
