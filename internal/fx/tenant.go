package fx

import (
	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
	tenantpostgres "github.com/DefensePoint/keycloak-monitoring/internal/tenant/postgres"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
	"github.com/DefensePoint/keycloak-monitoring/pkg/secretcrypto"
)

// TenantModule provides tenant domain dependencies.
var TenantModule = fx.Module("tenant",
	fx.Provide(
		provideTenantRepository,
		provideTenantService,
	),
)

// provideTenantRepository creates a tenant repository. client_secret
// encryption is opt-in via MONITORING_SECURITY_ENCRYPTION_KEY: unset means
// tenants keep working exactly as before (plaintext, no migration forced on
// existing deployments); set-but-invalid fails startup, since a broken key
// silently disables encryption in a way that's easy to miss.
func provideTenantRepository(db *database.Client, cfg *config.AppConfig, log *logger.Logger) (tenant.Repository, error) {
	if cfg.Security.EncryptionKey == "" {
		warnIfEncryptedTenantsExist(db, log)
		return tenantpostgres.NewRepository(db, nil, log), nil
	}

	encryptor, err := secretcrypto.New(cfg.Security.EncryptionKey)
	if err != nil {
		return nil, err
	}

	if cfg.Security.RunSecretMigration {
		// Runs before the repository is handed out, so nothing reads a
		// still-plaintext client_secret while this is in progress. A failure
		// here is intentionally non-fatal: it's an opt-in operational step,
		// not something that should be able to take the service down, and
		// per-tenant failures are already logged individually inside it.
		if err := db.MigrateEncryptClientSecrets(encryptor); err != nil {
			log.Error("Secret migration finished with errors, see prior log entries", logger.Err(err))
		}
	}

	return tenantpostgres.NewRepository(db, encryptor, log), nil
}

// warnIfEncryptedTenantsExist distinguishes "keyless from day one" from "just
// broke every encrypted tenant" — both produce an identical-looking service
// otherwise, since neither logs anything beyond a generic warning. Counting
// encrypted rows directly (bypassing the repository, which needs the key to
// even construct) lets the message name the actual number of tenants at risk.
func warnIfEncryptedTenantsExist(db *database.Client, log *logger.Logger) {
	var count int64
	err := db.DB().
		Model(&database.KeycloakTenant{}).
		Where("client_secret LIKE ?", secretcrypto.EncryptedPrefix+"%").
		Count(&count).Error
	if err != nil {
		log.Warn("MONITORING_SECURITY_ENCRYPTION_KEY not set - tenant client_secret will be stored in plaintext",
			logger.Err(err))
		return
	}

	if count == 0 {
		log.Warn("MONITORING_SECURITY_ENCRYPTION_KEY not set - tenant client_secret will be stored in plaintext")
		return
	}

	log.Error("encryption key not configured but tenant(s) have encrypted client_secret values; "+
		"those tenants cannot authenticate until MONITORING_SECURITY_ENCRYPTION_KEY is restored",
		logger.Int64("affected_tenant_count", count))
}

// provideTenantService creates a tenant service.
func provideTenantService(repo tenant.Repository) tenant.Service {
	return tenant.NewService(repo)
}
