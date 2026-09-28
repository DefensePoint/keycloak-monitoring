package domain

import "time"

// Event represents a security monitoring event.
type Event struct {
	ID uint `json:"id"`
	// TenantID identifies the monitored Keycloak instance this event came
	// from. Set by whichever mirror wrote the row; both are per-tenant.
	TenantID     string    `json:"tenant_id"`
	EventID      string    `json:"event_id"`
	Type         string    `json:"type"`
	Category     string    `json:"category"`
	Severity     string    `json:"severity"`
	Description  string    `json:"description"`
	Source       string    `json:"source"`
	SourceIP     string    `json:"source_ip"`
	SourceSystem string    `json:"source_system"`
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	ClientID     string    `json:"client_id"`
	Location     string    `json:"location"`
	RawData      string    `json:"raw_data"`
	Status       string    `json:"status"`
	Timestamp    time.Time `json:"timestamp"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// AMFAEventID is the correlation id shared between a Keycloak login event
	// (carried in its details as "amfa_event_id") and its AMFA counterpart
	// (AMFA's auth_event.id). It is the merge key: the two writers use it to
	// converge a login onto one row regardless of arrival order.
	AMFAEventID *string `json:"amfa_event_id,omitempty"`

	// AMFA-specific fields, set only for events that have an AMFA counterpart
	// (mirrored or merged from a tenant's Adaptive MFA database); nil otherwise.
	RiskLevel        *int     `json:"risk_level,omitempty"`
	FinalStatus      *string  `json:"final_status,omitempty"`
	IsVPN            *bool    `json:"is_vpn,omitempty"`
	Country          *string  `json:"country,omitempty"`
	City             *string  `json:"city,omitempty"`
	Lat              *float64 `json:"lat,omitempty"`
	Long             *float64 `json:"long,omitempty"`
	OperatingSystem  *string  `json:"operating_system,omitempty"`
	Browser          *string  `json:"browser,omitempty"`
	Device           *string  `json:"device,omitempty"`
	SystemLanguage   *string  `json:"system_language,omitempty"`
	ScreenResolution *string  `json:"screen_resolution,omitempty"`
}

// AuthMethod represents the authentication method used for a user
type AuthMethod string

const (
	AuthMethodSimple AuthMethod = "simple" // Username/password authentication
	AuthMethodOAuth  AuthMethod = "oauth"  // OAuth2/OIDC authentication
)

// User represents a user account.
type User struct {
	ID                 uint       `json:"id"`
	Subject            string     `json:"subject"`
	Email              string     `json:"email"`
	EmailVerified      bool       `json:"email_verified"`
	Name               string     `json:"name"`
	GivenName          string     `json:"given_name"`
	FamilyName         string     `json:"family_name"`
	PreferredUsername  string     `json:"preferred_username"`
	Locale             string     `json:"locale"`
	Username           string     `json:"username,omitempty"`
	PasswordHash       string     `json:"-"`
	AuthMethod         AuthMethod `json:"auth_method"` // Authentication method (simple or oauth)
	MustChangePassword bool       `json:"must_change_password"`
	PasswordChangedAt  *time.Time `json:"password_changed_at,omitempty"`
	IsActive           bool       `json:"is_active"`
	IsBlocked          bool       `json:"is_blocked"`
	BlockedReason      string     `json:"blocked_reason,omitempty"`
	BlockedAt          *time.Time `json:"blocked_at,omitempty"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP        string     `json:"last_login_ip,omitempty"`
	LastAccessedAt     *time.Time `json:"last_accessed_at,omitempty"`
	LoginCount         int        `json:"login_count"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// GetID returns the user's database ID (implements UserWithID interface for auth)
func (u *User) GetID() uint {
	return u.ID
}

// ============================================================================
// KEYCLOAK DOMAIN MODELS
// ============================================================================

// KeycloakTenant represents a monitored Keycloak instance.
type KeycloakTenant struct {
	ID                uint       `json:"id"`
	TenantID          string     `json:"tenant_id"`
	Name              string     `json:"name"`
	Description       string     `json:"description,omitempty"`
	ServerURL         string     `json:"server_url"`
	AdminRealm        string     `json:"admin_realm"`
	ClientID          string     `json:"client_id,omitempty"`
	ClientSecret      string     `json:"-"` // Never expose in JSON
	Configuration     string     `json:"configuration,omitempty"`
	DefaultRealm      string     `json:"default_realm,omitempty"`
	Enabled           bool       `json:"enabled"`
	LastHealthCheck   time.Time  `json:"last_health_check"`
	HealthStatus      string     `json:"health_status"`
	HealthMessage     string     `json:"health_message,omitempty"`
	LastError         string     `json:"last_error,omitempty"`
	LastErrorAt       *time.Time `json:"last_error_at,omitempty"`
	IsDefault         bool       `json:"is_default"`
	IsConfigDefined   bool       `json:"is_config_defined"`
	Tags              []string   `json:"tags,omitempty"`
	Owner             string     `json:"owner,omitempty"`
	InfinispanEnabled bool       `json:"infinispan_enabled"`
	InfinispanPort    int        `json:"infinispan_port,omitempty"`

	Amfa *TenantAmfa `json:"amfa,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TenantAmfa is a tenant's AMFA integration settings.
//
// Only the API endpoint is carried. Reading AMFA over its HTTP API needs no
// database credentials, which is what makes these settings safe to expose in a
// tenant form at all.
type TenantAmfa struct {
	Enabled            bool   `json:"enabled"`
	APIBaseURL         string `json:"api_base_url"`
	EventsLookbackDays int    `json:"events_lookback_days,omitempty"`
	APITimeoutSeconds  int    `json:"api_timeout_seconds,omitempty"`
}

// KeycloakEvent represents a Keycloak event.
type KeycloakEvent struct {
	TenantID   string            `json:"tenant_id"`
	EventID    string            `json:"event_id"`
	Time       time.Time         `json:"time"`
	RealmID    string            `json:"realm_id"`
	RealmName  string            `json:"realm_name"`
	ClientID   string            `json:"client_id"`
	SessionID  string            `json:"session_id"`
	IPAddress  string            `json:"ip_address"`
	EventType  string            `json:"event_type"`
	EventError string            `json:"event_error,omitempty"`
	UserID     string            `json:"user_id,omitempty"`
	Username   string            `json:"username,omitempty"`
	Email      string            `json:"email,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
	Success    bool              `json:"success"`
}

// KeycloakMetrics represents Keycloak metrics snapshot.
type KeycloakMetrics struct {
	TenantID          string    `json:"tenant_id"`
	Time              time.Time `json:"time"`
	RealmName         string    `json:"realm_name"`
	TotalUsers        int       `json:"total_users"`
	EnabledUsers      int       `json:"enabled_users"`
	DisabledUsers     int       `json:"disabled_users"`
	ActiveSessions    int       `json:"active_sessions"`
	OfflineSessions   int       `json:"offline_sessions"`
	TotalClients      int       `json:"total_clients"`
	LoginEvents       int       `json:"login_events"`
	LogoutEvents      int       `json:"logout_events"`
	FailedLoginEvents int       `json:"failed_login_events"`
	RegisterEvents    int       `json:"register_events"`
}

// KeycloakHealth represents Keycloak health status.
type KeycloakHealth struct {
	TenantID      string    `json:"tenant_id"`
	Time          time.Time `json:"time"`
	Status        string    `json:"status"`
	ResponseTime  int64     `json:"response_time_ms"`
	ServerVersion string    `json:"server_version,omitempty"`
	UptimeMillis  int64     `json:"uptime_millis"`
	MemoryUsed    int64     `json:"memory_used_bytes"`
	MemoryMax     int64     `json:"memory_max_bytes"`
	MemoryFree    int64     `json:"memory_free_bytes"`
	ErrorMessage  string    `json:"error_message,omitempty"`
}

// KeycloakRealmInfo represents Keycloak realm information.
type KeycloakRealmInfo struct {
	ID                        uint      `json:"id"`
	TenantID                  string    `json:"tenant_id"`
	RealmID                   string    `json:"realm_id"`
	RealmName                 string    `json:"realm_name"`
	DisplayName               string    `json:"display_name,omitempty"`
	Enabled                   bool      `json:"enabled"`
	SslRequired               string    `json:"ssl_required,omitempty"`
	RegistrationAllowed       bool      `json:"registration_allowed"`
	RememberMe                bool      `json:"remember_me"`
	VerifyEmail               bool      `json:"verify_email"`
	LoginWithEmailAllowed     bool      `json:"login_with_email_allowed"`
	DuplicateEmailsAllowed    bool      `json:"duplicate_emails_allowed"`
	ResetPasswordAllowed      bool      `json:"reset_password_allowed"`
	EditUsernameAllowed       bool      `json:"edit_username_allowed"`
	BruteForceProtected       bool      `json:"brute_force_protected"`
	EventsEnabled             bool      `json:"events_enabled"`
	EventsListeners           []string  `json:"events_listeners,omitempty"`
	EnabledEventTypes         []string  `json:"enabled_event_types,omitempty"`
	AdminEventsEnabled        bool      `json:"admin_events_enabled"`
	AdminEventsDetailsEnabled bool      `json:"admin_events_details_enabled"`
	LastChecked               time.Time `json:"last_checked"`
	IsHealthy                 bool      `json:"is_healthy"`
	HealthMessage             string    `json:"health_message,omitempty"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// ============================================================================
// CONFIGURATION ALERT MODELS
// ============================================================================

// AlertSeverity represents the severity level of an alert
type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityError    AlertSeverity = "error"
	AlertSeverityCritical AlertSeverity = "critical"
)

// String returns the string representation of the severity.
func (s AlertSeverity) String() string {
	return string(s)
}

// AlertStatus represents the current status of an alert
type AlertStatus string

const (
	AlertStatusActive    AlertStatus = "active"
	AlertStatusResolved  AlertStatus = "resolved"
	AlertStatusAcknowled AlertStatus = "acknowledged"
	AlertStatusIgnored   AlertStatus = "ignored"
)

// AlertType represents the type/category of alert
type AlertType string

const (
	AlertTypeConfiguration    AlertType = "configuration"
	AlertTypeSecurity         AlertType = "security"
	AlertTypeCompliance       AlertType = "compliance"
	AlertTypePerformance      AlertType = "performance"
	AlertTypeIdentityProvider AlertType = "identity_provider"
	AlertTypeRealm            AlertType = "realm"
	AlertTypeClient           AlertType = "client"
	AlertTypeEvent            AlertType = "event" // Event-based alerts
)

// AlertSource represents where the alert originated from
type AlertSource string

const (
	AlertSourceConfiguration AlertSource = "configuration" // Config checks
	AlertSourceEvent         AlertSource = "event"         // Keycloak events
	AlertSourceLog           AlertSource = "log"           // Log analysis (future)
	AlertSourceMetric        AlertSource = "metric"        // Metric thresholds (future)
)

// Alert represents a unified alert that can come from multiple sources.
// Formerly known as ConfigurationAlert.
type Alert struct {
	ID             uint          `json:"id"`
	TenantID       string        `json:"tenant_id"`       // Multi-tenancy support
	AlertID        string        `json:"alert_id"`        // Unique identifier for deduplication
	Source         AlertSource   `json:"source"`          // Where the alert originated (config, event, log, metric)
	Type           AlertType     `json:"type"`            // Type of alert
	Severity       AlertSeverity `json:"severity"`        // Severity level
	Status         AlertStatus   `json:"status"`          // Current status
	Title          string        `json:"title"`           // Short title
	Description    string        `json:"description"`     // Detailed description
	ResourceType   string        `json:"resource_type"`   // Type of resource (e.g., "realm", "client", "identity_provider")
	ResourceID     string        `json:"resource_id"`     // ID of the affected resource
	ResourceName   string        `json:"resource_name"`   // Human-readable name
	RealmName      string        `json:"realm_name"`      // Keycloak realm name
	CheckType      string        `json:"check_type"`      // Type of check that generated this alert
	RuleID         *string       `json:"rule_id"`         // Alert rule that triggered this (for event/log/metric alerts)
	EventID        *string       `json:"event_id"`        // Source event ID (for event-based alerts)
	Recommendation string        `json:"recommendation"`  // Suggested remediation
	Metadata       string        `json:"metadata"`        // Additional context (JSON)
	FirstDetected  time.Time     `json:"first_detected"`  // When first detected
	LastSeen       time.Time     `json:"last_seen"`       // When last seen
	ResolvedAt     *time.Time    `json:"resolved_at"`     // When resolved
	AcknowledgedAt *time.Time    `json:"acknowledged_at"` // When acknowledged
	AcknowledgedBy string        `json:"acknowledged_by"` // Who acknowledged
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// ConfigurationAlert is an alias for backward compatibility
// Deprecated: Use Alert instead
type ConfigurationAlert = Alert

// AlertRule represents a rule that can trigger alerts based on events, logs, or metrics
type AlertRule struct {
	ID          string        `json:"id"`          // Unique rule ID
	TenantID    *string       `json:"tenant_id"`   // nil for global rules, set for tenant-specific
	Name        string        `json:"name"`        // Human-readable name
	Description string        `json:"description"` // What this rule detects
	Enabled     bool          `json:"enabled"`     // Whether rule is active
	Source      AlertSource   `json:"source"`      // What source to evaluate (event, log, metric)
	Severity    AlertSeverity `json:"severity"`    // Severity of alerts generated

	// Conditions (rule-specific logic)
	Conditions RuleConditions `json:"conditions"` // Matching conditions

	// Alert template
	TitleTemplate          string `json:"title_template"`          // Go template for alert title
	DescriptionTemplate    string `json:"description_template"`    // Go template for alert description
	RecommendationTemplate string `json:"recommendation_template"` // Go template for recommendation

	// Notification settings
	NotifySlack  bool    `json:"notify_slack"`  // Send Slack notification
	SlackChannel *string `json:"slack_channel"` // Override default channel

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy string    `json:"created_by"` // User who created the rule
}

// RuleConditions defines the conditions for an alert rule
type RuleConditions struct {
	// Event-based conditions
	EventType     *string `json:"event_type,omitempty"`     // e.g., "LOGIN_ERROR", "LOGOUT"
	EventCategory *string `json:"event_category,omitempty"` // e.g., "LOGIN", "ADMIN"

	// Pattern matching
	FieldMatches map[string]string `json:"field_matches,omitempty"` // Field name -> regex pattern

	// Threshold conditions
	ThresholdCount  *int    `json:"threshold_count,omitempty"`  // Number of occurrences
	ThresholdWindow *string `json:"threshold_window,omitempty"` // Time window (e.g., "5m", "1h")

	// Additional filters
	RealmName   *string `json:"realm_name,omitempty"`   // Specific realm
	ClientID    *string `json:"client_id,omitempty"`    // Specific client
	UserPattern *string `json:"user_pattern,omitempty"` // User email/username pattern
}

// ============================================================================
// RBAC DOMAIN MODELS
// ============================================================================

// Role represents a role in the system.
type Role struct {
	ID          uint         `json:"id"`
	Name        string       `json:"name"`
	DisplayName string       `json:"display_name"`
	Description string       `json:"description,omitempty"`
	IsSystem    bool         `json:"is_system"`
	IsActive    bool         `json:"is_active"`
	Permissions []Permission `json:"permissions,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Permission represents a permission in the system.
type Permission struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description,omitempty"`
	Resource    string    `json:"resource"`
	Action      string    `json:"action"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserRole represents a user's role assignment with optional tenant scoping.
type UserRole struct {
	ID         uint       `json:"id"`
	UserID     uint       `json:"user_id"`
	RoleID     uint       `json:"role_id"`
	Role       *Role      `json:"role,omitempty"`
	TenantID   *string    `json:"tenant_id,omitempty"` // Null = global, non-null = tenant-scoped
	AssignedBy string     `json:"assigned_by,omitempty"`
	AssignedAt time.Time  `json:"assigned_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// TenantPolicy represents tenant-level access restrictions for a user.
type TenantPolicy struct {
	ID       uint   `json:"id"`
	UserID   uint   `json:"user_id"`
	TenantID string `json:"tenant_id"`
	// AllowedRealms lists the only realms this policy grants. An empty list
	// grants no realm; unrestricted is the absence of a policy row.
	AllowedRealms []string `json:"allowed_realms,omitempty"`
	// RealmsUnrestricted reports an allowed_realms column holding no list at
	// all, which grants every realm.
	RealmsUnrestricted bool      `json:"-"`
	GrantedBy          string    `json:"granted_by,omitempty"`
	GrantedAt          time.Time `json:"granted_at"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// UserWithRoles extends User with role and permission information.
type UserWithRoles struct {
	User
	Roles       []UserRole     `json:"roles"`
	Permissions []string       `json:"permissions"`
	Policies    []TenantPolicy `json:"tenant_policies,omitempty"`
}

// ============================================================================
// OPERATOR METRICS DOMAIN MODELS
// ============================================================================

// OperatorAction represents an action taken by an operator on an alert.
type OperatorAction struct {
	ID                            uint       `json:"ID"`
	TenantID                      string     `json:"TenantID"`
	AlertID                       uint       `json:"AlertID"`
	OperatorID                    uint       `json:"OperatorID"`
	OperatorEmail                 string     `json:"OperatorEmail"`
	ActionType                    string     `json:"ActionType"`
	ActionTime                    time.Time  `json:"ActionTime"`
	PreviousStatus                string     `json:"PreviousStatus"`
	PreviousActionTime            *time.Time `json:"PreviousActionTime"`
	AlertSeverity                 string     `json:"AlertSeverity"`
	AlertType                     string     `json:"AlertType"`
	RealmName                     string     `json:"RealmName"`
	TimeFromDetectionSeconds      int        `json:"TimeFromDetectionSeconds"`
	TimeFromPreviousActionSeconds int        `json:"TimeFromPreviousActionSeconds"`
	ResponseTimeSeconds           int        `json:"ResponseTimeSeconds"`
	ResolutionTimeSeconds         int        `json:"ResolutionTimeSeconds"`
	Comment                       string     `json:"Comment"`
	Metadata                      []byte     `json:"Metadata"`
	CreatedAt                     time.Time  `json:"CreatedAt"`
	UpdatedAt                     time.Time  `json:"UpdatedAt"`
}

// OperatorMetricsSummary represents aggregated metrics for an operator over a time period
type OperatorMetricsSummary struct {
	OperatorEmail string    `json:"operator_email"`
	OperatorName  string    `json:"operator_name"`
	TenantID      string    `json:"tenant_id"`
	PeriodStart   time.Time `json:"period_start"`
	PeriodEnd     time.Time `json:"period_end"`

	// Action counts
	TotalAlertsHandled int `json:"total_alerts_handled"`
	AlertsAcknowledged int `json:"alerts_acknowledged"`
	AlertsResolved     int `json:"alerts_resolved"`
	AlertsIgnored      int `json:"alerts_ignored"`
	AlertsCommented    int `json:"alerts_commented"`

	// By severity
	CriticalAlertsHandled int `json:"critical_alerts_handled"`
	HighAlertsHandled     int `json:"high_alerts_handled"`
	MediumAlertsHandled   int `json:"medium_alerts_handled"`
	LowAlertsHandled      int `json:"low_alerts_handled"`

	// By type
	AlertsByType map[string]int `json:"alerts_by_type"` // "configuration": 10, "security": 5, etc.

	// Time metrics (in seconds) - From detection to action
	AvgResponseTime      float64 `json:"avg_response_time"` // Average time to first action
	MedianResponseTime   float64 `json:"median_response_time"`
	AvgResolutionTime    float64 `json:"avg_resolution_time"` // Average time to resolution
	MedianResolutionTime float64 `json:"median_resolution_time"`
	MinResponseTime      int     `json:"min_response_time"`
	MaxResponseTime      int     `json:"max_response_time"`

	// Action-specific time metrics (in seconds)
	AvgTimeToAcknowledge float64 `json:"avg_time_to_acknowledge"` // Average time from detection to acknowledge
	AvgTimeToResolve     float64 `json:"avg_time_to_resolve"`     // Average time from detection to resolve
	AvgTimeToIgnore      float64 `json:"avg_time_to_ignore"`      // Average time from detection to ignore

	// Transition time metrics (in seconds) - Time between state changes
	AvgAcknowledgeToResolveTime float64 `json:"avg_acknowledge_to_resolve_time"` // Avg time from acknowledged → resolved
	AvgAcknowledgeToIgnoreTime  float64 `json:"avg_acknowledge_to_ignore_time"`  // Avg time from acknowledged → ignored
	AvgActiveToAcknowledgeTime  float64 `json:"avg_active_to_acknowledge_time"`  // Avg time from active → acknowledged
	AvgActiveToResolveTime      float64 `json:"avg_active_to_resolve_time"`      // Avg time from active → resolved (without acknowledge)
	AvgActiveToIgnoreTime       float64 `json:"avg_active_to_ignore_time"`       // Avg time from active → ignored (without acknowledge)

	// Performance indicators
	TotalWorkTimeSeconds int     `json:"total_work_time_seconds"` // Total time spent on alerts
	TotalWorkTimeHours   float64 `json:"total_work_time_hours"`   // Converted to hours for reporting
}

// ============================================================================
// NOTIFICATION DOMAIN MODELS
// ============================================================================

// NotificationLog represents a notification log entry for tracking alert notifications.
type NotificationLog struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Alert reference
	AlertID string `json:"alert_id"`

	// Notification channel information
	Channel    string `json:"channel"`     // gitlab, slack, email
	ExternalID string `json:"external_id"` // GitLab issue ID, Slack message ID, etc.

	// Status
	Status       string `json:"status"`        // sent, failed, pending
	ErrorMessage string `json:"error_message"` // Error details if failed

	// Metadata
	Metadata []byte `json:"metadata"` // Additional context as JSON
}
