package tenant

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// ============================================================================
// REPOSITORY INTERFACES (use domain types directly)
// ============================================================================

// EventRepository defines the interface for Keycloak event persistence.
// Used by MonitorPoolManager.
type EventRepository interface {
	SaveEvents(ctx context.Context, events []*domain.KeycloakEvent) error
	CountEventsByType(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time) (int64, error)
}

// MetricsRepository defines the interface for Keycloak metrics persistence.
// Used by MonitorPoolManager.
type MetricsRepository interface {
	SaveMetrics(ctx context.Context, metrics *domain.KeycloakMetrics) error
}

// HealthRepository defines the interface for Keycloak health persistence.
// Used by MonitorPoolManager.
type HealthRepository interface {
	SaveHealth(ctx context.Context, health *domain.KeycloakHealth) error
}

// RealmRepository defines the interface for Keycloak realm persistence.
// Used by MonitorPoolManager.
type RealmRepository interface {
	SaveRealm(ctx context.Context, realm *domain.KeycloakRealmInfo) error
	GetRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error)
}

// GeneralEventRepository defines the interface for general event persistence.
// Used by MonitorPoolManager.
type GeneralEventRepository interface {
	Save(ctx context.Context, event *domain.Event) error
	// MergeKeycloakEvent saves a Keycloak event, merging any pre-existing
	// standalone AMFA row that shares its amfa_event_id into one row.
	MergeKeycloakEvent(ctx context.Context, event *domain.Event) error
}

// ============================================================================
// PROCESSING INTERFACES
// ============================================================================

// EventAlertConverter defines the interface for converting events to alerts.
// Used by MonitorPoolManager.
type EventAlertConverter interface {
	ConvertEvent(ctx context.Context, event *domain.KeycloakEvent) error
}

// HealthAlertManager defines the interface for health status alert management.
// Used by MonitorPoolManager.
type HealthAlertManager interface {
	ProcessHealthStatus(ctx context.Context, tenantID, tenantName, status, errorMessage string, responseTime int) error
}

// ============================================================================
// KEYCLOAK MONITOR INTERFACES
// ============================================================================

// KeycloakMonitor defines the interface for a Keycloak monitor.
// This decouples tenant from the keycloak domain implementation.
type KeycloakMonitor interface {
	Start(ctx context.Context) error
	Stop() error
}

// KeycloakClient defines the interface for a Keycloak client.
// This decouples tenant from the keycloak domain implementation.
type KeycloakClient interface {
	IsHealthy(ctx context.Context) bool
	Close() error
}

// ============================================================================
// FACTORY CONFIGURATION
// ============================================================================

// MonitorConfig contains configuration for creating monitors.
type MonitorConfig struct {
	TenantID      string
	TenantName    string
	ServerURL     string
	AdminRealm    string
	ClientID      string
	ClientSecret  string
	Configuration string
}

// KeycloakMonitorFactory creates Keycloak monitors and clients.
// This factory is implemented by the keycloak package and injected via DI.
type KeycloakMonitorFactory interface {
	CreateClient(ctx context.Context, cfg *MonitorConfig, instanceCfg any) (KeycloakClient, error)
	CreateMonitor(
		tenantID string,
		tenantName string,
		client KeycloakClient,
		instanceCfg any,
		eventRepo EventRepository,
		generalEventRepo GeneralEventRepository,
		metricsRepo MetricsRepository,
		healthRepo HealthRepository,
		realmRepo RealmRepository,
		eventAlertConverter EventAlertConverter,
		healthAlertManager HealthAlertManager,
	) KeycloakMonitor
}
