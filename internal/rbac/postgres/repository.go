// Package postgres provides the PostgreSQL implementation of the RBAC repositories.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// RoleRepository implements rbac.RoleRepository using PostgreSQL.
type RoleRepository struct {
	db *database.Client
}

// NewRoleRepository creates a new role repository.
func NewRoleRepository(db *database.Client) rbac.RoleRepository {
	return &RoleRepository{db: db}
}

func (r *RoleRepository) CreateRole(ctx context.Context, role *domain.Role) error {
	dbRole := &database.Role{
		Name:        role.Name,
		DisplayName: role.DisplayName,
		Description: role.Description,
		IsSystem:    role.IsSystem,
		IsActive:    role.IsActive,
	}
	if err := r.db.DB().WithContext(ctx).Create(dbRole).Error; err != nil {
		return err
	}
	role.ID = dbRole.ID
	role.CreatedAt = dbRole.CreatedAt
	role.UpdatedAt = dbRole.UpdatedAt
	return nil
}

func (r *RoleRepository) GetRoleByID(ctx context.Context, roleID uint) (*domain.Role, error) {
	var dbRole database.Role
	if err := r.db.DB().WithContext(ctx).First(&dbRole, roleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return convertRole(&dbRole), nil
}

func (r *RoleRepository) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	var dbRole database.Role
	if err := r.db.DB().WithContext(ctx).Where("name = ?", name).First(&dbRole).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return convertRole(&dbRole), nil
}

func (r *RoleRepository) ListRoles(ctx context.Context, includeInactive bool) ([]*domain.Role, error) {
	var dbRoles []database.Role
	query := r.db.DB().WithContext(ctx)
	if !includeInactive {
		query = query.Where("is_active = ?", true)
	}
	if err := query.Find(&dbRoles).Error; err != nil {
		return nil, err
	}
	return convertRoleList(dbRoles), nil
}

func (r *RoleRepository) UpdateRole(ctx context.Context, role *domain.Role) error {
	return r.db.DB().WithContext(ctx).Model(&database.Role{}).Where("id = ?", role.ID).Updates(map[string]interface{}{
		"display_name": role.DisplayName,
		"description":  role.Description,
		"is_active":    role.IsActive,
	}).Error
}

func (r *RoleRepository) DeleteRole(ctx context.Context, roleID uint) error {
	return r.db.DB().WithContext(ctx).Delete(&database.Role{}, roleID).Error
}

func (r *RoleRepository) GetRoleWithPermissions(ctx context.Context, roleID uint) (*domain.Role, error) {
	var dbRole database.Role
	if err := r.db.DB().WithContext(ctx).Preload("Permissions").First(&dbRole, roleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	role := convertRole(&dbRole)
	if len(dbRole.Permissions) > 0 {
		role.Permissions = make([]domain.Permission, len(dbRole.Permissions))
		for i, p := range dbRole.Permissions {
			role.Permissions[i] = domain.Permission{
				ID: p.ID, Name: p.Name, DisplayName: p.DisplayName,
				Description: p.Description, Resource: p.Resource, Action: p.Action,
				IsSystem: p.IsSystem, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
			}
		}
	}
	return role, nil
}

func (r *RoleRepository) AssignPermissionsToRole(ctx context.Context, roleID uint, permissionIDs []uint) error {
	var dbRole database.Role
	if err := r.db.DB().WithContext(ctx).First(&dbRole, roleID).Error; err != nil {
		return err
	}
	var permissions []database.Permission
	if err := r.db.DB().WithContext(ctx).Find(&permissions, permissionIDs).Error; err != nil {
		return err
	}
	return r.db.DB().WithContext(ctx).Model(&dbRole).Association("Permissions").Append(&permissions)
}

func (r *RoleRepository) RemovePermissionsFromRole(ctx context.Context, roleID uint, permissionIDs []uint) error {
	var dbRole database.Role
	if err := r.db.DB().WithContext(ctx).First(&dbRole, roleID).Error; err != nil {
		return err
	}
	var permissions []database.Permission
	if err := r.db.DB().WithContext(ctx).Find(&permissions, permissionIDs).Error; err != nil {
		return err
	}
	return r.db.DB().WithContext(ctx).Model(&dbRole).Association("Permissions").Delete(&permissions)
}

// PermissionRepository implements rbac.PermissionRepository.
type PermissionRepository struct {
	db *database.Client
}

func NewPermissionRepository(db *database.Client) rbac.PermissionRepository {
	return &PermissionRepository{db: db}
}

func (r *PermissionRepository) CreatePermission(ctx context.Context, permission *domain.Permission) error {
	dbPerm := &database.Permission{
		Name: permission.Name, DisplayName: permission.DisplayName, Description: permission.Description,
		Resource: permission.Resource, Action: permission.Action, IsSystem: permission.IsSystem,
	}
	if err := r.db.DB().WithContext(ctx).Create(dbPerm).Error; err != nil {
		return err
	}
	permission.ID = dbPerm.ID
	permission.CreatedAt = dbPerm.CreatedAt
	permission.UpdatedAt = dbPerm.UpdatedAt
	return nil
}

func (r *PermissionRepository) GetPermissionByID(ctx context.Context, permissionID uint) (*domain.Permission, error) {
	var dbPerm database.Permission
	if err := r.db.DB().WithContext(ctx).First(&dbPerm, permissionID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return convertPermission(&dbPerm), nil
}

func (r *PermissionRepository) GetPermissionByName(ctx context.Context, name string) (*domain.Permission, error) {
	var dbPerm database.Permission
	if err := r.db.DB().WithContext(ctx).Where("name = ?", name).First(&dbPerm).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return convertPermission(&dbPerm), nil
}

func (r *PermissionRepository) ListPermissions(ctx context.Context, resource string) ([]*domain.Permission, error) {
	var dbPerms []database.Permission
	query := r.db.DB().WithContext(ctx)
	if resource != "" {
		query = query.Where("resource = ?", resource)
	}
	if err := query.Find(&dbPerms).Error; err != nil {
		return nil, err
	}
	return convertPermissionList(dbPerms), nil
}

func (r *PermissionRepository) ListPermissionsByIDs(ctx context.Context, permissionIDs []uint) ([]*domain.Permission, error) {
	var dbPerms []database.Permission
	if err := r.db.DB().WithContext(ctx).Find(&dbPerms, permissionIDs).Error; err != nil {
		return nil, err
	}
	return convertPermissionList(dbPerms), nil
}

func (r *PermissionRepository) UpdatePermission(ctx context.Context, permission *domain.Permission) error {
	return r.db.DB().WithContext(ctx).Model(&database.Permission{}).Where("id = ?", permission.ID).Updates(map[string]interface{}{
		"display_name": permission.DisplayName,
		"description":  permission.Description,
	}).Error
}

func (r *PermissionRepository) DeletePermission(ctx context.Context, permissionID uint) error {
	return r.db.DB().WithContext(ctx).Delete(&database.Permission{}, permissionID).Error
}

// UserRoleRepository implements rbac.UserRoleRepository.
type UserRoleRepository struct {
	db *database.Client
}

func NewUserRoleRepository(db *database.Client) rbac.UserRoleRepository {
	return &UserRoleRepository{db: db}
}

func (r *UserRoleRepository) AssignRoleToUser(ctx context.Context, userRole *domain.UserRole) error {
	dbUR := &database.UserRole{
		UserID: userRole.UserID, RoleID: userRole.RoleID, TenantID: userRole.TenantID,
		AssignedBy: userRole.AssignedBy, AssignedAt: userRole.AssignedAt, ExpiresAt: userRole.ExpiresAt,
	}
	if err := r.db.DB().WithContext(ctx).Create(dbUR).Error; err != nil {
		return err
	}
	userRole.ID = dbUR.ID
	userRole.CreatedAt = dbUR.CreatedAt
	userRole.UpdatedAt = dbUR.UpdatedAt
	return nil
}

func (r *UserRoleRepository) RemoveRoleFromUser(ctx context.Context, userID, roleID uint, tenantID *string) error {
	var result *gorm.DB
	if tenantID != nil {
		result = r.db.DB().WithContext(ctx).Exec(
			"DELETE FROM user_roles WHERE user_id = ? AND role_id = ? AND tenant_id = ?",
			userID, roleID, *tenantID,
		)
	} else {
		result = r.db.DB().WithContext(ctx).Exec(
			"DELETE FROM user_roles WHERE user_id = ? AND role_id = ? AND tenant_id IS NULL",
			userID, roleID,
		)
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("role assignment not found")
	}
	return nil
}

func (r *UserRoleRepository) GetUserRoles(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
	var dbURs []database.UserRole
	if err := r.db.DB().WithContext(ctx).Preload("Role").Where("user_id = ?", userID).Find(&dbURs).Error; err != nil {
		return nil, err
	}
	return convertUserRoleList(dbURs), nil
}

func (r *UserRoleRepository) GetUserRolesForTenant(ctx context.Context, userID uint, tenantID string) ([]*domain.UserRole, error) {
	var dbURs []database.UserRole
	if err := r.db.DB().WithContext(ctx).Preload("Role").
		Where("user_id = ? AND (tenant_id IS NULL OR tenant_id = ?)", userID, tenantID).
		Find(&dbURs).Error; err != nil {
		return nil, err
	}
	return convertUserRoleList(dbURs), nil
}

func (r *UserRoleRepository) GetUserPermissions(ctx context.Context, userID uint, tenantID *string) ([]string, error) {
	query := `
		SELECT DISTINCT p.name FROM permissions p
		INNER JOIN role_permissions rp ON p.id = rp.permission_id
		INNER JOIN user_roles ur ON rp.role_id = ur.role_id
		WHERE ur.user_id = ? AND (ur.expires_at IS NULL OR ur.expires_at > NOW())
	`
	args := []interface{}{userID}
	if tenantID != nil {
		query += " AND (ur.tenant_id IS NULL OR ur.tenant_id = ?)"
		args = append(args, *tenantID)
	}
	var permissions []string
	if err := r.db.DB().WithContext(ctx).Raw(query, args...).Scan(&permissions).Error; err != nil {
		return nil, err
	}
	return permissions, nil
}

func (r *UserRoleRepository) HasPermission(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
	permissions, err := r.GetUserPermissions(ctx, userID, tenantID)
	if err != nil {
		return false, err
	}
	for _, perm := range permissions {
		if perm == permission {
			return true, nil
		}
	}
	return false, nil
}

func (r *UserRoleRepository) GetUsersWithRole(ctx context.Context, roleID uint) ([]*domain.User, error) {
	var dbUsers []database.User
	if err := r.db.DB().WithContext(ctx).
		Joins("INNER JOIN user_roles ON users.id = user_roles.user_id").
		Where("user_roles.role_id = ?", roleID).
		Find(&dbUsers).Error; err != nil {
		return nil, fmt.Errorf("failed to get users with role: %w", err)
	}

	users := make([]*domain.User, len(dbUsers))
	for i := range dbUsers {
		users[i] = convertUser(&dbUsers[i])
	}

	return users, nil
}

func (r *UserRoleRepository) CheckRoleExpiration(ctx context.Context) error {
	return r.db.DB().WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now()).
		Delete(&database.UserRole{}).Error
}

// TenantPolicyRepository implements rbac.TenantPolicyRepository.
type TenantPolicyRepository struct {
	db *database.Client
}

func NewTenantPolicyRepository(db *database.Client) rbac.TenantPolicyRepository {
	return &TenantPolicyRepository{db: db}
}

func (r *TenantPolicyRepository) CreatePolicy(ctx context.Context, policy *domain.TenantPolicy) error {
	dbPolicy := &database.TenantPolicy{
		UserID: policy.UserID, TenantID: policy.TenantID, AllowedRealms: marshalAllowedRealms(policy.AllowedRealms),
		GrantedBy: policy.GrantedBy, GrantedAt: policy.GrantedAt,
	}
	if err := r.db.DB().WithContext(ctx).Create(dbPolicy).Error; err != nil {
		return err
	}
	policy.ID = dbPolicy.ID
	policy.CreatedAt = dbPolicy.CreatedAt
	policy.UpdatedAt = dbPolicy.UpdatedAt
	return nil
}

func (r *TenantPolicyRepository) GetUserPolicies(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error) {
	var dbPolicies []database.TenantPolicy
	if err := r.db.DB().WithContext(ctx).Where("user_id = ?", userID).Find(&dbPolicies).Error; err != nil {
		return nil, err
	}
	return convertPolicyList(dbPolicies), nil
}

func (r *TenantPolicyRepository) GetPolicyByID(ctx context.Context, policyID uint) (*domain.TenantPolicy, error) {
	var dbPolicy database.TenantPolicy
	if err := r.db.DB().WithContext(ctx).Where("id = ?", policyID).First(&dbPolicy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return convertPolicy(&dbPolicy), nil
}

func (r *TenantPolicyRepository) GetUserPolicyForTenant(ctx context.Context, userID uint, tenantID string) (*domain.TenantPolicy, error) {
	var dbPolicy database.TenantPolicy
	if err := r.db.DB().WithContext(ctx).Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&dbPolicy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return convertPolicy(&dbPolicy), nil
}

func (r *TenantPolicyRepository) UpdatePolicy(ctx context.Context, policy *domain.TenantPolicy) error {
	return r.db.DB().WithContext(ctx).Model(&database.TenantPolicy{}).Where("id = ?", policy.ID).Updates(map[string]interface{}{
		"allowed_realms": marshalAllowedRealms(policy.AllowedRealms),
		"granted_by":     policy.GrantedBy,
		"granted_at":     policy.GrantedAt,
	}).Error
}

func (r *TenantPolicyRepository) DeletePolicy(ctx context.Context, policyID uint) error {
	return r.db.DB().WithContext(ctx).Delete(&database.TenantPolicy{}, policyID).Error
}

func (r *TenantPolicyRepository) DeleteUserPoliciesForTenant(ctx context.Context, userID uint, tenantID string) error {
	return r.db.DB().WithContext(ctx).Where("user_id = ? AND tenant_id = ?", userID, tenantID).Delete(&database.TenantPolicy{}).Error
}

func (r *TenantPolicyRepository) HasAccessToTenant(ctx context.Context, userID uint, tenantID string) (bool, error) {
	var count int64
	if err := r.db.DB().WithContext(ctx).Model(&database.TenantPolicy{}).Where("user_id = ? AND tenant_id = ?", userID, tenantID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *TenantPolicyRepository) HasAccessToRealm(ctx context.Context, userID uint, tenantID, realmName string) (bool, error) {
	var dbPolicy database.TenantPolicy
	if err := r.db.DB().WithContext(ctx).Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&dbPolicy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	if realmsUnrestricted(dbPolicy.AllowedRealms) {
		return true, nil
	}
	var allowedRealms []string
	if err := json.Unmarshal(dbPolicy.AllowedRealms, &allowedRealms); err != nil {
		return false, err
	}
	for _, realm := range allowedRealms {
		if realm == realmName {
			return true, nil
		}
	}
	return false, nil
}

// realmsUnrestricted reports an allowed_realms column holding no list, which
// grants every realm. Two shapes mean that: a SQL NULL column, and the legacy
// `null` literal json.Marshal produces for a nil slice. A stored `[]` is an
// empty list and stays fail-closed.
//
// tenant_policies is an opt-in restriction, not a grant.
func realmsUnrestricted(column []byte) bool {
	if len(column) == 0 {
		return true
	}
	var realms []string
	if err := json.Unmarshal(column, &realms); err != nil {
		return false
	}
	return realms == nil
}

// marshalAllowedRealms encodes a realm list for storage, keeping a nil slice as
// a SQL NULL column rather than the `null` literal.
func marshalAllowedRealms(realms []string) []byte {
	if realms == nil {
		return nil
	}
	encoded, _ := json.Marshal(realms)
	return encoded
}

// Conversion functions
func convertRole(db *database.Role) *domain.Role {
	return &domain.Role{
		ID: db.ID, Name: db.Name, DisplayName: db.DisplayName, Description: db.Description,
		IsSystem: db.IsSystem, IsActive: db.IsActive, CreatedAt: db.CreatedAt, UpdatedAt: db.UpdatedAt,
	}
}

func convertRoleList(dbRoles []database.Role) []*domain.Role {
	result := make([]*domain.Role, len(dbRoles))
	for i := range dbRoles {
		result[i] = convertRole(&dbRoles[i])
	}
	return result
}

func convertPermission(db *database.Permission) *domain.Permission {
	return &domain.Permission{
		ID: db.ID, Name: db.Name, DisplayName: db.DisplayName, Description: db.Description,
		Resource: db.Resource, Action: db.Action, IsSystem: db.IsSystem,
		CreatedAt: db.CreatedAt, UpdatedAt: db.UpdatedAt,
	}
}

func convertPermissionList(dbPerms []database.Permission) []*domain.Permission {
	result := make([]*domain.Permission, len(dbPerms))
	for i := range dbPerms {
		result[i] = convertPermission(&dbPerms[i])
	}
	return result
}

func convertUserRole(db *database.UserRole) *domain.UserRole {
	ur := &domain.UserRole{
		ID: db.ID, UserID: db.UserID, RoleID: db.RoleID, TenantID: db.TenantID,
		AssignedBy: db.AssignedBy, AssignedAt: db.AssignedAt, ExpiresAt: db.ExpiresAt,
		CreatedAt: db.CreatedAt, UpdatedAt: db.UpdatedAt,
	}
	if db.Role != nil {
		ur.Role = convertRole(db.Role)
	}
	return ur
}

func convertUserRoleList(dbURs []database.UserRole) []*domain.UserRole {
	result := make([]*domain.UserRole, len(dbURs))
	for i := range dbURs {
		result[i] = convertUserRole(&dbURs[i])
	}
	return result
}

func convertPolicy(db *database.TenantPolicy) *domain.TenantPolicy {
	var realms []string
	if len(db.AllowedRealms) > 0 {
		_ = json.Unmarshal(db.AllowedRealms, &realms)
	}
	return &domain.TenantPolicy{
		ID: db.ID, UserID: db.UserID, TenantID: db.TenantID, AllowedRealms: realms,
		RealmsUnrestricted: realmsUnrestricted(db.AllowedRealms),
		GrantedBy:          db.GrantedBy, GrantedAt: db.GrantedAt,
		CreatedAt: db.CreatedAt, UpdatedAt: db.UpdatedAt,
	}
}

func convertPolicyList(dbPolicies []database.TenantPolicy) []*domain.TenantPolicy {
	result := make([]*domain.TenantPolicy, len(dbPolicies))
	for i := range dbPolicies {
		result[i] = convertPolicy(&dbPolicies[i])
	}
	return result
}

func convertUser(db *database.User) *domain.User {
	if db == nil {
		return nil
	}

	// Convert LastAccessedAt from time.Time to *time.Time
	var lastAccessedAt *time.Time
	if !db.LastAccessedAt.IsZero() {
		lastAccessedAt = &db.LastAccessedAt
	}

	return &domain.User{
		ID:                 db.ID,
		Subject:            db.Subject,
		Email:              db.Email,
		EmailVerified:      db.EmailVerified,
		Name:               db.Name,
		GivenName:          db.GivenName,
		FamilyName:         db.FamilyName,
		PreferredUsername:  db.PreferredUsername,
		Locale:             db.Locale,
		Username:           db.Username,
		PasswordHash:       db.PasswordHash,
		AuthMethod:         domain.AuthMethod(db.AuthMethod),
		MustChangePassword: db.MustChangePassword,
		PasswordChangedAt:  db.PasswordChangedAt,
		IsActive:           db.IsActive,
		IsBlocked:          db.IsBlocked,
		BlockedReason:      db.BlockedReason,
		BlockedAt:          db.BlockedAt,
		LastLoginAt:        db.LastLoginAt,
		LastLoginIP:        db.LastLoginIP,
		LastAccessedAt:     lastAccessedAt,
		LoginCount:         db.LoginCount,
		CreatedAt:          db.CreatedAt,
		UpdatedAt:          db.UpdatedAt,
	}
}

// Interface assertions
var (
	_ rbac.RoleRepository         = (*RoleRepository)(nil)
	_ rbac.PermissionRepository   = (*PermissionRepository)(nil)
	_ rbac.UserRoleRepository     = (*UserRoleRepository)(nil)
	_ rbac.TenantPolicyRepository = (*TenantPolicyRepository)(nil)
)
