package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
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

// VersionInfo represents version information for Keycloak variants
type VersionInfo struct {
	LatestKeycloakVersion string    `json:"latest_keycloak_version"`
	LatestRHBKVersion     string    `json:"latest_rhbk_version"`
	KeycloakReleaseURL    string    `json:"keycloak_release_url"`
	RHBKReleaseURL        string    `json:"rhbk_release_url"`
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
	rhbkFallbackURL  string
	rhbkDocsBaseURL  string
}

// VersionCheckerConfig contains configuration for the version checker
type VersionCheckerConfig struct {
	CacheTTL         time.Duration
	HTTPTimeout      time.Duration
	GitHubAPIBaseURL string
	KeycloakOwner    string
	KeycloakRepo     string
	RHBKFallbackURL  string
	RHBKDocsBaseURL  string
}

// DefaultVersionCheckerConfig returns default configuration
func DefaultVersionCheckerConfig() VersionCheckerConfig {
	return VersionCheckerConfig{
		CacheTTL:         1 * time.Hour,
		HTTPTimeout:      10 * time.Second,
		GitHubAPIBaseURL: "https://api.github.com",
		KeycloakOwner:    "keycloak",
		KeycloakRepo:     "keycloak",
		RHBKFallbackURL:  "https://access.redhat.com/products/red-hat-build-of-keycloak",
		RHBKDocsBaseURL:  "https://docs.redhat.com/en/documentation/red_hat_build_of_keycloak",
	}
}

// NewVersionChecker creates a new version checker with the given configuration
func NewVersionChecker(cfg VersionCheckerConfig) *VersionChecker {
	return &VersionChecker{
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
		cacheTTL:         cfg.CacheTTL,
		githubAPIBaseURL: cfg.GitHubAPIBaseURL,
		keycloakOwner:    cfg.KeycloakOwner,
		keycloakRepo:     cfg.KeycloakRepo,
		rhbkFallbackURL:  cfg.RHBKFallbackURL,
		rhbkDocsBaseURL:  cfg.RHBKDocsBaseURL,
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
	keycloakVersion := strings.TrimPrefix(release.TagName, "v")
	versionInfo.LatestKeycloakVersion = keycloakVersion
	versionInfo.KeycloakReleaseURL = release.HTMLURL
	versionInfo.HasSecurityUpdates = ContainsSecurityUpdates(release.Body)

	// Fetch latest RHBK version from Red Hat documentation site
	rhbkVersion, rhbkURL, err := vc.fetchLatestRHBKVersion(ctx)
	if err != nil {
		// If RHBK version fetch fails, use fallback URL
		// Note: RHBK may not have the same version as upstream Keycloak
		log.Printf("version_checker: failed to fetch RHBK version: %v", err)
		versionInfo.LatestRHBKVersion = ""
		versionInfo.RHBKReleaseURL = vc.rhbkFallbackURL
	} else {
		versionInfo.LatestRHBKVersion = rhbkVersion
		versionInfo.RHBKReleaseURL = rhbkURL
	}

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
	req.Header.Set("User-Agent", "Point-Monitoring-Platform")

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

// fetchLatestRHBKVersion fetches the latest RHBK version from Red Hat documentation redirects
func (vc *VersionChecker) fetchLatestRHBKVersion(ctx context.Context) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", vc.rhbkDocsBaseURL, nil)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Accept", "text/html")

	resp, err := vc.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("RHBK docs returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	majorMinor, err := extractRHBKMinorFromURL(resp.Request.URL)
	if err != nil {
		majorMinor, err = extractRHBKMinorFromHTML(body)
		if err != nil {
			return "", "", err
		}
	}

	releaseURL := vc.buildRHBKReleaseURL(majorMinor)

	// Fetch release notes page to inspect patch-level versions
	version := majorMinor
	releaseBody, err := vc.fetchRHBKReleaseNotes(ctx, releaseURL)
	if err == nil {
		if latestPatch, patchErr := extractLatestRHBKPatchVersion(releaseBody, majorMinor); patchErr == nil {
			version = latestPatch
		} else {
			log.Printf("version_checker: failed to extract RHBK patch version from %s: %v", releaseURL, patchErr)
		}
	} else {
		log.Printf("version_checker: failed to fetch RHBK release notes: %v", err)
	}

	return version, releaseURL, nil
}

// CompareVersions compares two version strings
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
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

// IsRHBKVersion checks if a version string is RHBK (contains ".redhat-" suffix)
func IsRHBKVersion(version string) bool {
	return strings.Contains(version, ".redhat-")
}

// StripRHBKSuffix removes the ".redhat-XXXXX" suffix from RHBK versions
// Example: "25.0.1.redhat-00001" -> "25.0.1"
func StripRHBKSuffix(version string) string {
	if idx := strings.Index(version, ".redhat-"); idx > 0 {
		return version[:idx]
	}
	return version
}

// buildRHBKReleaseURL builds the Red Hat documentation URL for a specific RHBK minor release
func (vc *VersionChecker) buildRHBKReleaseURL(minorVersion string) string {
	// URL format: https://docs.redhat.com/en/documentation/red_hat_build_of_keycloak/{MINOR_VERSION}/html-single/release_notes/index
	return fmt.Sprintf("%s/%s/html-single/release_notes/index", vc.rhbkDocsBaseURL, minorVersion)
}

var (
	rhbkDocsMinorRegexp = regexp.MustCompile(`/red_hat_build_of_keycloak/([0-9]+\.[0-9]+)/`)
)

func extractRHBKMinorFromURL(u *url.URL) (string, error) {
	if u == nil {
		return "", fmt.Errorf("nil URL provided for RHBK version extraction")
	}

	matches := rhbkDocsMinorRegexp.FindStringSubmatch(u.Path)
	if len(matches) < 2 {
		return "", fmt.Errorf("failed to detect RHBK minor version from URL path")
	}
	return matches[1], nil
}

func extractRHBKMinorFromHTML(body []byte) (string, error) {
	matches := rhbkDocsMinorRegexp.FindSubmatch(body)
	if len(matches) < 2 {
		return "", fmt.Errorf("failed to detect RHBK minor version in documentation")
	}
	return string(matches[1]), nil
}

func extractLatestRHBKPatchVersion(body []byte, majorMinor string) (string, error) {
	if majorMinor == "" {
		return "", fmt.Errorf("majorMinor version not provided")
	}

	patchRegex := regexp.MustCompile(fmt.Sprintf(`%s\.([0-9]+)`, regexp.QuoteMeta(majorMinor)))
	matches := patchRegex.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no patch versions found for %s", majorMinor)
	}

	highestPatch := -1
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		var patch int
		if _, err := fmt.Sscanf(string(match[1]), "%d", &patch); err != nil {
			continue
		}
		if patch > highestPatch {
			highestPatch = patch
		}
	}

	if highestPatch < 0 {
		return "", fmt.Errorf("no valid patch numbers found for %s", majorMinor)
	}

	version := fmt.Sprintf("%s.%d", majorMinor, highestPatch)
	log.Printf("version_checker: latest patch for %s determined as %s", majorMinor, version)
	return version, nil
}

func (vc *VersionChecker) fetchRHBKReleaseNotes(ctx context.Context, releaseURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", releaseURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "text/html")

	resp, err := vc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("RHBK release notes returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("version_checker: fetched %d bytes from %s", len(body), releaseURL)

	return body, nil
}
