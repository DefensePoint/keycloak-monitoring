package keycloak

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{
			name:     "v1 greater than v2",
			v1:       "25.0.2",
			v2:       "25.0.1",
			expected: 1,
		},
		{
			name:     "v1 less than v2",
			v1:       "25.0.1",
			v2:       "25.0.2",
			expected: -1,
		},
		{
			name:     "v1 equals v2",
			v1:       "25.0.1",
			v2:       "25.0.1",
			expected: 0,
		},
		{
			name:     "Major version difference",
			v1:       "26.0.0",
			v2:       "25.0.1",
			expected: 1,
		},
		{
			name:     "Minor version difference",
			v1:       "25.1.0",
			v2:       "25.0.1",
			expected: 1,
		},
		{
			name:     "With v prefix",
			v1:       "v25.0.1",
			v2:       "v25.0.1",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestIsVersionOutdated(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		latestVersion  string
		expected       bool
	}{
		{
			name:           "Current is outdated",
			currentVersion: "25.0.1",
			latestVersion:  "25.0.2",
			expected:       true,
		},
		{
			name:           "Current is up to date",
			currentVersion: "25.0.2",
			latestVersion:  "25.0.2",
			expected:       false,
		},
		{
			name:           "Current is newer",
			currentVersion: "25.0.3",
			latestVersion:  "25.0.2",
			expected:       false,
		},
		{
			name:           "Patch version behind",
			currentVersion: "25.0.1",
			latestVersion:  "25.0.2",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsVersionOutdated(tt.currentVersion, tt.latestVersion)
			if result != tt.expected {
				t.Errorf("IsVersionOutdated(%q, %q) = %v, want %v",
					tt.currentVersion, tt.latestVersion, result, tt.expected)
			}
		})
	}
}

// TestNewVersionCheckerOptIn pins the airgap guarantee: the checker is the only
// component here that contacts a host we do not operate, so it must not exist
// unless a deployment asked for it.
func TestNewVersionCheckerOptIn(t *testing.T) {
	t.Run("disabled config builds no checker", func(t *testing.T) {
		cfg := DefaultVersionCheckerConfig()
		if cfg.Enabled {
			t.Fatal("DefaultVersionCheckerConfig must not enable the checker: " +
				"the defaults would then reach GitHub")
		}
		if got := NewVersionChecker(cfg); got != nil {
			t.Error("NewVersionChecker returned a checker for a disabled config; " +
				"callers rely on nil to mean no version checking")
		}
	})

	t.Run("zero-value config builds no checker", func(t *testing.T) {
		// A caller assembling the struct by hand, or unmarshalling a config
		// file with the key absent, must land on off rather than on.
		if got := NewVersionChecker(VersionCheckerConfig{}); got != nil {
			t.Error("NewVersionChecker returned a checker for a zero-value config")
		}
	})

	t.Run("enabled config builds a checker with its URLs", func(t *testing.T) {
		cfg := DefaultVersionCheckerConfig()
		cfg.Enabled = true
		cfg.GitHubAPIBaseURL = "https://github.example.invalid"

		vc := NewVersionChecker(cfg)
		if vc == nil {
			t.Fatal("NewVersionChecker returned nil for an enabled config")
		}
		if vc.githubAPIBaseURL != "https://github.example.invalid" {
			t.Errorf("configured base URL not applied: got %q", vc.githubAPIBaseURL)
		}
		if vc.cacheTTL != cfg.CacheTTL {
			t.Errorf("cacheTTL = %v, want %v", vc.cacheTTL, cfg.CacheTTL)
		}
	})
}

// End-to-end over the flag: a local server stands in for GitHub, so the enabled
// path is exercised without contacting anyone, and the disabled path can be
// shown to make no request at all.
func TestE2EVersionCheckerNetwork(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"26.4.0","html_url":"http://example.invalid/rel","body":"notes"}`))
	}))
	defer srv.Close()

	newCfg := func(enabled bool) VersionCheckerConfig {
		cfg := DefaultVersionCheckerConfig()
		cfg.Enabled = enabled
		cfg.HTTPTimeout = 5 * time.Second
		cfg.CacheTTL = time.Minute
		// every outbound base URL points at the local stub
		cfg.GitHubAPIBaseURL = srv.URL
		return cfg
	}

	t.Run("disabled makes no request", func(t *testing.T) {
		atomic.StoreInt64(&hits, 0)
		vc := NewVersionChecker(newCfg(false))
		if vc != nil {
			t.Fatal("expected no checker when disabled")
		}
		if got := atomic.LoadInt64(&hits); got != 0 {
			t.Errorf("disabled checker caused %d request(s); must be 0", got)
		}
	})

	t.Run("enabled fetches and reports a version", func(t *testing.T) {
		atomic.StoreInt64(&hits, 0)
		vc := NewVersionChecker(newCfg(true))
		if vc == nil {
			t.Fatal("expected a checker when enabled")
		}
		info, err := vc.GetLatestVersions(context.Background())
		if err != nil {
			t.Fatalf("GetLatestVersions: %v", err)
		}
		if info == nil || info.LatestKeycloakVersion == "" {
			t.Fatalf("no version returned: %+v", info)
		}
		if got := atomic.LoadInt64(&hits); got == 0 {
			t.Error("enabled checker made no request; the stub saw nothing")
		}
		t.Logf("enabled: version=%q after %d request(s)", info.LatestKeycloakVersion, atomic.LoadInt64(&hits))
	})
}

func TestIsUpstreamRelease(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"26.4.2", true},
		{"v26.4.2", true},
		{"9.0.3", true},
		{"26.0.6.vendor-00001", false},
		{"26.4.2-build.7", false},
		{"26.4.2.1", false},
		{"999.0.0-SNAPSHOT", false},
		{"26.4", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isUpstreamRelease(tt.version); got != tt.want {
			t.Errorf("isUpstreamRelease(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

// fixedHealthRepo answers GetLatestHealth with one server version. Every other
// HealthRepository method is left to the embedded nil interface.
type fixedHealthRepo struct {
	HealthRepository
	version string
}

func (r fixedHealthRepo) GetLatestHealth(context.Context, string) (*domain.KeycloakHealth, error) {
	return &domain.KeycloakHealth{ServerVersion: r.version}, nil
}

// A vendor build is never reported as behind upstream, however far its
// release number trails; a plain release still is.
func TestGetVersionInfo_ComparesOnlyUpstreamReleases(t *testing.T) {
	checker := &VersionChecker{
		cache:     &VersionInfo{LatestKeycloakVersion: "26.8.2", KeycloakReleaseURL: "https://example.invalid/26.8.2"},
		cacheTime: time.Now(),
		cacheTTL:  time.Hour,
	}
	tests := []struct {
		version      string
		wantOutdated bool
	}{
		{"26.4.2", true},
		{"26.8.2", false},
		{"26.0.6.vendor-00001", false},
		{"999.0.0-SNAPSHOT", false},
		{"", false},
	}
	for _, tt := range tests {
		svc := NewService(nil, nil, fixedHealthRepo{version: tt.version}, nil, nil, checker)
		info, err := svc.GetVersionInfo(context.Background(), "t1")
		if err != nil {
			t.Fatalf("GetVersionInfo(%q): %v", tt.version, err)
		}
		if info.IsKeycloakOutdated != tt.wantOutdated {
			t.Errorf("server %q: outdated = %v, want %v", tt.version, info.IsKeycloakOutdated, tt.wantOutdated)
		}
		if info.CurrentVersion != tt.version || info.LatestKeycloakVersion != "26.8.2" {
			t.Errorf("server %q: got current %q, latest %q", tt.version, info.CurrentVersion, info.LatestKeycloakVersion)
		}
	}
}

// Page loads ask for version info concurrently, and every call may refresh the
// cache, so GetLatestVersions must be safe to call from many goroutines. Run
// with -race.
func TestGetLatestVersions_ConcurrentCalls(t *testing.T) {
	var hits int64
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"26.8.2","html_url":"https://example.invalid/26.8.2","body":""}`))
	}))
	defer stub.Close()

	cfg := DefaultVersionCheckerConfig()
	cfg.Enabled = true
	cfg.GitHubAPIBaseURL = stub.URL
	cfg.CacheTTL = time.Nanosecond
	vc := NewVersionChecker(cfg)

	done := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go func() {
			info, err := vc.GetLatestVersions(context.Background())
			if err == nil && info.LatestKeycloakVersion != "26.8.2" {
				err = fmt.Errorf("latest version %q", info.LatestKeycloakVersion)
			}
			done <- err
		}()
	}
	for i := 0; i < 20; i++ {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

// releaseStub answers GitHub's latest-release request after delay, counting
// requests; fail makes it answer 503 instead.
func releaseStub(t *testing.T, delay time.Duration, fail *atomic.Bool) (*httptest.Server, *int64) {
	t.Helper()
	var hits int64
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		if fail != nil && fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"26.8.2","html_url":"https://example.invalid/26.8.2","body":""}`))
	}))
	t.Cleanup(stub.Close)
	return stub, &hits
}

func stubChecker(url string, ttl, timeout time.Duration) *VersionChecker {
	cfg := DefaultVersionCheckerConfig()
	cfg.Enabled = true
	cfg.GitHubAPIBaseURL = url
	cfg.CacheTTL = ttl
	cfg.HTTPTimeout = timeout
	return NewVersionChecker(cfg)
}

// callConcurrently calls GetLatestVersions from n goroutines and returns their
// errors and how long the slowest took.
func callConcurrently(vc *VersionChecker, n int) ([]error, time.Duration) {
	errs := make(chan error, n)
	start := time.Now()
	for i := 0; i < n; i++ {
		go func() {
			info, err := vc.GetLatestVersions(context.Background())
			if err == nil && info.LatestKeycloakVersion != "26.8.2" {
				err = fmt.Errorf("latest version %q", info.LatestKeycloakVersion)
			}
			errs <- err
		}()
	}
	out := make([]error, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, <-errs)
	}
	return out, time.Since(start)
}

func TestGetLatestVersions_ConcurrentCallersShareOneFetch(t *testing.T) {
	stub, hits := releaseStub(t, 200*time.Millisecond, nil)
	vc := stubChecker(stub.URL, time.Hour, 5*time.Second)

	errs, _ := callConcurrently(vc, 20)
	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if n := atomic.LoadInt64(hits); n != 1 {
		t.Errorf("20 concurrent callers made %d requests to GitHub, want 1", n)
	}
}

// When GitHub does not answer, every waiting caller is released when the one
// shared fetch times out, and later callers get the failure straight away
// until the retry interval has passed.
func TestGetLatestVersions_HangingGitHubCostsOneTimeout(t *testing.T) {
	stub, hits := releaseStub(t, 10*time.Second, nil)
	vc := stubChecker(stub.URL, time.Hour, 300*time.Millisecond)

	errs, took := callConcurrently(vc, 10)
	for _, err := range errs {
		if err == nil {
			t.Error("a caller got versions from a GitHub that never answered")
		}
	}
	if took > 2*time.Second {
		t.Errorf("10 concurrent callers took %v, want about one 300ms timeout", took)
	}

	start := time.Now()
	if _, err := vc.GetLatestVersions(context.Background()); err == nil {
		t.Error("the call after a failure succeeded")
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("the call after a failure took %v, want it answered from the remembered failure", d)
	}
	if n := atomic.LoadInt64(hits); n != 1 {
		t.Errorf("GitHub was asked %d times, want 1", n)
	}
}

func TestGetLatestVersions_RetriesAfterTheInterval(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	stub, hits := releaseStub(t, 0, &fail)
	// A 50ms cache TTL makes the retry interval 50ms too.
	vc := stubChecker(stub.URL, 50*time.Millisecond, time.Second)

	if _, err := vc.GetLatestVersions(context.Background()); err == nil {
		t.Fatal("the first call succeeded against a failing GitHub")
	}
	fail.Store(false)
	if _, err := vc.GetLatestVersions(context.Background()); err == nil {
		t.Error("a call within the retry interval asked GitHub again")
	}
	time.Sleep(80 * time.Millisecond)
	info, err := vc.GetLatestVersions(context.Background())
	if err != nil || info.LatestKeycloakVersion != "26.8.2" {
		t.Fatalf("after the retry interval: %v, %v", info, err)
	}
	if n := atomic.LoadInt64(hits); n != 2 {
		t.Errorf("GitHub was asked %d times, want 2", n)
	}
}

// The caller that starts a fetch can give up without failing the others
// waiting for it.
func TestGetLatestVersions_ACanceledCallerDoesNotFailOthers(t *testing.T) {
	stub, _ := releaseStub(t, 300*time.Millisecond, nil)
	vc := stubChecker(stub.URL, time.Hour, 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	first := make(chan error, 1)
	go func() {
		_, err := vc.GetLatestVersions(ctx)
		first <- err
	}()
	time.Sleep(10 * time.Millisecond)
	info, err := vc.GetLatestVersions(context.Background())
	if err != nil || info.LatestKeycloakVersion != "26.8.2" {
		t.Errorf("the waiting caller got %v, %v", info, err)
	}
	if err := <-first; err == nil {
		t.Error("the canceled caller got no error")
	}
}

// A failed fetch is logged once however many callers it answers, every error
// is an ErrVersionLookupFailed, and recovering is logged once.
func TestGetLatestVersions_LogsEachFailedFetchOnce(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	stub, hits := releaseStub(t, 0, &fail)
	var buf bytes.Buffer
	cfg := DefaultVersionCheckerConfig()
	cfg.Enabled = true
	cfg.GitHubAPIBaseURL = stub.URL
	cfg.CacheTTL = 50 * time.Millisecond
	cfg.Logger = logger.New(zerolog.New(&buf))
	vc := NewVersionChecker(cfg)

	for i := 0; i < 5; i++ {
		if _, err := vc.GetLatestVersions(context.Background()); !errors.Is(err, ErrVersionLookupFailed) {
			t.Fatalf("call %d: %v, want an ErrVersionLookupFailed", i, err)
		}
	}
	if n := strings.Count(buf.String(), "could not fetch the latest Keycloak release"); n != 1 {
		t.Errorf("5 calls answered by 1 failed fetch logged %d warnings, want 1: %s", n, buf.String())
	}

	fail.Store(false)
	time.Sleep(80 * time.Millisecond)
	for i := 0; i < 3; i++ {
		if _, err := vc.GetLatestVersions(context.Background()); err != nil {
			t.Fatalf("after recovery: %v", err)
		}
	}
	if n := strings.Count(buf.String(), "fetched the latest Keycloak release again"); n != 1 {
		t.Errorf("recovery logged %d times, want 1: %s", n, buf.String())
	}
	if n := atomic.LoadInt64(hits); n != 2 {
		t.Errorf("GitHub was asked %d times, want 2", n)
	}
}
