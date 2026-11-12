package fx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
)

func TestNewVersionChecker_DisabledBuildsNothing(t *testing.T) {
	var hits int64
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
	}))
	defer stub.Close()

	if vc := newVersionChecker(config.VersionCheckerConfig{GitHubAPIBaseURL: stub.URL}, nil); vc != nil {
		t.Fatal("a disabled version checker was built")
	}
	if n := atomic.LoadInt64(&hits); n != 0 {
		t.Errorf("a disabled version checker made %d request(s)", n)
	}
}

// The configured base URL, owner and repository reach the request the checker
// makes; settings left at zero fall back to the checker's defaults.
func TestNewVersionChecker_EnabledUsesTheConfiguredSource(t *testing.T) {
	var path atomic.Value
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path.Store(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"26.8.2","html_url":"https://example.invalid/26.8.2","body":""}`))
	}))
	defer stub.Close()

	vc := newVersionChecker(config.VersionCheckerConfig{
		Enabled:          true,
		GitHubAPIBaseURL: stub.URL,
		KeycloakOwner:    "acme",
		KeycloakRepo:     "idp",
		HTTPTimeout:      5 * time.Second,
	}, nil)
	if vc == nil {
		t.Fatal("an enabled version checker was not built")
	}
	info, err := vc.GetLatestVersions(context.Background())
	if err != nil {
		t.Fatalf("GetLatestVersions: %v", err)
	}
	if got, _ := path.Load().(string); got != "/repos/acme/idp/releases/latest" {
		t.Errorf("checker asked for %q, want /repos/acme/idp/releases/latest", got)
	}
	if info.LatestKeycloakVersion != "26.8.2" {
		t.Errorf("latest version %q, want 26.8.2", info.LatestKeycloakVersion)
	}
}

// healthWithVersion answers GetLatestHealth with one server version. Every
// other HealthRepository method is left to the embedded nil interface.
type healthWithVersion struct {
	keycloak.HealthRepository
	version string
}

func (r healthWithVersion) GetLatestHealth(context.Context, string) (*domain.KeycloakHealth, error) {
	return &domain.KeycloakHealth{ServerVersion: r.version}, nil
}

// The service the app is built with has the version checker the config asks
// for. The defect: provideKeycloakService passed nil, so an enabled checker
// was never used. Builds the service the way fx does, not the checker alone.
func TestProvideKeycloakService_UsesTheConfiguredVersionChecker(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"26.8.2","html_url":"https://example.invalid/26.8.2","body":""}`))
	}))
	defer stub.Close()

	for _, enabled := range []bool{true, false} {
		cfg := &config.AppConfig{}
		cfg.Keycloak.Global.VersionChecker = config.VersionCheckerConfig{
			Enabled:          enabled,
			GitHubAPIBaseURL: stub.URL,
			HTTPTimeout:      5 * time.Second,
		}
		svc := provideKeycloakService(cfg, nil, nil, nil, healthWithVersion{version: "26.4.2"}, nil)

		info, err := svc.GetVersionInfo(context.Background(), "t1")
		if err != nil {
			t.Fatalf("enabled=%v: GetVersionInfo: %v", enabled, err)
		}
		if !enabled {
			if info != nil {
				t.Errorf("a disabled checker answered %+v", info)
			}
			continue
		}
		if info == nil || info.LatestKeycloakVersion != "26.8.2" || !info.IsKeycloakOutdated {
			t.Errorf("enabled checker answered %+v, want 26.4.2 compared against 26.8.2", info)
		}
	}
}
