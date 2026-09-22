package keycloak

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// ============================================================================
// LOGGER INTERFACE
// ============================================================================

// Logger is the local interface for logging.
// This keeps the domain decoupled from the concrete logger implementation.
type Logger interface {
	Info(msg string, fields ...any)
	Error(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Debug(msg string, fields ...any)
}

// ============================================================================
// MONITOR-SPECIFIC INTERFACES (minimal interfaces for Monitor)
// ============================================================================
//
// These interfaces define only what the Monitor needs. Full repository interfaces
// (EventRepository, MetricsRepository, HealthRepository, RealmRepository) with
// additional read operations are defined in repository.go.
// ============================================================================

// MonitorEventRepository defines the minimal interface for event persistence used by Monitor.
type MonitorEventRepository interface {
	SaveEvents(ctx context.Context, events []*domain.KeycloakEvent) error
	CountEventsByType(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time) (int64, error)
}

// MonitorMetricsRepository defines the minimal interface for metrics persistence used by Monitor.
type MonitorMetricsRepository interface {
	SaveMetrics(ctx context.Context, metrics *domain.KeycloakMetrics) error
}

// MonitorHealthRepository defines the minimal interface for health persistence used by Monitor.
type MonitorHealthRepository interface {
	SaveHealth(ctx context.Context, health *domain.KeycloakHealth) error
}

// MonitorRealmRepository defines the minimal interface for realm persistence used by Monitor.
type MonitorRealmRepository interface {
	SaveRealm(ctx context.Context, realm *domain.KeycloakRealmInfo) error
	GetRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error)
}

// ============================================================================
// CROSS-DOMAIN INTERFACES (use domain types)
// ============================================================================

// EventSaver defines the interface for saving general events to the dashboard.
// Uses domain.Event directly.
type EventSaver interface {
	Save(ctx context.Context, event *domain.Event) error
	// MergeKeycloakEvent saves a Keycloak event, merging any pre-existing
	// standalone AMFA row that shares its amfa_event_id into one row.
	MergeKeycloakEvent(ctx context.Context, event *domain.Event) error
}

// EventAlertConverter defines the interface for converting Keycloak events to alerts.
// Uses domain.KeycloakEvent type
type EventAlertConverter interface {
	ConvertEvent(ctx context.Context, event *domain.KeycloakEvent) error
}

// HealthAlertManager defines the interface for health status alert management.
type HealthAlertManager interface {
	ProcessHealthStatus(ctx context.Context, tenantID, tenantName, status, errorMessage string, responseTime int) error
}

// TenantHealthUpdater lets the monitor keep the tenant record's health status
// current. Implemented by tenant.Service; kept as a local interface so this
// package does not import tenant.
type TenantHealthUpdater interface {
	UpdateHealth(ctx context.Context, tenantID, status, message string) error
	UpdateError(ctx context.Context, tenantID, errorMsg string) error
}

// UserFinder defines the interface for finding users in the platform.
type UserFinder interface {
	GetBySubject(ctx context.Context, subject string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	FindOrCreateBySubject(ctx context.Context, user *domain.User) (*domain.User, error)
}

// UserRoleManager defines the interface for managing user roles.
type UserRoleManager interface {
	GetUserRoles(ctx context.Context, userID uint) ([]*domain.UserRole, error)
	GetUserRolesForTenant(ctx context.Context, userID uint, tenantID string) ([]*domain.UserRole, error)
	AssignRoleToUser(ctx context.Context, userRole *domain.UserRole) error
	RemoveRoleFromUser(ctx context.Context, userID uint, roleID uint, tenantID *string) error
}

// RoleFinder defines the interface for finding roles.
type RoleFinder interface {
	GetRoleByName(ctx context.Context, name string) (*domain.Role, error)
}

// ============================================================================
// KEYCLOAK ADMIN API INTERFACE (Port for Keycloak Admin API access)
// ============================================================================

// AdminAPI defines the contract for accessing the Keycloak Admin API.
// This is the "port" in the Ports & Adapters pattern.
// The pkg/keycloakadmin.Client implements this interface.
type AdminAPI interface {
	// Lifecycle
	Close() error
	IsHealthy(ctx context.Context) bool
	GetRealms() []string

	// Realms
	GetAllRealms(ctx context.Context) ([]*keycloakadmin.RealmRepresentation, error)
	GetRealmInfo(ctx context.Context, realmName string) (*keycloakadmin.RealmRepresentation, error)

	// Authentication flows
	GetAuthenticationFlows(ctx context.Context, realmName string) ([]keycloakadmin.AuthenticationFlowRepresentation, error)
	GetFlowExecutions(ctx context.Context, realmName, flowAlias string) ([]keycloakadmin.AuthenticationExecutionInfoRepresentation, error)

	// Users
	GetRealmUsers(ctx context.Context, realmName string, first, max int) ([]keycloakadmin.UserRepresentation, error)
	GetUsers(ctx context.Context, realmName string, first, max int) ([]*keycloakadmin.UserRepresentation, error)
	GetUserByID(ctx context.Context, realmName, userID string) (*keycloakadmin.UserRepresentation, error)
	GetUserDetails(ctx context.Context, realmName, userID string) (*keycloakadmin.UserDetails, error)
	GetUserRoleMappings(ctx context.Context, realmName, userID string) (*keycloakadmin.RoleMappingsRepresentation, error)

	// Clients
	GetClients(ctx context.Context, realmName string) ([]*keycloakadmin.ClientRepresentation, error)

	// Metrics
	GetUsersCount(ctx context.Context, realmName string) (int, error)
	GetEnabledUsersCount(ctx context.Context, realmName string) (enabled, disabled int, err error)
	GetSessionsCount(ctx context.Context, realmName string) (active, offline int, err error)

	// Events
	GetEvents(ctx context.Context, realmName string, options *keycloakadmin.EventQueryOptions) ([]*keycloakadmin.EventRepresentation, error)
	GetRecentEvents(ctx context.Context, realmName string) ([]*keycloakadmin.EventRepresentation, error)

	// Health
	GetHealthMetrics(ctx context.Context) *keycloakadmin.HealthMetrics

	// Infinispan
	GetInfinispanMetrics(ctx context.Context) (*keycloakadmin.InfinispanMetrics, error)
}
