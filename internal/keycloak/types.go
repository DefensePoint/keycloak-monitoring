// Package keycloak provides the domain layer for Keycloak monitoring.
// This package follows the Clean Architecture pattern where:
// - Entities are defined here
// - Interfaces are defined here (Repository, Service, Ingester)
// - Implementations live in subpackages (postgres/, etc.)
package keycloak

import (
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// EventStats represents aggregated event statistics for a realm.
type EventStats struct {
	Realm            string    `json:"realm"`
	Start            time.Time `json:"start"`
	End              time.Time `json:"end"`
	LoginCount       int64     `json:"login_count"`
	LoginErrorCount  int64     `json:"login_error_count"`
	LogoutCount      int64     `json:"logout_count"`
	RegisterCount    int64     `json:"register_count"`
	CodeToTokenCount int64     `json:"code_to_token_count"`
	TotalEvents      int64     `json:"total_events"`
}

// Dashboard represents aggregated dashboard data for a tenant.
type Dashboard struct {
	Realm        string                             `json:"realm,omitempty"`
	Health       *domain.KeycloakHealth             `json:"health,omitempty"`
	Metrics      *domain.KeycloakMetrics            `json:"metrics,omitempty"`
	RealmInfo    *domain.KeycloakRealmInfo          `json:"realm_info,omitempty"`
	RealmMetrics map[string]*domain.KeycloakMetrics `json:"realm_metrics,omitempty"`
	Realms       []*RealmSummary                    `json:"realms,omitempty"`
	TotalRealms  int                                `json:"total_realms"`
	RecentEvents *RecentEventStats                  `json:"recent_events,omitempty"`
	VersionInfo  *VersionCheckResult                `json:"version_info,omitempty"`
}

// RealmSummary represents a summary of a realm for dashboard display.
type RealmSummary struct {
	RealmName       string                  `json:"realm_name"`
	Enabled         bool                    `json:"enabled"`
	IsHealthy       bool                    `json:"is_healthy"`
	EventsEnabled   bool                    `json:"events_enabled"`
	EventsListeners []string                `json:"events_listeners,omitempty"`
	Metrics         *domain.KeycloakMetrics `json:"metrics,omitempty"`
}

// RecentEventStats represents recent event statistics.
type RecentEventStats struct {
	Logins      int64  `json:"logins"`
	LoginErrors int64  `json:"login_errors"`
	TimePeriod  string `json:"time_period"`
}

// VersionCheckResult represents the result of a version check.
type VersionCheckResult struct {
	CurrentVersion        string    `json:"current_version"`
	LatestKeycloakVersion string    `json:"latest_keycloak_version"`
	KeycloakReleaseURL    string    `json:"keycloak_release_url"`
	IsKeycloakOutdated    bool      `json:"is_keycloak_outdated"`
	HasSecurityUpdates    bool      `json:"has_security_updates"`
	LastChecked           time.Time `json:"last_checked"`
}
