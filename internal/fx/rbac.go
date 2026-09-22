package fx

import (
	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
	"github.com/DefensePoint/keycloak-monitoring/rbac"
	rbacpostgres "github.com/DefensePoint/keycloak-monitoring/rbac/postgres"
)

// RBACModule provides RBAC domain dependencies.
var RBACModule = fx.Module("rbac",
	fx.Provide(
		provideRBACRoleRepository,
		provideRBACPermissionRepository,
		provideRBACUserRoleRepository,
		provideRBACTenantPolicyRepository,
		provideRBACService,
	),
)

func provideRBACRoleRepository(db *database.Client) rbac.RoleRepository {
	return rbacpostgres.NewRoleRepository(db)
}

func provideRBACPermissionRepository(db *database.Client) rbac.PermissionRepository {
	return rbacpostgres.NewPermissionRepository(db)
}

func provideRBACUserRoleRepository(db *database.Client) rbac.UserRoleRepository {
	return rbacpostgres.NewUserRoleRepository(db)
}

func provideRBACTenantPolicyRepository(db *database.Client) rbac.TenantPolicyRepository {
	return rbacpostgres.NewTenantPolicyRepository(db)
}

func provideRBACService(
	roleRepo rbac.RoleRepository,
	permissionRepo rbac.PermissionRepository,
	userRoleRepo rbac.UserRoleRepository,
	policyRepo rbac.TenantPolicyRepository,
	log *logger.Logger,
) rbac.Service {
	return rbac.NewService(roleRepo, permissionRepo, userRoleRepo, policyRepo, log)
}
