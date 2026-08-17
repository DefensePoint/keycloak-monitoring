package fx

import (
	"go.uber.org/fx"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/database"
	"repos.defensepoint.com/iam/oss-pmt/kmt/rbac"
	rbacpostgres "repos.defensepoint.com/iam/oss-pmt/kmt/rbac/postgres"
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
