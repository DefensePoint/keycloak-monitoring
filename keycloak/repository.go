package keycloak

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// EventReader provides read operations for events.
type EventReader interface {
	GetEvents(ctx context.Context, tenantID string, limit, offset int) ([]*domain.KeycloakEvent, error)
	GetEventsByRealm(ctx context.Context, tenantID, realmName string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error)
	GetEventsByType(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error)
	GetEventsByTimeRange(ctx context.Context, tenantID string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error)
	CountEvents(ctx context.Context, tenantID string) (int64, error)
	CountEventsByType(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time) (int64, error)
}

// EventWriter provides write operations for events.
type EventWriter interface {
	SaveEvent(ctx context.Context, event *domain.KeycloakEvent) error
	SaveEvents(ctx context.Context, events []*domain.KeycloakEvent) error
	DeleteEventsBefore(ctx context.Context, tenantID string, before time.Time) (int64, error)
}

// EventRepository combines read and write operations for events.
type EventRepository interface {
	EventReader
	EventWriter
}

// MetricsReader provides read operations for metrics.
type MetricsReader interface {
	GetLatestMetrics(ctx context.Context, tenantID string) (*domain.KeycloakMetrics, error)
	GetLatestMetricsByRealm(ctx context.Context, tenantID, realmName string) (*domain.KeycloakMetrics, error)
	GetMetricsHistory(ctx context.Context, tenantID string, from, to time.Time) ([]*domain.KeycloakMetrics, error)
	GetMetricsHistoryByRealm(ctx context.Context, tenantID, realmName string, from, to time.Time) ([]*domain.KeycloakMetrics, error)
}

// MetricsWriter provides write operations for metrics.
type MetricsWriter interface {
	SaveMetrics(ctx context.Context, metrics *domain.KeycloakMetrics) error
}

// MetricsRepository combines read and write operations for metrics.
type MetricsRepository interface {
	MetricsReader
	MetricsWriter
}

// HealthReader provides read operations for health status.
type HealthReader interface {
	GetLatestHealth(ctx context.Context, tenantID string) (*domain.KeycloakHealth, error)
	GetHealthHistory(ctx context.Context, tenantID string, from, to time.Time) ([]*domain.KeycloakHealth, error)
}

// HealthWriter provides write operations for health status.
type HealthWriter interface {
	SaveHealth(ctx context.Context, health *domain.KeycloakHealth) error
}

// HealthRepository combines read and write operations for health.
type HealthRepository interface {
	HealthReader
	HealthWriter
}

// RealmReader provides read operations for realm info.
type RealmReader interface {
	GetRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error)
	GetRealmByName(ctx context.Context, tenantID, realmName string) (*domain.KeycloakRealmInfo, error)
}

// RealmWriter provides write operations for realm info.
type RealmWriter interface {
	SaveRealm(ctx context.Context, realm *domain.KeycloakRealmInfo) error
	SaveRealms(ctx context.Context, realms []*domain.KeycloakRealmInfo) error
	DeleteRealm(ctx context.Context, tenantID, realmName string) error
}

// RealmRepository combines read and write operations for realms.
type RealmRepository interface {
	RealmReader
	RealmWriter
}

// TenantReader provides read operations for tenants.
type TenantReader interface {
	GetTenant(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error)
	GetTenantByID(ctx context.Context, id uint) (*domain.KeycloakTenant, error)
	ListTenants(ctx context.Context) ([]*domain.KeycloakTenant, error)
	ListEnabledTenants(ctx context.Context) ([]*domain.KeycloakTenant, error)
}

// TenantWriter provides write operations for tenants.
type TenantWriter interface {
	CreateTenant(ctx context.Context, tenant *domain.KeycloakTenant) error
	UpdateTenant(ctx context.Context, tenant *domain.KeycloakTenant) error
	DeleteTenant(ctx context.Context, tenantID string) error
	UpdateTenantHealth(ctx context.Context, tenantID string, status, message string) error
}

// TenantRepository combines read and write operations for tenants.
type TenantRepository interface {
	TenantReader
	TenantWriter
}
