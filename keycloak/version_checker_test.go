package keycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
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
