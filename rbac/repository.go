package rbac

import (
	"context"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// RoleRepository defines the interface for role persistence.
type RoleRepository interface {
	// CreateRole creates a new role.
	CreateRole(ctx context.Context, role *domain.Role) error

	// GetRoleByID retrieves a role by its ID.
	GetRoleByID(ctx context.Context, roleID uint) (*domain.Role, error)

	// GetRoleByName retrieves a role by its name.
	GetRoleByName(ctx context.Context, name string) (*domain.Role, error)

	// ListRoles retrieves all roles.
	ListRoles(ctx context.Context, includeInactive bool) ([]*domain.Role, error)

	// UpdateRole updates an existing role.
	UpdateRole(ctx context.Context, role *domain.Role) error

	// DeleteRole deletes a role.
	DeleteRole(ctx context.Context, roleID uint) error

	// GetRoleWithPermissions retrieves a role with its permissions.
	GetRoleWithPermissions(ctx context.Context, roleID uint) (*domain.Role, error)

	// AssignPermissionsToRole assigns permissions to a role.
	AssignPermissionsToRole(ctx context.Context, roleID uint, permissionIDs []uint) error

	// RemovePermissionsFromRole removes permissions from a role.
	RemovePermissionsFromRole(ctx context.Context, roleID uint, permissionIDs []uint) error
}

// PermissionRepository defines the interface for permission persistence.
type PermissionRepository interface {
	// CreatePermission creates a new permission.
	CreatePermission(ctx context.Context, permission *domain.Permission) error

	// GetPermissionByID retrieves a permission by its ID.
	GetPermissionByID(ctx context.Context, permissionID uint) (*domain.Permission, error)

	// GetPermissionByName retrieves a permission by its name.
	GetPermissionByName(ctx context.Context, name string) (*domain.Permission, error)

	// ListPermissions retrieves all permissions.
	ListPermissions(ctx context.Context, resource string) ([]*domain.Permission, error)

	// ListPermissionsByIDs retrieves permissions by their IDs.
	ListPermissionsByIDs(ctx context.Context, permissionIDs []uint) ([]*domain.Permission, error)

	// UpdatePermission updates an existing permission.
	UpdatePermission(ctx context.Context, permission *domain.Permission) error

	// DeletePermission deletes a permission.
	DeletePermission(ctx context.Context, permissionID uint) error
}

// UserRoleRepository defines the interface for user role assignment persistence.
type UserRoleRepository interface {
	// AssignRoleToUser assigns a role to a user.
	AssignRoleToUser(ctx context.Context, userRole *domain.UserRole) error

	// RemoveRoleFromUser removes a role from a user.
	RemoveRoleFromUser(ctx context.Context, userID, roleID uint, tenantID *string) error

	// GetUserRoles retrieves all role assignments for a user.
	GetUserRoles(ctx context.Context, userID uint) ([]*domain.UserRole, error)

	// GetUserRolesForTenant retrieves role assignments for a user in a tenant.
	GetUserRolesForTenant(ctx context.Context, userID uint, tenantID string) ([]*domain.UserRole, error)

	// GetUserPermissions retrieves all permissions for a user.
	GetUserPermissions(ctx context.Context, userID uint, tenantID *string) ([]string, error)

	// HasPermission checks if a user has a specific permission.
	HasPermission(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error)

	// GetUsersWithRole retrieves all users with a specific role.
	GetUsersWithRole(ctx context.Context, roleID uint) ([]*domain.User, error)

	// CheckRoleExpiration checks and removes expired role assignments.
	CheckRoleExpiration(ctx context.Context) error
}

// TenantPolicyRepository defines the interface for tenant policy persistence.
type TenantPolicyRepository interface {
	// CreatePolicy creates a new tenant policy.
	CreatePolicy(ctx context.Context, policy *domain.TenantPolicy) error

	// GetUserPolicies retrieves all policies for a user.
	GetUserPolicies(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error)

	// GetPolicyByID retrieves a single policy by its own ID, returning nil when
	// there is none.
	GetPolicyByID(ctx context.Context, policyID uint) (*domain.TenantPolicy, error)

	// GetUserPolicyForTenant retrieves a user's policy for a specific tenant.
	GetUserPolicyForTenant(ctx context.Context, userID uint, tenantID string) (*domain.TenantPolicy, error)

	// UpdatePolicy updates an existing policy.
	UpdatePolicy(ctx context.Context, policy *domain.TenantPolicy) error

	// DeletePolicy deletes a policy.
	DeletePolicy(ctx context.Context, policyID uint) error

	// DeleteUserPoliciesForTenant removes all policies for a user in a tenant.
	DeleteUserPoliciesForTenant(ctx context.Context, userID uint, tenantID string) error

	// HasAccessToTenant checks if a user has access to a tenant.
	HasAccessToTenant(ctx context.Context, userID uint, tenantID string) (bool, error)

	// HasAccessToRealm checks if a user has access to a realm.
	HasAccessToRealm(ctx context.Context, userID uint, tenantID, realmName string) (bool, error)
}
