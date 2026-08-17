// Package tenant provides the tenant domain model and services.
// Tenants represent Keycloak instances being monitored.
package tenant

import "repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"

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
	TenantID      string   `json:"tenant_id"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	ServerURL     string   `json:"server_url"`
	AdminRealm    string   `json:"admin_realm"`
	ClientID      string   `json:"client_id,omitempty"`
	ClientSecret  string   `json:"client_secret,omitempty"`
	Configuration string   `json:"configuration,omitempty"`
	DefaultRealm  string   `json:"default_realm,omitempty"`
	Enabled       bool     `json:"enabled"`
	IsDefault     bool     `json:"is_default,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Owner         string   `json:"owner,omitempty"`
}

// UpdateRequest represents the request to update a tenant.
type UpdateRequest struct {
	Name          *string  `json:"name,omitempty"`
	Description   *string  `json:"description,omitempty"`
	ServerURL     *string  `json:"server_url,omitempty"`
	AdminRealm    *string  `json:"admin_realm,omitempty"`
	ClientID      *string  `json:"client_id,omitempty"`
	ClientSecret  *string  `json:"client_secret,omitempty"`
	Configuration *string  `json:"configuration,omitempty"`
	DefaultRealm  *string  `json:"default_realm,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
	IsDefault     *bool    `json:"is_default,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Owner         *string  `json:"owner,omitempty"`
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
