package keycloak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// ErrVersionLookupFailed wraps every error GetLatestVersions returns for a
// failed fetch from GitHub, including a remembered one. The checker logs each
// failed fetch once, so callers need not log it again.
var ErrVersionLookupFailed = errors.New("failed to fetch latest Keycloak version")

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
	httpClient *http.Client
	log        *logger.Logger

	// mu guards the fields below it. Page loads call GetLatestVersions
	// concurrently: one of them fetches while the others wait for that fetch
	// and share its result, so GitHub is asked once however many pages load.
	mu        sync.Mutex
	cache     *VersionInfo
	cacheTime time.Time
	// failure and failedAt hold the last failed fetch, which is returned
	// until retryAfter has passed, so an unreachable GitHub costs one slow
	// page load per interval rather than one per page load.
	failure          error
	failedAt         time.Time
	inflight         *versionFetch
	cacheTTL         time.Duration
	githubAPIBaseURL string
	keycloakOwner    string
	keycloakRepo     string
}

// versionFetch is one fetch from GitHub that concurrent callers share.
type versionFetch struct {
	done chan struct{}
	info *VersionInfo
	err  error
}

// failureRetryInterval is how long a failed fetch is remembered before
// GitHub is asked again, unless the cache TTL is shorter.
const failureRetryInterval = time.Minute

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
	// Logger receives one line per failed fetch and one when fetching
	// recovers. Nil discards them.
	Logger *logger.Logger
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

	log := cfg.Logger
	if log == nil {
		log = logger.NewNoop()
	}
	return &VersionChecker{
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
		log:              log,
		cacheTTL:         cfg.CacheTTL,
		githubAPIBaseURL: cfg.GitHubAPIBaseURL,
		keycloakOwner:    cfg.KeycloakOwner,
		keycloakRepo:     cfg.KeycloakRepo,
	}
}

// GetLatestVersions returns the latest version information, from the cache
// while it is fresh. Concurrent callers share one fetch, and a failed fetch is
// returned again until the retry interval has passed.
func (vc *VersionChecker) GetLatestVersions(ctx context.Context) (*VersionInfo, error) {
	vc.mu.Lock()
	if vc.cache != nil && time.Since(vc.cacheTime) < vc.cacheTTL {
		info := vc.cache
		vc.mu.Unlock()
		return info, nil
	}
	if vc.failure != nil && time.Since(vc.failedAt) < vc.retryAfter() {
		err := vc.failure
		vc.mu.Unlock()
		return nil, err
	}
	f := vc.inflight
	if f == nil {
		f = &versionFetch{done: make(chan struct{})}
		vc.inflight = f
		// The fetch outlives the caller that started it, since others wait
		// for it; the HTTP client's timeout still bounds it.
		go vc.fetch(context.WithoutCancel(ctx), f)
	}
	vc.mu.Unlock()

	select {
	case <-f.done:
		return f.info, f.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// fetch asks GitHub for the latest release, records the result for later
// callers and releases everyone waiting on f.
func (vc *VersionChecker) fetch(ctx context.Context, f *versionFetch) {
	info, err := vc.fetchLatestVersions(ctx)

	vc.mu.Lock()
	recovered := err == nil && vc.failure != nil
	if err != nil {
		vc.failure, vc.failedAt = err, time.Now()
	} else {
		vc.cache, vc.cacheTime = info, time.Now()
		vc.failure = nil
	}
	retry := vc.retryAfter()
	vc.inflight = nil
	f.info, f.err = info, err
	vc.mu.Unlock()

	// Logged before the waiting callers are released, so the line is
	// written by the time any of them returns.
	if err != nil {
		vc.log.Warn("Version check: could not fetch the latest Keycloak release; no update notice is shown until a later attempt succeeds",
			logger.Err(err), logger.Dur("retry_after", retry))
	} else if recovered {
		vc.log.Info("Version check: fetched the latest Keycloak release again",
			logger.Str("latest_keycloak_version", info.LatestKeycloakVersion))
	}
	close(f.done)
}

func (vc *VersionChecker) fetchLatestVersions(ctx context.Context) (*VersionInfo, error) {
	release, err := vc.fetchLatestGitHubReleaseWithBody(ctx, vc.keycloakOwner, vc.keycloakRepo)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrVersionLookupFailed, err)
	}
	return &VersionInfo{
		LatestKeycloakVersion: strings.TrimPrefix(release.TagName, "v"),
		KeycloakReleaseURL:    release.HTMLURL,
		HasSecurityUpdates:    ContainsSecurityUpdates(release.Body),
		LastChecked:           time.Now(),
	}, nil
}

// retryAfter is how long a failed fetch is returned before GitHub is asked
// again: failureRetryInterval, or the cache TTL when that is shorter.
func (vc *VersionChecker) retryAfter() time.Duration {
	if vc.cacheTTL > 0 && vc.cacheTTL < failureRetryInterval {
		return vc.cacheTTL
	}
	return failureRetryInterval
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

// upstreamRelease matches a plain release number as upstream Keycloak reports
// it, such as 26.4.2.
var upstreamRelease = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

// isUpstreamRelease reports whether a server version can be compared with
// upstream Keycloak releases. A vendor or distribution build reports its own
// suffix after the release number and follows its own release line, so
// comparing it with upstream would announce updates its vendor has not
// shipped. Development builds such as 999.0.0-SNAPSHOT are skipped the same
// way.
func isUpstreamRelease(version string) bool {
	return upstreamRelease.MatchString(version)
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
