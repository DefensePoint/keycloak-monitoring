package local

import (
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"testing"
)

func TestBundledDemoConfigUsesEnvironmentSecrets(t *testing.T) {
	t.Setenv("BUNDLED_DB_PASSWORD", "db-$value")
	t.Setenv("BUNDLED_KC1_CLIENT_SECRET", "client-one-$value")
	t.Setenv("BUNDLED_KC2_CLIENT_SECRET", "client-two-$value")
	t.Setenv("BUNDLED_SESSION_SECRET", "session-$value")
	t.Setenv("MONITORING_AUTH_SIMPLE_DEFAULT_USER", "demo-operator")
	t.Setenv("MONITORING_AUTH_SIMPLE_DEFAULT_PASS", "Demo!Qx7nK2-2026")
	t.Setenv("MONITORING_AUTH_SIMPLE_DEFAULT_EMAIL", "demo@example.com")
	cfg, err := config.LoadConfigQuiet("config.bundled.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PlaceholderWarnings) != 0 {
		t.Fatalf("unresolved placeholders: %v", cfg.PlaceholderWarnings)
	}
	if cfg.Database.Host != "postgres" || cfg.Database.Password != "db-$value" {
		t.Fatal("demo database configuration did not resolve")
	}
	if cfg.Auth.Session.Secret != "session-$value" || cfg.Auth.Session.Secure {
		t.Fatal("demo session configuration did not resolve for local HTTP")
	}
	if cfg.Auth.Simple.DefaultUser != "demo-operator" || cfg.Auth.Simple.DefaultPass != "Demo!Qx7nK2-2026" {
		t.Fatal("demo admin bootstrap configuration did not resolve")
	}
	if len(cfg.Keycloak.Tenants) != 2 {
		t.Fatalf("expected two demo tenants, got %d", len(cfg.Keycloak.Tenants))
	}
	for id, want := range map[string]struct{ url, secret string }{
		"demo1": {"http://keycloak-1:8080", "client-one-$value"},
		"demo2": {"http://keycloak-2:8080", "client-two-$value"},
	} {
		tenant := cfg.Keycloak.Tenants[id]
		if !tenant.Enabled || tenant.ServerURL != want.url || tenant.ClientSecret != want.secret || tenant.ClientID != "monitoring-service" {
			t.Fatalf("tenant %s did not resolve its monitoring configuration", id)
		}
	}
	if !cfg.Keycloak.Global.Connection.AllowPrivateRanges {
		t.Fatal("demo must allow its internal Compose service addresses")
	}
	if cfg.Web.Server.Port != 7880 || cfg.Web.Server.APIHostURL != "http://api-server:7888" {
		t.Fatal("demo web proxy configuration is incorrect")
	}
}
