package operator

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Repository defines the interface for operator metrics persistence.
type Repository interface {
	// RecordAction records a new action taken by an operator.
	RecordAction(ctx context.Context, action *domain.OperatorAction) error

	// GetMetricsSummary retrieves aggregated metrics for a specific operator,
	// optionally restricted to a set of realms.
	GetMetricsSummary(ctx context.Context, tenantID, operatorEmail string, startDate, endDate time.Time, realmNames []string) (*domain.OperatorMetricsSummary, error)

	// GetAllOperatorsSummary retrieves metrics summaries for all operators,
	// optionally restricted to a set of realms.
	GetAllOperatorsSummary(ctx context.Context, tenantID string, startDate, endDate time.Time, realmNames []string) ([]*domain.OperatorMetricsSummary, error)

	// GetActions retrieves individual operator actions, optionally restricted
	// to a set of realms.
	GetActions(ctx context.Context, tenantID, operatorEmail string, startDate, endDate time.Time, limit int, realmNames []string) ([]*domain.OperatorAction, error)

	// GetLastActionForAlert retrieves the most recent action for a specific alert.
	GetLastActionForAlert(ctx context.Context, tenantID string, alertID uint) (*domain.OperatorAction, error)
}
