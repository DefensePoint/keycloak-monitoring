// Package postgres provides the PostgreSQL implementation of the tenant repository.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/database"
	"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/secretcrypto"
	"repos.defensepoint.com/iam/oss-pmt/kmt/tenant"
)

// Repository implements tenant.Repository using PostgreSQL via GORM.
type Repository struct {
	db        *database.Client
	encryptor *secretcrypto.Encryptor // nil means client_secret is stored as-is (no encryption key configured)
	log       *logger.Logger
}

// NewRepository creates a new PostgreSQL tenant repository. encryptor may be
// nil, in which case client_secret is read/written unchanged — existing
// plaintext rows keep working, and Decrypt() already passes through any
// value without the encrypted-value prefix regardless, but skipping the call
// entirely when there's no key avoids paying for it on every row.
func NewRepository(db *database.Client, encryptor *secretcrypto.Encryptor, log *logger.Logger) tenant.Repository {
	return &Repository{db: db, encryptor: encryptor, log: log}
}

// GetByID returns a tenant by its database ID.
func (r *Repository) GetByID(ctx context.Context, id uint) (*domain.KeycloakTenant, error) {
	var dbTenant database.KeycloakTenant
	if err := r.db.DB().WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&dbTenant).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("tenant with id %d not found", id)
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return r.convertTenant(&dbTenant)
}

// GetByTenantID returns a tenant by its tenant_id.
func (r *Repository) GetByTenantID(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
	var dbTenant database.KeycloakTenant
	if err := r.db.DB().WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		First(&dbTenant).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("tenant %s not found", tenantID)
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return r.convertTenant(&dbTenant)
}

// List returns all tenants.
func (r *Repository) List(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	var dbTenants []database.KeycloakTenant
	if err := r.db.DB().WithContext(ctx).
		Where("deleted_at IS NULL").
		Find(&dbTenants).Error; err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	return r.convertTenantList(ctx, dbTenants)
}

// ListEnabled returns all enabled tenants.
func (r *Repository) ListEnabled(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	var dbTenants []database.KeycloakTenant
	if err := r.db.DB().WithContext(ctx).
		Where("enabled = ? AND deleted_at IS NULL", true).
		Find(&dbTenants).Error; err != nil {
		return nil, fmt.Errorf("failed to list enabled tenants: %w", err)
	}
	return r.convertTenantList(ctx, dbTenants)
}

// GetDefault returns the default tenant.
func (r *Repository) GetDefault(ctx context.Context) (*domain.KeycloakTenant, error) {
	var dbTenant database.KeycloakTenant
	if err := r.db.DB().WithContext(ctx).
		Where("is_default = ? AND deleted_at IS NULL", true).
		First(&dbTenant).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no default tenant found")
		}
		return nil, fmt.Errorf("failed to get default tenant: %w", err)
	}
	return r.convertTenant(&dbTenant)
}

// Create creates a new tenant.
func (r *Repository) Create(ctx context.Context, t *domain.KeycloakTenant) error {
	dbTenant, err := r.convertToDBTenant(t)
	if err != nil {
		return err
	}
	if err := r.db.DB().WithContext(ctx).Create(&dbTenant).Error; err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}
	t.ID = dbTenant.ID
	t.CreatedAt = dbTenant.CreatedAt
	t.UpdatedAt = dbTenant.UpdatedAt
	return nil
}

// Update updates an existing tenant.
func (r *Repository) Update(ctx context.Context, t *domain.KeycloakTenant) error {
	dbTenant, err := r.convertToDBTenant(t)
	if err != nil {
		return err
	}
	dbTenant.ID = t.ID
	if err := r.db.DB().WithContext(ctx).Save(&dbTenant).Error; err != nil {
		return fmt.Errorf("failed to update tenant: %w", err)
	}
	t.UpdatedAt = dbTenant.UpdatedAt
	return nil
}

// Delete removes a tenant and every row it owns.
//
// This is a hard delete, not a soft delete: leftover rows from a deleted tenant
// hold unique keys (realm names, alert IDs, rule IDs) that would block any future
// tenant, and a soft-deleted tenant row would keep its tenant_id reserved.
// Telemetry is queued for drain at startup rather than deleted here.
func (r *Repository) Delete(ctx context.Context, tenantID string) error {
	if _, err := r.db.PurgeTenant(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}
	return nil
}

// UpdateHealth updates the health status of a tenant.
func (r *Repository) UpdateHealth(ctx context.Context, tenantID, status, message string) error {
	now := time.Now()
	if err := r.db.DB().WithContext(ctx).
		Model(&database.KeycloakTenant{}).
		Where("tenant_id = ?", tenantID).
		Updates(map[string]interface{}{
			"last_health_check": now,
			"health_status":     status,
			"health_message":    message,
			"updated_at":        now,
		}).Error; err != nil {
		return fmt.Errorf("failed to update tenant health: %w", err)
	}
	return nil
}

// UpdateError records an error for a tenant.
func (r *Repository) UpdateError(ctx context.Context, tenantID, errorMsg string) error {
	now := time.Now()
	if err := r.db.DB().WithContext(ctx).
		Model(&database.KeycloakTenant{}).
		Where("tenant_id = ?", tenantID).
		Updates(map[string]interface{}{
			"last_error":    errorMsg,
			"last_error_at": now,
			"updated_at":    now,
		}).Error; err != nil {
		return fmt.Errorf("failed to update tenant error: %w", err)
	}
	return nil
}

// UnsetDefault unsets all tenants as default.
func (r *Repository) UnsetDefault(ctx context.Context) error {
	if err := r.db.DB().WithContext(ctx).
		Model(&database.KeycloakTenant{}).
		Where("is_default = ?", true).
		Update("is_default", false).Error; err != nil {
		return fmt.Errorf("failed to unset default tenants: %w", err)
	}
	return nil
}

// convertTenant converts a database tenant to a domain tenant, decrypting
// client_secret if it carries the secretcrypto prefix (Decrypt passes
// legacy plaintext through unchanged, so this is safe to call unconditionally
// even before any row has been migrated to the encrypted form).
func (r *Repository) convertTenant(db *database.KeycloakTenant) (*domain.KeycloakTenant, error) {
	var tags []string
	if db.Tags != "" {
		_ = json.Unmarshal([]byte(db.Tags), &tags)
	}

	clientSecret := db.ClientSecret
	if r.encryptor != nil {
		decrypted, err := r.encryptor.Decrypt(db.ClientSecret)
		if err != nil {
			return nil, fmt.Errorf("client_secret could not be decrypted, check encryption key: %w", err)
		}
		clientSecret = decrypted
	} else if secretcrypto.IsEncrypted(db.ClientSecret) {
		// No encryptor configured, but this row was encrypted while one was.
		// Handing back the literal ciphertext would make it the credential
		// sent to Keycloak — fail the conversion instead of silently
		// returning an unusable value that looks like a working secret.
		return nil, fmt.Errorf("client_secret is encrypted but MONITORING_SECURITY_ENCRYPTION_KEY is not set")
	}

	return &domain.KeycloakTenant{
		ID:                db.ID,
		TenantID:          db.TenantID,
		Name:              db.Name,
		Description:       db.Description,
		ServerURL:         db.ServerURL,
		AdminRealm:        db.AdminRealm,
		ClientID:          db.ClientID,
		ClientSecret:      clientSecret,
		Configuration:     string(db.Configuration),
		DefaultRealm:      db.DefaultRealm,
		Enabled:           db.Enabled,
		LastHealthCheck:   db.LastHealthCheck,
		HealthStatus:      db.HealthStatus,
		HealthMessage:     db.HealthMessage,
		LastError:         db.LastError,
		LastErrorAt:       db.LastErrorAt,
		IsDefault:         db.IsDefault,
		Tags:              tags,
		Owner:             db.Owner,
		InfinispanEnabled: db.InfinispanEnabled,
		InfinispanPort:    db.InfinispanPort,
		CreatedAt:         db.CreatedAt,
		UpdatedAt:         db.UpdatedAt,
	}, nil
}

// convertTenantList converts a list of database tenants to domain tenants. A
// tenant that fails to convert (e.g. its client_secret can't be decrypted) is
// marked unhealthy and skipped rather than failing the whole batch — one bad
// row must not take down every tenant's monitoring, the API, and the UI along
// with it, which is what happens if this error propagates up through
// Service.LoadTenants into the Fx OnStart hook. Mirrors how monitor_pool.go
// already isolates a single tenant's CreateClient failure from the rest of
// the pool.
func (r *Repository) convertTenantList(ctx context.Context, dbTenants []database.KeycloakTenant) ([]*domain.KeycloakTenant, error) {
	result := make([]*domain.KeycloakTenant, 0, len(dbTenants))
	for i := range dbTenants {
		converted, err := r.convertTenant(&dbTenants[i])
		if err != nil {
			tenantID := dbTenants[i].TenantID
			r.log.Error("Skipping tenant that failed to load",
				logger.Str("tenant_id", tenantID), logger.Err(err))
			_ = r.UpdateHealth(ctx, tenantID, "unhealthy", err.Error())
			_ = r.UpdateError(ctx, tenantID, err.Error())
			continue
		}
		result = append(result, converted)
	}
	return result, nil
}

// convertToDBTenant converts a domain tenant to a database tenant, encrypting
// client_secret when an encryptor is configured.
func (r *Repository) convertToDBTenant(t *domain.KeycloakTenant) (*database.KeycloakTenant, error) {
	var tagsJSON string
	if len(t.Tags) > 0 {
		tagsBytes, _ := json.Marshal(t.Tags)
		tagsJSON = string(tagsBytes)
	}

	var configBytes []byte
	if t.Configuration != "" {
		configBytes = []byte(t.Configuration)
	}

	clientSecret := t.ClientSecret
	if r.encryptor != nil {
		encrypted, err := r.encryptor.Encrypt(t.ClientSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt client_secret for tenant %s: %w", t.TenantID, err)
		}
		clientSecret = encrypted
	}

	return &database.KeycloakTenant{
		ID:                t.ID,
		TenantID:          t.TenantID,
		Name:              t.Name,
		Description:       t.Description,
		ServerURL:         t.ServerURL,
		AdminRealm:        t.AdminRealm,
		ClientID:          t.ClientID,
		ClientSecret:      clientSecret,
		Configuration:     configBytes,
		DefaultRealm:      t.DefaultRealm,
		Enabled:           t.Enabled,
		LastHealthCheck:   t.LastHealthCheck,
		HealthStatus:      t.HealthStatus,
		HealthMessage:     t.HealthMessage,
		LastError:         t.LastError,
		LastErrorAt:       t.LastErrorAt,
		IsDefault:         t.IsDefault,
		Tags:              tagsJSON,
		Owner:             t.Owner,
		InfinispanEnabled: t.InfinispanEnabled,
		InfinispanPort:    t.InfinispanPort,
		CreatedAt:         t.CreatedAt,
		UpdatedAt:         t.UpdatedAt,
	}, nil
}

// Ensure Repository implements tenant.Repository
var _ tenant.Repository = (*Repository)(nil)
