package operator

import (
	"context"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// Repository defines the interface for operator metrics persistence.
type Repository interface {
	// RecordAction records a new action taken by an operator.
	RecordAction(ctx context.Context, action *domain.OperatorAction) error

	// GetMetricsSummary retrieves aggregated metrics for a specific operator.
	GetMetricsSummary(ctx context.Context, tenantID, operatorEmail string, startDate, endDate time.Time) (*domain.OperatorMetricsSummary, error)

	// GetAllOperatorsSummary retrieves metrics summaries for all operators.
	GetAllOperatorsSummary(ctx context.Context, tenantID string, startDate, endDate time.Time) ([]*domain.OperatorMetricsSummary, error)

	// GetActions retrieves individual operator actions.
	GetActions(ctx context.Context, tenantID, operatorEmail string, startDate, endDate time.Time, limit int) ([]*domain.OperatorAction, error)

	// GetLastActionForAlert retrieves the most recent action for a specific alert.
	GetLastActionForAlert(ctx context.Context, tenantID string, alertID uint) (*domain.OperatorAction, error)
}
