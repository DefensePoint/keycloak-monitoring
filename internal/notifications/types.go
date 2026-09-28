package notifications

import (
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Channel represents a notification channel type.
type Channel string

// Notification channels.
const (
	ChannelSlack  Channel = "slack"
	ChannelGitLab Channel = "gitlab"
	ChannelEmail  Channel = "email"
)

// String returns the string representation of the channel.
func (c Channel) String() string {
	return string(c)
}

// Status represents a notification status.
type Status string

// Notification statuses.
const (
	StatusSent    Status = "sent"
	StatusFailed  Status = "failed"
	StatusPending Status = "pending"
)

// String returns the string representation of the status.
func (s Status) String() string {
	return string(s)
}

// SeverityLevel returns the numeric level for severity comparison.
func SeverityLevel(s domain.AlertSeverity) int {
	switch s {
	case domain.AlertSeverityInfo:
		return 0
	case domain.AlertSeverityWarning:
		return 1
	case domain.AlertSeverityError:
		return 2
	case domain.AlertSeverityCritical:
		return 3
	default:
		return 0
	}
}
