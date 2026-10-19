package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httplib "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// GitLabNotifier handles sending notifications to GitLab as issues.
type GitLabNotifier struct {
	config     *config.GitLabConfig
	httpClient *http.Client
	logger     *logger.Logger
}

// NewGitLabNotifier creates a new GitLab notifier.
func NewGitLabNotifier(cfg *config.GitLabConfig, log *logger.Logger) *GitLabNotifier {
	notifier := &GitLabNotifier{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: log.WithComponent("gitlab_notifier"),
	}

	// Log project identifier type for debugging
	projectType := "numeric ID"
	if strings.Contains(cfg.ProjectID, "/") {
		projectType = "project path"
	}
	log.Info("Initialized GitLab notifier",
		logger.Str("url", cfg.URL),
		logger.Str("project_identifier", cfg.ProjectID),
		logger.Str("project_type", projectType),
		logger.Str("milestone", cfg.Milestone))

	return notifier
}

// Channel returns the channel type.
func (g *GitLabNotifier) Channel() Channel {
	return ChannelGitLab
}

// IsEnabled returns whether GitLab notifications are enabled.
func (g *GitLabNotifier) IsEnabled() bool {
	return g.config.Enabled
}

// gitLabIssue represents a GitLab issue structure for API calls.
type gitLabIssue struct {
	ID     int    `json:"id"`
	IID    int    `json:"iid"`
	WebURL string `json:"web_url"`
}

// gitLabIssueRequest represents a GitLab issue creation request.
type gitLabIssueRequest struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Labels      string `json:"labels,omitempty"`       // Comma-separated string
	MilestoneID int    `json:"milestone_id,omitempty"` // Milestone ID (numeric)
}

// Notify sends an alert to GitLab as an issue. The caller decides whether the
// alert joins an issue an open alert on the same resource already holds; see
// ShareIssue.
func (g *GitLabNotifier) Notify(ctx context.Context, alert *domain.Alert) (string, error) {
	if !g.config.Enabled {
		return "", fmt.Errorf("GitLab integration is not enabled")
	}

	g.logger.Info("Processing GitLab notification",
		logger.Str("alert_id", alert.AlertID),
		logger.Str("resource_type", alert.ResourceType))

	// Create the issue
	issueID, err := g.createIssue(ctx, alert)
	if err != nil {
		return "", fmt.Errorf("failed to create GitLab issue: %w", err)
	}

	g.logger.Info("Created GitLab issue",
		logger.Str("alert_id", alert.AlertID),
		logger.Str("issue_id", issueID))

	return issueID, nil
}

// ShareIssue records on an existing issue that another alert on the same
// resource is now open, so the issue covers them both, and reopens the issue
// if it was closed. replaces names the alert this one supersedes, if it took
// over that alert's issue; an issue still named after that alert is renamed
// after this one.
//
// The issue is edited before the comment is posted, in one request, so the
// comment never claims a reopen GitLab refused, and a refused edit posts
// nothing for the retry of the failed send to repeat.
func (g *GitLabNotifier) ShareIssue(ctx context.Context, issueIID string, alert *domain.Alert, replaces string) error {
	if !g.config.Enabled {
		return fmt.Errorf("GitLab integration is not enabled")
	}
	issueURL := g.issueURL(issueIID)

	issue, err := g.readIssue(ctx, issueURL)
	if err != nil {
		return fmt.Errorf("failed to read GitLab issue %s: %w", issueIID, err)
	}

	edit := map[string]string{}
	if issue.State == "closed" {
		edit["state_event"] = "reopen"
	}
	if replaces != "" && issueNamedAfter(issue.Title, replaces, alert) {
		edit["title"] = g.buildIssueTitle(alert)
	}
	if len(edit) > 0 {
		if err := g.sendIssueRequest(ctx, http.MethodPut, issueURL, edit, http.StatusOK); err != nil {
			return fmt.Errorf("failed to update GitLab issue %s: %w", issueIID, err)
		}
	}

	heading := "Also open: " + alert.Title
	if replaces != "" {
		heading = fmt.Sprintf("Replaces: %s\n\nNow open: %s", replaces, alert.Title)
	}
	reopened := ""
	if edit["state_event"] != "" {
		reopened = " It reopened this issue, which was closed while an alert on this resource is open."
	}
	note := map[string]string{"body": fmt.Sprintf(
		"%s\n\n%s\n\nKeycloak Monitoring Tool added this alert at %s.%s The issue stays open until the last alert on this resource resolves.",
		heading, gitlabCodeBlock(alert.Description), time.Now().UTC().Format(time.RFC3339), reopened)}
	if err := g.sendIssueRequest(ctx, http.MethodPost, issueURL+"/notes", note, http.StatusCreated); err != nil {
		return fmt.Errorf("failed to comment on GitLab issue %s: %w", issueIID, err)
	}
	return nil
}

// issueNamedAfter reports whether title is the one buildIssueTitle gave the
// replaced alert, whatever its severity: the replacement is on the same
// resource and realm. Any other title, from another alert or from a person
// in GitLab, is not the replaced alert's to hand on.
func issueNamedAfter(title, replaced string, alert *domain.Alert) bool {
	suffix := oneLine(fmt.Sprintf("] %s - %s (%s)", replaced, alert.ResourceName, alert.RealmName))
	return strings.HasPrefix(title, "[") && strings.HasSuffix(title, suffix) &&
		!strings.Contains(strings.TrimSuffix(title, suffix), "]")
}

// gitLabIssueState is the part of an issue ShareIssue reads.
type gitLabIssueState struct {
	State string `json:"state"`
	Title string `json:"title"`
}

// readIssue returns the state and title of the issue at issueURL.
func (g *GitLabNotifier) readIssue(ctx context.Context, issueURL string) (gitLabIssueState, error) {
	var issue gitLabIssueState
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issueURL, nil)
	if err != nil {
		return issue, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", g.config.Token)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return issue, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return issue, fmt.Errorf("GitLab API returned status %d: %s", resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return issue, fmt.Errorf("failed to parse issue: %w", err)
	}
	return issue, nil
}

// ResolveIssue comments on the alert's issue that it has been resolved, and
// closes it when no other open alert holds it, othersOpen being how many do.
// ignored is how many ignored alerts still hold it when it closes. A
// recurrence of the alert opens a new issue, or joins the one its resource has.
func (g *GitLabNotifier) ResolveIssue(ctx context.Context, issueIID string, alert *domain.Alert, othersOpen, ignored int) error {
	if !g.config.Enabled {
		return fmt.Errorf("GitLab integration is not enabled")
	}

	resolvedAt := time.Now()
	if alert.ResolvedAt != nil {
		resolvedAt = *alert.ResolvedAt
	}
	issueURL := g.issueURL(issueIID)

	reason := "Keycloak Monitoring Tool saw this alert resolved"
	if alert.ResolutionNote != "" {
		reason = alert.ResolutionNote + " Keycloak Monitoring Tool resolved this alert"
	}
	outcome := "closed the issue"
	if ignored > 0 {
		outcome = "closed the issue. Ignored, not resolved, on this resource: " + alertCount(ignored)
	}
	if othersOpen > 0 {
		outcome = fmt.Sprintf("left the issue open for %s still unresolved on this resource", alertCount(othersOpen))
	}
	note := map[string]string{"body": fmt.Sprintf(
		"Resolved: %s\n\n%s at %s and %s.",
		alert.Title, reason, resolvedAt.UTC().Format(time.RFC3339), outcome)}
	if err := g.sendIssueRequest(ctx, http.MethodPost, issueURL+"/notes", note, http.StatusCreated); err != nil {
		return fmt.Errorf("failed to comment on GitLab issue %s: %w", issueIID, err)
	}
	if othersOpen > 0 {
		return nil
	}
	if err := g.sendIssueRequest(ctx, http.MethodPut, issueURL, map[string]string{"state_event": "close"}, http.StatusOK); err != nil {
		return fmt.Errorf("failed to close GitLab issue %s: %w", issueIID, err)
	}
	return nil
}

// issueURL is the API address of one issue of the configured project.
func (g *GitLabNotifier) issueURL(issueIID string) string {
	return fmt.Sprintf("%s/api/v4/projects/%s/issues/%s",
		strings.TrimSuffix(g.config.URL, "/"), url.PathEscape(g.config.ProjectID), url.PathEscape(issueIID))
}

func alertCount(n int) string {
	if n == 1 {
		return "1 alert"
	}
	return fmt.Sprintf("%d alerts", n)
}

// sendIssueRequest sends one JSON request to the GitLab API and checks the status.
func (g *GitLabNotifier) sendIssueRequest(ctx context.Context, method, apiURL string, payload any, wantStatus int) error {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", g.config.Token)
	req.Header.Set(httplib.HeaderContentType, httplib.ContentTypeJSON)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != wantStatus {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitLab API returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// createIssue creates a new GitLab issue.
func (g *GitLabNotifier) createIssue(ctx context.Context, alert *domain.Alert) (string, error) {
	// Build issue title
	title := g.buildIssueTitle(alert)

	// Build issue description
	description := g.buildIssueDescription(alert)

	// Build API URL with encoded project identifier
	encodedProjectID := url.PathEscape(g.config.ProjectID)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/issues",
		strings.TrimSuffix(g.config.URL, "/"),
		encodedProjectID)

	g.logger.Info("Creating GitLab issue",
		logger.Str("project_id", g.config.ProjectID),
		logger.Str("encoded_project_id", encodedProjectID),
		logger.Str("title", title))

	// Create request body
	issueReq := gitLabIssueRequest{
		Title:       title,
		Description: description,
	}

	if len(g.config.Labels) > 0 {
		issueReq.Labels = strings.Join(g.config.Labels, ",")
	}

	// Add milestone if configured
	if g.config.Milestone != "" {
		g.logger.Debug("Attempting to set milestone",
			logger.Str("milestone_config", g.config.Milestone))

		milestoneID, err := g.getMilestoneID(ctx, g.config.Milestone)
		if err != nil {
			g.logger.Error("Failed to get milestone ID, creating issue without milestone",
				logger.Str("milestone", g.config.Milestone),
				logger.Err(err))
		} else if milestoneID > 0 {
			issueReq.MilestoneID = milestoneID
			g.logger.Debug("Setting milestone ID for issue",
				logger.Int("milestone_id", milestoneID))
		} else {
			g.logger.Warn("Milestone ID is 0, not setting milestone")
		}
	}

	bodyBytes, err := json.Marshal(issueReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	g.logger.Debug("GitLab issue creation request",
		logger.Str("api_url", apiURL),
		logger.Str("request_body", string(bodyBytes)))

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication header
	req.Header.Set("PRIVATE-TOKEN", g.config.Token)
	req.Header.Set(httplib.HeaderContentType, httplib.ContentTypeJSON)

	// Send request
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		g.logger.Error("GitLab issue creation failed",
			logger.Int("status_code", resp.StatusCode),
			logger.Str("response_body", string(body)))
		return "", fmt.Errorf("GitLab API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var issue gitLabIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	g.logger.Info("GitLab issue created successfully",
		logger.Int("issue_id", issue.ID),
		logger.Int("issue_iid", issue.IID),
		logger.Str("issue_url", issue.WebURL))

	// Return issue IID (internal ID) as string
	return strconv.Itoa(issue.IID), nil
}

// gitLabMilestone represents a GitLab milestone structure.
type gitLabMilestone struct {
	ID    int    `json:"id"`
	IID   int    `json:"iid"`
	Title string `json:"title"`
}

// getMilestoneID retrieves the milestone's internal ID by IID or title.
// If milestone is numeric, it's treated as an IID (the #number you see in GitLab UI).
// If milestone is non-numeric, it's searched by title.
func (g *GitLabNotifier) getMilestoneID(ctx context.Context, milestone string) (int, error) {
	g.logger.Info("Looking up milestone",
		logger.Str("milestone", milestone),
		logger.Str("project_id", g.config.ProjectID))

	// URL-encode the project identifier (handles both numeric IDs and paths like "namespace/project")
	encodedProjectID := url.PathEscape(g.config.ProjectID)

	// Build API URL for milestone search
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/milestones",
		strings.TrimSuffix(g.config.URL, "/"),
		encodedProjectID)

	// Add query parameters - search by IID if numeric, otherwise by title
	params := url.Values{}
	if milestoneIID, err := strconv.Atoi(milestone); err == nil {
		// Numeric milestone = IID (the #number you see in GitLab)
		params.Add("iids[]", strconv.Itoa(milestoneIID))
		g.logger.Debug("Searching milestone by IID", logger.Int("iid", milestoneIID))
	} else {
		// Non-numeric milestone = search by title
		params.Add("search", milestone)
		g.logger.Debug("Searching milestone by title", logger.Str("title", milestone))
	}
	params.Add("state", "active")

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())
	g.logger.Debug("GET milestones", logger.Str("url", fullURL))

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication header
	req.Header.Set("PRIVATE-TOKEN", g.config.Token)
	req.Header.Set(httplib.HeaderContentType, httplib.ContentTypeJSON)

	// Send request
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("GitLab API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var milestones []gitLabMilestone
	if err := json.NewDecoder(resp.Body).Decode(&milestones); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(milestones) == 0 {
		return 0, fmt.Errorf("milestone not found: %s", milestone)
	}

	// Return the internal ID of the found milestone
	ms := milestones[0]
	g.logger.Debug("Found milestone",
		logger.Int("milestone_id", ms.ID),
		logger.Int("milestone_iid", ms.IID),
		logger.Str("milestone_title", ms.Title))

	return ms.ID, nil
}

// buildIssueTitle builds a title for the GitLab issue.
func (g *GitLabNotifier) buildIssueTitle(alert *domain.Alert) string {
	// Format: [SEVERITY] Title - Resource Name (Realm)
	return oneLine(fmt.Sprintf("[%s] %s - %s (%s)",
		strings.ToUpper(alert.Severity.String()),
		alert.Title,
		alert.ResourceName,
		alert.RealmName))
}

// buildIssueDescription builds a description for the GitLab issue.
func (g *GitLabNotifier) buildIssueDescription(alert *domain.Alert) string {
	var buf strings.Builder

	buf.WriteString("## Alert Details\n\n")
	fmt.Fprintf(&buf, "- **Alert ID**: %s\n", gitlabCode(alert.AlertID))
	fmt.Fprintf(&buf, "- **Type**: %s\n", alert.Type)
	fmt.Fprintf(&buf, "- **Severity**: **%s**\n", strings.ToUpper(alert.Severity.String()))
	fmt.Fprintf(&buf, "- **Detected At**: %s\n", alert.FirstDetected.Format(time.RFC3339))
	buf.WriteString("\n")

	buf.WriteString("## Description\n\n")
	buf.WriteString(gitlabCodeBlock(alert.Description))
	buf.WriteString("\n\n")

	buf.WriteString("## Resource Information\n\n")
	fmt.Fprintf(&buf, "- **Resource Type**: %s\n", gitlabCode(alert.ResourceType))
	fmt.Fprintf(&buf, "- **Resource ID**: %s\n", gitlabCode(alert.ResourceID))
	fmt.Fprintf(&buf, "- **Resource Name**: %s\n", gitlabCode(alert.ResourceName))
	fmt.Fprintf(&buf, "- **Realm**: %s\n", gitlabCode(alert.RealmName))
	buf.WriteString("\n")

	if alert.Recommendation != "" {
		buf.WriteString("## Recommendation\n\n")
		buf.WriteString(alert.Recommendation)
		buf.WriteString("\n\n")
	}

	buf.WriteString("---\n")
	buf.WriteString("*This issue was automatically created by the Keycloak Monitoring Tool*\n")

	return buf.String()
}

// Ensure GitLabNotifier implements Notifier
var _ Notifier = (*GitLabNotifier)(nil)

// gitlabCode renders alert data as inline code. GitLab turns nothing inside
// code into a link, image, @mention or reference, so a value from outside, such
// as the client_id an unauthenticated request named, cannot ping people or
// show a disguised link. The fence is one backtick longer than the longest run
// in s, so s cannot close it early. Inline code is one line.
func gitlabCode(s string) string {
	s = oneLine(s)
	if s == "" {
		return ""
	}
	fence := strings.Repeat("`", longestBacktickRun(s)+1)
	return fence + " " + s + " " + fence
}

// gitlabCodeBlock renders multi-line alert data as a fenced code block, for
// the same reason as gitlabCode.
func gitlabCodeBlock(s string) string {
	fence := strings.Repeat("`", max(3, longestBacktickRun(s)+1))
	return fence + "text\n" + s + "\n" + fence
}

func longestBacktickRun(s string) int {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
			continue
		}
		run = 0
	}
	return longest
}

// oneLine replaces line breaks with spaces, for values that must stay on one
// line: an issue title, a mail header, inline code.
func oneLine(s string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(s)
}
