package notifications

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// emailSendTimeout bounds one email from connecting to the end of the SMTP
// conversation, as the GitLab client's 30s bounds one API call.
//
// Without it a mail server that accepted the connection and then stopped
// answering, or a firewall dropping its packets, held the send forever. The
// checkers and the Keycloak monitor notify inline, so that stopped their
// alerting and event collection until a restart, and shutdown waited on it.
const emailSendTimeout = 30 * time.Second

// EmailNotifier handles sending notifications via email.
type EmailNotifier struct {
	config *config.EmailConfig
	logger *logger.Logger
	// timeout is emailSendTimeout, shortened by tests.
	timeout time.Duration
}

// NewEmailNotifier creates a new email notifier.
func NewEmailNotifier(cfg *config.EmailConfig, log *logger.Logger) *EmailNotifier {
	return &EmailNotifier{
		config:  cfg,
		logger:  log.WithComponent("email_notifier"),
		timeout: emailSendTimeout,
	}
}

// Channel returns the channel type.
func (e *EmailNotifier) Channel() Channel {
	return ChannelEmail
}

// IsEnabled returns whether email notifications are enabled.
func (e *EmailNotifier) IsEnabled() bool {
	return e.config.Enabled
}

// Notify sends an alert via email.
func (e *EmailNotifier) Notify(ctx context.Context, alert *domain.Alert) (string, error) {
	if !e.config.Enabled {
		return "", fmt.Errorf("email integration is not enabled")
	}

	if len(e.config.To) == 0 {
		return "", fmt.Errorf("no recipient email addresses configured")
	}

	e.logger.Info("Sending email notification",
		logger.Str("alert_id", alert.AlertID),
		logger.Int("recipients", len(e.config.To)))

	// Build email subject
	subject := e.buildSubject(alert)

	// Build email body
	body, err := e.buildHTMLBody(alert)
	if err != nil {
		return "", fmt.Errorf("failed to build email body: %w", err)
	}

	// Send email
	if err := e.sendEmail(ctx, subject, body); err != nil {
		return "", fmt.Errorf("failed to send email: %w", err)
	}

	e.logger.Info("Email notification sent successfully",
		logger.Str("alert_id", alert.AlertID))

	// Return a message identifier (alert ID + timestamp)
	return fmt.Sprintf("%s-%d", alert.AlertID, time.Now().Unix()), nil
}

// buildSubject builds the email subject line.
func (e *EmailNotifier) buildSubject(alert *domain.Alert) string {
	// Use configured subject template if available
	if e.config.Subject != "" {
		// Simple template replacement
		subject := strings.ReplaceAll(e.config.Subject, "{{severity}}", strings.ToUpper(alert.Severity.String()))
		subject = strings.ReplaceAll(subject, "{{title}}", alert.Title)
		subject = strings.ReplaceAll(subject, "{{resource}}", alert.ResourceName)
		// A line break in alert data would end the Subject header and start
		// one of the sender's choosing.
		return oneLine(subject)
	}

	// Default subject format
	return oneLine(fmt.Sprintf("[%s] Keycloak Monitoring Tool Alert: %s - %s",
		strings.ToUpper(alert.Severity.String()),
		alert.Title,
		alert.ResourceName))
}

// buildHTMLBody builds an HTML email body.
func (e *EmailNotifier) buildHTMLBody(alert *domain.Alert) (string, error) {
	tmpl := template.Must(template.New("email").Parse(emailTemplate))

	data := map[string]interface{}{
		"Alert":         alert,
		"SeverityColor": e.getSeverityColor(alert.Severity),
		"SeverityLabel": strings.ToUpper(alert.Severity.String()),
		"TypeLabel":     alert.Type,
		"StatusLabel":   alert.Status,
		"Year":          time.Now().Year(),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// sendEmail sends an email using SMTP, within e.timeout and ctx.
func (e *EmailNotifier) sendEmail(ctx context.Context, subject, htmlBody string) error {
	// Build message headers
	headers := make(map[string]string)
	headers["From"] = e.config.From
	headers["To"] = strings.Join(e.config.To, ", ")
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	// Build message
	var message bytes.Buffer
	for k, v := range headers {
		fmt.Fprintf(&message, "%s: %s\r\n", k, v)
	}
	message.WriteString("\r\n")
	message.WriteString(htmlBody)

	addr := net.JoinHostPort(e.config.SMTPHost, fmt.Sprint(e.config.SMTPPort))

	// Setup authentication
	var auth smtp.Auth
	if e.config.Username != "" && e.config.Password != "" {
		auth = smtp.PlainAuth("", e.config.Username, e.config.Password, e.config.SMTPHost)
	}

	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	conn, err := e.dial(ctx, addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	// One deadline for the whole conversation, so a server that stops
	// answering at any step fails the send instead of holding it. Closing on
	// cancellation covers a context cancelled before the deadline.
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("failed to set SMTP deadline: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	if err := e.converse(conn, auth, message.Bytes()); err != nil {
		switch {
		// The connection's deadline is the context's, and it can fire first:
		// a timed-out read or write is this send running out of time too.
		case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, os.ErrDeadlineExceeded):
			return fmt.Errorf("SMTP server %s did not finish within %s: %w", addr, e.timeout, err)
		case ctx.Err() != nil:
			return fmt.Errorf("sending to SMTP server %s was cancelled: %w", addr, err)
		}
		return err
	}
	return nil
}

// dial connects to the SMTP server, over TLS from the start when use_tls is
// set.
func (e *EmailNotifier) dial(ctx context.Context, addr string) (net.Conn, error) {
	if e.config.UseTLS {
		d := tls.Dialer{Config: &tls.Config{
			ServerName:         e.config.SMTPHost,
			InsecureSkipVerify: e.config.SkipVerify,
		}}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("failed to connect with TLS: %w", err)
		}
		return conn, nil
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	return conn, nil
}

// converse runs the SMTP conversation on conn. Without use_tls it upgrades
// with STARTTLS when the server offers it, as smtp.SendMail did, and PlainAuth
// still refuses to send credentials over a connection that is not encrypted.
func (e *EmailNotifier) converse(conn net.Conn, auth smtp.Auth, message []byte) error {
	client, err := smtp.NewClient(conn, e.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if !e.config.UseTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: e.config.SMTPHost}); err != nil {
				return fmt.Errorf("STARTTLS failed: %w", err)
			}
		}
	}

	// Authenticate if needed
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	// Set sender
	if err := client.Mail(e.config.From); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, to := range e.config.To {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", to, err)
		}
	}

	// Send message
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}
	if _, err := w.Write(message); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	// Close ends the message and returns the server's verdict on it. Its
	// error used to be discarded, so a message the server refused was
	// reported as sent.
	if err := w.Close(); err != nil {
		return fmt.Errorf("SMTP server rejected the message: %w", err)
	}
	// The message is accepted at this point; a failed QUIT does not undo it.
	_ = client.Quit()
	return nil
}

// getSeverityColor returns the color for the given severity level.
func (e *EmailNotifier) getSeverityColor(severity domain.AlertSeverity) string {
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

// emailTemplate is the HTML email template.
const emailTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Keycloak Monitoring Tool Alert</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .container {
            background-color: #ffffff;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            overflow: hidden;
        }
        .header {
            background-color: {{.SeverityColor}};
            color: white;
            padding: 20px;
            text-align: center;
        }
        .header h1 {
            margin: 0;
            font-size: 24px;
        }
        .content {
            padding: 30px;
        }
        .alert-info {
            background-color: #f8f9fa;
            border-left: 4px solid {{.SeverityColor}};
            padding: 15px;
            margin: 20px 0;
        }
        .field {
            margin: 15px 0;
        }
        .field-label {
            font-weight: bold;
            color: #666;
            display: inline-block;
            min-width: 140px;
        }
        .field-value {
            display: inline-block;
        }
        .code {
            background-color: #f4f4f4;
            padding: 2px 6px;
            border-radius: 3px;
            font-family: 'Courier New', monospace;
            font-size: 14px;
        }
        .recommendation {
            background-color: #e7f3ff;
            border-left: 4px solid #0066cc;
            padding: 15px;
            margin: 20px 0;
        }
        .footer {
            background-color: #f8f9fa;
            padding: 20px;
            text-align: center;
            font-size: 12px;
            color: #666;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>{{.SeverityLabel}} ALERT</h1>
        </div>
        <div class="content">
            <h2>{{.Alert.Title}}</h2>

            <div class="alert-info">
                <div class="field">
                    <span class="field-label">Alert ID:</span>
                    <span class="field-value code">{{.Alert.AlertID}}</span>
                </div>
                <div class="field">
                    <span class="field-label">Severity:</span>
                    <span class="field-value"><strong>{{.SeverityLabel}}</strong></span>
                </div>
                <div class="field">
                    <span class="field-label">Type:</span>
                    <span class="field-value">{{.TypeLabel}}</span>
                </div>
                <div class="field">
                    <span class="field-label">Status:</span>
                    <span class="field-value">{{.StatusLabel}}</span>
                </div>
            </div>

            <h3>Description</h3>
            <p>{{.Alert.Description}}</p>

            <h3>Resource Information</h3>
            <div class="field">
                <span class="field-label">Resource Type:</span>
                <span class="field-value">{{.Alert.ResourceType}}</span>
            </div>
            <div class="field">
                <span class="field-label">Resource Name:</span>
                <span class="field-value">{{.Alert.ResourceName}}</span>
            </div>
            <div class="field">
                <span class="field-label">Resource ID:</span>
                <span class="field-value code">{{.Alert.ResourceID}}</span>
            </div>
            <div class="field">
                <span class="field-label">Realm:</span>
                <span class="field-value">{{.Alert.RealmName}}</span>
            </div>

            {{if .Alert.Recommendation}}
            <div class="recommendation">
                <h3 style="margin-top: 0;">Recommendation</h3>
                <p>{{.Alert.Recommendation}}</p>
            </div>
            {{end}}

            <h3>Timeline</h3>
            <div class="field">
                <span class="field-label">First Detected:</span>
                <span class="field-value">{{.Alert.FirstDetected}}</span>
            </div>
            <div class="field">
                <span class="field-label">Last Seen:</span>
                <span class="field-value">{{.Alert.LastSeen}}</span>
            </div>
        </div>
        <div class="footer">
            <p>This is an automated alert from Keycloak Monitoring Tool</p>
            <p>&copy; {{.Year}} Keycloak Monitoring Tool. All rights reserved.</p>
        </div>
    </div>
</body>
</html>`

// Ensure EmailNotifier implements Notifier
var _ Notifier = (*EmailNotifier)(nil)
