package config

import (
	"os"
	"path/filepath"
	"testing"
)

func loadBrandingConfig(t *testing.T, yaml string) *AppConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfigQuiet(path)
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	return cfg
}

func TestBranding_UnsetKeepsTheDefaultNames(t *testing.T) {
	cfg := loadBrandingConfig(t, "")

	want := BrandingConfig{Name: "Keycloak Monitoring Tool", ShortName: "KMT", LogoText: "Keycloak"}
	if cfg.Branding != want {
		t.Errorf("Branding = %+v, want %+v", cfg.Branding, want)
	}
	if cfg.Notifications.ProductName != "Keycloak Monitoring Tool" {
		t.Errorf("Notifications.ProductName = %q", cfg.Notifications.ProductName)
	}
	if cfg.Notifications.Slack.Username != "Keycloak Monitoring Tool" {
		t.Errorf("Slack.Username = %q, want the product name", cfg.Notifications.Slack.Username)
	}
	if got, want := cfg.Notifications.Email.Subject, "[{{severity}}] {{product}} Alert: {{title}}"; got != want {
		t.Errorf("Email.Subject = %q, want %q", got, want)
	}
}

func TestBranding_FromTheConfigFile(t *testing.T) {
	cfg := loadBrandingConfig(t, `
branding:
  name: "Acme Monitoring Platform"
  short_name: "ACME"
  logo_text: "ACME"
`)

	want := BrandingConfig{Name: "Acme Monitoring Platform", ShortName: "ACME", LogoText: "ACME"}
	if cfg.Branding != want {
		t.Errorf("Branding = %+v, want %+v", cfg.Branding, want)
	}
	if cfg.Notifications.ProductName != want.Name {
		t.Errorf("Notifications.ProductName = %q, want %q", cfg.Notifications.ProductName, want.Name)
	}
	if cfg.Notifications.Slack.Username != want.Name {
		t.Errorf("Slack.Username = %q, want %q", cfg.Notifications.Slack.Username, want.Name)
	}
}

func TestBranding_AnExplicitSlackUsernameIsKept(t *testing.T) {
	cfg := loadBrandingConfig(t, `
branding:
  name: "Acme Monitoring Platform"
notifications:
  slack:
    username: "Alerts Bot"
`)

	if cfg.Notifications.Slack.Username != "Alerts Bot" {
		t.Errorf("Slack.Username = %q, want the configured one", cfg.Notifications.Slack.Username)
	}
}

func TestBranding_FromEnvironmentVariables(t *testing.T) {
	t.Setenv("MONITORING_BRANDING_NAME", "Acme Monitoring Platform")
	t.Setenv("MONITORING_BRANDING_SHORT_NAME", "ACME")
	t.Setenv("MONITORING_BRANDING_LOGO_TEXT", "ACME")
	cfg := loadBrandingConfig(t, "")

	want := BrandingConfig{Name: "Acme Monitoring Platform", ShortName: "ACME", LogoText: "ACME"}
	if cfg.Branding != want {
		t.Errorf("Branding = %+v, want %+v", cfg.Branding, want)
	}
}

func TestBranding_BlankAndMultiLineValues(t *testing.T) {
	cfg := loadBrandingConfig(t, `
branding:
  name: "Acme\r\nBcc: someone@example.com"
  short_name: "   "
  logo_text: ""
`)

	if cfg.Branding.Name != "Acme Bcc: someone@example.com" {
		t.Errorf("Name = %q, want it on one line", cfg.Branding.Name)
	}
	if cfg.Branding.ShortName != "KMT" || cfg.Branding.LogoText != "Keycloak" {
		t.Errorf("blank names = %q/%q, want the defaults", cfg.Branding.ShortName, cfg.Branding.LogoText)
	}
}

func TestBranding_SlugAndFilePrefix(t *testing.T) {
	tests := []struct {
		branding   BrandingConfig
		slug, file string
	}{
		{BrandingConfig{}, "keycloak-monitoring-tool", "kmt"},
		{BrandingConfig{Name: "Keycloak Monitoring Tool", ShortName: "KMT"}, "keycloak-monitoring-tool", "kmt"},
		{BrandingConfig{Name: "Acme Monitoring Platform", ShortName: "ACME"}, "acme-monitoring-platform", "acme"},
		{BrandingConfig{Name: "  Acme -- Ops (EU)  ", ShortName: "Acme EU"}, "acme-ops-eu", "acme-eu"},
		{BrandingConfig{Name: "Überwachung 2", ShortName: "Ü"}, "berwachung-2", "kmt"},
		{BrandingConfig{Name: "***", ShortName: "***"}, "keycloak-monitoring-tool", "kmt"},
	}
	for _, tt := range tests {
		if got := tt.branding.Slug(); got != tt.slug {
			t.Errorf("%+v.Slug() = %q, want %q", tt.branding, got, tt.slug)
		}
		if got := tt.branding.FilePrefix(); got != tt.file {
			t.Errorf("%+v.FilePrefix() = %q, want %q", tt.branding, got, tt.file)
		}
	}
}
