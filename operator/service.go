package operator

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Service defines the interface for operator metrics operations.
type Service interface {
	// RecordAction records a new action taken by an operator.
	RecordAction(ctx context.Context, action *domain.OperatorAction) error

	// GetMetrics retrieves metrics for a specific operator, optionally
	// restricted to a set of realms.
	GetMetrics(ctx context.Context, tenantID, operatorEmail string, startDate, endDate time.Time, realmNames []string) (*domain.OperatorMetricsSummary, error)

	// GetAllOperatorsMetrics retrieves metrics for all operators, optionally
	// restricted to a set of realms.
	GetAllOperatorsMetrics(ctx context.Context, tenantID string, startDate, endDate time.Time, realmNames []string) ([]*domain.OperatorMetricsSummary, error)

	// GetActions retrieves individual operator actions, optionally restricted
	// to a set of realms.
	GetActions(ctx context.Context, tenantID, operatorEmail string, startDate, endDate time.Time, limit int, realmNames []string) ([]*domain.OperatorAction, error)

	// GetLastActionForAlert retrieves the most recent action for a specific alert.
	GetLastActionForAlert(ctx context.Context, tenantID string, alertID uint) (*domain.OperatorAction, error)
}

// service implements the Service interface.
type service struct {
	repo Repository
}

// NewService creates a new operator metrics service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

// RecordAction records a new action taken by an operator.
func (s *service) RecordAction(ctx context.Context, action *domain.OperatorAction) error {
	return s.repo.RecordAction(ctx, action)
}

// GetMetrics retrieves metrics for a specific operator, optionally restricted
// to a set of realms.
func (s *service) GetMetrics(ctx context.Context, tenantID, operatorEmail string, startDate, endDate time.Time, realmNames []string) (*domain.OperatorMetricsSummary, error) {
	return s.repo.GetMetricsSummary(ctx, tenantID, operatorEmail, startDate, endDate, realmNames)
}

// GetAllOperatorsMetrics retrieves metrics for all operators, optionally
// restricted to a set of realms.
func (s *service) GetAllOperatorsMetrics(ctx context.Context, tenantID string, startDate, endDate time.Time, realmNames []string) ([]*domain.OperatorMetricsSummary, error) {
	return s.repo.GetAllOperatorsSummary(ctx, tenantID, startDate, endDate, realmNames)
}

// GetActions retrieves individual operator actions, optionally restricted to a
// set of realms.
func (s *service) GetActions(ctx context.Context, tenantID, operatorEmail string, startDate, endDate time.Time, limit int, realmNames []string) ([]*domain.OperatorAction, error) {
	return s.repo.GetActions(ctx, tenantID, operatorEmail, startDate, endDate, limit, realmNames)
}

// GetLastActionForAlert retrieves the most recent action for a specific alert.
func (s *service) GetLastActionForAlert(ctx context.Context, tenantID string, alertID uint) (*domain.OperatorAction, error) {
	return s.repo.GetLastActionForAlert(ctx, tenantID, alertID)
}
