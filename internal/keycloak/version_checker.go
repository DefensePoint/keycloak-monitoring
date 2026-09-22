package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Body        string    `json:"body"`
}

// VersionInfo represents the latest upstream Keycloak release information.
type VersionInfo struct {
	LatestKeycloakVersion string    `json:"latest_keycloak_version"`
	KeycloakReleaseURL    string    `json:"keycloak_release_url"`
	LastChecked           time.Time `json:"last_checked"`
	HasSecurityUpdates    bool      `json:"has_security_updates"`
}

// VersionChecker handles checking for latest Keycloak versions
type VersionChecker struct {
	httpClient       *http.Client
	cache            *VersionInfo
	cacheTime        time.Time
	cacheTTL         time.Duration
	githubAPIBaseURL string
	keycloakOwner    string
	keycloakRepo     string
}

// VersionCheckerConfig contains configuration for the version checker
type VersionCheckerConfig struct {
	// Enabled must be true for NewVersionChecker to build a checker at all.
	// See that function for why the default is off.
	Enabled          bool
	CacheTTL         time.Duration
	HTTPTimeout      time.Duration
	GitHubAPIBaseURL string
	KeycloakOwner    string
	KeycloakRepo     string
}

// DefaultVersionCheckerConfig returns default configuration.
//
// Enabled is deliberately absent, so it is false: the defaults must not
// produce a checker that contacts GitHub. A caller that wants version
// checking sets Enabled itself, which is the opt-in.
func DefaultVersionCheckerConfig() VersionCheckerConfig {
	return VersionCheckerConfig{
		CacheTTL:         1 * time.Hour,
		HTTPTimeout:      10 * time.Second,
		GitHubAPIBaseURL: "https://api.github.com",
		KeycloakOwner:    "keycloak",
		KeycloakRepo:     "keycloak",
	}
}

// NewVersionChecker creates a version checker, or returns nil when version
// checking is disabled.
//
// The nil return is the point rather than a shortcut. This checker is the only
// component that contacts hosts we do not operate, and third-party calls are
// prohibited because deployments can be airgapped. Refusing to build the
// object means no caller can enable that traffic by wiring the checker without
// reading the config; a flag consulted only at the call site would be little
// better than a comment. Callers already handle a nil checker: the service
// guards on it and GetVersionInfo returns (nil, nil), which the UI renders as
// no version information rather than an error.
func NewVersionChecker(cfg VersionCheckerConfig) *VersionChecker {
	if !cfg.Enabled {
		return nil
	}

	return &VersionChecker{
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
		cacheTTL:         cfg.CacheTTL,
		githubAPIBaseURL: cfg.GitHubAPIBaseURL,
		keycloakOwner:    cfg.KeycloakOwner,
		keycloakRepo:     cfg.KeycloakRepo,
	}
}

// GetLatestVersions returns the latest version information
func (vc *VersionChecker) GetLatestVersions(ctx context.Context) (*VersionInfo, error) {
	// Return cached version if still valid
	if vc.cache != nil && time.Since(vc.cacheTime) < vc.cacheTTL {
		return vc.cache, nil
	}

	versionInfo := &VersionInfo{
		LastChecked: time.Now(),
	}

	// Fetch latest Keycloak version with release notes
	release, err := vc.fetchLatestGitHubReleaseWithBody(ctx, vc.keycloakOwner, vc.keycloakRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest Keycloak version: %w", err)
	}
	versionInfo.LatestKeycloakVersion = strings.TrimPrefix(release.TagName, "v")
	versionInfo.KeycloakReleaseURL = release.HTMLURL
	versionInfo.HasSecurityUpdates = ContainsSecurityUpdates(release.Body)

	// Update cache
	vc.cache = versionInfo
	vc.cacheTime = time.Now()

	return versionInfo, nil
}

// fetchLatestGitHubReleaseWithBody fetches the latest release from a GitHub repository with full body
func (vc *VersionChecker) fetchLatestGitHubReleaseWithBody(ctx context.Context, owner, repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", vc.githubAPIBaseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add GitHub API headers
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Keycloak-Monitoring-Tool")

	resp, err := vc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// CompareVersions compares two semantic version strings.
// Returns 1 if v1 > v2, -1 if v1 < v2, 0 if equal.
func CompareVersions(v1, v2 string) int {
	// Simple version comparison (works for x.y.z format)
	v1Parts := strings.Split(strings.TrimPrefix(v1, "v"), ".")
	v2Parts := strings.Split(strings.TrimPrefix(v2, "v"), ".")

	maxLen := len(v1Parts)
	if len(v2Parts) > maxLen {
		maxLen = len(v2Parts)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int
		if i < len(v1Parts) {
			// Ignore error - if parsing fails, p1 remains 0
			_, _ = fmt.Sscanf(v1Parts[i], "%d", &p1)
		}
		if i < len(v2Parts) {
			// Ignore error - if parsing fails, p2 remains 0
			_, _ = fmt.Sscanf(v2Parts[i], "%d", &p2)
		}

		if p1 > p2 {
			return 1
		} else if p1 < p2 {
			return -1
		}
	}

	return 0
}

// IsVersionOutdated checks if the current version is outdated compared to the latest
func IsVersionOutdated(currentVersion, latestVersion string) bool {
	return CompareVersions(currentVersion, latestVersion) < 0
}

// ContainsSecurityUpdates checks if release notes mention security fixes
func ContainsSecurityUpdates(releaseBody string) bool {
	if releaseBody == "" {
		return false
	}

	lowerBody := strings.ToLower(releaseBody)

	// Security indicators
	securityKeywords := []string{
		"cve-",                  // CVE identifiers
		"security",              // Direct security mentions
		"vulnerability",         // Vulnerability fixes
		"exploit",               // Exploit patches
		"xss",                   // Cross-site scripting
		"csrf",                  // Cross-site request forgery
		"sql injection",         // SQL injection fixes
		"rce",                   // Remote code execution
		"authentication bypass", // Auth bypass fixes
		"privilege escalation",  // Privilege escalation fixes
	}

	for _, keyword := range securityKeywords {
		if strings.Contains(lowerBody, keyword) {
			return true
		}
	}

	return false
}
