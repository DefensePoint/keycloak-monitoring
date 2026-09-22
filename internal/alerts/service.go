package alerts

import (
	"context"
	"fmt"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
)

// Service defines the interface for alert operations.
type Service interface {
	// GetAlert retrieves an alert by its alert_id.
	GetAlert(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)

	// GetAlertGlobal retrieves an alert by its alert_id across all tenants.
	GetAlertGlobal(ctx context.Context, alertID string) (*domain.Alert, error)

	// ListAlerts retrieves alerts with optional filtering.
	ListAlerts(ctx context.Context, tenantID string, opts *ListOptions) ([]*domain.Alert, int64, error)

	// ListAlertsByRealm retrieves alerts for a specific realm.
	ListAlertsByRealm(ctx context.Context, tenantID, realmName string, opts *ListOptions) ([]*domain.Alert, error)

	// GetStatistics returns aggregated alert statistics, optionally scoped to a
	// set of realms.
	GetStatistics(ctx context.Context, tenantID string, realmName ...string) (*Statistics, error)

	// UpdateStatus updates the status of an alert.
	UpdateStatus(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) (*domain.Alert, error)

	// ResolveAlert marks an alert as resolved.
	ResolveAlert(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)

	// DeleteAlert removes an alert.
	DeleteAlert(ctx context.Context, tenantID, alertID string) error

	// SaveAlert creates or updates an alert.
	SaveAlert(ctx context.Context, alert *domain.Alert) error
}

// service implements the Service interface.
type service struct {
	repo    Repository
	log     *logger.Logger
	metrics metrics.Recorder
}

// SetMetrics attaches a metrics recorder. Passing nil installs a no-op recorder.
func (s *service) SetMetrics(r metrics.Recorder) {
	if r == nil {
		r = metrics.NopRecorder{}
	}
	s.metrics = r
}

// recorder returns the configured recorder, falling back to a no-op.
func (s *service) recorder() metrics.Recorder {
	if s.metrics == nil {
		return metrics.NopRecorder{}
	}
	return s.metrics
}

// NewService creates a new alerts service.
func NewService(repo Repository, log *logger.Logger) Service {
	return &service{
		repo: repo,
		log:  log,
	}
}

// GetAlert retrieves an alert by its alert_id.
func (s *service) GetAlert(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	return s.repo.GetByAlertID(ctx, tenantID, alertID)
}

// GetAlertGlobal retrieves an alert by its alert_id across all tenants.
func (s *service) GetAlertGlobal(ctx context.Context, alertID string) (*domain.Alert, error) {
	return s.repo.GetByAlertIDGlobal(ctx, alertID)
}

// ListAlerts retrieves alerts with optional filtering.
func (s *service) ListAlerts(ctx context.Context, tenantID string, opts *ListOptions) ([]*domain.Alert, int64, error) {
	if opts == nil {
		opts = &ListOptions{Limit: 100, Offset: 0}
	}

	alerts, err := s.repo.List(ctx, tenantID, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list alerts: %w", err)
	}

	count, err := s.repo.Count(ctx, tenantID, opts)
	if err != nil {
		count = 0 // Non-fatal, continue with count=0
	}

	return alerts, count, nil
}

// ListAlertsByRealm retrieves alerts for a specific realm.
func (s *service) ListAlertsByRealm(ctx context.Context, tenantID, realmName string, opts *ListOptions) ([]*domain.Alert, error) {
	if opts == nil {
		opts = &ListOptions{Limit: 100, Offset: 0}
	}
	return s.repo.ListByRealm(ctx, tenantID, realmName, opts)
}

// GetStatistics returns aggregated alert statistics, optionally scoped to a set
// of realms.
func (s *service) GetStatistics(ctx context.Context, tenantID string, realmName ...string) (*Statistics, error) {
	return s.repo.GetStatistics(ctx, tenantID, realmName...)
}

// UpdateStatus updates the status of an alert.
func (s *service) UpdateStatus(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) (*domain.Alert, error) {
	// Validate status
	if !isValidStatus(status) {
		return nil, fmt.Errorf("invalid status: %s", status)
	}

	if err := s.repo.UpdateStatus(ctx, tenantID, alertID, status, acknowledgedBy); err != nil {
		return nil, fmt.Errorf("failed to update alert status: %w", err)
	}

	// Try to get updated alert
	alert, err := s.repo.GetByAlertID(ctx, tenantID, alertID)
	if err != nil {
		// Fallback for compatibility - UPDATE succeeded but GET failed
		s.log.Warn("Failed to retrieve updated alert after successful update",
			logger.Err(err),
			logger.Str("tenant_id", tenantID),
			logger.Str("alert_id", alertID))

		// Return minimal alert with updated status
		return &domain.Alert{
			AlertID:  alertID,
			TenantID: tenantID,
			Status:   status,
		}, nil
	}

	return alert, nil
}

// ResolveAlert marks an alert as resolved.
func (s *service) ResolveAlert(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	if err := s.repo.Resolve(ctx, tenantID, alertID); err != nil {
		return nil, fmt.Errorf("failed to resolve alert: %w", err)
	}

	return s.repo.GetByAlertID(ctx, tenantID, alertID)
}

// DeleteAlert removes an alert.
func (s *service) DeleteAlert(ctx context.Context, tenantID, alertID string) error {
	return s.repo.Delete(ctx, tenantID, alertID)
}

// SaveAlert creates or updates an alert.
func (s *service) SaveAlert(ctx context.Context, alert *domain.Alert) error {
	if err := s.repo.Save(ctx, alert); err != nil {
		return err
	}
	s.recorder().IncAlert(alert.Severity.String(), alert.CheckType)
	return nil
}

// isValidStatus checks if a status is valid.
func isValidStatus(status domain.AlertStatus) bool {
	return status == domain.AlertStatusActive ||
		status == domain.AlertStatusResolved ||
		status == domain.AlertStatusAcknowled ||
		status == domain.AlertStatusIgnored
}
