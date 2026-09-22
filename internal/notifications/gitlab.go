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
	ID          int      `json:"id"`
	IID         int      `json:"iid"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	State       string   `json:"state"`
	WebURL      string   `json:"web_url"`
}

// gitLabIssueRequest represents a GitLab issue creation request.
type gitLabIssueRequest struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Labels      string `json:"labels,omitempty"`       // Comma-separated string
	MilestoneID int    `json:"milestone_id,omitempty"` // Milestone ID (numeric)
}

// Notify sends an alert to GitLab as an issue.
// For identity provider alerts, it checks for existing open issues to avoid duplicates.
func (g *GitLabNotifier) Notify(ctx context.Context, alert *domain.Alert) (string, error) {
	if !g.config.Enabled {
		return "", fmt.Errorf("GitLab integration is not enabled")
	}

	g.logger.Info("Processing GitLab notification",
		logger.Str("alert_id", alert.AlertID),
		logger.Str("resource_type", alert.ResourceType))

	// For identity provider alerts, check for existing issues
	if alert.ResourceType == "identity_provider" {
		existingIssueID, err := g.findExistingIssue(ctx, alert)
		if err != nil {
			g.logger.Warn("Failed to check for existing GitLab issue",
				logger.Str("alert_id", alert.AlertID),
				logger.Err(err))
		} else if existingIssueID != "" {
			g.logger.Info("Found existing GitLab issue for identity provider, skipping creation",
				logger.Str("alert_id", alert.AlertID),
				logger.Str("resource_name", alert.ResourceName),
				logger.Str("existing_issue_id", existingIssueID))
			return existingIssueID, nil
		}
	}

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

// findExistingIssue searches for an existing open issue for the same identity provider.
func (g *GitLabNotifier) findExistingIssue(ctx context.Context, alert *domain.Alert) (string, error) {
	// Build search query to find issues for this specific identity provider
	// Search for: open issues with matching resource name and labels
	searchQuery := fmt.Sprintf("%s %s", alert.ResourceName, alert.RealmName)

	// Search for issues
	issues, err := g.searchIssues(ctx, searchQuery, "opened")
	if err != nil {
		return "", fmt.Errorf("failed to search GitLab issues: %w", err)
	}

	// Check if any of the found issues match our criteria
	for _, issue := range issues {
		// Check if issue title contains the resource name (identity provider name)
		if strings.Contains(strings.ToLower(issue.Title), strings.ToLower(alert.ResourceName)) {
			// Check if it has our monitoring labels
			hasLabel := false
			for _, label := range issue.Labels {
				for _, configLabel := range g.config.Labels {
					if label == configLabel {
						hasLabel = true
						break
					}
				}
				if hasLabel {
					break
				}
			}

			if hasLabel || len(g.config.Labels) == 0 {
				// Found a matching issue
				return strconv.Itoa(issue.IID), nil
			}
		}
	}

	return "", nil
}

// searchIssues searches for GitLab issues matching the query.
func (g *GitLabNotifier) searchIssues(ctx context.Context, query, state string) ([]gitLabIssue, error) {
	// Build API URL
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/issues",
		strings.TrimSuffix(g.config.URL, "/"),
		url.PathEscape(g.config.ProjectID))

	// Add query parameters
	params := url.Values{}
	params.Add("search", query)
	params.Add("state", state)
	params.Add("per_page", "20")
	if len(g.config.Labels) > 0 {
		params.Add("labels", strings.Join(g.config.Labels, ","))
	}

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication header
	req.Header.Set("PRIVATE-TOKEN", g.config.Token)
	req.Header.Set(httplib.HeaderContentType, httplib.ContentTypeJSON)

	// Send request
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var issues []gitLabIssue
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return issues, nil
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
	return fmt.Sprintf("[%s] %s - %s (%s)",
		strings.ToUpper(alert.Severity.String()),
		alert.Title,
		alert.ResourceName,
		alert.RealmName)
}

// buildIssueDescription builds a description for the GitLab issue.
func (g *GitLabNotifier) buildIssueDescription(alert *domain.Alert) string {
	var buf strings.Builder

	buf.WriteString("## Alert Details\n\n")
	buf.WriteString(fmt.Sprintf("- **Alert ID**: `%s`\n", alert.AlertID))
	buf.WriteString(fmt.Sprintf("- **Type**: %s\n", alert.Type))
	buf.WriteString(fmt.Sprintf("- **Severity**: **%s**\n", strings.ToUpper(alert.Severity.String())))
	buf.WriteString(fmt.Sprintf("- **Detected At**: %s\n", alert.FirstDetected.Format(time.RFC3339)))
	buf.WriteString("\n")

	buf.WriteString("## Description\n\n")
	buf.WriteString(alert.Description)
	buf.WriteString("\n\n")

	buf.WriteString("## Resource Information\n\n")
	buf.WriteString(fmt.Sprintf("- **Resource Type**: %s\n", alert.ResourceType))
	buf.WriteString(fmt.Sprintf("- **Resource ID**: `%s`\n", alert.ResourceID))
	buf.WriteString(fmt.Sprintf("- **Resource Name**: %s\n", alert.ResourceName))
	buf.WriteString(fmt.Sprintf("- **Realm**: %s\n", alert.RealmName))
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
