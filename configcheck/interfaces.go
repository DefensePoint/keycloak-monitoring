package configcheck

import (
	"context"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// ============================================================================
// EXTERNAL DEPENDENCY INTERFACES (Local interfaces for dependency injection)
// ============================================================================

// AlertStore defines the minimal interface for alert persistence
// used by the configcheck Service.
type AlertStore interface {
	GetAlertByID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
	SaveAlert(ctx context.Context, alert *domain.Alert) error
}

// NotificationService defines the interface for sending notifications.
type NotificationService interface {
	NotifyAlert(ctx context.Context, alert *domain.Alert) error
}

// ============================================================================
// KEYCLOAK CLIENT INTERFACE
// Uses types from the keycloakadmin package (keycloakadmin.ClientRepresentation, etc.)
// ============================================================================

// KeycloakClient defines the minimal interface for Keycloak operations
// needed by configuration checks.
type KeycloakClient interface {
	// GetAllRealms retrieves all realms from Keycloak.
	GetAllRealms(ctx context.Context) ([]*keycloakadmin.RealmRepresentation, error)

	// GetRealmInfo retrieves detailed information about a specific realm.
	GetRealmInfo(ctx context.Context, realmName string) (*keycloakadmin.RealmRepresentation, error)

	// GetClients retrieves all clients for a specific realm.
	GetClients(ctx context.Context, realmName string) ([]*keycloakadmin.ClientRepresentation, error)

	// GetIdentityProviders retrieves all identity providers for a specific realm.
	GetIdentityProviders(ctx context.Context, realmName string) ([]*keycloakadmin.IdentityProviderRepresentation, error)
}

// ============================================================================
// CHECK INTERFACE
// ============================================================================

// Check defines the interface for configuration checks.
type Check interface {
	// GetCheckType returns the type identifier for this check.
	GetCheckType() string

	// GetDescription returns a human-readable description of this check.
	GetDescription() string

	// Execute performs the check and returns any detected alerts.
	Execute(ctx context.Context) ([]*domain.Alert, error)
}
