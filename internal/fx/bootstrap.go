package fx

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/fx"

	"repos.defensepoint.com/iam/oss-pmt/kmt/auth"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/database"
	"repos.defensepoint.com/iam/oss-pmt/kmt/rbac"
	"repos.defensepoint.com/iam/oss-pmt/kmt/tenant"
	"repos.defensepoint.com/iam/oss-pmt/kmt/users"
)

// BootstrapModule provides the bootstrap lifecycle hooks for seeding initial data.
var BootstrapModule = fx.Module("bootstrap",
	fx.Provide(
		provideRBACSeeder,
	),
	fx.Invoke(registerBootstrapHooks),
)

// provideRBACSeeder creates the RBAC seeder.
func provideRBACSeeder(service rbac.Service, log *logger.Logger) *rbac.Seeder {
	return rbac.NewSeeder(service, log)
}

// BootstrapParams contains all dependencies needed for bootstrap.
type BootstrapParams struct {
	fx.In

	Config            *config.AppConfig
	Logger            *logger.Logger
	RBACSeeder        *rbac.Seeder
	RBACService       rbac.Service
	TenantService     tenant.Service
	UserRepo          users.Repository
	SimpleAuthService auth.SimpleAuthService `optional:"true"`
	DB                *database.Client
}

// registerBootstrapHooks registers the bootstrap lifecycle hooks.
func registerBootstrapHooks(lc fx.Lifecycle, p BootstrapParams) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return runBootstrap(ctx, p)
		},
	})
}

// runBootstrap executes all bootstrap operations in order.
func runBootstrap(ctx context.Context, p BootstrapParams) error {
	p.Logger.Info("Starting bootstrap process")

	// 1. Ensure RBAC roles and permissions exist
	if err := p.RBACSeeder.EnsureSystemRolesAndPermissions(ctx); err != nil {
		return fmt.Errorf("failed to seed RBAC: %w", err)
	}

	// 2. Create default admin user if simple auth is enabled
	if p.Config.Auth.Simple.Enabled && p.SimpleAuthService != nil {
		if err := ensureDefaultAdminUser(ctx, p); err != nil {
			return fmt.Errorf("failed to create default admin user: %w", err)
		}
	}

	// 3. Remove telemetry belonging to deleted tenants, before config sync can
	//    recreate any of them.
	drainPurgeQueue(ctx, p)

	// 4. Sync tenants from config to database
	if err := syncTenantsFromConfig(ctx, p); err != nil {
		p.Logger.Error("Failed to sync tenants from configuration", logger.Err(err))
		// Don't fail startup - tenants can be added via API
	}

	p.Logger.Info("Bootstrap process completed successfully")
	return nil
}

// drainPurgeQueueBudget bounds how long the startup drain may run.
//
// This hook runs inside fx's OnStart phase, which has no configured
// fx.StartTimeout and therefore shares the 15s fx.DefaultTimeout with RBAC
// seeding and config sync. The drain can touch up to drainBatchLimit rows per
// table per tenant across three hypertables, mostly in chunks the 7-day policy
// has compressed, so without its own budget an overrun would abort fx and the
// application would not start, every boot, because the queue row survives.
const drainPurgeQueueBudget = 5 * time.Second

// drainPurgeQueue removes telemetry belonging to deleted tenants. It runs before
// syncTenantsFromConfig so a config-defined tenant that was deleted has its old
// telemetry removed before config recreates it. A failure here is never fatal:
// the queue rows survive and the next startup retries.
func drainPurgeQueue(ctx context.Context, p BootstrapParams) {
	if p.DB == nil {
		return
	}

	// Cutting the drain short is safe precisely because the work is resumable:
	// deletes that already committed stand, the queue row survives a cancelled
	// pass, and the next boot picks up where this one stopped. Trading a hard
	// startup failure for slower convergence is the entire point of the bound.
	ctx, cancel := context.WithTimeout(ctx, drainPurgeQueueBudget)
	defer cancel()

	counts, err := p.DB.DrainPendingPurges(ctx)
	// Counts are reported even on the error path: a drain that removed 40,000
	// rows and then failed should still say so.
	for table, n := range counts {
		if n > 0 {
			p.Logger.Info("Drained telemetry table",
				logger.Str("table", table), logger.Int64("rows", n))
		}
	}
	if err != nil {
		p.Logger.Warn("Failed to drain tenant purge queue, will retry next startup", logger.Err(err))
	}
}

// ensureDefaultAdminUser creates the default admin user if no users exist.
func ensureDefaultAdminUser(ctx context.Context, p BootstrapParams) error {
	// Check if any users exist
	count, err := p.UserRepo.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to count users: %w", err)
	}

	if count > 0 {
		p.Logger.Debug("Users already exist, skipping default admin creation")

		// Still ensure admin user has admin role if configured
		if p.Config.Auth.Simple.DefaultEmail != "" {
			return ensureAdminRole(ctx, p)
		}
		return nil
	}

	// Validate required configuration
	if p.Config.Auth.Simple.DefaultUser == "" {
		return fmt.Errorf("SECURITY ERROR: default_user must be set via environment variable MONITORING_AUTH_SIMPLE_DEFAULT_USER")
	}
	if p.Config.Auth.Simple.DefaultPass == "" {
		return fmt.Errorf("SECURITY ERROR: default_pass must be set via environment variable MONITORING_AUTH_SIMPLE_DEFAULT_PASS")
	}
	if p.Config.Auth.Simple.DefaultEmail == "" {
		return fmt.Errorf("SECURITY ERROR: default_email must be set via environment variable MONITORING_AUTH_SIMPLE_DEFAULT_EMAIL")
	}

	p.Logger.Info("Creating initial admin user",
		logger.Str("username", p.Config.Auth.Simple.DefaultUser),
		logger.Str("email", p.Config.Auth.Simple.DefaultEmail))

	// Create the admin user using SimpleAuthService
	err = p.SimpleAuthService.CreateUser(
		ctx,
		p.Config.Auth.Simple.DefaultUser,  // username
		p.Config.Auth.Simple.DefaultPass,  // password
		p.Config.Auth.Simple.DefaultEmail, // email
		p.Config.Auth.Simple.DefaultUser,  // name (use username as name)
	)
	if err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	p.Logger.Info("Initial admin user created successfully")

	// Assign admin role to the new user
	return ensureAdminRole(ctx, p)
}

// ensureAdminRole ensures the default admin user has the admin role.
func ensureAdminRole(ctx context.Context, p BootstrapParams) error {
	if p.Config.Auth.Simple.DefaultEmail == "" {
		return nil
	}

	// Get user by email
	user, err := p.UserRepo.GetByEmail(ctx, p.Config.Auth.Simple.DefaultEmail)
	if err != nil {
		p.Logger.Warn("Could not find default admin user for role assignment",
			logger.Str("email", p.Config.Auth.Simple.DefaultEmail),
			logger.Err(err))
		return nil // Don't fail - user might not exist yet
	}

	// Check if already admin
	isAdmin, err := p.RBACService.IsAdmin(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to check admin status: %w", err)
	}

	if isAdmin {
		p.Logger.Debug("Default admin user already has admin role",
			logger.Uint("user_id", user.ID))
		return nil
	}

	// Assign admin role
	err = p.RBACSeeder.CreateDefaultAdminUser(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to assign admin role: %w", err)
	}

	p.Logger.Info("Admin role assigned to default user",
		logger.Uint("user_id", user.ID),
		logger.Str("email", user.Email))

	return nil
}

// syncTenantsFromConfig synchronizes tenants from configuration to database.
func syncTenantsFromConfig(ctx context.Context, p BootstrapParams) error {
	if len(p.Config.Keycloak.Tenants) == 0 {
		p.Logger.Info("No tenants defined in configuration - use API or UI to add tenants")
		return nil
	}

	p.Logger.Info("Syncing tenants from configuration",
		logger.Int("count", len(p.Config.Keycloak.Tenants)))

	for tenantID, cfgTenant := range p.Config.Keycloak.Tenants {
		if err := syncTenant(ctx, p, tenantID, cfgTenant); err != nil {
			p.Logger.Error("Failed to sync tenant from config",
				logger.Str("tenant_id", tenantID),
				logger.Err(err))
			// Continue with other tenants
			continue
		}
	}

	p.Logger.Info("Tenant configuration sync completed")
	return nil
}

// syncTenant creates or updates a single tenant from configuration.
func syncTenant(ctx context.Context, p BootstrapParams, tenantID string, cfgTenant config.TenantConfig) error {
	// Validate required fields
	if tenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if cfgTenant.ServerURL == "" {
		return fmt.Errorf("server_url is required for tenant %s", tenantID)
	}

	// Set defaults
	if cfgTenant.ClientID == "" {
		cfgTenant.ClientID = "admin-cli"
	}
	if cfgTenant.AdminRealm == "" {
		cfgTenant.AdminRealm = "master"
	}
	if cfgTenant.Name == "" {
		cfgTenant.Name = tenantID
	}

	// Check if tenant already exists
	existingTenant, err := p.TenantService.GetTenant(ctx, tenantID)
	if err == nil && existingTenant != nil {
		// Tenant exists - update it
		p.Logger.Debug("Updating tenant from configuration", logger.Str("tenant_id", tenantID))

		name := cfgTenant.Name
		description := cfgTenant.Description
		serverURL := cfgTenant.ServerURL
		adminRealm := cfgTenant.AdminRealm
		clientID := cfgTenant.ClientID
		clientSecret := cfgTenant.ClientSecret
		enabled := cfgTenant.Enabled
		isDefault := cfgTenant.IsDefault
		owner := cfgTenant.Owner
		defaultRealm := cfgTenant.Config.DefaultRealm

		updateReq := &tenant.UpdateRequest{
			Name:         &name,
			Description:  &description,
			ServerURL:    &serverURL,
			AdminRealm:   &adminRealm,
			ClientID:     &clientID,
			ClientSecret: &clientSecret,
			Enabled:      &enabled,
			IsDefault:    &isDefault,
			Tags:         cfgTenant.Tags,
			Owner:        &owner,
			DefaultRealm: &defaultRealm,
		}

		_, err := p.TenantService.UpdateTenant(ctx, tenantID, updateReq)
		return err
	}

	// Tenant doesn't exist - create it
	p.Logger.Info("Creating tenant from configuration", logger.Str("tenant_id", tenantID))

	createReq := &tenant.CreateRequest{
		TenantID:     tenantID,
		Name:         cfgTenant.Name,
		Description:  cfgTenant.Description,
		ServerURL:    cfgTenant.ServerURL,
		AdminRealm:   cfgTenant.AdminRealm,
		ClientID:     cfgTenant.ClientID,
		ClientSecret: cfgTenant.ClientSecret,
		Enabled:      cfgTenant.Enabled,
		IsDefault:    cfgTenant.IsDefault,
		Tags:         cfgTenant.Tags,
		Owner:        cfgTenant.Owner,
		DefaultRealm: cfgTenant.Config.DefaultRealm,
	}

	_, err = p.TenantService.CreateTenant(ctx, createReq)
	return err
}
