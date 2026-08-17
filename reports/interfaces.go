package reports

import (
	"context"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// EventLister defines the minimal interface for listing events
// used by the report Service.
type EventLister interface {
	List(ctx context.Context, opts *EventListOptions) ([]*domain.Event, error)
}

// AlertReader defines the minimal interface for reading alerts
// used by the report Service.
type AlertReader interface {
	ListActiveAlerts(ctx context.Context, tenantID string, opts *AlertListOptions) ([]*domain.Alert, error)
}

// OperatorMetricsReader defines the minimal interface for reading operator metrics
// used by the report Service.
type OperatorMetricsReader interface {
	GetAllOperatorsSummary(ctx context.Context, tenantID string, startDate, endDate time.Time) ([]*domain.OperatorMetricsSummary, error)
}

// EventListOptions contains options for listing events.
type EventListOptions struct {
	Limit     int
	Offset    int
	StartTime *string
	EndTime   *string
}
