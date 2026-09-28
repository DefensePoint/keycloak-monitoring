package reports

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
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
// used by the report Service. realmNames restricts the aggregate to those
// realms, and an empty list applies no filter.
type OperatorMetricsReader interface {
	GetAllOperatorsSummary(ctx context.Context, tenantID string, startDate, endDate time.Time, realmNames []string) ([]*domain.OperatorMetricsSummary, error)
}

// RealmLister lists the realms configured for a tenant. It is used to scope the
// events section of a report to the sources (keycloak:<realm>, amfa:<realm>)
// that belong to the tenant the report is generated for.
type RealmLister interface {
	ListRealms(ctx context.Context, tenantID string) ([]*domain.KeycloakRealmInfo, error)
}

// EventListOptions contains options for listing events.
type EventListOptions struct {
	Limit  int
	Offset int
	// TenantID scopes the query to the report tenant's rows. This is the
	// isolation boundary; Sources only narrows within it.
	TenantID  string
	StartTime *string
	EndTime   *string
	// Sources restricts the query to these exact event sources. The report
	// service populates it with the tenant's own keycloak:<realm> / amfa:<realm>
	// feeds so a report never includes another tenant's events.
	Sources []string
}
