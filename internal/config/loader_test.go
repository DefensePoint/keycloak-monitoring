package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Create a temporary empty config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write minimal config
	err := os.WriteFile(configPath, []byte(""), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Test HTTP server defaults
	if config.HTTP.Server.Host != "0.0.0.0" {
		t.Errorf("Expected HTTP host to be 0.0.0.0, got %s", config.HTTP.Server.Host)
	}
	if config.HTTP.Server.Port != 7888 {
		t.Errorf("Expected HTTP port to be 7888, got %d", config.HTTP.Server.Port)
	}

	// Test Web server defaults
	if config.Web.Server.Port != 7880 {
		t.Errorf("Expected Web port to be 7880, got %d", config.Web.Server.Port)
	}
	if config.Web.Server.Host != "0.0.0.0" {
		t.Errorf("Expected Web host to be 0.0.0.0, got %s", config.Web.Server.Host)
	}

	// Test Database defaults
	if config.Database.Host != "localhost" {
		t.Errorf("Expected DB host to be localhost, got %s", config.Database.Host)
	}
	if config.Database.Port != 5432 {
		t.Errorf("Expected DB port to be 5432, got %d", config.Database.Port)
	}
	if config.Database.MaxConns != 25 {
		t.Errorf("Expected DB max_conns to be 25, got %d", config.Database.MaxConns)
	}

	// Test Auth defaults
	if !config.Auth.Simple.Enabled {
		t.Error("Expected simple auth to be enabled by default")
	}
	if config.Auth.OAuth2.Enabled {
		t.Error("Expected OAuth2 to be disabled by default")
	}

	// Test Logging defaults
	if config.Logging.Level != "info" {
		t.Errorf("Expected logging level to be info, got %s", config.Logging.Level)
	}
	if config.Logging.Format != "json" {
		t.Errorf("Expected logging format to be json, got %s", config.Logging.Format)
	}
}

func TestLoadConfig_CustomValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
http:
  server:
    host: "127.0.0.1"
    port: 8080

database:
  host: "db.example.com"
  port: 5433
  database: "testdb"
  user: "testuser"
  max_conns: 50

logging:
  level: "debug"
  format: "console"
  output:
    pretty: true

auth:
  simple:
    enabled: false
  oauth2:
    enabled: true
    provider_url: "https://auth.example.com"
    client_id: "test-client"

keycloak:
  global:
    realms:
      - realm1
      - realm2
  tenants:
    test-tenant:
      name: "Test Tenant"
      server_url: "https://keycloak.example.com"
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "test-secret"
      enabled: true
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Test custom HTTP values
	if config.HTTP.Server.Host != "127.0.0.1" {
		t.Errorf("Expected HTTP host 127.0.0.1, got %s", config.HTTP.Server.Host)
	}
	if config.HTTP.Server.Port != 8080 {
		t.Errorf("Expected HTTP port 8080, got %d", config.HTTP.Server.Port)
	}

	// Test custom database values
	if config.Database.Host != "db.example.com" {
		t.Errorf("Expected DB host db.example.com, got %s", config.Database.Host)
	}
	if config.Database.Port != 5433 {
		t.Errorf("Expected DB port 5433, got %d", config.Database.Port)
	}
	if config.Database.MaxConns != 50 {
		t.Errorf("Expected DB max_conns 50, got %d", config.Database.MaxConns)
	}

	// Test custom logging values
	if config.Logging.Level != "debug" {
		t.Errorf("Expected logging level debug, got %s", config.Logging.Level)
	}
	if config.Logging.Format != "console" {
		t.Errorf("Expected logging format console, got %s", config.Logging.Format)
	}
	if !config.Logging.Output.Pretty {
		t.Error("Expected pretty logging to be enabled")
	}

	// Test custom auth values
	if config.Auth.Simple.Enabled {
		t.Error("Expected simple auth to be disabled")
	}
	if !config.Auth.OAuth2.Enabled {
		t.Error("Expected OAuth2 to be enabled")
	}
	if config.Auth.OAuth2.ProviderURL != "https://auth.example.com" {
		t.Errorf("Expected provider URL https://auth.example.com, got %s", config.Auth.OAuth2.ProviderURL)
	}

	// Test custom Keycloak values (new hierarchical structure)
	if len(config.Keycloak.Global.Realms) != 2 {
		t.Errorf("Expected 2 realms in global config, got %d", len(config.Keycloak.Global.Realms))
	}
	if len(config.Keycloak.Tenants) != 1 {
		t.Errorf("Expected 1 tenant, got %d", len(config.Keycloak.Tenants))
	}
	tenant, exists := config.Keycloak.Tenants["test-tenant"]
	if !exists {
		t.Error("Expected test-tenant to exist in tenants map")
	}
	if tenant.ServerURL != "https://keycloak.example.com" {
		t.Errorf("Expected tenant server_url https://keycloak.example.com, got %s", tenant.ServerURL)
	}
}

func TestLoadConfig_EnvironmentVariables(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write minimal config
	err := os.WriteFile(configPath, []byte(""), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variables
	if err := os.Setenv("MONITORING_HTTP_SERVER_PORT", "9999"); err != nil {
		t.Fatalf("Failed to set env var: %v", err)
	}
	if err := os.Setenv("MONITORING_DATABASE_HOST", "env-db-host"); err != nil {
		t.Fatalf("Failed to set env var: %v", err)
	}
	if err := os.Setenv("MONITORING_LOGGING_LEVEL", "debug"); err != nil {
		t.Fatalf("Failed to set env var: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("MONITORING_HTTP_SERVER_PORT")
		_ = os.Unsetenv("MONITORING_DATABASE_HOST")
		_ = os.Unsetenv("MONITORING_LOGGING_LEVEL")
	}()

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Environment variables should override defaults
	if config.HTTP.Server.Port != 9999 {
		t.Errorf("Expected HTTP port from env 9999, got %d", config.HTTP.Server.Port)
	}
	if config.Database.Host != "env-db-host" {
		t.Errorf("Expected DB host from env env-db-host, got %s", config.Database.Host)
	}
	if config.Logging.Level != "debug" {
		t.Errorf("Expected logging level from env debug, got %s", config.Logging.Level)
	}
}

func TestLoadConfig_NonExistentFile(t *testing.T) {
	// Loading with non-existent file path should fail
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("LoadConfig() should fail for non-existent file path")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write invalid YAML
	invalidYAML := `
http:
  server:
    port: "not a number"
  - invalid: syntax
`

	err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("LoadConfig() should fail for invalid YAML")
	}
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	// When path is empty, should look for config.yaml in current directory
	// This might not exist, so we just verify it doesn't panic
	_, err := LoadConfig("")
	// Should either succeed or fail gracefully, but not panic
	_ = err
}

func TestLoadConfig_RemovedMCPPageSizeKey(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
mcp:
  enabled: true
  max_page_size: 50
  absolute_max_page_size: 500
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	removedErr := config.MCP.RemovedKeyError()
	if removedErr == nil {
		t.Fatal("a config still carrying mcp.absolute_max_page_size must not be usable")
	}
	for _, want := range []string{"mcp.default_page_size", "mcp.max_page_size"} {
		if !strings.Contains(removedErr.Error(), want) {
			t.Errorf("error = %v, want it to name %s", removedErr, want)
		}
	}
}

func TestLoadConfig_CurrentMCPPageSizeKeys(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
mcp:
  enabled: true
  default_page_size: 100
  max_page_size: 500
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if err := config.MCP.RemovedKeyError(); err != nil {
		t.Fatalf("the current key names must load cleanly: %v", err)
	}
	if config.MCP.DefaultPageSize != 100 {
		t.Errorf("Expected default_page_size 100, got %d", config.MCP.DefaultPageSize)
	}
	if config.MCP.MaxPageSize != 500 {
		t.Errorf("Expected max_page_size 500, got %d", config.MCP.MaxPageSize)
	}
}

func TestAppConfig_NewLogger(t *testing.T) {
	tests := []struct {
		name           string
		config         AppConfig
		expectedFormat string // We can't check logger type directly, but we can verify it doesn't panic
	}{
		{
			name: "JSON logger",
			config: AppConfig{
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
					Output: LogOutputConfig{Pretty: false},
				},
			},
			expectedFormat: "json",
		},
		{
			name: "Console logger",
			config: AppConfig{
				Logging: LoggingConfig{
					Level:  "debug",
					Format: "console",
					Output: LogOutputConfig{Pretty: true},
				},
			},
			expectedFormat: "console",
		},
		{
			name: "JSON with pretty (should create console)",
			config: AppConfig{
				Logging: LoggingConfig{
					Level:  "warn",
					Format: "json",
					Output: LogOutputConfig{Pretty: true},
				},
			},
			expectedFormat: "console",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := tt.config.NewLogger()
			if logger == nil {
				t.Error("NewLogger() returned nil")
			}
			// Just verify it doesn't panic and returns a logger
		})
	}
}

func TestDatabaseConfig_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
database:
  timeout: 45s
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if config.Database.Timeout != 45*time.Second {
		t.Errorf("Expected timeout 45s, got %v", config.Database.Timeout)
	}
}

func TestSessionConfig_Durations(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
auth:
  session:
    max_age: 2h
    state_max_age: 15m
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if config.Auth.Session.MaxAge != 2*time.Hour {
		t.Errorf("Expected max_age 2h, got %v", config.Auth.Session.MaxAge)
	}
	if config.Auth.Session.StateMaxAge != 15*time.Minute {
		t.Errorf("Expected state_max_age 15m, got %v", config.Auth.Session.StateMaxAge)
	}
}

func TestKeycloakConfig_Polling(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
keycloak:
  global:
    polling:
      metrics_interval: 10m
      events_interval: 1m
      health_interval: 2m
      realm_info_interval: 30m
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if config.Keycloak.Global.Polling.MetricsInterval != 10*time.Minute {
		t.Errorf("Expected metrics_interval 10m, got %v", config.Keycloak.Global.Polling.MetricsInterval)
	}
	if config.Keycloak.Global.Polling.EventsInterval != 1*time.Minute {
		t.Errorf("Expected events_interval 1m, got %v", config.Keycloak.Global.Polling.EventsInterval)
	}
}

func TestOAuth2Config_Scopes(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
auth:
  oauth2:
    scopes:
      - openid
      - profile
      - email
      - groups
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	expectedScopes := []string{"openid", "profile", "email", "groups"}
	if len(config.Auth.OAuth2.Scopes) != len(expectedScopes) {
		t.Errorf("Expected %d scopes, got %d", len(expectedScopes), len(config.Auth.OAuth2.Scopes))
	}

	for i, scope := range expectedScopes {
		if i >= len(config.Auth.OAuth2.Scopes) || config.Auth.OAuth2.Scopes[i] != scope {
			t.Errorf("Expected scope %s at index %d, got %v", scope, i, config.Auth.OAuth2.Scopes)
		}
	}
}

// TestLoadConfig_VersionCheckerOffByDefault pins the airgap default. The
// version checker is the only component that contacts a host DefensePoint does
// not operate, so an empty config, or one that omits the key, must leave it
// off. A default of true would reach api.github.com in every deployment,
// including airgapped ones.
func TestLoadConfig_VersionCheckerOffByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if config.Keycloak.Global.VersionChecker.Enabled {
		t.Error("version checker is enabled by default; it must be opt-in")
	}
}

// TestLoadConfig_VersionCheckerOptIn checks the flag is actually readable from
// the file, so a deployment that wants version checking can turn it on.
func TestLoadConfig_VersionCheckerOptIn(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	body := []byte("keycloak:\n  global:\n    version_checker:\n      enabled: true\n")
	if err := os.WriteFile(configPath, body, 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if !config.Keycloak.Global.VersionChecker.Enabled {
		t.Error("version_checker.enabled: true was not read from the config file")
	}
}
