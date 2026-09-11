// Package tenant provides the tenant domain model and services.
// Tenants represent Keycloak instances being monitored.
package tenant

import "github.com/DefensePoint/keycloak-monitoring/internal/domain"

// Event represents a change event for a tenant.
type Event string

const (
	EventAdded    Event = "added"
	EventUpdated  Event = "updated"
	EventRemoved  Event = "removed"
	EventEnabled  Event = "enabled"
	EventDisabled Event = "disabled"
)

// ChangeCallback is called when a tenant changes.
type ChangeCallback func(event Event, tenant *domain.KeycloakTenant)

// CreateRequest represents the request to create a new tenant.
type CreateRequest struct {
	TenantID      string       `json:"tenant_id"`
	Name          string       `json:"name"`
	Description   string       `json:"description,omitempty"`
	ServerURL     string       `json:"server_url"`
	AdminRealm    string       `json:"admin_realm"`
	ClientID      string       `json:"client_id,omitempty"`
	ClientSecret  string       `json:"client_secret,omitempty"`
	Configuration string       `json:"configuration,omitempty"`
	DefaultRealm  string       `json:"default_realm,omitempty"`
	Enabled       bool         `json:"enabled"`
	IsDefault     bool         `json:"is_default,omitempty"`
	Tags          []string     `json:"tags,omitempty"`
	Owner         string       `json:"owner,omitempty"`
	Amfa          *AmfaRequest `json:"amfa,omitempty"`
}

// AmfaRequest carries a tenant's AMFA settings on create and update.
//
// The API endpoint only. Reading AMFA over HTTP needs no database credentials,
// so none are accepted here.
type AmfaRequest struct {
	Enabled            bool   `json:"enabled"`
	APIBaseURL         string `json:"api_base_url"`
	EventsLookbackDays int    `json:"events_lookback_days,omitempty"`
	APITimeoutSeconds  int    `json:"api_timeout_seconds,omitempty"`
}

// UpdateRequest represents the request to update a tenant.
type UpdateRequest struct {
	Name          *string      `json:"name,omitempty"`
	Description   *string      `json:"description,omitempty"`
	ServerURL     *string      `json:"server_url,omitempty"`
	AdminRealm    *string      `json:"admin_realm,omitempty"`
	ClientID      *string      `json:"client_id,omitempty"`
	ClientSecret  *string      `json:"client_secret,omitempty"`
	Configuration *string      `json:"configuration,omitempty"`
	DefaultRealm  *string      `json:"default_realm,omitempty"`
	Enabled       *bool        `json:"enabled,omitempty"`
	IsDefault     *bool        `json:"is_default,omitempty"`
	Tags          []string     `json:"tags,omitempty"`
	Owner         *string      `json:"owner,omitempty"`
	Amfa          *AmfaRequest `json:"amfa,omitempty"`
}

// TestConnectionRequest represents the request to test a tenant connection.
type TestConnectionRequest struct {
	ServerURL    string `json:"server_url"`
	AdminRealm   string `json:"admin_realm"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// HealthStatus represents the health status response.
type HealthStatus struct {
	TenantID        string  `json:"tenant_id"`
	Status          string  `json:"health_status"`
	Message         string  `json:"health_message"`
	LastHealthCheck string  `json:"last_health_check"`
	LastError       *string `json:"last_error"`
	LastErrorAt     *string `json:"last_error_at"`
}

// toDomain converts the request block to its domain form. A nil receiver
// returns nil so an absent block leaves a tenant's AMFA settings untouched.
func (a *AmfaRequest) toDomain() *domain.TenantAmfa {
	if a == nil {
		return nil
	}
	return &domain.TenantAmfa{
		Enabled:            a.Enabled,
		APIBaseURL:         a.APIBaseURL,
		EventsLookbackDays: a.EventsLookbackDays,
		APITimeoutSeconds:  a.APITimeoutSeconds,
	}
}
