package keycloakadmin

import (
	"net/http"
	"time"
)

// ClientConfig configures the Keycloak Admin API client
type ClientConfig struct {
	// Connection credentials — authentication uses the OAuth2 client_credentials grant.
	ServerURL    string
	AdminRealm   string
	ClientID     string
	ClientSecret string

	// HTTP settings
	Timeout       time.Duration // default: 30s
	SkipTLSVerify bool
	MaxRetries    int           // default: 3
	RetryBackoff  time.Duration // default: 5s

	// HTTPClient, when non-nil, replaces the default http.Client. This is
	// the integration point for SSRF-aware transports (see pkg/saferequest)
	// so the dial-time IP guard applies to every outbound call this client
	// makes — not just the initial test-connection. Timeout / TLS settings
	// above are ignored when HTTPClient is supplied; configure them on the
	// injected client instead.
	HTTPClient *http.Client

	// Realms to monitor (empty = all realms)
	Realms []string

	// Events configuration (for GetRecentEvents)
	EventTypes            []string
	EventLookbackDuration time.Duration
	MaxEventsPerPoll      int

	// Infinispan metrics port (0 = use default server port)
	InfinispanPort int
}

func DefaultConfig() *ClientConfig {
	return &ClientConfig{
		Timeout:               30 * time.Second,
		MaxRetries:            3,
		RetryBackoff:          5 * time.Second,
		EventLookbackDuration: 5 * time.Minute,
		MaxEventsPerPoll:      1000,
	}
}

// Validate checks if the configuration is valid.
//
// Authentication uses the OAuth2 client_credentials grant, so a confidential
// client id and secret are required.
func (c *ClientConfig) Validate() error {
	if c.ServerURL == "" {
		return ErrServerURLRequired
	}
	if c.AdminRealm == "" {
		return ErrAdminRealmRequired
	}
	if c.ClientID == "" {
		return ErrClientIDRequired
	}
	if c.ClientSecret == "" {
		return ErrClientSecretRequired
	}
	return nil
}

// ApplyDefaults fills in default values for unset fields
func (c *ClientConfig) ApplyDefaults() {
	defaults := DefaultConfig()

	if c.Timeout == 0 {
		c.Timeout = defaults.Timeout
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaults.MaxRetries
	}
	if c.RetryBackoff == 0 {
		c.RetryBackoff = defaults.RetryBackoff
	}
	if c.EventLookbackDuration == 0 {
		c.EventLookbackDuration = defaults.EventLookbackDuration
	}
	if c.MaxEventsPerPoll == 0 {
		c.MaxEventsPerPoll = defaults.MaxEventsPerPoll
	}
}
