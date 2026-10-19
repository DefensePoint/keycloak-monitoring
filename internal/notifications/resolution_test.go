package notifications_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
)

// Resolution notices used to be stubs: they logged "processed" and "Updated
// GitLab issue" and sent nothing. These tests run the real Slack and GitLab
// notifiers against a local server and check what actually goes out.

type recordedRequest struct {
	Method string
	Path   string
	Token  string
	Body   map[string]any
}

type recorder struct {
	mu       sync.Mutex
	requests []recordedRequest
	// closed holds the IIDs of issues the fake GitLab reports as closed.
	closed map[int]bool
	// titles holds each issue's current title, set on creation and by edits.
	titles map[int]string
	// refuseUpdates makes the fake GitLab answer 403 to every issue edit
	// (PUT), as for a token allowed to comment but not to edit or reopen.
	refuseUpdates bool
}

func (r *recorder) server(t *testing.T, status func(method, path string) int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		r.mu.Lock()
		r.requests = append(r.requests, recordedRequest{
			Method: req.Method, Path: req.URL.EscapedPath(), Token: req.Header.Get("PRIVATE-TOKEN"), Body: body,
		})
		r.mu.Unlock()
		w.WriteHeader(status(req.Method, req.URL.Path))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (r *recorder) all() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequest(nil), r.requests...)
}

func resolvedAlert() *domain.Alert {
	return &domain.Alert{
		TenantID: "tenant-1", AlertID: "tenant-health-down-tenant-1", Title: "Tenant 'E2E' is DOWN",
		Severity: domain.AlertSeverityCritical, Status: domain.AlertStatusResolved, ResourceName: "E2E",
	}
}

func TestNotifyAlertResolution_PostsToSlackForAnAnnouncedOccurrence(t *testing.T) {
	var rec recorder
	srv := rec.server(t, func(string, string) int { return http.StatusOK })
	repo := newMockRepository()
	repo.alreadySent["tenant-health-down-tenant-1:slack"] = true
	svc := notifications.NewFullService(repo, &config.NotificationsConfig{
		Slack: config.SlackConfig{Enabled: true, WebhookURL: srv.URL + "/hook", MinSeverity: "info"},
	}, logger.NewNoop())

	if err := svc.NotifyAlertResolution(context.Background(), resolvedAlert()); err != nil {
		t.Fatalf("NotifyAlertResolution: %v", err)
	}

	got := rec.all()
	if len(got) != 1 {
		t.Fatalf("webhook calls = %d, want 1", len(got))
	}
	if text, _ := got[0].Body["text"].(string); !strings.Contains(text, "Resolved") || !strings.Contains(text, "is DOWN") {
		t.Errorf("message text = %q, want a resolution naming the alert", text)
	}
}

func TestNotifyAlertResolution_StaysQuietForAnOccurrenceNeverAnnounced(t *testing.T) {
	var rec recorder
	srv := rec.server(t, func(string, string) int { return http.StatusOK })
	repo := newMockRepository() // nothing sent during this occurrence
	svc := notifications.NewFullService(repo, &config.NotificationsConfig{
		Slack:  config.SlackConfig{Enabled: true, WebhookURL: srv.URL + "/hook", MinSeverity: "info"},
		GitLab: config.GitLabConfig{Enabled: true, URL: srv.URL, Token: "t", ProjectID: "g/p", MinSeverity: "info"},
	}, logger.NewNoop())

	if err := svc.NotifyAlertResolution(context.Background(), resolvedAlert()); err != nil {
		t.Fatalf("NotifyAlertResolution: %v", err)
	}
	if got := rec.all(); len(got) != 0 {
		t.Errorf("sent %d requests for an occurrence nobody was told about: %+v", len(got), got)
	}
}

func TestNotifyAlertResolution_CommentsOnAndClosesTheGitLabIssue(t *testing.T) {
	var rec recorder
	srv := rec.server(t, func(method, path string) int {
		if method == http.MethodPost && strings.HasSuffix(path, "/notes") {
			return http.StatusCreated
		}
		return http.StatusOK
	})
	repo := newMockRepository()
	repo.alreadySent["tenant-health-down-tenant-1:gitlab"] = true
	repo.logs = append(repo.logs, &domain.NotificationLog{
		AlertID: "tenant-health-down-tenant-1", Channel: "gitlab", Status: "sent", ExternalID: "42",
	})
	svc := notifications.NewFullService(repo, &config.NotificationsConfig{
		GitLab: config.GitLabConfig{Enabled: true, URL: srv.URL + "/", Token: "secret-token", ProjectID: "group/project", MinSeverity: "info"},
	}, logger.NewNoop())

	if err := svc.NotifyAlertResolution(context.Background(), resolvedAlert()); err != nil {
		t.Fatalf("NotifyAlertResolution: %v", err)
	}

	got := rec.all()
	if len(got) != 2 {
		t.Fatalf("GitLab calls = %d, want a comment and a close: %+v", len(got), got)
	}
	const issue = "/api/v4/projects/group%2Fproject/issues/42"
	if got[0].Method != http.MethodPost || got[0].Path != issue+"/notes" {
		t.Errorf("first call = %s %s, want POST %s/notes", got[0].Method, got[0].Path, issue)
	}
	if body, _ := got[0].Body["body"].(string); !strings.Contains(body, "Resolved") {
		t.Errorf("comment = %q, want it to say the alert resolved", body)
	}
	if got[1].Method != http.MethodPut || got[1].Path != issue || got[1].Body["state_event"] != "close" {
		t.Errorf("second call = %s %s %v, want PUT %s state_event=close", got[1].Method, got[1].Path, got[1].Body, issue)
	}
	for _, r := range got {
		if r.Token != "secret-token" {
			t.Errorf("%s %s sent token %q", r.Method, r.Path, r.Token)
		}
	}
}

// A resolution that is not a recovery (a tenant disabled while its alert was
// open) carries a note, and both channels must show it rather than read as if
// the tenant recovered.
func TestNotifyAlertResolution_ShowsTheResolutionNote(t *testing.T) {
	var rec recorder
	srv := rec.server(t, func(method, path string) int {
		if method == http.MethodPost && strings.HasSuffix(path, "/notes") {
			return http.StatusCreated
		}
		return http.StatusOK
	})
	repo := newMockRepository()
	repo.alreadySent["tenant-health-down-tenant-1:slack"] = true
	repo.alreadySent["tenant-health-down-tenant-1:gitlab"] = true
	repo.logs = append(repo.logs, &domain.NotificationLog{
		AlertID: "tenant-health-down-tenant-1", Channel: "gitlab", Status: "sent", ExternalID: "42",
	})
	svc := notifications.NewFullService(repo, &config.NotificationsConfig{
		Slack:  config.SlackConfig{Enabled: true, WebhookURL: srv.URL + "/hook", MinSeverity: "info"},
		GitLab: config.GitLabConfig{Enabled: true, URL: srv.URL, Token: "t", ProjectID: "g/p", MinSeverity: "info"},
	}, logger.NewNoop())

	alert := resolvedAlert()
	alert.ResolutionNote = "Tenant disabled: it is no longer monitored, so this alert was closed without a recovery."
	if err := svc.NotifyAlertResolution(context.Background(), alert); err != nil {
		t.Fatalf("NotifyAlertResolution: %v", err)
	}

	var slackText, gitlabBody string
	for _, r := range rec.all() {
		if r.Path == "/hook" {
			if atts, _ := r.Body["attachments"].([]any); len(atts) > 0 {
				slackText, _ = atts[0].(map[string]any)["text"].(string)
			}
		}
		if strings.HasSuffix(r.Path, "/notes") {
			gitlabBody, _ = r.Body["body"].(string)
		}
	}
	if !strings.Contains(slackText, "without a recovery") {
		t.Errorf("Slack resolution text = %q, want the resolution note", slackText)
	}
	if !strings.Contains(gitlabBody, "without a recovery") {
		t.Errorf("GitLab resolution comment = %q, want the resolution note", gitlabBody)
	}
}
