package rbac

import (
	"context"
	"fmt"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// Seeder handles seeding of initial RBAC data.
type Seeder struct {
	service Service
	logger  *logger.Logger
}

// NewSeeder creates a new RBAC seeder.
func NewSeeder(service Service, log *logger.Logger) *Seeder {
	return &Seeder{
		service: service,
		logger:  log,
	}
}

// SeedRolesAndPermissions seeds the system with initial roles and permissions.
func (s *Seeder) SeedRolesAndPermissions(ctx context.Context) error {
	s.logger.Info("Starting RBAC seed process")

	// Seed permissions first
	if err := s.seedPermissions(ctx); err != nil {
		return fmt.Errorf("failed to seed permissions: %w", err)
	}

	// Seed roles
	if err := s.seedRoles(ctx); err != nil {
		return fmt.Errorf("failed to seed roles: %w", err)
	}

	s.logger.Info("RBAC seed completed successfully")
	return nil
}

// seedPermissions creates all system permissions.
func (s *Seeder) seedPermissions(ctx context.Context) error {
	s.logger.Info("Seeding permissions")

	permissionDefs := GetSystemPermissions()

	for _, permDef := range permissionDefs {
		// Check if permission already exists
		existing, err := s.service.ListPermissions(ctx, permDef.Resource)
		if err != nil {
			return fmt.Errorf("failed to check permissions for resource %s: %w", permDef.Resource, err)
		}

		// Check if this specific permission exists
		found := false
		for _, p := range existing {
			if p.Name == permDef.Name {
				found = true
				break
			}
		}

		if found {
			s.logger.Debug("Permission already exists, skipping", logger.Str("permission", permDef.Name))
			continue
		}

		// Create permission
		_, err = s.service.CreatePermission(
			ctx,
			permDef.Name,
			permDef.DisplayName,
			permDef.Description,
			permDef.Resource,
			permDef.Action,
			true, // System permission
		)
		if err != nil {
			return fmt.Errorf("failed to create permission %s: %w", permDef.Name, err)
		}

		s.logger.Debug("Permission created", logger.Str("permission", permDef.Name))
	}

	s.logger.Info("Permissions seeded successfully", logger.Int("count", len(permissionDefs)))
	return nil
}

// seedRoles creates all system roles and assigns permissions.
func (s *Seeder) seedRoles(ctx context.Context) error {
	s.logger.Info("Seeding roles")

	roleDefs := GetSystemRoles()

	for _, roleDef := range roleDefs {
		// Check if role already exists
		existing, err := s.service.GetRoleByName(ctx, roleDef.Name)
		if err != nil {
			return fmt.Errorf("failed to check role %s: %w", roleDef.Name, err)
		}

		var roleID uint
		if existing != nil {
			s.logger.Debug("Role already exists, updating permissions", logger.Str("role", roleDef.Name))
			roleID = existing.ID
		} else {
			// Create role
			role, err := s.service.CreateRole(
				ctx,
				roleDef.Name,
				roleDef.DisplayName,
				roleDef.Description,
				true, // System role
			)
			if err != nil {
				return fmt.Errorf("failed to create role %s: %w", roleDef.Name, err)
			}
			roleID = role.ID
			s.logger.Debug("Role created", logger.Str("role", roleDef.Name))
		}

		// Assign permissions to role
		if len(roleDef.Permissions) > 0 {
			err := s.service.AssignPermissionsToRole(ctx, roleID, roleDef.Permissions)
			if err != nil {
				// Log warning but don't fail if some permissions already exist
				s.logger.Warn("Failed to assign some permissions to role",
					logger.Str("role", roleDef.Name),
					logger.Err(err))
			} else {
				s.logger.Debug("Permissions assigned to role",
					logger.Str("role", roleDef.Name),
					logger.Int("permission_count", len(roleDef.Permissions)))
			}
		}
	}

	s.logger.Info("Roles seeded successfully", logger.Int("count", len(roleDefs)))
	return nil
}

// EnsureSystemRolesAndPermissions ensures system roles and permissions exist.
// This can be called on application startup to ensure the system is properly initialized.
func (s *Seeder) EnsureSystemRolesAndPermissions(ctx context.Context) error {
	s.logger.Info("Ensuring system roles and permissions exist")

	// Check if any roles exist
	roles, err := s.service.ListRoles(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to list roles: %w", err)
	}

	if len(roles) == 0 {
		s.logger.Info("No roles found, running full seed")
		return s.SeedRolesAndPermissions(ctx)
	}

	// Check if system roles exist
	systemRoles := GetSystemRoles()
	for _, roleDef := range systemRoles {
		role, err := s.service.GetRoleByName(ctx, roleDef.Name)
		if err != nil {
			return fmt.Errorf("failed to check role %s: %w", roleDef.Name, err)
		}
		if role == nil {
			s.logger.Info("System role missing, running full seed", logger.Str("role", roleDef.Name))
			return s.SeedRolesAndPermissions(ctx)
		}
	}

	s.logger.Info("System roles and permissions verified")
	return nil
}

// CreateDefaultAdminUser assigns the admin role to a user.
// This should only be called during initial setup for the first admin user.
func (s *Seeder) CreateDefaultAdminUser(ctx context.Context, userID uint) error {
	s.logger.Info("Assigning admin role to user", logger.Uint("user_id", userID))

	// Get admin role
	adminRole, err := s.service.GetRoleByName(ctx, RoleAdmin)
	if err != nil {
		return fmt.Errorf("failed to get admin role: %w", err)
	}
	if adminRole == nil {
		return fmt.Errorf("admin role not found - run seed first")
	}

	// Assign admin role to user (global scope)
	err = s.service.AssignRoleToUser(ctx, userID, adminRole.ID, nil, "system", nil)
	if err != nil {
		return fmt.Errorf("failed to assign admin role to user: %w", err)
	}

	s.logger.Info("Admin role assigned to user", logger.Uint("user_id", userID))
	return nil
}
