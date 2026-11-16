package notifications

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

const brandedProduct = "Acme Monitoring Platform"

// brandedNotifiers builds the three notifiers the way the service does, for
// a deployment named brandedProduct, all sending to srv.
func brandedNotifiers(t *testing.T, srv *httptest.Server, product string) (*EmailNotifier, *SlackNotifier, *GitLabNotifier) {
	t.Helper()
	svc := NewFullService(nil, &config.NotificationsConfig{
		BaseURL:     "https://monitoring.example.com",
		ProductName: product,
		GitLab:      config.GitLabConfig{Enabled: true, URL: srv.URL, ProjectID: "1"},
		Slack:       config.SlackConfig{Enabled: true, WebhookURL: srv.URL + "/slack"},
		Email:       config.EmailConfig{Enabled: true, Subject: "[{{severity}}] {{product}} Alert: {{title}}"},
	}, logger.NewNoop()).(*service)
	return svc.email.(*EmailNotifier), svc.slack.(*SlackNotifier), svc.gitlab.(*GitLabNotifier)
}

// recordingServer answers like Slack and GitLab and keeps every body sent.
func recordingServer(t *testing.T) (*httptest.Server, func() string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		switch {
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"state":"opened","title":"x"}`))
		case strings.HasSuffix(r.URL.Path, "/notes"):
			w.WriteHeader(http.StatusCreated)
		default:
			_, _ = w.Write([]byte(`ok`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() string {
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(bodies, "\n")
	}
}

func TestBranding_EveryNotificationCarriesTheProductName(t *testing.T) {
	srv, sent := recordingServer(t)
	email, slack, gitlab := brandedNotifiers(t, srv, brandedProduct)
	alert := hostileAlert()
	ctx := context.Background()

	html, err := email.buildHTMLBody(alert)
	if err != nil {
		t.Fatalf("buildHTMLBody: %v", err)
	}
	slackMsg := slack.buildSlackMessage(alert)
	if err := slack.NotifyResolution(ctx, alert); err != nil {
		t.Fatalf("NotifyResolution: %v", err)
	}
	if err := gitlab.ShareIssue(ctx, "7", alert, ""); err != nil {
		t.Fatalf("ShareIssue: %v", err)
	}
	if err := gitlab.ResolveIssue(ctx, "7", alert, 0, 0); err != nil {
		t.Fatalf("ResolveIssue: %v", err)
	}

	texts := map[string]string{
		"email subject":            email.buildSubject(alert),
		"email body":               html,
		"slack footer":             slackMsg.Attachments[0].Footer,
		"slack quick link":         slackMsg.Attachments[0].Fields[len(slackMsg.Attachments[0].Fields)-1].Value,
		"slack resolution, gitlab": sent(),
		"gitlab issue description": gitlab.buildIssueDescription(alert),
	}
	for where, text := range texts {
		if !strings.Contains(text, brandedProduct) {
			t.Errorf("%s does not name %q:\n%s", where, brandedProduct, text)
		}
		if strings.Contains(text, config.DefaultBrandingName) {
			t.Errorf("%s still names %q:\n%s", where, config.DefaultBrandingName, text)
		}
	}
	if got := email.buildSubject(alert); !strings.HasPrefix(got, "[ERROR] "+brandedProduct+" Alert: ") {
		t.Errorf("email subject = %q", got)
	}
	if !strings.Contains(html, "<title>"+brandedProduct+" Alert</title>") {
		t.Errorf("email title does not carry the product name")
	}
}

func TestBranding_NotificationsDefaultToTheDefaultName(t *testing.T) {
	srv, _ := recordingServer(t)
	email, slack, gitlab := brandedNotifiers(t, srv, "")
	alert := hostileAlert()

	email.config.Subject = ""
	if got := email.buildSubject(alert); !strings.HasPrefix(got, "[ERROR] Keycloak Monitoring Tool Alert: ") {
		t.Errorf("default email subject = %q", got)
	}
	if got := slack.buildSlackMessage(alert).Attachments[0].Footer; got != "Keycloak Monitoring Tool" {
		t.Errorf("slack footer = %q", got)
	}
	if got := gitlab.buildIssueDescription(alert); !strings.Contains(got, "automatically created by Keycloak Monitoring Tool") {
		t.Errorf("gitlab description does not end with the default name:\n%s", got)
	}
}

func TestBranding_ProductNameCannotInjectSlackLinkMarkup(t *testing.T) {
	srv, _ := recordingServer(t)
	_, slack, _ := brandedNotifiers(t, srv, "Acme <!channel> <https://evil.example|x>")
	fields := slack.buildSlackMessage(hostileAlert()).Attachments[0].Fields
	link := fields[len(fields)-1].Value
	if strings.Contains(link, "<!channel>") || strings.Contains(link, "<https://evil.example") {
		t.Errorf("quick link carries raw Slack markup from the product name: %s", link)
	}
}
