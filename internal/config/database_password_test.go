package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDatabasePassword(t *testing.T) {
	for _, tc := range []struct {
		password      string
		wantErr       bool
		wantPublished bool
	}{
		{"", true, false},
		{"   ", true, false},
		{"${PMP_UNSET_DB_PASSWORD}", true, false},
		{"pre-${PMP_UNSET_DB_PASSWORD}", true, false},
		{ExampleDatabasePassword, false, true},
		{"MONITORING_PASSWORD", false, true},
		{"a-real-and-long-password", false, false},
	} {
		published, err := ValidateDatabasePassword("database.password", tc.password)
		if (err != nil) != tc.wantErr {
			t.Errorf("%q: err = %v, want error %v", tc.password, err, tc.wantErr)
		}
		if err != nil && !strings.Contains(err.Error(), "database.password") {
			t.Errorf("%q: error %q does not name the key", tc.password, err)
		}
		if published != tc.wantPublished {
			t.Errorf("%q: published = %v, want %v", tc.password, published, tc.wantPublished)
		}
	}
}

// The MCP variable only applies when config.yaml has the key, so the
// message must say so rather than send the reader to a variable they have
// already set.
func TestValidateDatabasePassword_MCPSaysTheKeyMustBePresent(t *testing.T) {
	_, err := ValidateDatabasePassword("mcp.database.password", "")
	if err == nil || !strings.Contains(err.Error(), "keep a password key") ||
		!strings.Contains(err.Error(), "MONITORING_MCP_DATABASE_PASSWORD") {
		t.Fatalf("err = %v, want it to say the password key must stay in config.yaml", err)
	}
}

// No password is built in: a config that sets none loads with none, so
// startup refuses it instead of connecting with a password anyone can read.
func TestLoadConfig_HasNoBuiltInDatabasePassword(t *testing.T) {
	cfg, err := LoadConfigQuiet(writeConfig(t, "database:\n  host: \"db\"\n"))
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	if cfg.Database.Password != "" {
		t.Fatalf("database.password = %q with none configured, want empty", cfg.Database.Password)
	}
}

// The environment variable still sets it when config.yaml has no password
// key: the key stays registered, only its default is gone.
func TestLoadConfig_DatabasePasswordFromEnvironmentAlone(t *testing.T) {
	t.Setenv("MONITORING_DATABASE_PASSWORD", "from-the-environment")
	cfg, err := LoadConfigQuiet(writeConfig(t, "database:\n  host: \"db\"\n"))
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	if cfg.Database.Password != "from-the-environment" {
		t.Fatalf("database.password = %q, want the environment's", cfg.Database.Password)
	}
}

// Copying the example as it is must not silence alerts, turn off database
// encryption, or carry a literal secret.
func TestShippedExample_IsSafeToCopy(t *testing.T) {
	example, err := os.ReadFile(filepath.Join("..", "..", "config.yaml.example"))
	if err != nil {
		t.Fatalf("read config.yaml.example: %v", err)
	}
	t.Setenv("KC_STAGING_CLIENT_SECRET", "x")
	t.Setenv("PROD_KEYCLOAK_CLIENT_SECRET", "x")
	cfg, err := LoadConfigQuiet(writeConfig(t, string(example)))
	if err != nil {
		t.Fatalf("config.yaml.example: %v", err)
	}
	for _, s := range cfg.AlertSilences {
		if s.Enabled {
			t.Errorf("silence %q is enabled; a copied example would silence realm %q", s.ID, s.Matchers.RealmName)
		}
	}
	if cfg.Database.SSLMode != "require" {
		t.Errorf("database.ssl_mode = %q, want require", cfg.Database.SSLMode)
	}
	if cfg.MCP.Database != nil && cfg.MCP.Database.SSLMode != "require" {
		t.Errorf("mcp.database.ssl_mode = %q, want require", cfg.MCP.Database.SSLMode)
	}
	if cfg.Database.Password != "" {
		t.Errorf("database.password = %q, want none in the example", cfg.Database.Password)
	}
	if cfg.Auth.OAuth2.ClientSecret != "" {
		t.Errorf("auth.oauth2.client_secret = %q, want none in the example", cfg.Auth.OAuth2.ClientSecret)
	}
	for id, tenant := range cfg.Keycloak.Tenants {
		if tenant.ClientSecret != "x" {
			t.Errorf("tenant %s client_secret = %q, want it from an environment variable", id, tenant.ClientSecret)
		}
	}
}
