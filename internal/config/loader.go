package config

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/spf13/viper"
)

// LoadConfig loads configuration using Viper, narrating every step to
// stdout, including a raw dump of the config file contents.
func LoadConfig(configPath string) (*AppConfig, error) {
	return loadConfig(configPath, os.Stdout)
}

// LoadConfigQuiet loads configuration without the debug narration. The
// -healthcheck probes must use this: the narration dumps raw config contents
// (secrets included) and Docker stores probe output in the container health
// log, which both leaks the secrets into `docker inspect` and truncates away
// the probe verdict at 4096 bytes.
func LoadConfigQuiet(configPath string) (*AppConfig, error) {
	return loadConfig(configPath, io.Discard)
}

// narrator emits the loader's debug narration. Write errors are swallowed
// on purpose: narration must never fail config loading, and the quiet path
// writes to io.Discard anyway.
type narrator struct{ out io.Writer }

func (n narrator) println(a ...any)               { _, _ = fmt.Fprintln(n.out, a...) }
func (n narrator) printf(format string, a ...any) { _, _ = fmt.Fprintf(n.out, format, a...) }

func loadConfig(configPath string, out io.Writer) (*AppConfig, error) {
	say := narrator{out: out}
	v := viper.New()

	say.println()
	say.println("╔══════════════════════════════════════════════════════════════════════════════╗")
	say.println("║                         CONFIGURATION LOADER DEBUG                           ║")
	say.println("╚══════════════════════════════════════════════════════════════════════════════╝")
	say.println()

	// Step 1: Determine config file path
	say.println("┌─────────────────────────────────────────────────────────────────────────────┐")
	say.println("│ STEP 1: Config File Resolution                                             │")
	say.println("└─────────────────────────────────────────────────────────────────────────────┘")

	if configPath != "" {
		say.printf("  ✓ Config path provided via CLI/env: %s\n", configPath)
		v.SetConfigFile(configPath)
	} else {
		say.println("  ⚠ No config path provided")
		say.println("  → Looking for 'config.yaml' in current directory...")
		cwd, _ := os.Getwd()
		say.printf("  → Current working directory: %s\n", cwd)
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
	}
	say.println()

	// Step 2: Set defaults
	say.println("┌─────────────────────────────────────────────────────────────────────────────┐")
	say.println("│ STEP 2: Setting Default Values                                             │")
	say.println("└─────────────────────────────────────────────────────────────────────────────┘")
	say.println("  ✓ Default values loaded (see setDefaults() for all values)")
	setDefaults(v)
	say.println()

	// Step 3: Read config file
	say.println("┌─────────────────────────────────────────────────────────────────────────────┐")
	say.println("│ STEP 3: Reading Config File                                                │")
	say.println("└─────────────────────────────────────────────────────────────────────────────┘")

	configFileLoaded := false
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			say.println("  ✗ Config file NOT FOUND")
			say.println("  → Will use defaults + environment variables only")
		} else {
			say.printf("  ✗ Error reading config file: %v\n", err)
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	} else {
		configFileLoaded = true
		say.printf("  ✓ Config file LOADED: %s\n", v.ConfigFileUsed())

		// Print RAW file contents for debugging
		say.println()
		say.println("  ┌─── RAW CONFIG FILE CONTENTS ───────────────────────────────────────────────┐")
		if rawContent, err := os.ReadFile(v.ConfigFileUsed()); err == nil {
			lines := strings.Split(string(rawContent), "\n")
			for i, line := range lines {
				if i < 30 { // Only show first 30 lines
					say.printf("  │ %3d: %s\n", i+1, line)
				}
			}
			if len(lines) > 30 {
				say.printf("  │ ... (%d more lines)\n", len(lines)-30)
			}
		} else {
			say.printf("  │ ERROR reading file: %v\n", err)
		}
		say.println("  └─────────────────────────────────────────────────────────────────────────────┘")
	}
	say.println()

	// Step 4: Environment variables
	say.println("┌─────────────────────────────────────────────────────────────────────────────┐")
	say.println("│ STEP 4: Environment Variables (DATABASE)                                   │")
	say.println("└─────────────────────────────────────────────────────────────────────────────┘")
	say.println("  → Prefix: MONITORING_")
	say.println("  → Example: MONITORING_DATABASE_HOST, MONITORING_DATABASE_PORT, etc.")
	say.println()

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
	say.println("  Database environment variables:")
	foundAny := false
	for _, envKey := range dbEnvVars {
		value := os.Getenv(envKey)
		if value != "" {
			foundAny = true
			if strings.Contains(strings.ToLower(envKey), "password") {
				value = "********"
			}
			say.printf("    ✓ %s = %s\n", envKey, value)
		} else {
			say.printf("    ✗ %s (not set)\n", envKey)
		}
	}
	if !foundAny {
		say.println("    → No database env vars set, will use config file or defaults")
	}
	say.println()

	// Unmarshal
	var config AppConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	config.MCP.RemovedKeys = removedMCPKeysInFile(v)

	// Step 5: Final configuration summary
	say.println("┌─────────────────────────────────────────────────────────────────────────────┐")
	say.println("│ STEP 5: Final Configuration (with source)                                  │")
	say.println("└─────────────────────────────────────────────────────────────────────────────┘")
	say.println()

	printConfigWithSource(say, v, &config, configFileLoaded)

	say.println()
	say.println("╔══════════════════════════════════════════════════════════════════════════════╗")
	say.println("║                      END CONFIGURATION LOADER DEBUG                          ║")
	say.println("╚══════════════════════════════════════════════════════════════════════════════╝")
	say.println()

	return &config, nil
}

// printConfigWithSource prints database configuration with source
func printConfigWithSource(say narrator, v *viper.Viper, config *AppConfig, configFileLoaded bool) {
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

	say.println("  ┌─── DATABASE CONFIG ─────────────────────────────────────────────────────────┐")
	say.printf("  │ host     = %-25s  ← %s\n", config.Database.Host, getSource("database.host"))
	say.printf("  │ port     = %-25d  ← %s\n", config.Database.Port, getSource("database.port"))
	say.printf("  │ database = %-25s  ← %s\n", config.Database.Database, getSource("database.database"))
	say.printf("  │ user     = %-25s  ← %s\n", config.Database.User, getSource("database.user"))
	if config.Database.Password != "" {
		say.printf("  │ password = %-25s  ← %s\n", "********", getSource("database.password"))
	} else {
		say.printf("  │ password = %-25s  ← %s\n", "(empty)", getSource("database.password"))
	}
	say.printf("  │ ssl_mode = %-25s  ← %s\n", config.Database.SSLMode, getSource("database.ssl_mode"))
	say.println("  └─────────────────────────────────────────────────────────────────────────────┘")
	say.println()
	say.printf("  → Connection string: postgres://%s:***@%s:%d/%s?sslmode=%s\n",
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
