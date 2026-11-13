package mcp

import (
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

func TestImplementation_NamesTheServerAfterTheBranding(t *testing.T) {
	tests := []struct {
		branding    config.BrandingConfig
		name, title string
	}{
		{config.BrandingConfig{}, "keycloak-monitoring-tool", "Keycloak Monitoring Tool"},
		{config.BrandingConfig{Name: "Acme Monitoring Platform"}, "acme-monitoring-platform", "Acme Monitoring Platform"},
	}
	for _, tt := range tests {
		impl := implementation(tt.branding)
		if impl.Name != tt.name || impl.Title != tt.title {
			t.Errorf("implementation(%+v) = %q / %q, want %q / %q", tt.branding, impl.Name, impl.Title, tt.name, tt.title)
		}
	}
}

// The server built from the config names itself after its branding, as a
// client sees it when it connects.
func TestServer_ConnectedClientSeesTheBrandedName(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Branding = config.BrandingConfig{Name: "Acme Monitoring Platform"}
	ts := newTestServerCustom(t, cfg, testLogger(), nil, acceptingValidator("pat_good"),
		&mockPermissionService{}, &mockTenantReader{}, &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{})

	info := connectClient(t, ts, "pat_good").InitializeResult().ServerInfo
	if info.Name != "acme-monitoring-platform" || info.Title != "Acme Monitoring Platform" {
		t.Errorf("server info = %q / %q, want the branded name", info.Name, info.Title)
	}
}
