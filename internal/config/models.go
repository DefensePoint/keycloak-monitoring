package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// AppConfig represents the complete application configuration
type AppConfig struct {
	Database      DatabaseConfig      `mapstructure:"database"`
	HTTP          HTTPConfig          `mapstructure:"http"`
	Web           WebConfig           `mapstructure:"web"`
	Auth          AuthConfig          `mapstructure:"auth"`
	Logging       LoggingConfig       `mapstructure:"logging"`
	Keycloak      KeycloakConfig      `mapstructure:"keycloak"`
	Notifications NotificationsConfig `mapstructure:"notifications"`
	AlertSilences AlertSilencesConfig `mapstructure:"alert_silences"`
	Security      SecurityConfig      `mapstructure:"security"`
	MCP           MCPConfig           `mapstructure:"mcp"`
}

// SecurityConfig holds settings for protecting stored secrets.
type SecurityConfig struct {
	// EncryptionKey is a base64-encoded 32-byte AES-256 key used to encrypt
	// tenant client_secret values at rest (pkg/secretcrypto). Generate with:
	// openssl rand -base64 32
	EncryptionKey string `mapstructure:"encryption_key"`

	// RunSecretMigration triggers a one-time pass, on this boot only, that
	// encrypts every tenant's client_secret that isn't already encrypted.
	// Only takes effect when EncryptionKey is also set. Deliberately opt-in
	// per boot rather than automatic: an environment shouldn't silently
	// re-encrypt every tenant just because the key happens to be configured.
	// Meant to be set once, for the boot that performs the migration, then
	// unset again.
	RunSecretMigration bool `mapstructure:"run_secret_migration"`
}

// DatabaseConfig represents PostgreSQL database configuration
type DatabaseConfig struct {
	Host     string        `mapstructure:"host"`
	Port     int           `mapstructure:"port"`
	Database string        `mapstructure:"database"`
	User     string        `mapstructure:"user"`
	Password string        `mapstructure:"password"`
	SSLMode  string        `mapstructure:"ssl_mode"`
	MaxConns int           `mapstructure:"max_conns"`
	MinConns int           `mapstructure:"min_conns"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// HTTPConfig represents HTTP API configuration
type HTTPConfig struct {
	Server          HTTPServerConfig      `mapstructure:"server"`
	SecurityHeaders SecurityHeadersConfig `mapstructure:"security_headers"`
	Metrics         MetricsConfig         `mapstructure:"metrics"`
}

// MetricsConfig controls the Prometheus /metrics endpoint. The endpoint
// exposes tenant names and infrastructure topology, so it must not be routed
// through a public reverse proxy.
type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled"`

	// AuthToken, when set, requires callers to present it as a bearer token.
	AuthToken string `mapstructure:"auth_token"`
}

// HTTPServerConfig represents HTTP server configuration
type HTTPServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// SecurityHeadersConfig controls the response security headers added to every
// HTTP response. Empty values for individual headers disable that header.
// HSTS should only be enabled once the deployment is exclusively served over
// HTTPS (with HTTP -> HTTPS redirects in place).
type SecurityHeadersConfig struct {
	Enabled               bool   `mapstructure:"enabled"`
	HSTS                  string `mapstructure:"hsts"`
	ContentSecurityPolicy string `mapstructure:"content_security_policy"`
	XContentTypeOptions   string `mapstructure:"x_content_type_options"`
	XFrameOptions         string `mapstructure:"x_frame_options"`
	ReferrerPolicy        string `mapstructure:"referrer_policy"`
}

// WebConfig represents web frontend server configuration
type WebConfig struct {
	Server WebServerConfig `mapstructure:"server"`
}

// WebServerConfig represents web server configuration
type WebServerConfig struct {
	Host       string `mapstructure:"host"`
	Port       int    `mapstructure:"port"`
	StaticDir  string `mapstructure:"static_dir"`
	APIHostURL string `mapstructure:"api_host_url"` // Backend API server hostname for proxying
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level  string          `mapstructure:"level"`
	Format string          `mapstructure:"format"`
	Output LogOutputConfig `mapstructure:"output"`
}

// LogOutputConfig represents log output configuration
type LogOutputConfig struct {
	Pretty bool `mapstructure:"pretty"`
}

// AuthConfig represents authentication configuration
type AuthConfig struct {
	Simple  SimpleAuthConfig `mapstructure:"simple"`
	OAuth2  OAuth2Config     `mapstructure:"oauth2"`
	Session SessionConfig    `mapstructure:"session"`
}

// SimpleAuthConfig represents simple username/password authentication configuration
type SimpleAuthConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	DefaultUser  string `mapstructure:"default_user"`  // Default admin username
	DefaultPass  string `mapstructure:"default_pass"`  // Default admin password
	DefaultEmail string `mapstructure:"default_email"` // Default admin email
}

// OAuth2Config represents OAuth2/OIDC provider configuration
type OAuth2Config struct {
	Enabled      bool     `mapstructure:"enabled"`
	ProviderURL  string   `mapstructure:"provider_url"`
	ClientID     string   `mapstructure:"client_id"`
	ClientSecret string   `mapstructure:"client_secret"`
	RedirectURL  string   `mapstructure:"redirect_url"`
	Scopes       []string `mapstructure:"scopes"`

	// Initial admin users (for OAuth2/Keycloak authentication)
	// Users matching these emails will be automatically assigned admin role on first login
	AdminUsers []string `mapstructure:"admin_users"`

	// Advanced settings
	SkipIssuerCheck bool `mapstructure:"skip_issuer_check"`
	SkipExpiryCheck bool `mapstructure:"skip_expiry_check"`
}

// SessionConfig represents session configuration
type SessionConfig struct {
	Secret      string        `mapstructure:"secret"`
	Name        string        `mapstructure:"name"`
	MaxAge      time.Duration `mapstructure:"max_age"`
	Secure      bool          `mapstructure:"secure"`
	SameSite    string        `mapstructure:"same_site"`
	StateMaxAge time.Duration `mapstructure:"state_max_age"`
}

// ============================================================================
// KEYCLOAK MONITORING CONFIGURATION
// ============================================================================

// KeycloakConfig represents the complete Keycloak monitoring configuration
// Structured with global defaults and per-tenant overrides
type KeycloakConfig struct {
	Global  KeycloakInstanceConfig  `mapstructure:"global"`  // Global default settings
	Tenants map[string]TenantConfig `mapstructure:"tenants"` // Map of tenant_id -> tenant config
}

// TenantConfig represents configuration for a single Keycloak tenant
// The tenant_id is the map key, not included in the struct
type TenantConfig struct {
	Name         string                 `mapstructure:"name"`           // Display name
	Description  string                 `mapstructure:"description"`    // Optional description
	ServerURL    string                 `mapstructure:"server_url"`     // Keycloak server URL
	AdminRealm   string                 `mapstructure:"admin_realm"`    // Admin realm name (typically "master")
	ClientID     string                 `mapstructure:"client_id"`      // Client ID for client_credentials grant
	ClientSecret string                 `mapstructure:"client_secret"`  // Client secret (if using confidential client)
	Enabled      bool                   `mapstructure:"enabled"`        // Enable monitoring for this tenant
	IsDefault    bool                   `mapstructure:"is_default"`     // Mark as default tenant (auto-selected on load)
	Tags         []string               `mapstructure:"tags"`           // Optional tags for organization
	Owner        string                 `mapstructure:"owner"`          // Optional owner/team identifier
	Config       KeycloakInstanceConfig `mapstructure:"config"`         // Override global settings for this instance
	Amfa         *AmfaTenantConfig      `mapstructure:"amfa,omitempty"` // Optional AMFA integration for this tenant
}

// AmfaTenantConfig represents AMFA integration configuration for a tenant.
// When Enabled is true, the platform reads this tenant's AMFA events either
// through AMFA's read-only HTTP API (preferred) or by connecting directly to its
// database (the original path, retained for deployments not yet upgraded).
//
// When both API and Database are configured, API wins: it needs no database
// credentials and does not couple this platform to AMFA's schema.
type AmfaTenantConfig struct {
	Enabled               bool               `mapstructure:"enabled"`
	EventsLookbackDays    int                `mapstructure:"events_lookback_days"`
	ExpectedSchemaVersion string             `mapstructure:"expected_schema_version"`
	API                   AmfaAPIConfig      `mapstructure:"api"`
	Database              AmfaDatabaseConfig `mapstructure:"database"`
}

// AmfaAPIConfig represents AMFA read-only monitoring API settings for a tenant.
//
// No credentials appear here. Requests authenticate with a service-account token
// obtained from the tenant's own Keycloak using the confidential client this
// platform already uses for the Admin API, so there is nothing extra to store.
type AmfaAPIConfig struct {
	BaseURL string        `mapstructure:"base_url"`
	Timeout time.Duration `mapstructure:"timeout"`
}

// Configured reports whether this tenant should read AMFA over HTTP.
func (a *AmfaAPIConfig) Configured() bool {
	return strings.TrimSpace(a.BaseURL) != ""
}

// Validate checks the API settings and applies defaults.
func (a *AmfaAPIConfig) Validate() error {
	a.BaseURL = strings.TrimRight(strings.TrimSpace(a.BaseURL), "/")

	parsed, err := url.Parse(a.BaseURL)
	if err != nil {
		return fmt.Errorf("amfa.api.base_url is not a valid URL: %w", err)
	}
	// Checked here rather than left to the first request: a missing scheme or
	// host produces a URL that parses cleanly but fails at every poll, and
	// startup is where an operator will see the message.
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("amfa.api.base_url must use http or https, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("amfa.api.base_url must include a host")
	}

	if a.Timeout == 0 {
		a.Timeout = 30 * time.Second
	}
	return nil
}

// AmfaDatabaseConfig represents PostgreSQL connection settings for an AMFA tenant.
type AmfaDatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Database        string        `mapstructure:"database"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	SSLMode         string        `mapstructure:"sslmode"`
	MaxConns        int           `mapstructure:"max_conns"`
	MinConns        int           `mapstructure:"min_conns"`
	Timeout         time.Duration `mapstructure:"timeout"`
	ReadReplicaHost string        `mapstructure:"read_replica_host"`
}

// Validate verifies the AMFA tenant configuration and applies defaults for
// any unset optional fields. When Enabled is false the call is a no-op and
// no fields are required.
func (a *AmfaTenantConfig) Validate() error {
	if !a.Enabled {
		return nil
	}

	// The API path is preferred and needs no database settings, so validate it
	// instead of them rather than as well as them. Requiring both would force
	// every deployment to keep database credentials in config purely to satisfy
	// validation, which is the thing moving to the API removes.
	if a.API.Configured() {
		if err := a.API.Validate(); err != nil {
			return err
		}
		if a.EventsLookbackDays == 0 {
			a.EventsLookbackDays = 30
		}
		if a.EventsLookbackDays > 90 {
			a.EventsLookbackDays = 90
		}
		return nil
	}

	if a.Database.Host == "" {
		return fmt.Errorf("amfa requires either amfa.api.base_url or amfa.database.host when amfa.enabled=true")
	}
	if a.Database.Port == 0 {
		a.Database.Port = 5432
	}
	if a.Database.Database == "" {
		return fmt.Errorf("amfa.database.database is required when amfa.enabled=true")
	}
	if a.Database.User == "" {
		return fmt.Errorf("amfa.database.user is required when amfa.enabled=true")
	}
	if a.Database.SSLMode == "" {
		a.Database.SSLMode = "require"
	}
	if a.Database.MaxConns == 0 {
		a.Database.MaxConns = 5
	}
	if a.Database.MinConns == 0 {
		a.Database.MinConns = 1
	}
	if a.Database.Timeout == 0 {
		a.Database.Timeout = 10 * time.Second
	}
	if a.EventsLookbackDays == 0 {
		a.EventsLookbackDays = 30
	}
	if a.EventsLookbackDays > 90 {
		a.EventsLookbackDays = 90
	}
	return nil
}

// KeycloakInstanceConfig contains default settings applied to all tenants
// Individual tenants can override these settings if needed
type KeycloakInstanceConfig struct {
	Realms            []string                 `mapstructure:"realms"`             // Default realms to monitor
	DefaultRealm      string                   `mapstructure:"default_realm"`      // Default realm to display in UI
	InfinispanEnabled bool                     `mapstructure:"infinispan_enabled"` // Whether InfiniSpan clustering is enabled
	InfinispanPort    int                      `mapstructure:"infinispan_port"`    // Port for InfiniSpan metrics (optional, defaults to same as server_url)
	Polling           KeycloakPollingConfig    `mapstructure:"polling"`            // Polling intervals
	Events            KeycloakEventsConfig     `mapstructure:"events"`             // Event collection settings
	Connection        KeycloakConnectionConfig `mapstructure:"connection"`         // Connection settings
	ConfigChecker     ConfigCheckerConfig      `mapstructure:"config_checker"`     // Configuration checker settings
	EventChecker      EventCheckerConfig       `mapstructure:"event_checker"`      // Event checker settings
	VersionChecker    VersionCheckerConfig     `mapstructure:"version_checker"`    // Version checker settings
	AmfaChecker       AmfaCheckerConfig        `mapstructure:"amfa_checker"`       // AMFA risk-alert checker settings
	AmfaMirror        AmfaMirrorConfig         `mapstructure:"amfa_mirror"`        // AMFA events mirror (unified Events list)
}

// KeycloakPollingConfig represents polling intervals for Keycloak monitoring
type KeycloakPollingConfig struct {
	MetricsInterval   time.Duration `mapstructure:"metrics_interval"`
	EventsInterval    time.Duration `mapstructure:"events_interval"`
	HealthInterval    time.Duration `mapstructure:"health_interval"`
	RealmInfoInterval time.Duration `mapstructure:"realm_info_interval"`
}

// KeycloakEventsConfig represents event collection configuration
type KeycloakEventsConfig struct {
	Types            []string      `mapstructure:"types"`
	MaxEventsPerPoll int           `mapstructure:"max_events_per_poll"`
	LookbackDuration time.Duration `mapstructure:"lookback_duration"`
}

// KeycloakConnectionConfig represents connection settings for Keycloak API
type KeycloakConnectionConfig struct {
	Timeout       time.Duration `mapstructure:"timeout"`
	SkipTLSVerify bool          `mapstructure:"skip_tls_verify"`
	MaxRetries    int           `mapstructure:"max_retries"`
	RetryBackoff  time.Duration `mapstructure:"retry_backoff"`

	// AllowPrivateRanges permits Keycloak server URLs that resolve to
	// RFC1918 / IPv6 ULA addresses. Defaults to true to support on-prem
	// deployments where the customer's Keycloak lives on an internal
	// network. Loopback, link-local (cloud metadata) and unspecified
	// addresses are blocked regardless of this flag.
	AllowPrivateRanges bool `mapstructure:"allow_private_ranges"`
}

// ConfigCheckerConfig represents configuration checker settings
type ConfigCheckerConfig struct {
	PollInterval time.Duration       `mapstructure:"poll_interval"`
	Checks       ConfigCheckerChecks `mapstructure:"checks"`
}

// ConfigCheckerChecks contains settings for specific checks
type ConfigCheckerChecks struct {
	IdentityProviderMetadata IdentityProviderMetadataCheckConfig `mapstructure:"identity_provider_metadata"`
	RealmSecurity            RealmSecurityCheckConfig            `mapstructure:"realm_security"`
	ClientSecurity           ClientSecurityCheckConfig           `mapstructure:"client_security"`
	Health                   HealthCheckConfig                   `mapstructure:"health"`
}

// IdentityProviderMetadataCheckConfig represents settings for IDP metadata check
type IdentityProviderMetadataCheckConfig struct {
	Enabled                          bool          `mapstructure:"enabled"`
	CheckSAMLOnly                    bool          `mapstructure:"check_saml_only"`
	ExcludedRealms                   []string      `mapstructure:"excluded_realms"`
	ExcludedProviders                []string      `mapstructure:"excluded_providers"`
	CertificateExpirationWarningDays int           `mapstructure:"certificate_expiration_warning_days"` // Days before expiration to create warning (default: 30)
	CertificateCheckTimeout          time.Duration `mapstructure:"certificate_check_timeout"`           // HTTP timeout for fetching metadata (default: 30s)
}

// RealmSecurityCheckConfig represents settings for realm security check
type RealmSecurityCheckConfig struct {
	Enabled                       bool `mapstructure:"enabled"`
	CheckSSLRequired              bool `mapstructure:"check_ssl_required"`
	CheckBruteForce               bool `mapstructure:"check_brute_force"`
	CheckPasswordPolicy           bool `mapstructure:"check_password_policy"`
	CheckEmailVerification        bool `mapstructure:"check_email_verification"`
	CheckAdminEvents              bool `mapstructure:"check_admin_events"`
	CheckUserEvents               bool `mapstructure:"check_user_events"`
	CheckDuplicateEmails          bool `mapstructure:"check_duplicate_emails"`
	MinPasswordLength             int  `mapstructure:"min_password_length"`               // Minimum acceptable password length (default: 8)
	RequirePasswordComplexity     bool `mapstructure:"require_password_complexity"`       // Require uppercase, lowercase, digits, special chars
	MaxAccessTokenLifespanMinutes int  `mapstructure:"max_access_token_lifespan_minutes"` // Maximum access token lifespan in minutes (default: 15)
	MaxSSOSessionIdleMinutes      int  `mapstructure:"max_sso_session_idle_minutes"`      // Maximum SSO session idle time in minutes (default: 30)
}

// ClientSecurityCheckConfig represents settings for client security check
type ClientSecurityCheckConfig struct {
	Enabled                 bool `mapstructure:"enabled"`
	CheckRedirectURIs       bool `mapstructure:"check_redirect_uris"`
	CheckPublicClients      bool `mapstructure:"check_public_clients"`
	CheckDirectAccessGrants bool `mapstructure:"check_direct_access_grants"`
	CheckClientSecret       bool `mapstructure:"check_client_secret"`
	CheckImplicitFlow       bool `mapstructure:"check_implicit_flow"`
	CheckStandardFlow       bool `mapstructure:"check_standard_flow"`
	AllowLocalhostRedirects bool `mapstructure:"allow_localhost_redirects"` // Allow localhost redirects in production (default: false)
	ProductionEnvironment   bool `mapstructure:"production_environment"`    // Enable production-specific checks (default: true)
}

// HealthCheckConfig represents settings for health monitoring check
type HealthCheckConfig struct {
	Enabled               bool `mapstructure:"enabled"`
	MemoryWarningPercent  int  `mapstructure:"memory_warning_percent"`  // Warn at this memory percentage (default: 80)
	MemoryCriticalPercent int  `mapstructure:"memory_critical_percent"` // Critical at this memory percentage (default: 90)
}

// VersionCheckerConfig represents version checking configuration
type VersionCheckerConfig struct {
	// Enabled turns the version checker on. It defaults to false because the
	// checker is the only part of this tool that reaches a host we do not
	// operate: GitHub's API for upstream Keycloak releases.
	//
	// Third-party calls are prohibited here, since deployments can be
	// airgapped, where these would fail on an HTTP timeout with nothing on
	// screen to explain why. Off by default means a deployment has to opt in
	// deliberately, and NewVersionChecker returns nil when it is false, so no
	// wiring mistake can enable the traffic by accident.
	Enabled          bool          `mapstructure:"enabled"`
	CacheTTL         time.Duration `mapstructure:"cache_ttl"`
	HTTPTimeout      time.Duration `mapstructure:"http_timeout"`
	GitHubAPIBaseURL string        `mapstructure:"github_api_base_url"`
	KeycloakOwner    string        `mapstructure:"keycloak_owner"`
	KeycloakRepo     string        `mapstructure:"keycloak_repo"`
}

// EventCheckerConfig represents event-based alert checker settings
type EventCheckerConfig struct {
	PollInterval time.Duration      `mapstructure:"poll_interval"`
	Checks       EventCheckerChecks `mapstructure:"checks"`
}

// EventCheckerChecks contains settings for specific event-based checks
type EventCheckerChecks struct {
	LoginError   LoginErrorCheckConfig       `mapstructure:"login_error"`
	IdpError     IdpErrorCheckConfig         `mapstructure:"idp_error"`
	CodeToToken  CodeToTokenErrorCheckConfig `mapstructure:"code_to_token"`
	ClientError  ClientErrorCheckConfig      `mapstructure:"client_error"`
	CommonErrors CommonErrorCheckConfig      `mapstructure:"common_errors"`
}

// LoginErrorCheckConfig represents settings for LOGIN_ERROR event check
type LoginErrorCheckConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Threshold  int           `mapstructure:"threshold"`   // Number of errors to trigger alert (default: 10)
	TimeWindow time.Duration `mapstructure:"time_window"` // Time window to count errors (default: 5m)
}

// IdpErrorCheckConfig represents settings for identity provider error checks
type IdpErrorCheckConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Threshold  int           `mapstructure:"threshold"`   // Number of errors to trigger alert (default: 1)
	TimeWindow time.Duration `mapstructure:"time_window"` // Time window to count errors (default: 5m)
	EventTypes []string      `mapstructure:"event_types"` // Which IDP error types to monitor
}

// CodeToTokenErrorCheckConfig represents settings for CODE_TO_TOKEN_ERROR event check
type CodeToTokenErrorCheckConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Threshold  int           `mapstructure:"threshold"`   // Number of errors to trigger alert (default: 1)
	TimeWindow time.Duration `mapstructure:"time_window"` // Time window to count errors (default: 5m)
}

// ClientErrorCheckConfig represents settings for CLIENT_LOGIN_ERROR event check
type ClientErrorCheckConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Threshold  int           `mapstructure:"threshold"`   // Number of errors to trigger alert (default: 10)
	TimeWindow time.Duration `mapstructure:"time_window"` // Time window to count errors (default: 5m)
}

// CommonErrorCheckConfig represents settings for common low-priority error checks
type CommonErrorCheckConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Threshold  int           `mapstructure:"threshold"`   // Number of errors to trigger alert (default: 50)
	TimeWindow time.Duration `mapstructure:"time_window"` // Time window to count errors (default: 5m)
	EventTypes []string      `mapstructure:"event_types"` // Which error types to monitor (REGISTER_ERROR, RESET_PASSWORD_ERROR, LOGOUT_ERROR)
}

// AmfaCheckerConfig configures the AMFA risk-alert checker. It mirrors the
// shape of EventCheckerConfig so operators see a familiar layout. It lives at
// keycloak.global.amfa_checker. Disabled by default; operators opt in.
type AmfaCheckerConfig struct {
	Enabled      bool                    `mapstructure:"enabled"`       // master switch; default false
	PollInterval time.Duration           `mapstructure:"poll_interval"` // default 1m
	Checks       AmfaCheckerChecksConfig `mapstructure:"checks"`
}

// AmfaCheckerChecksConfig holds the per-rule toggles and thresholds.
type AmfaCheckerChecksConfig struct {
	RiskRejected        PerEventCheckConfig  `mapstructure:"risk_rejected"`          // rule 1
	RepeatedRisky       RepeatedRiskyConfig  `mapstructure:"repeated_risky"`         // rule 2
	VPNRisky            PerEventCheckConfig  `mapstructure:"vpn_risky"`              // rule 3
	LoginError          ThresholdCheckConfig `mapstructure:"login_error"`            // rule 4a
	ClientLoginError    ThresholdCheckConfig `mapstructure:"client_login_error"`     // rule 4b
	LoginErrorByClient  ThresholdCheckConfig `mapstructure:"login_error_by_client"`  // per-client login-error burst
	RealmRejectBurst    ThresholdCheckConfig `mapstructure:"realm_reject_burst"`     // many distinct users rejected in a realm
	LoginErrorByAccount ThresholdCheckConfig `mapstructure:"login_error_by_account"` // per-account login-error burst
}

// PerEventCheckConfig is the config shape for per-event rules (1 and 3) that
// have no tunable thresholds, only an enable flag.
type PerEventCheckConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// RepeatedRiskyConfig configures rule 2 (repeated risky attempts per user/day).
type RepeatedRiskyConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	Threshold    int           `mapstructure:"threshold"`      // default 3
	Window       time.Duration `mapstructure:"window"`         // default 24h
	MinRiskLevel int           `mapstructure:"min_risk_level"` // default 2
}

// ThresholdCheckConfig configures the event-type threshold rules (4a, 4b).
type ThresholdCheckConfig struct {
	Enabled   bool          `mapstructure:"enabled"`
	Threshold int           `mapstructure:"threshold"` // default 5
	Window    time.Duration `mapstructure:"window"`    // default 5m
}

// ApplyDefaults fills in zero-valued fields with conservative defaults. Called
// AmfaMirrorConfig configures the AMFA events mirror, which copies each
// AMFA-enabled tenant's login events into the platform events table so the
// Events page can serve one unified Keycloak+AMFA list. It lives at
// keycloak.global.amfa_mirror. Disabled by default; operators opt in.
type AmfaMirrorConfig struct {
	Enabled      bool          `mapstructure:"enabled"`       // master switch; default false
	PollInterval time.Duration `mapstructure:"poll_interval"` // default 30s
	BackfillDays int           `mapstructure:"backfill_days"` // first-run lookback; default 1
}

// ApplyDefaults fills zero values with production defaults.
func (c *AmfaMirrorConfig) ApplyDefaults() {
	if c.PollInterval <= 0 {
		c.PollInterval = 30 * time.Second
	}
	if c.BackfillDays <= 0 {
		c.BackfillDays = 1
	}
}

// by the fx layer after reading config and before constructing services.
func (c *AmfaCheckerConfig) ApplyDefaults() {
	if c.PollInterval <= 0 {
		c.PollInterval = time.Minute
	}
	if c.Checks.RepeatedRisky.Threshold <= 0 {
		c.Checks.RepeatedRisky.Threshold = 3
	}
	if c.Checks.RepeatedRisky.Window <= 0 {
		c.Checks.RepeatedRisky.Window = 24 * time.Hour
	}
	if c.Checks.RepeatedRisky.MinRiskLevel <= 0 {
		c.Checks.RepeatedRisky.MinRiskLevel = 2
	}
	if c.Checks.LoginError.Threshold <= 0 {
		c.Checks.LoginError.Threshold = 5
	}
	if c.Checks.LoginError.Window <= 0 {
		c.Checks.LoginError.Window = 5 * time.Minute
	}
	if c.Checks.ClientLoginError.Threshold <= 0 {
		c.Checks.ClientLoginError.Threshold = 5
	}
	if c.Checks.ClientLoginError.Window <= 0 {
		c.Checks.ClientLoginError.Window = 5 * time.Minute
	}
	if c.Checks.LoginErrorByClient.Threshold <= 0 {
		c.Checks.LoginErrorByClient.Threshold = 5
	}
	if c.Checks.LoginErrorByClient.Window <= 0 {
		c.Checks.LoginErrorByClient.Window = 5 * time.Minute
	}
	if c.Checks.RealmRejectBurst.Threshold <= 0 {
		c.Checks.RealmRejectBurst.Threshold = 10
	}
	if c.Checks.RealmRejectBurst.Window <= 0 {
		c.Checks.RealmRejectBurst.Window = 10 * time.Minute
	}
	if c.Checks.LoginErrorByAccount.Threshold <= 0 {
		c.Checks.LoginErrorByAccount.Threshold = 5
	}
	if c.Checks.LoginErrorByAccount.Window <= 0 {
		c.Checks.LoginErrorByAccount.Window = 5 * time.Minute
	}
}

// ============================================================================
// ALERTS NOTIFICATION CONFIGURATION
// ============================================================================

// NotificationsConfig represents notification integrations configuration
type NotificationsConfig struct {
	BaseURL string       `mapstructure:"base_url"` // Base URL for the Keycloak Monitoring Tool web UI (e.g., https://monitoring.example.com)
	GitLab  GitLabConfig `mapstructure:"gitlab"`
	Slack   SlackConfig  `mapstructure:"slack"`
	Email   EmailConfig  `mapstructure:"email"`
}

// GitLabConfig represents GitLab issue integration configuration
type GitLabConfig struct {
	Enabled     bool     `mapstructure:"enabled"`
	URL         string   `mapstructure:"url"`          // GitLab instance URL (e.g., https://gitlab.com)
	Token       string   `mapstructure:"token"`        // Personal access token or project token
	ProjectID   string   `mapstructure:"project_id"`   // Project ID or path (e.g., "12345" or "group/project")
	Milestone   string   `mapstructure:"milestone"`    // Milestone title or ID to assign issues to (e.g., "1010" or "Sprint Q1")
	Labels      []string `mapstructure:"labels"`       // Labels to add to issues
	MinSeverity string   `mapstructure:"min_severity"` // Minimum severity level to create issues (info, warning, error, critical)
}

// SlackConfig represents Slack webhook integration configuration
type SlackConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	WebhookURL  string `mapstructure:"webhook_url"`  // Slack incoming webhook URL
	Channel     string `mapstructure:"channel"`      // Optional: Override default channel
	Username    string `mapstructure:"username"`     // Optional: Bot username
	IconEmoji   string `mapstructure:"icon_emoji"`   // Optional: Bot icon emoji
	MinSeverity string `mapstructure:"min_severity"` // Minimum severity level to send messages (info, warning, error, critical)
}

// EmailConfig represents email notification configuration
type EmailConfig struct {
	Enabled     bool     `mapstructure:"enabled"`
	SMTPHost    string   `mapstructure:"smtp_host"`    // SMTP server hostname
	SMTPPort    int      `mapstructure:"smtp_port"`    // SMTP server port
	Username    string   `mapstructure:"username"`     // SMTP authentication username
	Password    string   `mapstructure:"password"`     // SMTP authentication password
	From        string   `mapstructure:"from"`         // Sender email address
	To          []string `mapstructure:"to"`           // Recipient email addresses
	UseTLS      bool     `mapstructure:"use_tls"`      // Use TLS encryption
	SkipVerify  bool     `mapstructure:"skip_verify"`  // Skip TLS certificate verification
	Subject     string   `mapstructure:"subject"`      // Email subject template
	MinSeverity string   `mapstructure:"min_severity"` // Minimum severity level to send emails (info, warning, error, critical)
}

// ============================================================================
// ALERT SILENCES CONFIGURATION
// ============================================================================

// AlertSilencesConfig represents alert silence rules configuration
type AlertSilencesConfig []AlertSilenceConfig

// AlertSilenceConfig represents a single alert silence rule
type AlertSilenceConfig struct {
	ID          string                     `mapstructure:"id"`                  // Unique ID for the silence rule
	Name        string                     `mapstructure:"name"`                // Silence rule name
	Description string                     `mapstructure:"description"`         // Why this silence exists
	Enabled     bool                       `mapstructure:"enabled"`             // Whether silence is active
	Matchers    AlertSilenceMatchersConfig `mapstructure:"matchers"`            // Pattern matching rules
	Duration    string                     `mapstructure:"duration,omitempty"`  // Duration (e.g., "2h", "30m") - empty = permanent
	StartsAt    string                     `mapstructure:"starts_at,omitempty"` // Start time (RFC3339 format) - empty = now
	EndsAt      string                     `mapstructure:"ends_at,omitempty"`   // End time (RFC3339 format) - computed from duration
	CreatedBy   string                     `mapstructure:"created_by"`          // Who created this silence
	Comment     string                     `mapstructure:"comment,omitempty"`   // Additional context
	TenantID    string                     `mapstructure:"tenant_id,omitempty"` // Optional: Restrict to specific tenant (empty = all)
}

// AlertSilenceMatchersConfig represents patterns for matching alerts to silence
type AlertSilenceMatchersConfig struct {
	AlertType    string            `mapstructure:"alert_type,omitempty"`    // Alert type pattern (e.g., "LOGIN_ERROR")
	Source       string            `mapstructure:"source,omitempty"`        // Alert source (event, log, metric, configuration)
	Severity     string            `mapstructure:"severity,omitempty"`      // Alert severity (info, warning, error, critical)
	RealmName    string            `mapstructure:"realm_name,omitempty"`    // Realm name pattern (supports wildcards)
	ClientID     string            `mapstructure:"client_id,omitempty"`     // Client ID pattern (supports wildcards)
	Username     string            `mapstructure:"username,omitempty"`      // Username pattern (supports wildcards)
	FieldMatches map[string]string `mapstructure:"field_matches,omitempty"` // Additional field matching
}
