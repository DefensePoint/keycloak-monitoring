package notifications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// An event alert's description carries the client_id an unauthenticated token
// request named, verbatim from Keycloak.
const hostileClient = "<!channel> <https://evil.example/login|Re-verify> [Reset](https://evil.example/r) ![p](https://evil.example/p.png) @all"

func hostileAlert() *domain.Alert {
	return &domain.Alert{
		AlertID:       "a1",
		TenantID:      "tenant-a",
		Source:        domain.AlertSourceEvent,
		Type:          domain.AlertTypeEvent,
		Severity:      domain.AlertSeverityError,
		Status:        domain.AlertStatusActive,
		Title:         "CLIENT_LOGIN_ERROR in master",
		Description:   "Event type: CLIENT_LOGIN_ERROR, User: , Client: " + hostileClient,
		ResourceType:  "event",
		ResourceID:    "ev-1",
		ResourceName:  "CLIENT_LOGIN_ERROR",
		RealmName:     "master",
		FirstDetected: time.Now(),
	}
}

func TestSlackMessage_EscapesAlertText(t *testing.T) {
	s := NewSlackNotifier(&config.SlackConfig{Enabled: true}, "", logger.NewNoop())
	raw, err := json.Marshal(s.buildSlackMessage(hostileAlert()))
	if err != nil {
		t.Fatal(err)
	}
	// json.Marshal writes < and > as < and >; decode back to what
	// Slack receives as text.
	var decoded slackMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	text := decoded.Attachments[0].Text
	for _, bad := range []string{"<!channel>", "<https://"} {
		if strings.Contains(text, bad) {
			t.Errorf("attachment text still holds %q: %q", bad, text)
		}
	}
	if !strings.Contains(text, "&lt;!channel&gt;") {
		t.Errorf("attachment text does not show the escaped mention: %q", text)
	}
}

func TestSlackResolution_EscapesAlertText(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	alert := hostileAlert()
	alert.Title = "<!here> resolved"
	alert.ResolutionNote = "<https://evil.example|see note>"
	s := NewSlackNotifier(&config.SlackConfig{Enabled: true, WebhookURL: srv.URL}, "", logger.NewNoop())
	if err := s.NotifyResolution(context.Background(), alert); err != nil {
		t.Fatalf("NotifyResolution: %v", err)
	}
	var decoded slackMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	all := decoded.Text + decoded.Attachments[0].Title + decoded.Attachments[0].Text
	for _, bad := range []string{"<!here>", "<https://"} {
		if strings.Contains(all, bad) {
			t.Errorf("resolution message still holds %q: %q", bad, all)
		}
	}
}

// outsideCode returns the text outside code: fenced blocks and inline code
// removed. Anything hostile left there would be rendered by GitLab.
func outsideCode(md string) string {
	var out strings.Builder
	inBlock, fence := false, ""
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if !inBlock && strings.HasPrefix(trimmed, "```") {
			inBlock, fence = true, trimmed[:strings.LastIndex(trimmed, "`")+1]
			continue
		}
		if inBlock {
			if trimmed == fence {
				inBlock = false
			}
			continue
		}
		// drop inline code spans of any fence length
		for {
			i := strings.Index(line, "`")
			if i < 0 {
				break
			}
			n := 0
			for i+n < len(line) && line[i+n] == '`' {
				n++
			}
			f := strings.Repeat("`", n)
			j := strings.Index(line[i+n:], f)
			if j < 0 {
				break
			}
			line = line[:i] + line[i+n+j+n:]
		}
		out.WriteString(line + "\n")
	}
	return out.String()
}

func TestGitLabDescription_KeepsAlertTextInCode(t *testing.T) {
	g := NewGitLabNotifier(&config.GitLabConfig{Enabled: true}, logger.NewNoop())
	desc := g.buildIssueDescription(hostileAlert())

	rendered := outsideCode(desc)
	for _, bad := range []string{"[Reset](", "![p](", "@all", "<!channel>"} {
		if strings.Contains(rendered, bad) {
			t.Errorf("%q is outside code in the issue description:\n%s", bad, desc)
		}
	}
	if !strings.Contains(desc, hostileClient) {
		t.Errorf("the client text was lost rather than shown as code:\n%s", desc)
	}
}

func TestGitLabDescription_BackticksCannotCloseTheFence(t *testing.T) {
	alert := hostileAlert()
	alert.Description = "a ```` b\n````\n[Reset](https://evil.example/r)\n/close"
	alert.ResourceName = "x `` [Reset](https://evil.example/r)"
	g := NewGitLabNotifier(&config.GitLabConfig{Enabled: true}, logger.NewNoop())
	desc := g.buildIssueDescription(alert)

	// The section must be exactly one block whose fence is longer than any
	// backtick run in the text, so the text cannot close it or open its own.
	section := strings.SplitN(strings.SplitN(desc, "## Description\n\n", 2)[1], "\n\n## Resource", 2)[0]
	fence := "`````"
	if want := fence + "text\n" + alert.Description + "\n" + fence; section != want {
		t.Errorf("description section =\n%s\nwant one %d-backtick block around the text", section, len(fence))
	}

	// The resource name is inline code; its own backticks must not end it.
	var line string
	for _, l := range strings.Split(desc, "\n") {
		if strings.HasPrefix(l, "- **Resource Name**:") {
			line = l
		}
	}
	if strings.Contains(outsideCode(line), "[Reset](") {
		t.Errorf("the resource name escaped its code span: %q", line)
	}
}

func TestGitLabCode(t *testing.T) {
	for in, want := range map[string]string{
		"plain":     "` plain `",
		"a`b":       "`` a`b ``",
		"a``b":      "``` a``b ```",
		"two\nline": "` two line `",
		"":          "",
	} {
		if got := gitlabCode(in); got != want {
			t.Errorf("gitlabCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGitLabTitle_IsOneLine(t *testing.T) {
	alert := hostileAlert()
	alert.Title = "a\n/close\r\nb"
	g := NewGitLabNotifier(&config.GitLabConfig{Enabled: true}, logger.NewNoop())
	if title := g.buildIssueTitle(alert); strings.ContainsAny(title, "\r\n") {
		t.Fatalf("issue title holds a line break: %q", title)
	}
}

func TestEmailSubject_IsOneLine(t *testing.T) {
	alert := hostileAlert()
	alert.Title = "x\r\nBcc: someone@evil.example"
	for _, subject := range []string{"", "[{{severity}}] {{title}}"} {
		e := NewEmailNotifier(&config.EmailConfig{Enabled: true, Subject: subject}, logger.NewNoop())
		if got := e.buildSubject(alert); strings.ContainsAny(got, "\r\n") {
			t.Errorf("subject (template %q) holds a line break: %q", subject, got)
		}
	}
}
