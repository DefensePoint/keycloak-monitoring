package notifications

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// An incoming webhook URL is the credential, and its secret parts are the path.
const (
	webhookPath   = "/services/T000/B000/fakeSECRETtoken"
	webhookSecret = "fakeSECRETtoken"
)

func downAlert() *domain.Alert {
	return &domain.Alert{
		AlertID:       "tenant-health-down-t1",
		TenantID:      "t1",
		Source:        domain.AlertSourceMetric,
		Type:          domain.AlertTypePerformance,
		Severity:      domain.AlertSeverityCritical,
		Status:        domain.AlertStatusActive,
		Title:         "Tenant 't1' is DOWN",
		ResourceType:  "tenant",
		ResourceName:  "t1",
		FirstDetected: time.Now(),
	}
}

func TestSlackNotify_NetworkFailureKeepsTheWebhookURLOutOfTheError(t *testing.T) {
	refused := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	refusedURL := refused.URL
	refused.Close()

	// The handler never reads the request body, so the server does not notice
	// the client hanging up; the test releases it instead.
	release := make(chan struct{})
	silent := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	defer func() { close(release); silent.Close() }()

	cases := []struct {
		name    string
		url     string
		timeout time.Duration
		target  string
	}{
		{"connection refused", refusedURL + webhookPath, time.Second, refusedURL},
		{"timeout", silent.URL + webhookPath, 50 * time.Millisecond, silent.URL},
		{"dns failure", "http://t48-no-such-host.invalid" + webhookPath, time.Second, "http://t48-no-such-host.invalid"},
		{"invalid url", "http://[::1" + webhookPath, time.Second, "the Slack webhook URL"},
		{"no scheme", "hooks.slack.invalid" + webhookPath, time.Second, "the Slack webhook URL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSlackNotifier(&config.SlackConfig{Enabled: true, WebhookURL: tc.url}, "", logger.NewNoop())
			s.httpClient.Timeout = tc.timeout

			_, err := s.Notify(context.Background(), downAlert())
			if err == nil {
				t.Fatal("expected the send to fail")
			}
			msg := err.Error()
			for _, secret := range []string{webhookSecret, webhookPath, "T000", "B000"} {
				if strings.Contains(msg, secret) {
					t.Errorf("error holds %q: %s", secret, msg)
				}
			}
			prefix := tc.target + ": "
			i := strings.Index(msg, prefix)
			if i < 0 || strings.TrimSpace(msg[i+len(prefix):]) == "" {
				t.Errorf("error does not name %q and a cause: %s", tc.target, msg)
			}
		})
	}
}

func TestSlackNotify_ResolutionFailureKeepsTheWebhookURLOutOfTheError(t *testing.T) {
	refused := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	refusedURL := refused.URL
	refused.Close()

	s := NewSlackNotifier(&config.SlackConfig{Enabled: true, WebhookURL: refusedURL + webhookPath}, "", logger.NewNoop())
	err := s.NotifyResolution(context.Background(), downAlert())
	if err == nil {
		t.Fatal("expected the resolution send to fail")
	}
	if strings.Contains(err.Error(), webhookSecret) {
		t.Errorf("error holds the webhook secret: %s", err)
	}
}

func TestSlackNotify_SuccessAndSlackErrorResponsesAreUnchanged(t *testing.T) {
	var status int
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := NewSlackNotifier(&config.SlackConfig{Enabled: true, WebhookURL: srv.URL + webhookPath}, "", logger.NewNoop())

	status, body = http.StatusOK, "ok"
	if _, err := s.Notify(context.Background(), downAlert()); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	status, body = http.StatusBadRequest, "invalid_payload"
	_, err := s.Notify(context.Background(), downAlert())
	if err == nil {
		t.Fatal("expected the send to fail")
	}
	if want := "slack API returned status 400: invalid_payload"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}
