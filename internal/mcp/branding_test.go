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
