package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/spf13/viper"
)

// LoadConfig loads configuration using Viper
func LoadConfig(configPath string) (*AppConfig, error) {
	v := viper.New()

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                         CONFIGURATION LOADER DEBUG                           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Step 1: Determine config file path
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ STEP 1: Config File Resolution                                             │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	if configPath != "" {
		fmt.Printf("  ✓ Config path provided via CLI/env: %s\n", configPath)
		v.SetConfigFile(configPath)
	} else {
		fmt.Println("  ⚠ No config path provided")
		fmt.Println("  → Looking for 'config.yaml' in current directory...")
		cwd, _ := os.Getwd()
		fmt.Printf("  → Current working directory: %s\n", cwd)
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
	}
	fmt.Println()

	// Step 2: Set defaults
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ STEP 2: Setting Default Values                                             │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println("  ✓ Default values loaded (see setDefaults() for all values)")
	setDefaults(v)
	fmt.Println()

	// Step 3: Read config file
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ STEP 3: Reading Config File                                                │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	configFileLoaded := false
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Println("  ✗ Config file NOT FOUND")
			fmt.Println("  → Will use defaults + environment variables only")
		} else {
			fmt.Printf("  ✗ Error reading config file: %v\n", err)
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	} else {
		configFileLoaded = true
		fmt.Printf("  ✓ Config file LOADED: %s\n", v.ConfigFileUsed())

		// Print RAW file contents for debugging
		fmt.Println()
		fmt.Println("  ┌─── RAW CONFIG FILE CONTENTS ───────────────────────────────────────────────┐")
		if rawContent, err := os.ReadFile(v.ConfigFileUsed()); err == nil {
			lines := strings.Split(string(rawContent), "\n")
			for i, line := range lines {
				if i < 30 { // Only show first 30 lines
					fmt.Printf("  │ %3d: %s\n", i+1, line)
				}
			}
			if len(lines) > 30 {
				fmt.Printf("  │ ... (%d more lines)\n", len(lines)-30)
			}
		} else {
			fmt.Printf("  │ ERROR reading file: %v\n", err)
		}
		fmt.Println("  └─────────────────────────────────────────────────────────────────────────────┘")
	}
	fmt.Println()

	// Step 4: Environment variables
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ STEP 4: Environment Variables (DATABASE)                                   │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println("  → Prefix: MONITORING_")
	fmt.Println("  → Example: MONITORING_DATABASE_HOST, MONITORING_DATABASE_PORT, etc.")
	fmt.Println()

	v.AutomaticEnv()
	v.SetEnvPrefix("MONITORING")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Show only DATABASE env vars
	dbEnvVars := []string{
		"MONITORING_DATABASE_HOST",
		"MONITORING_DATABASE_PORT",
		"MONITORING_DATABASE_DATABASE",
		"MONITORING_DATABASE_USER",
		"MONITORING_DATABASE_PASSWORD",
		"MONITORING_DATABASE_SSL_MODE",
	}
	fmt.Println("  Database environment variables:")
	foundAny := false
	for _, envKey := range dbEnvVars {
		value := os.Getenv(envKey)
		if value != "" {
			foundAny = true
			if strings.Contains(strings.ToLower(envKey), "password") {
				value = "********"
			}
			fmt.Printf("    ✓ %s = %s\n", envKey, value)
		} else {
			fmt.Printf("    ✗ %s (not set)\n", envKey)
		}
	}
	if !foundAny {
		fmt.Println("    → No database env vars set, will use config file or defaults")
	}
	fmt.Println()

	// Unmarshal
	var config AppConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	config.MCP.RemovedKeys = removedMCPKeysInFile(v)

	// Step 5: Final configuration summary
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ STEP 5: Final Configuration (with source)                                  │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	printConfigWithSource(v, &config, configFileLoaded)

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                      END CONFIGURATION LOADER DEBUG                          ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	return &config, nil
}

// printConfigWithSource prints database configuration with source
func printConfigWithSource(v *viper.Viper, config *AppConfig, configFileLoaded bool) {
	getSource := func(key string) string {
		envKey := "MONITORING_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
		if os.Getenv(envKey) != "" {
			return "ENV VAR (" + envKey + ")"
		}
		if configFileLoaded && v.IsSet(key) {
			return "CONFIG FILE"
		}
		return "DEFAULT"
	}

	fmt.Println("  ┌─── DATABASE CONFIG ─────────────────────────────────────────────────────────┐")
	fmt.Printf("  │ host     = %-25s  ← %s\n", config.Database.Host, getSource("database.host"))
	fmt.Printf("  │ port     = %-25d  ← %s\n", config.Database.Port, getSource("database.port"))
	fmt.Printf("  │ database = %-25s  ← %s\n", config.Database.Database, getSource("database.database"))
	fmt.Printf("  │ user     = %-25s  ← %s\n", config.Database.User, getSource("database.user"))
	if config.Database.Password != "" {
		fmt.Printf("  │ password = %-25s  ← %s\n", "********", getSource("database.password"))
	} else {
		fmt.Printf("  │ password = %-25s  ← %s\n", "(empty)", getSource("database.password"))
	}
	fmt.Printf("  │ ssl_mode = %-25s  ← %s\n", config.Database.SSLMode, getSource("database.ssl_mode"))
	fmt.Println("  └─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Printf("  → Connection string: postgres://%s:***@%s:%d/%s?sslmode=%s\n",
		config.Database.User, config.Database.Host, config.Database.Port, config.Database.Database, config.Database.SSLMode)
}

func setDefaults(v *viper.Viper) {
	// =============================================================================
	// HTTP API Server Defaults
	// =============================================================================
	v.SetDefault("http.server.enabled", true)
	v.SetDefault("http.server.host", "0.0.0.0")
	v.SetDefault("http.server.port", 7888)

	// =============================================================================
	// Web Frontend Server Defaults
	// =============================================================================
	v.SetDefault("web.server.enabled", true)
	v.SetDefault("web.server.host", "0.0.0.0")
	v.SetDefault("web.server.port", 7880)
	v.SetDefault("web.server.static_dir", "web/dist")
	v.SetDefault("web.server.api_host_url", "http://localhost:7888")

	// =============================================================================
	// Database Defaults
	// =============================================================================
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.database", "monitoring")
	v.SetDefault("database.user", "monitoring")
	v.SetDefault("database.password", "monitoring_password")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_conns", 25)
	v.SetDefault("database.min_conns", 5)
	v.SetDefault("database.timeout", "30s")

	// =============================================================================
	// Security Defaults
	// =============================================================================
	// Empty by default (client_secret stored as plaintext). Viper's
	// AutomaticEnv only binds MONITORING_SECURITY_ENCRYPTION_KEY once the key
	// is registered here or present in config.yaml — an env var alone,
	// without either, is silently ignored.
	v.SetDefault("security.encryption_key", "")
	v.SetDefault("security.run_secret_migration", false)

	// =============================================================================
	// Authentication Defaults
	// =============================================================================
	// Simple Auth
	v.SetDefault("auth.simple.enabled", true)
	v.SetDefault("auth.simple.default_user", "admin")
	v.SetDefault("auth.simple.default_email", "admin@example.com")

	// OAuth2/OIDC
	v.SetDefault("auth.oauth2.enabled", false)
	v.SetDefault("auth.oauth2.skip_issuer_check", false)
	v.SetDefault("auth.oauth2.skip_expiry_check", false)
	v.SetDefault("auth.oauth2.scopes", []string{"openid", "profile", "email"})

	// Session
	v.SetDefault("auth.session.name", "monitoring_session")
	v.SetDefault("auth.session.max_age", "24h")
	v.SetDefault("auth.session.secure", false)
	v.SetDefault("auth.session.same_site", "lax")
	v.SetDefault("auth.session.state_max_age", "10m")

	// HTTP security headers — defaults follow OWASP secure headers guidance.
	// HSTS stays empty by default since enabling it on an HTTP-only deployment
	// would break access; operators must opt in once HTTPS is mandatory.
	v.SetDefault("http.security_headers.enabled", true)
	v.SetDefault("http.security_headers.hsts", "")
	v.SetDefault("http.security_headers.content_security_policy",
		"default-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
	v.SetDefault("http.security_headers.x_content_type_options", "nosniff")
	v.SetDefault("http.security_headers.x_frame_options", "DENY")
	v.SetDefault("http.security_headers.referrer_policy", "strict-origin-when-cross-origin")

	// =============================================================================
	// Logging Defaults
	// =============================================================================
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output.pretty", false)

	// =============================================================================
	// Global Keycloak Monitoring Defaults
	// =============================================================================
	v.SetDefault("keycloak.global.infinispan_enabled", false)
	v.SetDefault("keycloak.global.polling.metrics_interval", "5m")
	v.SetDefault("keycloak.global.polling.events_interval", "30s")
	v.SetDefault("keycloak.global.polling.health_interval", "1m")
	v.SetDefault("keycloak.global.polling.realm_info_interval", "15m")
	v.SetDefault("keycloak.global.events.max_events_per_poll", 1000)
	v.SetDefault("keycloak.global.events.lookback_duration", "1h")
	v.SetDefault("keycloak.global.connection.timeout", "30s")
	v.SetDefault("keycloak.global.connection.skip_tls_verify", false)
	v.SetDefault("keycloak.global.connection.max_retries", 3)
	v.SetDefault("keycloak.global.connection.retry_backoff", "5s")
	// SSRF guard: on-prem deployments need to reach Keycloak on internal
	// networks, so private IP ranges are allowed by default. Cloud-hosted
	// deployments can flip this to false for extra hardening.
	v.SetDefault("keycloak.global.connection.allow_private_ranges", true)

	// =============================================================================
	// Global Keycloak Configuration Checker Defaults
	// =============================================================================
	v.SetDefault("keycloak.global.config_checker.poll_interval", "5m")

	// Identity Provider Metadata Check defaults
	v.SetDefault("keycloak.global.config_checker.checks.identity_provider_metadata.enabled", true)
	v.SetDefault("keycloak.global.config_checker.checks.identity_provider_metadata.check_saml_only", true)
	v.SetDefault("keycloak.global.config_checker.checks.identity_provider_metadata.excluded_realms", []string{})
	v.SetDefault("keycloak.global.config_checker.checks.identity_provider_metadata.excluded_providers", []string{})
	v.SetDefault("keycloak.global.config_checker.checks.identity_provider_metadata.certificate_expiration_warning_days", 30)
	v.SetDefault("keycloak.global.config_checker.checks.identity_provider_metadata.certificate_check_timeout", "30s")

	// Realm Security Check defaults
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.enabled", true)
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.check_ssl_required", true)
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.check_brute_force", true)
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.check_password_policy", true)
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.check_email_verification", true)
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.check_admin_events", true)
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.check_user_events", true)
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.check_duplicate_emails", true)
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.min_password_length", 8)
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.require_password_complexity", true)
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.max_access_token_lifespan_minutes", 15)
	v.SetDefault("keycloak.global.config_checker.checks.realm_security.max_sso_session_idle_minutes", 30)

	// Client Security Check defaults
	v.SetDefault("keycloak.global.config_checker.checks.client_security.enabled", true)
	v.SetDefault("keycloak.global.config_checker.checks.client_security.check_redirect_uris", true)
	v.SetDefault("keycloak.global.config_checker.checks.client_security.check_public_clients", true)
	v.SetDefault("keycloak.global.config_checker.checks.client_security.check_direct_access_grants", true)
	v.SetDefault("keycloak.global.config_checker.checks.client_security.check_client_secret", true)
	v.SetDefault("keycloak.global.config_checker.checks.client_security.check_implicit_flow", true)
	v.SetDefault("keycloak.global.config_checker.checks.client_security.check_standard_flow", false)
	v.SetDefault("keycloak.global.config_checker.checks.client_security.allow_localhost_redirects", false)
	v.SetDefault("keycloak.global.config_checker.checks.client_security.production_environment", true)

	// =============================================================================
	// Global Keycloak Version Checker Defaults
	// =============================================================================
	// Off by default: the version checker is the only component that calls a
	// host we do not operate, and this tool must run airgapped.
	v.SetDefault("keycloak.global.version_checker.enabled", false)
	v.SetDefault("keycloak.global.version_checker.cache_ttl", "1h")
	v.SetDefault("keycloak.global.version_checker.http_timeout", "10s")
	v.SetDefault("keycloak.global.version_checker.github_api_base_url", "https://api.github.com")
	v.SetDefault("keycloak.global.version_checker.keycloak_owner", "keycloak")
	v.SetDefault("keycloak.global.version_checker.keycloak_repo", "keycloak")

	// =============================================================================
	// Notification Integrations Defaults
	// =============================================================================
	// Base URL for Keycloak Monitoring Tool web UI (used in notification links)
	v.SetDefault("notifications.base_url", "")

	// GitLab defaults
	v.SetDefault("notifications.gitlab.enabled", false)
	v.SetDefault("notifications.gitlab.url", "https://gitlab.com")
	v.SetDefault("notifications.gitlab.token", "")
	v.SetDefault("notifications.gitlab.project_id", "")
	v.SetDefault("notifications.gitlab.milestone", "")
	v.SetDefault("notifications.gitlab.min_severity", "critical") // Only create GitLab issues for critical

	// Slack defaults
	v.SetDefault("notifications.slack.enabled", false)
	v.SetDefault("notifications.slack.webhook_url", "")
	v.SetDefault("notifications.slack.channel", "")
	v.SetDefault("notifications.slack.username", "Keycloak Monitoring Tool")
	v.SetDefault("notifications.slack.icon_emoji", ":shield:")
	v.SetDefault("notifications.slack.min_severity", "warning") // Only send Slack messages for warning and above

	// Email defaults
	v.SetDefault("notifications.email.enabled", false)
	v.SetDefault("notifications.email.smtp_host", "smtp.gmail.com")
	v.SetDefault("notifications.email.smtp_port", 587)
	v.SetDefault("notifications.email.username", "")
	v.SetDefault("notifications.email.password", "")
	v.SetDefault("notifications.email.from", "alerts@example.com")
	v.SetDefault("notifications.email.to", []string{"admin@example.com"})
	v.SetDefault("notifications.email.use_tls", true)
	v.SetDefault("notifications.email.skip_verify", false)
	v.SetDefault("notifications.email.subject", "[{{severity}}] Keycloak Monitoring Tool Alert: {{title}}")
	v.SetDefault("notifications.email.min_severity", "error") // Only send emails for error and above

	setMCPDefaults(v)
}

// NewLogger creates a logger from logging configuration
func (c *AppConfig) NewLogger() *logger.Logger {
	pretty := c.Logging.Output.Pretty
	if c.Logging.Format == "console" || pretty {
		return logger.NewConsole(c.Logging.Level, pretty)
	}
	return logger.NewJSON(c.Logging.Level)
}
