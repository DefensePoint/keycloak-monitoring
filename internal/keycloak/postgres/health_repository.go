package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// HealthRepository implements keycloak.HealthRepository using GORM.
type HealthRepository struct {
	db *gorm.DB
}

// NewHealthRepository creates a new HealthRepository.
func NewHealthRepository(db *gorm.DB) *HealthRepository {
	return &HealthRepository{db: db}
}

// GetLatestHealth returns the latest health status for a tenant.
func (r *HealthRepository) GetLatestHealth(ctx context.Context, tenantID string) (*domain.KeycloakHealth, error) {
	var dbHealth database.KeycloakHealth

	result := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("time DESC").
		First(&dbHealth)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get latest health: %w", result.Error)
	}

	return convertDBHealthToDomain(&dbHealth), nil
}

// GetHealthHistory returns health status history for a tenant.
func (r *HealthRepository) GetHealthHistory(ctx context.Context, tenantID string, from, to time.Time) ([]*domain.KeycloakHealth, error) {
	var dbHealth []*database.KeycloakHealth

	result := r.db.WithContext(ctx).
		Where("tenant_id = ? AND time >= ? AND time <= ?", tenantID, from, to).
		Order("time ASC").
		Find(&dbHealth)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get health history: %w", result.Error)
	}

	return convertDBHealthListToDomain(dbHealth), nil
}

// SaveHealth saves health status.
func (r *HealthRepository) SaveHealth(ctx context.Context, health *domain.KeycloakHealth) error {
	dbHealth := convertDomainHealthToDB(health)

	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "time"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "response_time", "server_version",
			"uptime_millis", "memory_used", "memory_max", "memory_free",
			"error_message",
		}),
	}).Create(dbHealth)

	if result.Error != nil {
		return fmt.Errorf("failed to save health: %w", result.Error)
	}

	return nil
}

// convertDBHealthToDomain converts a database health to domain health.
func convertDBHealthToDomain(h *database.KeycloakHealth) *domain.KeycloakHealth {
	return &domain.KeycloakHealth{
		TenantID:      h.TenantID,
		Time:          h.Time,
		Status:        h.Status,
		ResponseTime:  h.ResponseTime,
		ServerVersion: h.ServerVersion,
		UptimeMillis:  h.UptimeMillis,
		MemoryUsed:    h.MemoryUsed,
		MemoryMax:     h.MemoryMax,
		MemoryFree:    h.MemoryFree,
		ErrorMessage:  h.ErrorMessage,
	}
}

// convertDBHealthListToDomain converts a list of database health records.
func convertDBHealthListToDomain(dbHealth []*database.KeycloakHealth) []*domain.KeycloakHealth {
	result := make([]*domain.KeycloakHealth, len(dbHealth))
	for i, h := range dbHealth {
		result[i] = convertDBHealthToDomain(h)
	}
	return result
}

// convertDomainHealthToDB converts domain health to database health.
func convertDomainHealthToDB(h *domain.KeycloakHealth) *database.KeycloakHealth {
	return &database.KeycloakHealth{
		TenantID:      h.TenantID,
		Time:          h.Time,
		Status:        h.Status,
		ResponseTime:  h.ResponseTime,
		ServerVersion: h.ServerVersion,
		UptimeMillis:  h.UptimeMillis,
		MemoryUsed:    h.MemoryUsed,
		MemoryMax:     h.MemoryMax,
		MemoryFree:    h.MemoryFree,
		ErrorMessage:  h.ErrorMessage,
	}
}

// Ensure HealthRepository implements keycloak.HealthRepository
var _ keycloak.HealthRepository = (*HealthRepository)(nil)
