package rbac

import (
	"context"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// TenantVisibilityChecker is the slice of Service that FilterTenantsByUserRoles needs.
type TenantVisibilityChecker interface {
	IsAdmin(ctx context.Context, userID uint) (bool, error)
	GetUserRoles(ctx context.Context, userID uint) ([]*domain.UserRole, error)
}

// FilterTenantsByUserRoles returns the subset of tenants the user's role
// assignments make visible: admins and holders of a global (tenant-unscoped)
// role see every tenant, everyone else only the tenants their roles name.
// Fail-closed: a nil checker or a failed role lookup yields an empty list.
func FilterTenantsByUserRoles(ctx context.Context, checker TenantVisibilityChecker, userID uint, tenants []*domain.KeycloakTenant, log *logger.Logger) []*domain.KeycloakTenant {
	if checker == nil {
		log.Warn("RBAC service not configured, returning empty tenant list for security")
		return []*domain.KeycloakTenant{}
	}

	isAdmin, err := checker.IsAdmin(ctx, userID)
	if err == nil && isAdmin {
		return tenants
	}

	userRoles, err := checker.GetUserRoles(ctx, userID)
	if err != nil {
		log.Error("Failed to get user roles for tenant filtering", logger.Err(err))
		return []*domain.KeycloakTenant{}
	}

	allowedTenants := make(map[string]bool)
	hasGlobalRole := false

	for _, userRole := range userRoles {
		if userRole.TenantID == nil {
			hasGlobalRole = true
			break
		}
		allowedTenants[*userRole.TenantID] = true
	}

	if hasGlobalRole {
		return tenants
	}

	var filtered []*domain.KeycloakTenant
	for _, t := range tenants {
		if allowedTenants[t.TenantID] {
			filtered = append(filtered, t)
		}
	}

	return filtered
}
