package database

import (
	"time"

	"gorm.io/gorm"
)

// Event represents a generic monitoring event in the system.
// This can be used for Keycloak events or any other monitoring data.
type Event struct {
	ID        uint      `gorm:"primaryKey;index:idx_events_tenant_timestamp_id,priority:3,sort:desc"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// TenantID identifies which monitored Keycloak instance this row belongs
	// to. Every writer knows it: the Keycloak monitor and the AMFA mirror are
	// both constructed per tenant.
	//
	// Rows written before this column existed carry '' (the column default),
	// meaning "owner unknown". Those are excluded from every tenant-scoped
	// query, because equality against a real tenant id never matches ''. They
	// are also the reason tenant deletion could not previously remove a
	// tenant's events: there was nothing to delete by.
	TenantID string `gorm:"index;uniqueIndex:idx_events_tenant_event,priority:1;index:idx_events_tenant_timestamp_id,priority:1;default:''"`

	// Event identification
	//
	// event_id is unique per tenant, not globally: two tenants pointed at the
	// same Keycloak see the same event ids, and a global unique index made them
	// silently overwrite each other's rows.
	EventID   string    `gorm:"not null;uniqueIndex:idx_events_tenant_event,priority:2"`
	Timestamp time.Time `gorm:"index;not null;index:idx_events_tenant_timestamp_id,priority:2,sort:desc"`

	// Event classification
	Type        string `gorm:"index"`
	Category    string `gorm:"index"`
	Severity    string `gorm:"index"`
	Description string `gorm:"type:text"`

	// Source information
	Source       string `gorm:"index"`
	SourceIP     string
	SourceSystem string

	// User/Entity information
	UserID   string `gorm:"index"`
	Username string
	Email    string
	ClientID string `gorm:"index"`

	// Additional context
	Location string `gorm:"type:text"`
	RawData  []byte `gorm:"type:jsonb"`

	// AMFAEventID is the correlation/merge key shared with AMFA (auth_event.id),
	// carried on Keycloak login events as the "amfa_event_id" detail. Indexed
	// so both writers can look up a login's counterpart row quickly.
	AMFAEventID *string `gorm:"column:amfa_event_id;index"`

	// AMFA-specific fields (nullable; populated for events that have an AMFA
	// counterpart, mirrored or merged from a tenant's Adaptive MFA database)
	RiskLevel        *int     `gorm:"index"`
	FinalStatus      *string  `gorm:"type:text"`
	IsVPN            *bool    `gorm:"column:is_vpn"`
	Country          *string  `gorm:"type:text"`
	City             *string  `gorm:"type:text"`
	Lat              *float64 `gorm:"type:double precision"`
	Long             *float64 `gorm:"type:double precision"`
	OperatingSystem  *string  `gorm:"type:text"`
	Browser          *string  `gorm:"type:text"`
	Device           *string  `gorm:"type:text"`
	SystemLanguage   *string  `gorm:"type:text"`
	ScreenResolution *string  `gorm:"type:text"`

	// Metadata
	Status string `gorm:"type:text;not null;default:'active';index"`
}

// TableName specifies the table name for the Event model.
func (Event) TableName() string {
	return "events"
}

// User represents an authenticated user in the system.
// This stores user information from the OAuth2/OIDC provider.
type User struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// Identity provider fields (for OAuth2/OIDC)
	Subject           string `gorm:"uniqueIndex"` // OAuth2 "sub" claim. Unique user ID from IdP (nullable for simple auth users)
	Email             string `gorm:"uniqueIndex;not null"`
	EmailVerified     bool   `gorm:"default:false"`
	Name              string
	GivenName         string
	FamilyName        string
	PreferredUsername string `gorm:"index"`
	Locale            string

	// Simple auth fields (for username/password authentication)
	Username     string `gorm:"uniqueIndex"` // For simple auth (nullable for OAuth2 users)
	PasswordHash string // Bcrypt hashed password for simple auth
	AuthMethod   string `gorm:"default:'simple';index"` // 'simple' or 'oauth2'

	// Session tracking
	LastLoginAt    *time.Time
	LastLoginIP    string
	LoginCount     int `gorm:"default:0"`
	LastAccessedAt time.Time

	// Account status
	IsActive  bool `gorm:"default:true;index"`
	IsBlocked bool `gorm:"default:false;index"`

	// Password management
	MustChangePassword bool       `gorm:"default:false;index"` // Force password change on next login
	PasswordChangedAt  *time.Time // Last time password was changed

	// Audit fields
	BlockedAt     *time.Time
	BlockedReason string
}

// TableName specifies the table name for the User model.
func (User) TableName() string {
	return "users"
}

// Session represents an HTTP session in the database.
type Session struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time

	// Session identification
	SessionID string `gorm:"uniqueIndex;not null;size:255"` // Random session identifier

	// Session data
	Data []byte `gorm:"type:jsonb;not null"` // Serialized session data

	// Timestamps
	ExpiresAt      time.Time `gorm:"not null;index"` // When the session expires
	LastAccessedAt time.Time `gorm:"not null;index"` // Last time session was accessed

	// User reference
	UserID *uint `gorm:"index"` // Foreign key to users table
}

// TableName specifies the table name for the Session model.
func (Session) TableName() string {
	return "sessions"
}

// ============================================================================
// KEYCLOAK TENANT MANAGEMENT MODELS
// ============================================================================

// KeycloakTenant represents a monitored Keycloak instance (tenant)
// This is a regular PostgreSQL table for managing multiple Keycloak instances
type KeycloakTenant struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// Tenant identification
	TenantID    string `gorm:"uniqueIndex;not null;size:100"` // Unique identifier (e.g., "prod-main", "customer-abc")
	Name        string `gorm:"not null;size:255"`             // Display name
	Description string `gorm:"type:text"`                     // Optional description

	// Keycloak connection settings
	ServerURL    string `gorm:"not null"`            // Base URL of Keycloak instance
	AdminRealm   string `gorm:"not null"`            // Admin realm (usually "master")
	ClientID     string `gorm:"default:'admin-cli'"` // Client ID for client_credentials grant
	ClientSecret string // Client secret

	// Monitoring configuration (JSON)
	// Stores polling intervals, event filters, connection settings, etc.
	Configuration []byte `gorm:"type:jsonb"`

	// Default realm to display in UI
	DefaultRealm string `gorm:"size:255"` // Default realm name

	// Status tracking
	Enabled         bool       `gorm:"default:true;index"`      // Whether monitoring is enabled
	LastHealthCheck time.Time  `gorm:"index"`                   // Last time health was checked
	HealthStatus    string     `gorm:"default:'unknown';index"` // "healthy", "unhealthy", "unknown"
	HealthMessage   string     `gorm:"type:text"`               // Detailed health status message
	LastError       string     `gorm:"type:text"`               // Last error encountered
	LastErrorAt     *time.Time // When the last error occurred

	// Default tenant (only one tenant can be default at a time)
	IsDefault bool `gorm:"default:false;index"` // Whether this is the default tenant

	// Config-defined tenants are managed exclusively via config.yaml + restart;
	// set only by the bootstrap config-sync path, never via the public API.
	IsConfigDefined bool `gorm:"default:false;index"`

	// Metadata
	Tags  string `gorm:"type:text"` // JSON array of tags for organization
	Owner string // Owner/team responsible for this tenant

	// High Availability
	InfinispanEnabled bool `gorm:"default:false"` // Whether this tenant has InfiniSpan enabled
	InfinispanPort    int  `gorm:"default:0"`     // Port for InfiniSpan metrics (0 = use same port as server_url)

	// AMFA integration. Only the API endpoint is stored: reading AMFA over its
	// HTTP API needs no database credentials, and a tenant form must never ask
	// for a database host and password.
	// No index: this column is never used in a SQL WHERE/ORDER BY (only
	// compared in Go after the row is loaded), and it's low-cardinality, so an
	// index would add write overhead on every tenant insert/update with no
	// matching read benefit.
	AmfaEnabled          bool   `gorm:"default:false"`
	AmfaAPIBaseURL       string `gorm:"size:512"`
	AmfaEventsLookback   int    `gorm:"default:0"` // Days; 0 applies the platform default
	AmfaAPITimeoutSecond int    `gorm:"default:0"` // 0 applies the platform default
}

// TableName specifies the table name for KeycloakTenant.
func (KeycloakTenant) TableName() string {
	return "keycloak_tenants"
}

// ============================================================================
// KEYCLOAK MONITORING MODELS (TIMESCALEDB OPTIMIZED)
// ============================================================================

// KeycloakEvent represents a Keycloak event (login, logout, registration, etc.)
// This is a time-series table (hypertable) optimized for TimescaleDB
type KeycloakEvent struct {
	// Tenant dimension - identifies which Keycloak instance this event belongs to
	TenantID string `gorm:"primaryKey;not null;size:100;index:idx_keycloak_events_tenant"`
	// Time dimension - PRIMARY for TimescaleDB hypertable
	Time time.Time `gorm:"primaryKey;not null;index:idx_keycloak_events_time,priority:1"` // TimescaleDB time column

	// Event identification (composite primary key with Time)
	EventID string `gorm:"primaryKey;not null;size:255"` // Keycloak event ID

	// Realm and client (high cardinality dimensions)
	RealmID   string `gorm:"not null;index:idx_keycloak_events_realm"`
	RealmName string `gorm:"index:idx_keycloak_events_realm_name"`
	ClientID  string `gorm:"index:idx_keycloak_events_client"`

	// Session tracking
	SessionID string `gorm:"index:idx_keycloak_events_session"`

	// Network information
	IPAddress string `gorm:"index:idx_keycloak_events_ip"`

	// Event classification
	EventType  string `gorm:"not null;index:idx_keycloak_events_type"` // LOGIN, LOGOUT, REGISTER, CODE_TO_TOKEN, etc.
	EventError string `gorm:"index:idx_keycloak_events_error"`         // Error type if event failed

	// User information
	UserID   string `gorm:"index:idx_keycloak_events_user"`
	Username string `gorm:"index:idx_keycloak_events_username"`
	Email    string

	// Event details (JSONB for flexible schema)
	Details []byte `gorm:"type:jsonb"` // Additional event details as JSON

	// Status
	Success bool `gorm:"not null;default:true;index:idx_keycloak_events_success"`
}

// TableName specifies the table name for KeycloakEvent.
func (KeycloakEvent) TableName() string {
	return "keycloak_events"
}

// KeycloakMetrics stores periodic snapshots of Keycloak metrics
// This is a time-series table (hypertable) optimized for TimescaleDB
type KeycloakMetrics struct {
	// Tenant dimension - identifies which Keycloak instance these metrics belong to
	TenantID string `gorm:"primaryKey;not null;size:100;index:idx_keycloak_metrics_tenant"`

	// Time dimension - PRIMARY for TimescaleDB hypertable
	Time time.Time `gorm:"primaryKey;not null;index:idx_keycloak_metrics_time,priority:1"`

	// Realm dimension (composite primary key with Time)
	RealmName string `gorm:"primaryKey;not null;size:255;index:idx_keycloak_metrics_realm"`

	// User metrics
	TotalUsers    int `gorm:"not null;default:0"`
	EnabledUsers  int `gorm:"not null;default:0"`
	DisabledUsers int `gorm:"not null;default:0"`

	// Session metrics
	ActiveSessions  int `gorm:"not null;default:0"`
	OfflineSessions int `gorm:"not null;default:0"`

	// Client metrics
	TotalClients int `gorm:"not null;default:0"`

	// Event metrics (aggregated from events in the last collection period)
	LoginEvents       int `gorm:"not null;default:0"`
	LogoutEvents      int `gorm:"not null;default:0"`
	FailedLoginEvents int `gorm:"not null;default:0"`
	RegisterEvents    int `gorm:"not null;default:0"`

	// Additional metrics as JSON
	RawMetrics []byte `gorm:"type:jsonb"`
}

// TableName specifies the table name for KeycloakMetrics.
func (KeycloakMetrics) TableName() string {
	return "keycloak_metrics"
}

// KeycloakHealth stores Keycloak server health check results
// This is a time-series table (hypertable) optimized for TimescaleDB
type KeycloakHealth struct {
	// Tenant dimension - identifies which Keycloak instance this health check belongs to
	TenantID string `gorm:"primaryKey;not null;size:100;index:idx_keycloak_health_tenant"`

	// Time dimension - PRIMARY for TimescaleDB hypertable
	Time time.Time `gorm:"primaryKey;not null;index:idx_keycloak_health_time"`

	// Overall health status
	Status string `gorm:"not null;index:idx_keycloak_health_status"` // UP, DOWN, DEGRADED

	// Response metrics
	ResponseTime int64 `gorm:"not null;default:0"` // Response time in milliseconds

	// Server info
	ServerVersion string

	// Memory metrics (from server info)
	MemoryUsed int64 `gorm:"not null;default:0"` // Bytes
	MemoryMax  int64 `gorm:"not null;default:0"` // Bytes
	MemoryFree int64 `gorm:"not null;default:0"` // Bytes

	// Uptime
	UptimeMillis int64 `gorm:"not null;default:0"`

	// Additional health data as JSON
	RawData []byte `gorm:"type:jsonb"`

	// Error information
	ErrorMessage string `gorm:"type:text"`
}

// TableName specifies the table name for KeycloakHealth.
func (KeycloakHealth) TableName() string {
	return "keycloak_health"
}

// ============================================================================
// KEYCLOAK RELATIONAL MODELS (REGULAR POSTGRESQL TABLES)
// ============================================================================

// KeycloakRealmInfo stores information about Keycloak realms
// This is a regular PostgreSQL table, not a hypertable
type KeycloakRealmInfo struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// Tenant dimension - identifies which Keycloak instance this realm belongs to
	// Participates in both composite unique indexes below, since a bare
	// uniqueIndex:idx_keycloak_realms_tenant_realm_name on RealmName alone
	// (without TenantID) makes a realm name globally unique across all
	// tenants instead of unique per-tenant, wrongly blocking any second
	// tenant pointed at the same Keycloak instance from saving realm info.
	TenantID string `gorm:"not null;size:100;index:idx_keycloak_realms_tenant;uniqueIndex:idx_keycloak_realms_tenant_realm;uniqueIndex:idx_keycloak_realms_tenant_realm_name"`

	// Realm identification
	RealmID     string `gorm:"not null;uniqueIndex:idx_keycloak_realms_tenant_realm"`
	RealmName   string `gorm:"not null;uniqueIndex:idx_keycloak_realms_tenant_realm_name"`
	DisplayName string

	// Realm settings
	Enabled                bool `gorm:"default:true"`
	SslRequired            string
	RegistrationAllowed    bool `gorm:"default:false"`
	RememberMe             bool `gorm:"default:false"`
	VerifyEmail            bool `gorm:"default:false"`
	LoginWithEmailAllowed  bool `gorm:"default:true"`
	DuplicateEmailsAllowed bool `gorm:"default:false"`
	ResetPasswordAllowed   bool `gorm:"default:false"`
	EditUsernameAllowed    bool `gorm:"default:false"`
	BruteForceProtected    bool `gorm:"default:false"`

	// Event configuration
	EventsEnabled             bool   `gorm:"default:false"`
	EventsListeners           string `gorm:"type:text"` // JSON array of listeners
	EnabledEventTypes         string `gorm:"type:text"` // JSON array of event types
	AdminEventsEnabled        bool   `gorm:"default:false"`
	AdminEventsDetailsEnabled bool   `gorm:"default:false"`

	// Additional realm data as JSON
	RawData []byte `gorm:"type:jsonb"`

	// Monitoring status
	LastChecked   time.Time `gorm:"index"`
	IsHealthy     bool      `gorm:"default:true;index"`
	HealthMessage string    `gorm:"type:text"`
}

// TableName specifies the table name for KeycloakRealmInfo.
func (KeycloakRealmInfo) TableName() string {
	return "keycloak_realms"
}

// ============================================================================
// CONFIGURATION ALERTS MODELS (REGULAR POSTGRESQL TABLES)
// ============================================================================

// ConfigurationAlert represents a configuration issue or misconfiguration alert
// This is a regular PostgreSQL table for storing detected configuration issues
// Alert represents a unified alert from multiple sources (config, events, logs, metrics).
// Formerly known as ConfigurationAlert.
type Alert struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// Multi-tenancy support
	// TenantID also participates in idx_alert_tenant below, since a bare
	// uniqueIndex:idx_alert_tenant on AlertID alone (without TenantID) makes
	// an alert_id globally unique across all tenants instead of unique
	// per-tenant, wrongly merging two different tenants' alerts for the same
	// finding whenever they share a realm name (same class of bug as
	// KeycloakRealmInfo.TenantID above).
	TenantID string `gorm:"not null;index:idx_alerts_tenant;uniqueIndex:idx_alert_tenant"` // Tenant identifier

	// Alert identification (unique to prevent duplicates per tenant)
	AlertID string `gorm:"uniqueIndex:idx_alert_tenant;not null;size:255"` // Unique ID for deduplication per tenant

	// Alert source and classification
	Source   string `gorm:"not null;default:'configuration';index:idx_alerts_source"` // configuration, event, log, metric
	Type     string `gorm:"not null;index:idx_alerts_type"`                           // configuration, security, compliance, event, etc.
	Severity string `gorm:"not null;index:idx_alerts_severity"`                       // info, warning, error, critical
	Status   string `gorm:"not null;default:'active';index:idx_alerts_status"`

	// Alert content
	Title          string `gorm:"not null;size:255"`
	Description    string `gorm:"type:text"`
	Recommendation string `gorm:"type:text"`

	// Resource information
	ResourceType string `gorm:"not null;index:idx_alerts_resource_type"` // realm, client, identity_provider, etc.
	ResourceID   string `gorm:"not null;index:idx_alerts_resource_id"`
	ResourceName string `gorm:"not null;index:idx_alerts_resource_name"`
	RealmName    string `gorm:"not null;index:idx_alerts_realm"`

	// Source tracking
	CheckType string  `gorm:"index:idx_alerts_check_type"` // Type of check that generated this (for config alerts)
	RuleID    *string `gorm:"index:idx_alerts_rule_id"`    // Alert rule that triggered this (for event/log/metric alerts)
	EventID   *string `gorm:"index:idx_alerts_event_id"`   // Source event ID (for event-based alerts)

	// Metadata
	Metadata []byte `gorm:"type:jsonb"` // Additional context as JSON

	// Tracking
	FirstDetected  time.Time  `gorm:"not null;index:idx_alerts_first_detected"`
	LastSeen       time.Time  `gorm:"not null;index:idx_alerts_last_seen"`
	ResolvedAt     *time.Time `gorm:"index:idx_alerts_resolved"`
	AcknowledgedAt *time.Time `gorm:"index:idx_alerts_acknowledged"`
	AcknowledgedBy string     `gorm:"size:255"`
}

// TableName specifies the table name for Alert.
// We keep the old name "configuration_alerts" for backward compatibility.
func (Alert) TableName() string {
	return "configuration_alerts"
}

// ConfigurationAlert is a backward compatibility alias
// Deprecated: Use Alert instead
type ConfigurationAlert = Alert

// AlertRule represents a rule that can trigger alerts based on events, logs, or metrics
type AlertRule struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// Multi-tenancy support (nil for global rules)
	TenantID *string `gorm:"index:idx_alert_rules_tenant"` // Tenant identifier (NULL = global)

	// Rule identification
	RuleID string `gorm:"uniqueIndex;not null;size:255"` // Unique rule identifier (e.g., "failed-login-threshold")

	// Rule metadata
	Name        string `gorm:"not null;size:255"`
	Description string `gorm:"type:text"`
	Enabled     bool   `gorm:"not null;default:true;index:idx_alert_rules_enabled"`

	// Source and severity
	Source   string `gorm:"not null;index:idx_alert_rules_source"`   // event, log, metric
	Severity string `gorm:"not null;index:idx_alert_rules_severity"` // info, warning, error, critical

	// Conditions (stored as JSON)
	Conditions []byte `gorm:"type:jsonb;not null"` // RuleConditions as JSON

	// Alert templates
	TitleTemplate          string `gorm:"not null;type:text"`
	DescriptionTemplate    string `gorm:"type:text"`
	RecommendationTemplate string `gorm:"type:text"`

	// Notification settings
	NotifySlack  bool    `gorm:"not null;default:false"`
	SlackChannel *string `gorm:"size:255"` // Override default channel

	// Metadata
	CreatedBy string `gorm:"size:255"` // Email/ID of creator
}

// TableName specifies the table name for AlertRule.
func (AlertRule) TableName() string {
	return "alert_rules"
}

// ============================================================================
// NOTIFICATION TRACKING MODELS (REGULAR POSTGRESQL TABLES)
// ============================================================================

// NotificationLog represents a log entry for sent notifications
// This tracks which alerts have been sent to which notification channels
type NotificationLog struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time

	// Alert reference
	AlertID string `gorm:"not null;index:idx_notification_logs_alert_id"`

	// Notification channel information
	Channel    string `gorm:"not null;index:idx_notification_logs_channel"` // gitlab, slack, email
	ExternalID string `gorm:"index:idx_notification_logs_external_id"`      // GitLab issue ID, Slack message ID, etc.

	// Status
	Status       string `gorm:"not null;default:'sent';index:idx_notification_logs_status"` // sent, failed, pending
	ErrorMessage string `gorm:"type:text"`

	// Metadata
	Metadata []byte `gorm:"type:jsonb"` // Additional context as JSON
}

// TableName specifies the table name for NotificationLog.
func (NotificationLog) TableName() string {
	return "notification_logs"
}

// ============================================================================
// OPERATOR METRICS MODELS (REGULAR POSTGRESQL TABLES)
// ============================================================================

// OperatorAction represents an action taken by an operator on an alert
// This tracks detailed metrics about operator performance and response times
type OperatorAction struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time

	// Multi-tenancy support
	TenantID string `gorm:"not null;index:idx_operator_actions_tenant"`

	// Alert reference
	AlertID uint                `gorm:"not null;index:idx_operator_actions_alert_id"` // FK to ConfigurationAlert
	Alert   *ConfigurationAlert `gorm:"foreignKey:AlertID;constraint:OnDelete:CASCADE"`

	// Operator information
	OperatorID    uint   `gorm:"not null;index:idx_operator_actions_operator_id"` // FK to User
	Operator      *User  `gorm:"foreignKey:OperatorID;constraint:OnDelete:SET NULL"`
	OperatorEmail string `gorm:"not null;index:idx_operator_actions_operator_email;size:255"`

	// Action details
	ActionType string    `gorm:"not null;index:idx_operator_actions_type"` // acknowledged, resolved, ignored, commented, status_changed
	ActionTime time.Time `gorm:"not null;index:idx_operator_actions_time"`

	// Previous state information (for tracking transitions)
	PreviousStatus     string     `gorm:"size:50"` // Previous alert status before this action
	PreviousActionTime *time.Time // Time of the previous action on this alert

	// Alert context at time of action
	AlertSeverity string `gorm:"not null;index:idx_operator_actions_severity"`   // critical, high, medium, low
	AlertType     string `gorm:"not null;index:idx_operator_actions_alert_type"` // configuration, security, compliance, etc.
	RealmName     string `gorm:"not null;index:idx_operator_actions_realm"`

	// Time metrics - Absolute times from alert detection
	TimeFromDetectionSeconds int `gorm:"index:idx_operator_actions_response_time"` // Time from FirstDetected to this action

	// Time metrics - Transition times (time between actions)
	TimeFromPreviousActionSeconds int `gorm:"index:idx_operator_actions_transition_time"` // Time since last action on this alert

	// Legacy fields (kept for backwards compatibility)
	ResponseTimeSeconds   int // Deprecated: use TimeFromDetectionSeconds
	ResolutionTimeSeconds int // Deprecated: calculated from final resolved action

	// Additional metadata
	Comment  string `gorm:"type:text"`  // Optional comment provided by operator
	Metadata []byte `gorm:"type:jsonb"` // Additional context as JSON
}

// TableName specifies the table name for OperatorAction.
func (OperatorAction) TableName() string {
	return "operator_actions"
}

// OperatorMetricsSummary represents aggregated metrics for an operator over a time period
// This is computed on-demand and not stored in the database
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
// RBAC MODELS (ROLE-BASED ACCESS CONTROL)
// ============================================================================

// Role represents a role in the system (e.g., Admin, Operator, Viewer)
// Roles define a set of permissions that can be assigned to users
type Role struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// Role identification
	Name        string `gorm:"uniqueIndex;not null;size:100"` // e.g., "admin", "operator", "viewer"
	DisplayName string `gorm:"not null;size:255"`             // e.g., "System Administrator"
	Description string `gorm:"type:text"`

	// System role flag - system roles cannot be deleted
	IsSystem bool `gorm:"default:false;index"`

	// Role status
	IsActive bool `gorm:"default:true;index"`

	// Relationships
	Permissions []Permission `gorm:"many2many:role_permissions;"`
	Users       []User       `gorm:"many2many:user_roles;"`
}

// TableName specifies the table name for Role.
func (Role) TableName() string {
	return "roles"
}

// Permission represents a permission in the system
// Permissions are granular actions that can be performed (e.g., "tenants:read", "alerts:write")
type Permission struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// Permission identification
	Name        string `gorm:"uniqueIndex;not null;size:100"` // e.g., "tenants:read", "alerts:acknowledge"
	DisplayName string `gorm:"not null;size:255"`             // e.g., "Read Tenants"
	Description string `gorm:"type:text"`

	// Resource and action
	Resource string `gorm:"not null;index;size:100"` // e.g., "tenants", "alerts", "events"
	Action   string `gorm:"not null;index;size:100"` // e.g., "read", "write", "delete", "acknowledge"

	// System permission flag - system permissions cannot be deleted
	IsSystem bool `gorm:"default:false;index"`

	// Relationships
	Roles []Role `gorm:"many2many:role_permissions;"`
}

// TableName specifies the table name for Permission.
func (Permission) TableName() string {
	return "permissions"
}

// UserRole represents the many-to-many relationship between users and roles
// Supports tenant-scoped role assignments for multi-tenancy
type UserRole struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time

	// User and Role references
	UserID uint  `gorm:"not null;index:idx_user_roles_user;uniqueIndex:idx_user_roles_unique"`
	User   *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`

	RoleID uint  `gorm:"not null;index:idx_user_roles_role;uniqueIndex:idx_user_roles_unique"`
	Role   *Role `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE"`

	// Tenant scoping - if null, role applies to all tenants (global)
	// If specified, role only applies to this specific tenant
	TenantID *string `gorm:"index:idx_user_roles_tenant;uniqueIndex:idx_user_roles_unique;size:100"`

	// Assignment metadata
	AssignedBy string     `gorm:"size:255"` // Username or system that assigned this role
	AssignedAt time.Time  `gorm:"not null"`
	ExpiresAt  *time.Time `gorm:"index"` // Optional expiration for temporary role assignments
}

// TableName specifies the table name for UserRole.
func (UserRole) TableName() string {
	return "user_roles"
}

// RolePermission represents the many-to-many relationship between roles and permissions
type RolePermission struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`

	// Role and Permission references
	RoleID uint  `gorm:"not null;index:idx_role_permissions_role;uniqueIndex:idx_role_permissions_unique"`
	Role   *Role `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE"`

	PermissionID uint        `gorm:"not null;index:idx_role_permissions_permission;uniqueIndex:idx_role_permissions_unique"`
	Permission   *Permission `gorm:"foreignKey:PermissionID;constraint:OnDelete:CASCADE"`
}

// TableName specifies the table name for RolePermission.
func (RolePermission) TableName() string {
	return "role_permissions"
}

// TenantPolicy represents tenant-level access policies for users
// This allows operators and viewers to be restricted to specific tenants and realms
type TenantPolicy struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// User reference
	UserID uint  `gorm:"not null;index:idx_tenant_policies_user"`
	User   *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`

	// Tenant scoping
	TenantID string `gorm:"not null;index:idx_tenant_policies_tenant;size:100"`

	// Realm restrictions (JSON array of realm names)
	// If specified, user only has access to listed realms
	// An empty list `[]` grants no realm; a SQL NULL column and the stored `null`
	// literal both grant every realm
	AllowedRealms []byte `gorm:"type:jsonb"` // JSON array: ["realm1", "realm2"]

	// Policy metadata
	GrantedBy string    `gorm:"size:255"` // Username or system that granted this policy
	GrantedAt time.Time `gorm:"not null"`
}

// TableName specifies the table name for TenantPolicy.
func (TenantPolicy) TableName() string {
	return "tenant_policies"
}

// APIToken represents a personal access token for programmatic API access.
// Only the SHA-256 hex digest of the token is stored; the plaintext is shown
// once at creation and never persisted. Last-use tracking is deferred; there
// is no last_used_at column yet.
type APIToken struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`

	// User reference
	UserID uint  `gorm:"not null;index:idx_api_tokens_user"`
	User   *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`

	// Token identification
	Name        string `gorm:"not null;size:255"`
	TokenDigest string `gorm:"uniqueIndex;not null;size:64"` // SHA-256 hex of the plaintext token

	// Tenant restrictions (JSON array of tenant IDs)
	// If null or empty, the token inherits the user's RBAC scope
	TenantIDs []byte `gorm:"type:jsonb"` // JSON array: ["tenant1", "tenant2"]

	// Lifecycle
	ExpiresAt *time.Time `gorm:"index"`
	RevokedAt *time.Time `gorm:"index"`
}

// TableName specifies the table name for APIToken.
func (APIToken) TableName() string {
	return "api_tokens"
}

// PendingTenantPurge is a work queue entry for telemetry belonging to a deleted
// tenant. Rows are written inside the deletion transaction and removed once the
// tenant's telemetry has been fully drained at startup.
type PendingTenantPurge struct {
	TenantID    string    `gorm:"primaryKey;size:100"`
	RequestedAt time.Time `gorm:"not null"`
}

// TableName specifies the table name for PendingTenantPurge.
func (PendingTenantPurge) TableName() string {
	return "pending_tenant_purges"
}

// AmfaMirrorWatermark records how far the AMFA events mirror has read for one
// (tenant, realm) pair, so a restart resumes where it left off.
//
// The mirror used to re-derive this from the events table with
// MAX(timestamp) WHERE source = 'amfa:<realm>'. That was wrong twice over.
// The events table has no tenant column and a source string is only
// "amfa:<realm>", while realm names are unique per-tenant rather than globally
// (see KeycloakRealmInfo.TenantID), so two tenants monitoring a realm of the
// same name shared one watermark and whichever polled second skipped its own
// older events for good. It was also derived from rows that
// ReconcileAMFAMerges deletes once they merge onto their Keycloak twin, so the
// value could go backwards or vanish. Keying the position on (tenant, realm)
// here removes both problems.
type AmfaMirrorWatermark struct {
	TenantID  string    `gorm:"primaryKey;size:100"`
	Realm     string    `gorm:"primaryKey;size:255"`
	EventTime time.Time `gorm:"not null"`
	UpdatedAt time.Time
}

// TableName specifies the table name for AmfaMirrorWatermark.
func (AmfaMirrorWatermark) TableName() string {
	return "amfa_mirror_watermarks"
}
