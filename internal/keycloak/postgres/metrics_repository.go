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

// MetricsRepository implements keycloak.MetricsRepository using GORM.
type MetricsRepository struct {
	db *gorm.DB
}

// NewMetricsRepository creates a new MetricsRepository.
func NewMetricsRepository(db *gorm.DB) *MetricsRepository {
	return &MetricsRepository{db: db}
}

// GetLatestMetrics returns the latest metrics for a tenant.
func (r *MetricsRepository) GetLatestMetrics(ctx context.Context, tenantID string) (*domain.KeycloakMetrics, error) {
	var dbMetrics database.KeycloakMetrics

	result := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("time DESC").
		First(&dbMetrics)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get latest metrics: %w", result.Error)
	}

	return convertDBMetricsToDomain(&dbMetrics), nil
}

// GetLatestMetricsByRealm returns the latest metrics for a specific realm.
func (r *MetricsRepository) GetLatestMetricsByRealm(ctx context.Context, tenantID, realmName string) (*domain.KeycloakMetrics, error) {
	var dbMetrics database.KeycloakMetrics

	result := r.db.WithContext(ctx).
		Where("tenant_id = ? AND realm_name = ?", tenantID, realmName).
		Order("time DESC").
		First(&dbMetrics)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get latest metrics by realm: %w", result.Error)
	}

	return convertDBMetricsToDomain(&dbMetrics), nil
}

// GetMetricsHistory returns metrics history for a tenant.
func (r *MetricsRepository) GetMetricsHistory(ctx context.Context, tenantID string, from, to time.Time) ([]*domain.KeycloakMetrics, error) {
	var dbMetrics []*database.KeycloakMetrics

	result := r.db.WithContext(ctx).
		Where("tenant_id = ? AND time >= ? AND time <= ?", tenantID, from, to).
		Order("time ASC").
		Find(&dbMetrics)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get metrics history: %w", result.Error)
	}

	return convertDBMetricsListToDomain(dbMetrics), nil
}

// GetMetricsHistoryByRealm returns metrics history for a specific realm.
func (r *MetricsRepository) GetMetricsHistoryByRealm(ctx context.Context, tenantID, realmName string, from, to time.Time) ([]*domain.KeycloakMetrics, error) {
	var dbMetrics []*database.KeycloakMetrics

	result := r.db.WithContext(ctx).
		Where("tenant_id = ? AND realm_name = ? AND time >= ? AND time <= ?", tenantID, realmName, from, to).
		Order("time ASC").
		Find(&dbMetrics)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get metrics history by realm: %w", result.Error)
	}

	return convertDBMetricsListToDomain(dbMetrics), nil
}

// SaveMetrics saves metrics.
func (r *MetricsRepository) SaveMetrics(ctx context.Context, metrics *domain.KeycloakMetrics) error {
	dbMetrics := convertDomainMetricsToDB(metrics)

	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "time"}, {Name: "realm_name"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"total_users", "enabled_users", "disabled_users",
			"active_sessions", "offline_sessions", "total_clients",
			"login_events", "logout_events", "failed_login_events", "register_events",
		}),
	}).Create(dbMetrics)

	if result.Error != nil {
		return fmt.Errorf("failed to save metrics: %w", result.Error)
	}

	return nil
}

// convertDBMetricsToDomain converts a database metrics to domain metrics.
func convertDBMetricsToDomain(m *database.KeycloakMetrics) *domain.KeycloakMetrics {
	return &domain.KeycloakMetrics{
		TenantID:          m.TenantID,
		Time:              m.Time,
		RealmName:         m.RealmName,
		TotalUsers:        m.TotalUsers,
		EnabledUsers:      m.EnabledUsers,
		DisabledUsers:     m.DisabledUsers,
		ActiveSessions:    m.ActiveSessions,
		OfflineSessions:   m.OfflineSessions,
		TotalClients:      m.TotalClients,
		LoginEvents:       m.LoginEvents,
		LogoutEvents:      m.LogoutEvents,
		FailedLoginEvents: m.FailedLoginEvents,
		RegisterEvents:    m.RegisterEvents,
	}
}

// convertDBMetricsListToDomain converts a list of database metrics.
func convertDBMetricsListToDomain(dbMetrics []*database.KeycloakMetrics) []*domain.KeycloakMetrics {
	metrics := make([]*domain.KeycloakMetrics, len(dbMetrics))
	for i, m := range dbMetrics {
		metrics[i] = convertDBMetricsToDomain(m)
	}
	return metrics
}

// convertDomainMetricsToDB converts domain metrics to database metrics.
func convertDomainMetricsToDB(m *domain.KeycloakMetrics) *database.KeycloakMetrics {
	return &database.KeycloakMetrics{
		TenantID:          m.TenantID,
		Time:              m.Time,
		RealmName:         m.RealmName,
		TotalUsers:        m.TotalUsers,
		EnabledUsers:      m.EnabledUsers,
		DisabledUsers:     m.DisabledUsers,
		ActiveSessions:    m.ActiveSessions,
		OfflineSessions:   m.OfflineSessions,
		TotalClients:      m.TotalClients,
		LoginEvents:       m.LoginEvents,
		LogoutEvents:      m.LogoutEvents,
		FailedLoginEvents: m.FailedLoginEvents,
		RegisterEvents:    m.RegisterEvents,
	}
}

// Ensure MetricsRepository implements keycloak.MetricsRepository
var _ keycloak.MetricsRepository = (*MetricsRepository)(nil)
