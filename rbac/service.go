package rbac

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// ErrAdminRoleTenantScoped is returned when an admin role assignment carries a
// tenant scope. The administrator role is global-only: a tenant-scoped admin
// grant would otherwise be treated as a platform administrator.
var ErrAdminRoleTenantScoped = errors.New("admin role cannot be assigned with a tenant scope; the administrator role is global-only")

// Service defines the interface for RBAC operations.
type Service interface {
	// Permission checking
	HasPermission(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error)
	GetUserPermissions(ctx context.Context, userID uint, tenantID *string) ([]string, error)
	HasAccessToTenant(ctx context.Context, userID uint, tenantID string) (bool, error)
	HasAccessToRealm(ctx context.Context, userID uint, tenantID, realmName string) (bool, error)

	// Role management
	GetUserRoles(ctx context.Context, userID uint) ([]*domain.UserRole, error)
	GetUserRolesForTenant(ctx context.Context, userID uint, tenantID string) ([]*domain.UserRole, error)
	AssignRoleToUser(ctx context.Context, userID, roleID uint, tenantID *string, assignedBy string, expiresAt *time.Time) error
	RemoveRoleFromUser(ctx context.Context, userID, roleID uint, tenantID *string) error

	// Tenant policy management
	CreateTenantPolicy(ctx context.Context, userID uint, tenantID string, allowedRealms []string, grantedBy string) error
	UpdateTenantPolicy(ctx context.Context, policyID uint, allowedRealms []string, grantedBy string) error
	DeleteTenantPolicy(ctx context.Context, policyID uint) error
	GetUserPolicies(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error)

	// Role and permission CRUD
	CreateRole(ctx context.Context, name, displayName, description string, isSystem bool) (*domain.Role, error)
	GetRoleByName(ctx context.Context, name string) (*domain.Role, error)
	GetRoleWithPermissions(ctx context.Context, roleID uint) (*domain.Role, error)
	ListRoles(ctx context.Context, includeInactive bool) ([]*domain.Role, error)
	UpdateRole(ctx context.Context, role *domain.Role) error
	DeleteRole(ctx context.Context, roleID uint) error
	AssignPermissionsToRole(ctx context.Context, roleID uint, permissionNames []string) error
	CreatePermission(ctx context.Context, name, displayName, description, resource, action string, isSystem bool) (*domain.Permission, error)
	ListPermissions(ctx context.Context, resource string) ([]*domain.Permission, error)

	// Utility
	IsAdmin(ctx context.Context, userID uint) (bool, error)
	IsOperator(ctx context.Context, userID uint) (bool, error)
	CheckRoleExpiration(ctx context.Context) error
	GetUserWithRolesAndPermissions(ctx context.Context, userID uint, tenantID *string) (*domain.UserWithRoles, error)
}

// service implements the Service interface.
type service struct {
	roleRepo       RoleRepository
	permissionRepo PermissionRepository
	userRoleRepo   UserRoleRepository
	policyRepo     TenantPolicyRepository
	logger         *logger.Logger
}

// NewService creates a new RBAC service.
func NewService(
	roleRepo RoleRepository,
	permissionRepo PermissionRepository,
	userRoleRepo UserRoleRepository,
	policyRepo TenantPolicyRepository,
	log *logger.Logger,
) Service {
	return &service{
		roleRepo:       roleRepo,
		permissionRepo: permissionRepo,
		userRoleRepo:   userRoleRepo,
		policyRepo:     policyRepo,
		logger:         log.WithComponent("rbac_service"),
	}
}

// HasPermission checks if a user has a specific permission.
func (s *service) HasPermission(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
	return s.userRoleRepo.HasPermission(ctx, userID, permission, tenantID)
}

// GetUserPermissions retrieves all permissions for a user.
func (s *service) GetUserPermissions(ctx context.Context, userID uint, tenantID *string) ([]string, error) {
	return s.userRoleRepo.GetUserPermissions(ctx, userID, tenantID)
}

// HasAccessToTenant checks if a user has access to a tenant.
func (s *service) HasAccessToTenant(ctx context.Context, userID uint, tenantID string) (bool, error) {
	roles, err := s.userRoleRepo.GetUserRoles(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user roles: %w", err)
	}

	for _, userRole := range roles {
		if userRole.Role != nil && userRole.Role.Name == RoleAdmin && userRole.TenantID == nil {
			return true, nil
		}
	}

	return s.policyRepo.HasAccessToTenant(ctx, userID, tenantID)
}

// HasAccessToRealm checks if a user has access to a realm.
func (s *service) HasAccessToRealm(ctx context.Context, userID uint, tenantID, realmName string) (bool, error) {
	roles, err := s.userRoleRepo.GetUserRoles(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user roles: %w", err)
	}

	for _, userRole := range roles {
		if userRole.Role != nil && userRole.Role.Name == RoleAdmin && userRole.TenantID == nil {
			return true, nil
		}
	}

	hasAccess, err := s.policyRepo.HasAccessToTenant(ctx, userID, tenantID)
	if err != nil || !hasAccess {
		return false, err
	}

	return s.policyRepo.HasAccessToRealm(ctx, userID, tenantID, realmName)
}

// GetUserRoles retrieves all roles for a user.
func (s *service) GetUserRoles(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
	return s.userRoleRepo.GetUserRoles(ctx, userID)
}

// GetUserRolesForTenant retrieves roles for a user in a tenant.
func (s *service) GetUserRolesForTenant(ctx context.Context, userID uint, tenantID string) ([]*domain.UserRole, error) {
	return s.userRoleRepo.GetUserRolesForTenant(ctx, userID, tenantID)
}

// AssignRoleToUser assigns a role to a user.
func (s *service) AssignRoleToUser(ctx context.Context, userID, roleID uint, tenantID *string, assignedBy string, expiresAt *time.Time) error {
	role, err := s.roleRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return fmt.Errorf("failed to get role: %w", err)
	}
	if role == nil {
		return fmt.Errorf("role not found")
	}

	if role.Name == RoleAdmin && tenantID != nil {
		return ErrAdminRoleTenantScoped
	}

	userRole := &domain.UserRole{
		UserID:     userID,
		RoleID:     roleID,
		TenantID:   tenantID,
		AssignedBy: assignedBy,
		AssignedAt: time.Now(),
		ExpiresAt:  expiresAt,
	}

	if err := s.userRoleRepo.AssignRoleToUser(ctx, userRole); err != nil {
		return fmt.Errorf("failed to assign role to user: %w", err)
	}

	s.logger.Info("Role assigned to user",
		logger.Uint("user_id", userID),
		logger.Uint("role_id", roleID),
		logger.Str("role_name", role.Name),
		logger.Str("assigned_by", assignedBy))

	return nil
}

// RemoveRoleFromUser removes a role from a user.
func (s *service) RemoveRoleFromUser(ctx context.Context, userID, roleID uint, tenantID *string) error {
	if err := s.userRoleRepo.RemoveRoleFromUser(ctx, userID, roleID, tenantID); err != nil {
		return fmt.Errorf("failed to remove role from user: %w", err)
	}

	s.logger.Info("Role removed from user",
		logger.Uint("user_id", userID),
		logger.Uint("role_id", roleID))

	return nil
}

// ErrAllowedRealmBlank is returned when a tenant policy names a blank realm.
// It is a caller mistake, not a server fault, so handlers answer 400.
var ErrAllowedRealmBlank = errors.New("tenant policy names a blank realm")

// validateAllowedRealms rejects a realm list carrying a blank entry.
//
// A blank realm names no realm, so it can only ever be a mistake, and stored
// it is a fail-open one: readers that drop it end up with no realm filter and
// widen the caller to the whole tenant. The read path in
// allowedRealmsWithAdmin drops blanks defensively; this stops them being
// written at all, so a policy means what it says on disk.
//
// A nil list is not a list: it means unrestricted, and stays valid. An empty
// list is a real, fail-closed restriction and also stays valid.
func validateAllowedRealms(allowedRealms []string) error {
	for i, realm := range allowedRealms {
		if strings.TrimSpace(realm) == "" {
			return fmt.Errorf("%w: allowed_realms[%d] is blank, name a realm or pass no list to leave the tenant unrestricted", ErrAllowedRealmBlank, i)
		}
	}
	return nil
}

// CreateTenantPolicy creates a tenant policy.
func (s *service) CreateTenantPolicy(ctx context.Context, userID uint, tenantID string, allowedRealms []string, grantedBy string) error {
	if err := validateAllowedRealms(allowedRealms); err != nil {
		return err
	}

	policy := &domain.TenantPolicy{
		UserID:        userID,
		TenantID:      tenantID,
		AllowedRealms: allowedRealms,
		GrantedBy:     grantedBy,
		GrantedAt:     time.Now(),
	}

	if err := s.policyRepo.CreatePolicy(ctx, policy); err != nil {
		return fmt.Errorf("failed to create tenant policy: %w", err)
	}

	s.logger.Info("Tenant policy created",
		logger.Uint("user_id", userID),
		logger.Str("tenant_id", tenantID),
		logger.Int("allowed_realms", len(allowedRealms)),
		logger.Str("granted_by", grantedBy))

	return nil
}

// UpdateTenantPolicy updates a tenant policy.
func (s *service) UpdateTenantPolicy(ctx context.Context, policyID uint, allowedRealms []string, grantedBy string) error {
	if err := validateAllowedRealms(allowedRealms); err != nil {
		return err
	}

	policy, err := s.policyRepo.GetPolicyByID(ctx, policyID)
	if err != nil {
		return fmt.Errorf("failed to get policy: %w", err)
	}
	if policy == nil {
		return fmt.Errorf("policy not found")
	}

	policy.AllowedRealms = allowedRealms
	policy.GrantedBy = grantedBy
	policy.GrantedAt = time.Now()

	if err := s.policyRepo.UpdatePolicy(ctx, policy); err != nil {
		return fmt.Errorf("failed to update tenant policy: %w", err)
	}

	s.logger.Info("Tenant policy updated",
		logger.Uint("policy_id", policyID),
		logger.Int("allowed_realms", len(allowedRealms)))

	return nil
}

// DeleteTenantPolicy deletes a tenant policy.
func (s *service) DeleteTenantPolicy(ctx context.Context, policyID uint) error {
	if err := s.policyRepo.DeletePolicy(ctx, policyID); err != nil {
		return fmt.Errorf("failed to delete tenant policy: %w", err)
	}

	s.logger.Info("Tenant policy deleted", logger.Uint("policy_id", policyID))
	return nil
}

// GetUserPolicies retrieves all policies for a user.
func (s *service) GetUserPolicies(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error) {
	return s.policyRepo.GetUserPolicies(ctx, userID)
}

// CreateRole creates a new role.
func (s *service) CreateRole(ctx context.Context, name, displayName, description string, isSystem bool) (*domain.Role, error) {
	role := &domain.Role{
		Name:        name,
		DisplayName: displayName,
		Description: description,
		IsSystem:    isSystem,
		IsActive:    true,
	}
	if err := s.roleRepo.CreateRole(ctx, role); err != nil {
		return nil, fmt.Errorf("failed to create role: %w", err)
	}

	s.logger.Info("Role created", logger.Str("role_name", name))
	return role, nil
}

// GetRoleByName retrieves a role by name.
func (s *service) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	return s.roleRepo.GetRoleByName(ctx, name)
}

// GetRoleWithPermissions retrieves a role with permissions.
func (s *service) GetRoleWithPermissions(ctx context.Context, roleID uint) (*domain.Role, error) {
	return s.roleRepo.GetRoleWithPermissions(ctx, roleID)
}

// ListRoles lists all roles.
func (s *service) ListRoles(ctx context.Context, includeInactive bool) ([]*domain.Role, error) {
	return s.roleRepo.ListRoles(ctx, includeInactive)
}

// UpdateRole updates a role.
func (s *service) UpdateRole(ctx context.Context, role *domain.Role) error {
	if err := s.roleRepo.UpdateRole(ctx, role); err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	s.logger.Info("Role updated",
		logger.Uint("role_id", role.ID),
		logger.Str("role_name", role.Name))

	return nil
}

// DeleteRole deletes a role.
func (s *service) DeleteRole(ctx context.Context, roleID uint) error {
	if err := s.roleRepo.DeleteRole(ctx, roleID); err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	s.logger.Info("Role deleted", logger.Uint("role_id", roleID))
	return nil
}

// AssignPermissionsToRole assigns permissions to a role.
func (s *service) AssignPermissionsToRole(ctx context.Context, roleID uint, permissionNames []string) error {
	var permissionIDs []uint
	for _, name := range permissionNames {
		perm, err := s.permissionRepo.GetPermissionByName(ctx, name)
		if err != nil {
			return fmt.Errorf("failed to get permission %s: %w", name, err)
		}
		if perm == nil {
			return fmt.Errorf("permission not found: %s", name)
		}
		permissionIDs = append(permissionIDs, perm.ID)
	}

	if err := s.roleRepo.AssignPermissionsToRole(ctx, roleID, permissionIDs); err != nil {
		return fmt.Errorf("failed to assign permissions to role: %w", err)
	}

	s.logger.Info("Permissions assigned to role",
		logger.Uint("role_id", roleID),
		logger.Int("permission_count", len(permissionNames)))

	return nil
}

// CreatePermission creates a new permission.
func (s *service) CreatePermission(ctx context.Context, name, displayName, description, resource, action string, isSystem bool) (*domain.Permission, error) {
	permission := &domain.Permission{
		Name:        name,
		DisplayName: displayName,
		Description: description,
		Resource:    resource,
		Action:      action,
		IsSystem:    isSystem,
	}
	if err := s.permissionRepo.CreatePermission(ctx, permission); err != nil {
		return nil, fmt.Errorf("failed to create permission: %w", err)
	}

	s.logger.Info("Permission created", logger.Str("permission_name", name))
	return permission, nil
}

// ListPermissions lists permissions.
func (s *service) ListPermissions(ctx context.Context, resource string) ([]*domain.Permission, error) {
	return s.permissionRepo.ListPermissions(ctx, resource)
}

// IsAdmin reports whether a user is a platform administrator. Only a global
// (non-tenant-scoped) admin grant that has not expired confers platform admin;
// a tenant-scoped admin assignment must never be treated as global.
func (s *service) IsAdmin(ctx context.Context, userID uint) (bool, error) {
	roles, err := s.userRoleRepo.GetUserRoles(ctx, userID)
	if err != nil {
		return false, err
	}
	now := time.Now()
	for _, userRole := range roles {
		if userRole.Role == nil || userRole.Role.Name != RoleAdmin {
			continue
		}
		if userRole.TenantID != nil {
			continue
		}
		if userRole.ExpiresAt != nil && !userRole.ExpiresAt.After(now) {
			continue
		}
		return true, nil
	}
	return false, nil
}

// IsOperator checks if a user is operator.
func (s *service) IsOperator(ctx context.Context, userID uint) (bool, error) {
	roles, err := s.userRoleRepo.GetUserRoles(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, userRole := range roles {
		if userRole.Role != nil && userRole.Role.Name == RoleOperator {
			return true, nil
		}
	}
	return false, nil
}

// CheckRoleExpiration checks and removes expired roles.
func (s *service) CheckRoleExpiration(ctx context.Context) error {
	if err := s.userRoleRepo.CheckRoleExpiration(ctx); err != nil {
		return fmt.Errorf("failed to check role expiration: %w", err)
	}
	return nil
}

// GetUserWithRolesAndPermissions retrieves user with roles and permissions.
func (s *service) GetUserWithRolesAndPermissions(ctx context.Context, userID uint, tenantID *string) (*domain.UserWithRoles, error) {
	// Get roles
	var roles []*domain.UserRole
	var err error
	if tenantID != nil {
		roles, err = s.userRoleRepo.GetUserRolesForTenant(ctx, userID, *tenantID)
	} else {
		roles, err = s.userRoleRepo.GetUserRoles(ctx, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles: %w", err)
	}

	// Get permissions
	permissions, err := s.userRoleRepo.GetUserPermissions(ctx, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user permissions: %w", err)
	}

	// Get policies
	policies, err := s.policyRepo.GetUserPolicies(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user policies: %w", err)
	}

	// Convert to non-pointer slices
	userRoles := make([]domain.UserRole, len(roles))
	for i, role := range roles {
		userRoles[i] = *role
	}

	tenantPolicies := make([]domain.TenantPolicy, len(policies))
	for i, policy := range policies {
		tenantPolicies[i] = *policy
	}

	return &domain.UserWithRoles{
		Roles:       userRoles,
		Permissions: permissions,
		Policies:    tenantPolicies,
	}, nil
}
