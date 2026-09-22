package rbac

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/rs/zerolog"
)

// newTestLogger creates a logger that discards output for tests
func newTestLogger() *logger.Logger {
	return logger.New(zerolog.New(io.Discard))
}

// Mock repositories
type mockRoleRepository struct {
	createRoleFn             func(ctx context.Context, role *domain.Role) error
	getRoleByIDFn            func(ctx context.Context, roleID uint) (*domain.Role, error)
	getRoleByNameFn          func(ctx context.Context, name string) (*domain.Role, error)
	listRolesFn              func(ctx context.Context, includeInactive bool) ([]*domain.Role, error)
	updateRoleFn             func(ctx context.Context, role *domain.Role) error
	deleteRoleFn             func(ctx context.Context, roleID uint) error
	getRoleWithPermissionsFn func(ctx context.Context, roleID uint) (*domain.Role, error)
	assignPermissionsFn      func(ctx context.Context, roleID uint, permissionIDs []uint) error
}

func (m *mockRoleRepository) CreateRole(ctx context.Context, role *domain.Role) error {
	if m.createRoleFn != nil {
		return m.createRoleFn(ctx, role)
	}
	return nil
}

func (m *mockRoleRepository) GetRoleByID(ctx context.Context, roleID uint) (*domain.Role, error) {
	if m.getRoleByIDFn != nil {
		return m.getRoleByIDFn(ctx, roleID)
	}
	return nil, nil
}

func (m *mockRoleRepository) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	if m.getRoleByNameFn != nil {
		return m.getRoleByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockRoleRepository) ListRoles(ctx context.Context, includeInactive bool) ([]*domain.Role, error) {
	if m.listRolesFn != nil {
		return m.listRolesFn(ctx, includeInactive)
	}
	return nil, nil
}

func (m *mockRoleRepository) UpdateRole(ctx context.Context, role *domain.Role) error {
	if m.updateRoleFn != nil {
		return m.updateRoleFn(ctx, role)
	}
	return nil
}

func (m *mockRoleRepository) DeleteRole(ctx context.Context, roleID uint) error {
	if m.deleteRoleFn != nil {
		return m.deleteRoleFn(ctx, roleID)
	}
	return nil
}

func (m *mockRoleRepository) GetRoleWithPermissions(ctx context.Context, roleID uint) (*domain.Role, error) {
	if m.getRoleWithPermissionsFn != nil {
		return m.getRoleWithPermissionsFn(ctx, roleID)
	}
	return nil, nil
}

func (m *mockRoleRepository) AssignPermissionsToRole(ctx context.Context, roleID uint, permissionIDs []uint) error {
	if m.assignPermissionsFn != nil {
		return m.assignPermissionsFn(ctx, roleID, permissionIDs)
	}
	return nil
}

func (m *mockRoleRepository) RemovePermissionsFromRole(ctx context.Context, roleID uint, permissionIDs []uint) error {
	return nil
}

type mockPermissionRepository struct {
	createPermissionFn  func(ctx context.Context, permission *domain.Permission) error
	getPermissionByIDFn func(ctx context.Context, permissionID uint) (*domain.Permission, error)
	getByNameFn         func(ctx context.Context, name string) (*domain.Permission, error)
	listPermissionsFn   func(ctx context.Context, resource string) ([]*domain.Permission, error)
}

func (m *mockPermissionRepository) CreatePermission(ctx context.Context, permission *domain.Permission) error {
	if m.createPermissionFn != nil {
		return m.createPermissionFn(ctx, permission)
	}
	return nil
}

func (m *mockPermissionRepository) GetPermissionByID(ctx context.Context, permissionID uint) (*domain.Permission, error) {
	if m.getPermissionByIDFn != nil {
		return m.getPermissionByIDFn(ctx, permissionID)
	}
	return nil, nil
}

func (m *mockPermissionRepository) GetPermissionByName(ctx context.Context, name string) (*domain.Permission, error) {
	if m.getByNameFn != nil {
		return m.getByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockPermissionRepository) ListPermissions(ctx context.Context, resource string) ([]*domain.Permission, error) {
	if m.listPermissionsFn != nil {
		return m.listPermissionsFn(ctx, resource)
	}
	return nil, nil
}

func (m *mockPermissionRepository) ListPermissionsByIDs(ctx context.Context, permissionIDs []uint) ([]*domain.Permission, error) {
	return nil, nil
}

func (m *mockPermissionRepository) UpdatePermission(ctx context.Context, permission *domain.Permission) error {
	return nil
}

func (m *mockPermissionRepository) DeletePermission(ctx context.Context, permissionID uint) error {
	return nil
}

type mockUserRoleRepository struct {
	assignRoleFn            func(ctx context.Context, userRole *domain.UserRole) error
	removeRoleFn            func(ctx context.Context, userID, roleID uint, tenantID *string) error
	getUserRolesFn          func(ctx context.Context, userID uint) ([]*domain.UserRole, error)
	getUserRolesForTenantFn func(ctx context.Context, userID uint, tenantID string) ([]*domain.UserRole, error)
	getUserPermissionsFn    func(ctx context.Context, userID uint, tenantID *string) ([]string, error)
	hasPermissionFn         func(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error)
	checkExpirationFn       func(ctx context.Context) error
}

func (m *mockUserRoleRepository) AssignRoleToUser(ctx context.Context, userRole *domain.UserRole) error {
	if m.assignRoleFn != nil {
		return m.assignRoleFn(ctx, userRole)
	}
	return nil
}

func (m *mockUserRoleRepository) RemoveRoleFromUser(ctx context.Context, userID, roleID uint, tenantID *string) error {
	if m.removeRoleFn != nil {
		return m.removeRoleFn(ctx, userID, roleID, tenantID)
	}
	return nil
}

func (m *mockUserRoleRepository) GetUserRoles(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
	if m.getUserRolesFn != nil {
		return m.getUserRolesFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockUserRoleRepository) GetUserRolesForTenant(ctx context.Context, userID uint, tenantID string) ([]*domain.UserRole, error) {
	if m.getUserRolesForTenantFn != nil {
		return m.getUserRolesForTenantFn(ctx, userID, tenantID)
	}
	return nil, nil
}

func (m *mockUserRoleRepository) GetUserPermissions(ctx context.Context, userID uint, tenantID *string) ([]string, error) {
	if m.getUserPermissionsFn != nil {
		return m.getUserPermissionsFn(ctx, userID, tenantID)
	}
	return nil, nil
}

func (m *mockUserRoleRepository) HasPermission(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
	if m.hasPermissionFn != nil {
		return m.hasPermissionFn(ctx, userID, permission, tenantID)
	}
	return false, nil
}

func (m *mockUserRoleRepository) GetUsersWithRole(ctx context.Context, roleID uint) ([]*domain.User, error) {
	return nil, nil
}

func (m *mockUserRoleRepository) CheckRoleExpiration(ctx context.Context) error {
	if m.checkExpirationFn != nil {
		return m.checkExpirationFn(ctx)
	}
	return nil
}

type mockTenantPolicyRepository struct {
	createPolicyFn      func(ctx context.Context, policy *domain.TenantPolicy) error
	getUserPoliciesFn   func(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error)
	getPolicyByIDFn     func(ctx context.Context, policyID uint) (*domain.TenantPolicy, error)
	updatePolicyFn      func(ctx context.Context, policy *domain.TenantPolicy) error
	deletePolicyFn      func(ctx context.Context, policyID uint) error
	hasAccessToTenantFn func(ctx context.Context, userID uint, tenantID string) (bool, error)
	hasAccessToRealmFn  func(ctx context.Context, userID uint, tenantID, realmName string) (bool, error)
}

func (m *mockTenantPolicyRepository) CreatePolicy(ctx context.Context, policy *domain.TenantPolicy) error {
	if m.createPolicyFn != nil {
		return m.createPolicyFn(ctx, policy)
	}
	return nil
}

func (m *mockTenantPolicyRepository) GetUserPolicies(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error) {
	if m.getUserPoliciesFn != nil {
		return m.getUserPoliciesFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockTenantPolicyRepository) GetPolicyByID(ctx context.Context, policyID uint) (*domain.TenantPolicy, error) {
	if m.getPolicyByIDFn != nil {
		return m.getPolicyByIDFn(ctx, policyID)
	}
	return nil, nil
}

func (m *mockTenantPolicyRepository) GetUserPolicyForTenant(ctx context.Context, userID uint, tenantID string) (*domain.TenantPolicy, error) {
	return nil, nil
}

func (m *mockTenantPolicyRepository) UpdatePolicy(ctx context.Context, policy *domain.TenantPolicy) error {
	if m.updatePolicyFn != nil {
		return m.updatePolicyFn(ctx, policy)
	}
	return nil
}

func (m *mockTenantPolicyRepository) DeletePolicy(ctx context.Context, policyID uint) error {
	if m.deletePolicyFn != nil {
		return m.deletePolicyFn(ctx, policyID)
	}
	return nil
}

func (m *mockTenantPolicyRepository) DeleteUserPoliciesForTenant(ctx context.Context, userID uint, tenantID string) error {
	return nil
}

func (m *mockTenantPolicyRepository) HasAccessToTenant(ctx context.Context, userID uint, tenantID string) (bool, error) {
	if m.hasAccessToTenantFn != nil {
		return m.hasAccessToTenantFn(ctx, userID, tenantID)
	}
	return false, nil
}

func (m *mockTenantPolicyRepository) HasAccessToRealm(ctx context.Context, userID uint, tenantID, realmName string) (bool, error) {
	if m.hasAccessToRealmFn != nil {
		return m.hasAccessToRealmFn(ctx, userID, tenantID, realmName)
	}
	return false, nil
}

// Helper to create service with mocks
func newTestService(
	roleRepo *mockRoleRepository,
	permRepo *mockPermissionRepository,
	userRoleRepo *mockUserRoleRepository,
	policyRepo *mockTenantPolicyRepository,
) Service {
	if roleRepo == nil {
		roleRepo = &mockRoleRepository{}
	}
	if permRepo == nil {
		permRepo = &mockPermissionRepository{}
	}
	if userRoleRepo == nil {
		userRoleRepo = &mockUserRoleRepository{}
	}
	if policyRepo == nil {
		policyRepo = &mockTenantPolicyRepository{}
	}
	return NewService(roleRepo, permRepo, userRoleRepo, policyRepo, newTestLogger())
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name       string
		userID     uint
		permission string
		tenantID   *string
		mockFn     func(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error)
		want       bool
		wantErr    bool
	}{
		{
			name:       "user has permission",
			userID:     1,
			permission: "alerts:read",
			tenantID:   nil,
			mockFn: func(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
				return true, nil
			},
			want:    true,
			wantErr: false,
		},
		{
			name:       "user does not have permission",
			userID:     1,
			permission: "alerts:delete",
			tenantID:   nil,
			mockFn: func(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
				return false, nil
			},
			want:    false,
			wantErr: false,
		},
		{
			name:       "error checking permission",
			userID:     1,
			permission: "alerts:read",
			tenantID:   nil,
			mockFn: func(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
				return false, errors.New("database error")
			},
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRoleRepo := &mockUserRoleRepository{hasPermissionFn: tt.mockFn}
			svc := newTestService(nil, nil, userRoleRepo, nil)

			got, err := svc.HasPermission(context.Background(), tt.userID, tt.permission, tt.tenantID)
			if (err != nil) != tt.wantErr {
				t.Errorf("HasPermission() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("HasPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetUserPermissions(t *testing.T) {
	tests := []struct {
		name     string
		userID   uint
		tenantID *string
		mockFn   func(ctx context.Context, userID uint, tenantID *string) ([]string, error)
		want     []string
		wantErr  bool
	}{
		{
			name:     "get permissions success",
			userID:   1,
			tenantID: nil,
			mockFn: func(ctx context.Context, userID uint, tenantID *string) ([]string, error) {
				return []string{"alerts:read", "alerts:write", "tenants:read"}, nil
			},
			want:    []string{"alerts:read", "alerts:write", "tenants:read"},
			wantErr: false,
		},
		{
			name:     "no permissions",
			userID:   2,
			tenantID: nil,
			mockFn: func(ctx context.Context, userID uint, tenantID *string) ([]string, error) {
				return []string{}, nil
			},
			want:    []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRoleRepo := &mockUserRoleRepository{getUserPermissionsFn: tt.mockFn}
			svc := newTestService(nil, nil, userRoleRepo, nil)

			got, err := svc.GetUserPermissions(context.Background(), tt.userID, tt.tenantID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetUserPermissions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("GetUserPermissions() count = %v, want %v", len(got), len(tt.want))
			}
		})
	}
}

func TestHasAccessToTenant(t *testing.T) {
	tests := []struct {
		name         string
		userID       uint
		tenantID     string
		getUserRoles func(ctx context.Context, userID uint) ([]*domain.UserRole, error)
		hasAccessFn  func(ctx context.Context, userID uint, tenantID string) (bool, error)
		want         bool
		wantErr      bool
	}{
		{
			name:     "admin has access to all tenants",
			userID:   1,
			tenantID: "tenant-1",
			getUserRoles: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				return []*domain.UserRole{
					{UserID: 1, Role: &domain.Role{Name: RoleAdmin}},
				}, nil
			},
			hasAccessFn: nil, // Should not be called for admin
			want:        true,
			wantErr:     false,
		},
		{
			name:     "non-admin with policy access",
			userID:   2,
			tenantID: "tenant-1",
			getUserRoles: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				return []*domain.UserRole{
					{UserID: 2, Role: &domain.Role{Name: RoleViewer}},
				}, nil
			},
			hasAccessFn: func(ctx context.Context, userID uint, tenantID string) (bool, error) {
				return true, nil
			},
			want:    true,
			wantErr: false,
		},
		{
			name:     "non-admin without access",
			userID:   3,
			tenantID: "tenant-1",
			getUserRoles: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				return []*domain.UserRole{
					{UserID: 3, Role: &domain.Role{Name: RoleViewer}},
				}, nil
			},
			hasAccessFn: func(ctx context.Context, userID uint, tenantID string) (bool, error) {
				return false, nil
			},
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRoleRepo := &mockUserRoleRepository{getUserRolesFn: tt.getUserRoles}
			policyRepo := &mockTenantPolicyRepository{hasAccessToTenantFn: tt.hasAccessFn}
			svc := newTestService(nil, nil, userRoleRepo, policyRepo)

			got, err := svc.HasAccessToTenant(context.Background(), tt.userID, tt.tenantID)
			if (err != nil) != tt.wantErr {
				t.Errorf("HasAccessToTenant() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("HasAccessToTenant() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetUserRoles(t *testing.T) {
	tests := []struct {
		name    string
		userID  uint
		mockFn  func(ctx context.Context, userID uint) ([]*domain.UserRole, error)
		want    int
		wantErr bool
	}{
		{
			name:   "get roles success",
			userID: 1,
			mockFn: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				return []*domain.UserRole{
					{UserID: 1, RoleID: 1, Role: &domain.Role{Name: RoleAdmin}},
					{UserID: 1, RoleID: 2, Role: &domain.Role{Name: RoleOperator}},
				}, nil
			},
			want:    2,
			wantErr: false,
		},
		{
			name:   "no roles",
			userID: 2,
			mockFn: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				return []*domain.UserRole{}, nil
			},
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRoleRepo := &mockUserRoleRepository{getUserRolesFn: tt.mockFn}
			svc := newTestService(nil, nil, userRoleRepo, nil)

			got, err := svc.GetUserRoles(context.Background(), tt.userID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetUserRoles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != tt.want {
				t.Errorf("GetUserRoles() count = %v, want %v", len(got), tt.want)
			}
		})
	}
}

func TestAssignRoleToUser(t *testing.T) {
	tests := []struct {
		name       string
		userID     uint
		roleID     uint
		tenantID   *string
		assignedBy string
		expiresAt  *time.Time
		getRoleFn  func(ctx context.Context, roleID uint) (*domain.Role, error)
		assignFn   func(ctx context.Context, userRole *domain.UserRole) error
		wantErr    bool
	}{
		{
			name:       "successful assignment",
			userID:     1,
			roleID:     1,
			tenantID:   nil,
			assignedBy: "admin@example.com",
			expiresAt:  nil,
			getRoleFn: func(ctx context.Context, roleID uint) (*domain.Role, error) {
				return &domain.Role{ID: 1, Name: RoleOperator}, nil
			},
			assignFn: func(ctx context.Context, userRole *domain.UserRole) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:       "role not found",
			userID:     1,
			roleID:     999,
			tenantID:   nil,
			assignedBy: "admin@example.com",
			getRoleFn: func(ctx context.Context, roleID uint) (*domain.Role, error) {
				return nil, nil
			},
			assignFn: nil,
			wantErr:  true,
		},
		{
			name:       "assignment error",
			userID:     1,
			roleID:     1,
			tenantID:   nil,
			assignedBy: "admin@example.com",
			getRoleFn: func(ctx context.Context, roleID uint) (*domain.Role, error) {
				return &domain.Role{ID: 1, Name: RoleOperator}, nil
			},
			assignFn: func(ctx context.Context, userRole *domain.UserRole) error {
				return errors.New("database error")
			},
			wantErr: true,
		},
		{
			name:       "tenant-scoped admin assignment rejected",
			userID:     1,
			roleID:     1,
			tenantID:   func() *string { s := "tenant-a"; return &s }(),
			assignedBy: "admin@example.com",
			getRoleFn: func(ctx context.Context, roleID uint) (*domain.Role, error) {
				return &domain.Role{ID: 1, Name: RoleAdmin}, nil
			},
			assignFn: func(ctx context.Context, userRole *domain.UserRole) error {
				t.Fatalf("assignRole must not be called for a rejected admin assignment")
				return nil
			},
			wantErr: true,
		},
		{
			name:       "global admin assignment allowed",
			userID:     1,
			roleID:     1,
			tenantID:   nil,
			assignedBy: "admin@example.com",
			getRoleFn: func(ctx context.Context, roleID uint) (*domain.Role, error) {
				return &domain.Role{ID: 1, Name: RoleAdmin}, nil
			},
			assignFn: func(ctx context.Context, userRole *domain.UserRole) error {
				return nil
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roleRepo := &mockRoleRepository{getRoleByIDFn: tt.getRoleFn}
			userRoleRepo := &mockUserRoleRepository{assignRoleFn: tt.assignFn}
			svc := newTestService(roleRepo, nil, userRoleRepo, nil)

			err := svc.AssignRoleToUser(context.Background(), tt.userID, tt.roleID, tt.tenantID, tt.assignedBy, tt.expiresAt)
			if (err != nil) != tt.wantErr {
				t.Errorf("AssignRoleToUser() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAssignRoleToUser_AdminTenantScopedReturnsSentinel(t *testing.T) {
	roleRepo := &mockRoleRepository{
		getRoleByIDFn: func(ctx context.Context, roleID uint) (*domain.Role, error) {
			return &domain.Role{ID: 1, Name: RoleAdmin}, nil
		},
	}
	userRoleRepo := &mockUserRoleRepository{}
	svc := newTestService(roleRepo, nil, userRoleRepo, nil)

	tenant := "tenant-a"
	err := svc.AssignRoleToUser(context.Background(), 1, 1, &tenant, "admin@example.com", nil)
	if !errors.Is(err, ErrAdminRoleTenantScoped) {
		t.Errorf("AssignRoleToUser() error = %v, want ErrAdminRoleTenantScoped", err)
	}
}

func TestRemoveRoleFromUser(t *testing.T) {
	tests := []struct {
		name     string
		userID   uint
		roleID   uint
		tenantID *string
		mockFn   func(ctx context.Context, userID, roleID uint, tenantID *string) error
		wantErr  bool
	}{
		{
			name:     "successful removal",
			userID:   1,
			roleID:   1,
			tenantID: nil,
			mockFn: func(ctx context.Context, userID, roleID uint, tenantID *string) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:     "removal error",
			userID:   1,
			roleID:   1,
			tenantID: nil,
			mockFn: func(ctx context.Context, userID, roleID uint, tenantID *string) error {
				return errors.New("database error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRoleRepo := &mockUserRoleRepository{removeRoleFn: tt.mockFn}
			svc := newTestService(nil, nil, userRoleRepo, nil)

			err := svc.RemoveRoleFromUser(context.Background(), tt.userID, tt.roleID, tt.tenantID)
			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveRoleFromUser() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateRole(t *testing.T) {
	tests := []struct {
		name        string
		roleName    string
		displayName string
		description string
		isSystem    bool
		mockFn      func(ctx context.Context, role *domain.Role) error
		wantErr     bool
	}{
		{
			name:        "successful create",
			roleName:    "custom-role",
			displayName: "Custom Role",
			description: "A custom role",
			isSystem:    false,
			mockFn: func(ctx context.Context, role *domain.Role) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:        "create error",
			roleName:    "duplicate-role",
			displayName: "Duplicate Role",
			description: "",
			isSystem:    false,
			mockFn: func(ctx context.Context, role *domain.Role) error {
				return errors.New("duplicate key")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roleRepo := &mockRoleRepository{createRoleFn: tt.mockFn}
			svc := newTestService(roleRepo, nil, nil, nil)

			role, err := svc.CreateRole(context.Background(), tt.roleName, tt.displayName, tt.description, tt.isSystem)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateRole() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && role == nil {
				t.Error("CreateRole() returned nil role on success")
			}
		})
	}
}

func TestIsAdmin(t *testing.T) {
	tests := []struct {
		name    string
		userID  uint
		mockFn  func(ctx context.Context, userID uint) ([]*domain.UserRole, error)
		want    bool
		wantErr bool
	}{
		{
			name:   "user is admin",
			userID: 1,
			mockFn: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				return []*domain.UserRole{
					{UserID: 1, Role: &domain.Role{Name: RoleAdmin}},
				}, nil
			},
			want:    true,
			wantErr: false,
		},
		{
			name:   "user is not admin",
			userID: 2,
			mockFn: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				return []*domain.UserRole{
					{UserID: 2, Role: &domain.Role{Name: RoleOperator}},
				}, nil
			},
			want:    false,
			wantErr: false,
		},
		{
			name:   "user has no roles",
			userID: 3,
			mockFn: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				return []*domain.UserRole{}, nil
			},
			want:    false,
			wantErr: false,
		},
		{
			name:   "tenant-scoped admin is not a platform admin",
			userID: 4,
			mockFn: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				tenant := "tenant-a"
				return []*domain.UserRole{
					{UserID: 4, Role: &domain.Role{Name: RoleAdmin}, TenantID: &tenant},
				}, nil
			},
			want:    false,
			wantErr: false,
		},
		{
			name:   "expired global admin is not a platform admin",
			userID: 5,
			mockFn: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				past := time.Now().Add(-time.Hour)
				return []*domain.UserRole{
					{UserID: 5, Role: &domain.Role{Name: RoleAdmin}, ExpiresAt: &past},
				}, nil
			},
			want:    false,
			wantErr: false,
		},
		{
			name:   "tenant-scoped admin plus global admin is a platform admin",
			userID: 6,
			mockFn: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				tenant := "tenant-a"
				return []*domain.UserRole{
					{UserID: 6, Role: &domain.Role{Name: RoleAdmin}, TenantID: &tenant},
					{UserID: 6, Role: &domain.Role{Name: RoleAdmin}},
				}, nil
			},
			want:    true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRoleRepo := &mockUserRoleRepository{getUserRolesFn: tt.mockFn}
			svc := newTestService(nil, nil, userRoleRepo, nil)

			got, err := svc.IsAdmin(context.Background(), tt.userID)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsAdmin() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsAdmin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsOperator(t *testing.T) {
	tests := []struct {
		name    string
		userID  uint
		mockFn  func(ctx context.Context, userID uint) ([]*domain.UserRole, error)
		want    bool
		wantErr bool
	}{
		{
			name:   "user is operator",
			userID: 1,
			mockFn: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				return []*domain.UserRole{
					{UserID: 1, Role: &domain.Role{Name: RoleOperator}},
				}, nil
			},
			want:    true,
			wantErr: false,
		},
		{
			name:   "user is not operator",
			userID: 2,
			mockFn: func(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
				return []*domain.UserRole{
					{UserID: 2, Role: &domain.Role{Name: RoleViewer}},
				}, nil
			},
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRoleRepo := &mockUserRoleRepository{getUserRolesFn: tt.mockFn}
			svc := newTestService(nil, nil, userRoleRepo, nil)

			got, err := svc.IsOperator(context.Background(), tt.userID)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsOperator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsOperator() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateTenantPolicy(t *testing.T) {
	tests := []struct {
		name          string
		userID        uint
		tenantID      string
		allowedRealms []string
		grantedBy     string
		mockFn        func(ctx context.Context, policy *domain.TenantPolicy) error
		wantErr       bool
	}{
		{
			name:          "successful create",
			userID:        1,
			tenantID:      "tenant-1",
			allowedRealms: []string{"master", "test"},
			grantedBy:     "admin@example.com",
			mockFn: func(ctx context.Context, policy *domain.TenantPolicy) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:          "create error",
			userID:        1,
			tenantID:      "tenant-1",
			allowedRealms: []string{"master"},
			grantedBy:     "admin@example.com",
			mockFn: func(ctx context.Context, policy *domain.TenantPolicy) error {
				return errors.New("database error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policyRepo := &mockTenantPolicyRepository{createPolicyFn: tt.mockFn}
			svc := newTestService(nil, nil, nil, policyRepo)

			err := svc.CreateTenantPolicy(context.Background(), tt.userID, tt.tenantID, tt.allowedRealms, tt.grantedBy)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateTenantPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteTenantPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policyID uint
		mockFn   func(ctx context.Context, policyID uint) error
		wantErr  bool
	}{
		{
			name:     "successful delete",
			policyID: 1,
			mockFn: func(ctx context.Context, policyID uint) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:     "delete error",
			policyID: 999,
			mockFn: func(ctx context.Context, policyID uint) error {
				return errors.New("not found")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policyRepo := &mockTenantPolicyRepository{deletePolicyFn: tt.mockFn}
			svc := newTestService(nil, nil, nil, policyRepo)

			err := svc.DeleteTenantPolicy(context.Background(), tt.policyID)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteTenantPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestListRoles(t *testing.T) {
	tests := []struct {
		name            string
		includeInactive bool
		mockFn          func(ctx context.Context, includeInactive bool) ([]*domain.Role, error)
		want            int
		wantErr         bool
	}{
		{
			name:            "list active roles",
			includeInactive: false,
			mockFn: func(ctx context.Context, includeInactive bool) ([]*domain.Role, error) {
				return []*domain.Role{
					{ID: 1, Name: RoleAdmin, IsActive: true},
					{ID: 2, Name: RoleOperator, IsActive: true},
				}, nil
			},
			want:    2,
			wantErr: false,
		},
		{
			name:            "list all roles including inactive",
			includeInactive: true,
			mockFn: func(ctx context.Context, includeInactive bool) ([]*domain.Role, error) {
				return []*domain.Role{
					{ID: 1, Name: RoleAdmin, IsActive: true},
					{ID: 2, Name: RoleOperator, IsActive: true},
					{ID: 3, Name: "deprecated-role", IsActive: false},
				}, nil
			},
			want:    3,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roleRepo := &mockRoleRepository{listRolesFn: tt.mockFn}
			svc := newTestService(roleRepo, nil, nil, nil)

			roles, err := svc.ListRoles(context.Background(), tt.includeInactive)
			if (err != nil) != tt.wantErr {
				t.Errorf("ListRoles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(roles) != tt.want {
				t.Errorf("ListRoles() count = %v, want %v", len(roles), tt.want)
			}
		})
	}
}

func TestCheckRoleExpiration(t *testing.T) {
	tests := []struct {
		name    string
		mockFn  func(ctx context.Context) error
		wantErr bool
	}{
		{
			name: "successful check",
			mockFn: func(ctx context.Context) error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "check error",
			mockFn: func(ctx context.Context) error {
				return errors.New("database error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRoleRepo := &mockUserRoleRepository{checkExpirationFn: tt.mockFn}
			svc := newTestService(nil, nil, userRoleRepo, nil)

			err := svc.CheckRoleExpiration(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckRoleExpiration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// UpdateTenantPolicy takes a policy ID, not a user ID.
func TestUpdateTenantPolicy_UpdatesThePolicyWithThatID(t *testing.T) {
	policyA := &domain.TenantPolicy{ID: 5, UserID: 9, TenantID: "tenant-a", AllowedRealms: []string{"realmA"}}
	policyB := &domain.TenantPolicy{ID: 7, UserID: 5, TenantID: "tenant-b", AllowedRealms: []string{"realmB"}}

	var updated *domain.TenantPolicy
	policyRepo := &mockTenantPolicyRepository{
		getPolicyByIDFn: func(_ context.Context, policyID uint) (*domain.TenantPolicy, error) {
			for _, p := range []*domain.TenantPolicy{policyA, policyB} {
				if p.ID == policyID {
					return p, nil
				}
			}
			return nil, nil
		},
		getUserPoliciesFn: func(_ context.Context, userID uint) ([]*domain.TenantPolicy, error) {
			var out []*domain.TenantPolicy
			for _, p := range []*domain.TenantPolicy{policyA, policyB} {
				if p.UserID == userID {
					out = append(out, p)
				}
			}
			return out, nil
		},
		updatePolicyFn: func(_ context.Context, policy *domain.TenantPolicy) error {
			updated = policy
			return nil
		},
	}
	svc := newTestService(nil, nil, nil, policyRepo)

	if err := svc.UpdateTenantPolicy(context.Background(), policyA.ID, []string{"realmC"}, "admin@example.com"); err != nil {
		t.Fatalf("UpdateTenantPolicy() error = %v", err)
	}

	if updated == nil {
		t.Fatal("no policy was updated")
	}
	if updated.ID != policyA.ID {
		t.Errorf("updated policy %d, want %d", updated.ID, policyA.ID)
	}
	if !reflect.DeepEqual(policyB.AllowedRealms, []string{"realmB"}) {
		t.Errorf("policy %d was rewritten to %v; it belongs to user %d, not policy %d",
			policyB.ID, policyB.AllowedRealms, policyB.UserID, policyA.ID)
	}
}

func TestUpdateTenantPolicy_UnknownIDIsNotFound(t *testing.T) {
	policyRepo := &mockTenantPolicyRepository{
		updatePolicyFn: func(context.Context, *domain.TenantPolicy) error {
			t.Error("an unknown policy ID reached UpdatePolicy")
			return nil
		},
	}
	svc := newTestService(nil, nil, nil, policyRepo)

	if err := svc.UpdateTenantPolicy(context.Background(), 404, []string{"realmC"}, "admin@example.com"); err == nil {
		t.Fatal("UpdateTenantPolicy() error = nil, want a not-found error")
	}
}
