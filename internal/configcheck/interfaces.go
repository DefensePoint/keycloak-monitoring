package configcheck

import (
	"context"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts/clearing"
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
	ResolveAlert(ctx context.Context, tenantID, alertID string) error
	ListOpenAlertsByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error)
}

// Silencer reports whether a configured silence rule covers an alert, and
// which one. alerts.SilenceManager implements it.
type Silencer interface {
	ShouldSilence(alert *domain.Alert) (bool, string)
}

// NotificationService defines the interface for sending notifications.
type NotificationService interface {
	NotifyAlert(ctx context.Context, alert *domain.Alert) error
	NotifyAlertResolution(ctx context.Context, alert *domain.Alert) error
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

// Evaluator is a Check that also reports which realms and resources it read,
// so the service can resolve the alerts whose problem it no longer finds.
// Every configuration check implements it; Execute returns only the alerts.
type Evaluator interface {
	Check
	// Evaluate runs the check. On error the Evaluation still names the
	// tenant, so the service knows whose run failed.
	Evaluate(ctx context.Context) (Evaluation, error)
}

// Evaluation is one run of an Evaluator: what it detected and read, and the
// scope that judges the alerts of realms it did not check.
type Evaluation struct {
	clearing.Evaluation
	scope *realmScope
}

// goneWith reports whether alert's realm was out of the run or gone, so its
// problem went with it; see realmScope.goneWith.
func (e *Evaluation) goneWith(alert *domain.Alert) bool {
	return e.scope != nil && e.scope.goneWith(alert)
}

// unreadableRealms lists the realms the run could not read; call it after
// goneWith, which can find more.
func (e *Evaluation) unreadableRealms() []string {
	if e.scope == nil {
		return nil
	}
	return e.scope.unreadableRealms()
}

// subCheckSwitches is a Check whose sub-checks can each be switched off in
// the configuration. A switched-off sub-check stops looking for its problem;
// that does not fix it, so the service leaves the alerts it raised as they
// are.
type subCheckSwitches interface {
	// looksFor reports whether the sub-check that raises alertType, the
	// alert_type in an alert's metadata, is on. A type no sub-check raises
	// any more counts as looked for, so its alert can resolve.
	looksFor(alertType string) bool
}

var (
	_ subCheckSwitches = (*RealmSecurityCheck)(nil)
	_ subCheckSwitches = (*ClientSecurityCheck)(nil)
)

var (
	_ Evaluator = (*RealmSecurityCheck)(nil)
	_ Evaluator = (*ClientSecurityCheck)(nil)
	_ Evaluator = (*IdentityProviderMetadataCheck)(nil)
)
