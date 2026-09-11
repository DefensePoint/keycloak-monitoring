package tenant

import (
	"context"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Repository defines the interface for tenant persistence.
type Repository interface {
	Reader
	Writer
}

// Reader defines read operations for tenants.
type Reader interface {
	// GetByID returns a tenant by its database ID.
	GetByID(ctx context.Context, id uint) (*domain.KeycloakTenant, error)

	// GetByTenantID returns a tenant by its tenant_id.
	GetByTenantID(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error)

	// List returns all tenants.
	List(ctx context.Context) ([]*domain.KeycloakTenant, error)

	// ListEnabled returns all enabled tenants.
	ListEnabled(ctx context.Context) ([]*domain.KeycloakTenant, error)

	// GetDefault returns the default tenant.
	GetDefault(ctx context.Context) (*domain.KeycloakTenant, error)
}

// Writer defines write operations for tenants.
type Writer interface {
	// Create creates a new tenant.
	Create(ctx context.Context, tenant *domain.KeycloakTenant) error

	// Update updates an existing tenant.
	Update(ctx context.Context, tenant *domain.KeycloakTenant) error

	// Delete hard-deletes a tenant and every row it owns. This is
	// irreversible: the tenant row and its non-telemetry data are removed
	// outright, and its telemetry is queued for drain at startup.
	Delete(ctx context.Context, tenantID string) error

	// UpdateHealth updates the health status of a tenant.
	UpdateHealth(ctx context.Context, tenantID, status, message string) error

	// UpdateError records an error for a tenant.
	UpdateError(ctx context.Context, tenantID, errorMsg string) error

	// UnsetDefault unsets all tenants as default.
	UnsetDefault(ctx context.Context) error
}
