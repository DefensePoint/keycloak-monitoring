package keycloakadmin

// ============================================================================
// AUTHENTICATION MODELS
// ============================================================================

// TokenResponse represents an OAuth2 token response from Keycloak
type TokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	NotBeforePolicy  int    `json:"not-before-policy"`
	SessionState     string `json:"session_state"`
	Scope            string `json:"scope"`
}

// ============================================================================
// REALM MODELS
// ============================================================================

// RealmRepresentation represents a Keycloak realm
type RealmRepresentation struct {
	ID                           string   `json:"id"`
	Realm                        string   `json:"realm"`
	DisplayName                  string   `json:"displayName"`
	DisplayNameHTML              string   `json:"displayNameHtml"`
	Enabled                      bool     `json:"enabled"`
	SslRequired                  string   `json:"sslRequired"`
	RegistrationAllowed          bool     `json:"registrationAllowed"`
	RegistrationEmailAsUsername  bool     `json:"registrationEmailAsUsername"`
	RememberMe                   bool     `json:"rememberMe"`
	VerifyEmail                  bool     `json:"verifyEmail"`
	LoginWithEmailAllowed        bool     `json:"loginWithEmailAllowed"`
	DuplicateEmailsAllowed       bool     `json:"duplicateEmailsAllowed"`
	ResetPasswordAllowed         bool     `json:"resetPasswordAllowed"`
	EditUsernameAllowed          bool     `json:"editUsernameAllowed"`
	BruteForceProtected          bool     `json:"bruteForceProtected"`
	PermanentLockout             bool     `json:"permanentLockout"`
	MaxFailureWaitSeconds        int      `json:"maxFailureWaitSeconds"`
	MinimumQuickLoginWaitSeconds int      `json:"minimumQuickLoginWaitSeconds"`
	WaitIncrementSeconds         int      `json:"waitIncrementSeconds"`
	QuickLoginCheckMilliSeconds  int64    `json:"quickLoginCheckMilliSeconds"`
	MaxDeltaTimeSeconds          int      `json:"maxDeltaTimeSeconds"`
	FailureFactor                int      `json:"failureFactor"`
	PasswordPolicy               string   `json:"passwordPolicy"`
	EventsEnabled                bool     `json:"eventsEnabled"`
	EventsListeners              []string `json:"eventsListeners"`
	EnabledEventTypes            []string `json:"enabledEventTypes"`
	AdminEventsEnabled           bool     `json:"adminEventsEnabled"`
	AdminEventsDetailsEnabled    bool     `json:"adminEventsDetailsEnabled"`
	// BrowserFlow is the alias of the authentication flow bound to interactive
	// browser logins at the realm level (default "browser").
	BrowserFlow string `json:"browserFlow"`
}

// AuthenticationFlowRepresentation is one authentication flow in a realm
// (GET /admin/realms/{realm}/authentication/flows). Only top-level flows are
// returned by that endpoint; their executions include nested subflows.
type AuthenticationFlowRepresentation struct {
	ID          string `json:"id"`
	Alias       string `json:"alias"`
	Description string `json:"description"`
	ProviderID  string `json:"providerId"`
	TopLevel    bool   `json:"topLevel"`
	BuiltIn     bool   `json:"builtIn"`
}

// AuthenticationExecutionInfoRepresentation is one entry in a realm
// authentication flow's flattened execution list
// (GET /admin/realms/{realm}/authentication/flows/{flowAlias}/executions).
// Subflow rows have AuthenticationFlow=true and an empty ProviderID; authenticator
// rows carry the ProviderID of the authenticator implementation.
type AuthenticationExecutionInfoRepresentation struct {
	ID                 string `json:"id"`
	Requirement        string `json:"requirement"`
	DisplayName        string `json:"displayName"`
	ProviderID         string `json:"providerId"`
	AuthenticationFlow bool   `json:"authenticationFlow"`
	Level              int    `json:"level"`
	Index              int    `json:"index"`
}

// ============================================================================
// USER MODELS
// ============================================================================

// UserRepresentation represents a Keycloak user
type UserRepresentation struct {
	ID               string              `json:"id"`
	CreatedTimestamp int64               `json:"createdTimestamp"`
	Username         string              `json:"username"`
	Enabled          bool                `json:"enabled"`
	EmailVerified    bool                `json:"emailVerified"`
	FirstName        string              `json:"firstName"`
	LastName         string              `json:"lastName"`
	Email            string              `json:"email"`
	FederationLink   string              `json:"federationLink"`
	Attributes       map[string][]string `json:"attributes"`
	RequiredActions  []string            `json:"requiredActions"`
	NotBefore        int                 `json:"notBefore"`
	Access           map[string]bool     `json:"access"`
}

// GroupRepresentation represents a Keycloak group
type GroupRepresentation struct {
	ID         string                `json:"id"`
	Name       string                `json:"name"`
	Path       string                `json:"path"`
	SubGroups  []GroupRepresentation `json:"subGroups"`
	Attributes map[string][]string   `json:"attributes"`
}

// RoleRepresentation represents a Keycloak role
type RoleRepresentation struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Composite   bool   `json:"composite"`
	ClientRole  bool   `json:"clientRole"`
	ContainerID string `json:"containerId"`
}

// RoleMappingsRepresentation represents role mappings for a user
type RoleMappingsRepresentation struct {
	RealmMappings  []RoleRepresentation           `json:"realmMappings"`
	ClientMappings map[string]*ClientRoleMappings `json:"clientMappings"`
}

// ClientRoleMappings represents client-specific role mappings
type ClientRoleMappings struct {
	ID       string               `json:"id"`
	Client   string               `json:"client"`
	Mappings []RoleRepresentation `json:"mappings"`
}

// UserDetails represents complete user information including groups and roles
type UserDetails struct {
	User         *UserRepresentation         `json:"user"`
	Groups       []GroupRepresentation       `json:"groups"`
	RoleMappings *RoleMappingsRepresentation `json:"roleMappings"`
}

// ============================================================================
// CLIENT MODELS
// ============================================================================

// ClientRepresentation represents a Keycloak client
type ClientRepresentation struct {
	ID                        string            `json:"id"`
	ClientID                  string            `json:"clientId"`
	Name                      string            `json:"name"`
	Description               string            `json:"description"`
	RootURL                   string            `json:"rootUrl"`
	AdminURL                  string            `json:"adminUrl"`
	BaseURL                   string            `json:"baseUrl"`
	SurrogateAuthRequired     bool              `json:"surrogateAuthRequired"`
	Enabled                   bool              `json:"enabled"`
	AlwaysDisplayInConsole    bool              `json:"alwaysDisplayInConsole"`
	ClientAuthenticatorType   string            `json:"clientAuthenticatorType"`
	Secret                    string            `json:"secret"`
	RedirectUris              []string          `json:"redirectUris"`
	WebOrigins                []string          `json:"webOrigins"`
	NotBefore                 int               `json:"notBefore"`
	BearerOnly                bool              `json:"bearerOnly"`
	ConsentRequired           bool              `json:"consentRequired"`
	StandardFlowEnabled       bool              `json:"standardFlowEnabled"`
	ImplicitFlowEnabled       bool              `json:"implicitFlowEnabled"`
	DirectAccessGrantsEnabled bool              `json:"directAccessGrantsEnabled"`
	ServiceAccountsEnabled    bool              `json:"serviceAccountsEnabled"`
	PublicClient              bool              `json:"publicClient"`
	FrontchannelLogout        bool              `json:"frontchannelLogout"`
	Protocol                  string            `json:"protocol"`
	Attributes                map[string]string `json:"attributes"`
	FullScopeAllowed          bool              `json:"fullScopeAllowed"`
	NodeReRegistrationTimeout int               `json:"nodeReRegistrationTimeout"`
	ProtocolMappers           []interface{}     `json:"protocolMappers"`
}

// ============================================================================
// EVENT MODELS
// ============================================================================

// EventRepresentation represents an event in Keycloak
type EventRepresentation struct {
	ID        string            `json:"id"`
	Time      int64             `json:"time"`
	Type      string            `json:"type"`
	RealmID   string            `json:"realmId"`
	ClientID  string            `json:"clientId"`
	UserID    string            `json:"userId"`
	SessionID string            `json:"sessionId"`
	IPAddress string            `json:"ipAddress"`
	Error     string            `json:"error"`
	Details   map[string]string `json:"details"`
}

// AdminEventRepresentation represents an admin event in Keycloak
type AdminEventRepresentation struct {
	Time           int64        `json:"time"`
	RealmID        string       `json:"realmId"`
	AuthDetails    *AuthDetails `json:"authDetails"`
	ResourceType   string       `json:"resourceType"`
	OperationType  string       `json:"operationType"`
	ResourcePath   string       `json:"resourcePath"`
	Representation string       `json:"representation"`
	Error          string       `json:"error"`
}

// AuthDetails represents authentication details for admin events
type AuthDetails struct {
	RealmID   string `json:"realmId"`
	ClientID  string `json:"clientId"`
	UserID    string `json:"userId"`
	IPAddress string `json:"ipAddress"`
}

// ============================================================================
// SERVER INFO MODELS
// ============================================================================

// ServerInfoRepresentation represents Keycloak server information
type ServerInfoRepresentation struct {
	SystemInfo      *SystemInfo              `json:"systemInfo"`
	MemoryInfo      *MemoryInfo              `json:"memoryInfo"`
	ProfileInfo     *ProfileInfo             `json:"profileInfo"`
	Themes          map[string][]interface{} `json:"themes"`
	SocialProviders []interface{}            `json:"socialProviders"`
	Providers       map[string]interface{}   `json:"providers"`
}

// SystemInfo represents system information
type SystemInfo struct {
	Version        string `json:"version"`
	ServerTime     string `json:"serverTime"`
	Uptime         string `json:"uptime"`
	UptimeMillis   int64  `json:"uptimeMillis"`
	JavaVersion    string `json:"javaVersion"`
	JavaVendor     string `json:"javaVendor"`
	JavaVM         string `json:"javaVm"`
	JavaVMVersion  string `json:"javaVmVersion"`
	JavaRuntime    string `json:"javaRuntime"`
	JavaHome       string `json:"javaHome"`
	OSName         string `json:"osName"`
	OSArchitecture string `json:"osArchitecture"`
	OSVersion      string `json:"osVersion"`
	FileEncoding   string `json:"fileEncoding"`
	UserName       string `json:"userName"`
	UserDir        string `json:"userDir"`
	UserTimezone   string `json:"userTimezone"`
	UserLocale     string `json:"userLocale"`
}

// MemoryInfo represents memory information
type MemoryInfo struct {
	Total          int64  `json:"total"`
	TotalFormated  string `json:"totalFormated"`
	Used           int64  `json:"used"`
	UsedFormated   string `json:"usedFormated"`
	Free           int64  `json:"free"`
	FreeFormated   string `json:"freeFormated"`
	FreePercentage int64  `json:"freePercentage"`
}

// ProfileInfo represents profile information
type ProfileInfo struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	DisabledFeatures     []string `json:"disabledFeatures"`
	PreviewFeatures      []string `json:"previewFeatures"`
	ExperimentalFeatures []string `json:"experimentalFeatures"`
}

// ============================================================================
// IDENTITY PROVIDER MODELS
// ============================================================================

// IdentityProviderRepresentation represents a Keycloak identity provider
// Note: We only include fields we actually use. Keycloak returns many other fields
// that can be inconsistent types (bool/string) across versions.
type IdentityProviderRepresentation struct {
	Alias       string            `json:"alias"`
	DisplayName string            `json:"displayName"`
	InternalID  string            `json:"internalId"`
	ProviderID  string            `json:"providerId"`
	Enabled     bool              `json:"enabled"`
	Config      map[string]string `json:"config"`
}

// ============================================================================
// ERROR MODELS
// ============================================================================

// ErrorResponse represents an error response from Keycloak
type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	ErrorMessage     string `json:"errorMessage"`
}
