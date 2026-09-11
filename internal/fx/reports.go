package fx

import (
	"context"
	"time"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/alerts"
	"github.com/DefensePoint/keycloak-monitoring/events"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/operator"
	"github.com/DefensePoint/keycloak-monitoring/reports"
)

// ReportsModule provides report domain dependencies.
var ReportsModule = fx.Module("reports",
	fx.Provide(
		provideReportsEventLister,
		provideReportsAlertReader,
		provideReportsOperatorReader,
		provideChromePDFGenerator,
		provideReportsService,
	),
)

// eventListerAdapter adapts events.Repository to reports.EventLister.
type eventListerAdapter struct {
	repo events.Repository
}

func provideReportsEventLister(repo events.Repository) reports.EventLister {
	return &eventListerAdapter{repo: repo}
}

func (a *eventListerAdapter) List(ctx context.Context, opts *reports.EventListOptions) ([]*domain.Event, error) {
	eventsOpts := &events.ListOptions{
		Limit:     opts.Limit,
		Offset:    opts.Offset,
		StartTime: opts.StartTime,
		EndTime:   opts.EndTime,
		TenantID:  opts.TenantID,
		Sources:   opts.Sources,
	}

	// events.Repository already returns []*domain.Event
	return a.repo.List(ctx, eventsOpts)
}

// alertReaderAdapter adapts alerts.Service to reports.AlertReader.
type alertReaderAdapter struct {
	service alerts.Service
}

func provideReportsAlertReader(service alerts.Service) reports.AlertReader {
	return &alertReaderAdapter{service: service}
}

func (a *alertReaderAdapter) ListActiveAlerts(ctx context.Context, tenantID string, opts *reports.AlertListOptions) ([]*domain.Alert, error) {
	// Convert reports.AlertListOptions to alerts.ListOptions
	domainOpts := &alerts.ListOptions{
		Limit:      opts.Limit,
		Offset:     opts.Offset,
		RealmNames: opts.RealmNames,
	}

	// alerts.Alert is an alias to domain.Alert, so no conversion needed
	domainAlerts, _, err := a.service.ListAlerts(ctx, tenantID, domainOpts)
	return domainAlerts, err
}

// operatorReaderAdapter adapts operator.Service to reports.OperatorMetricsReader.
type operatorReaderAdapter struct {
	service operator.Service
}

func provideReportsOperatorReader(service operator.Service) reports.OperatorMetricsReader {
	return &operatorReaderAdapter{service: service}
}

func (a *operatorReaderAdapter) GetAllOperatorsSummary(ctx context.Context, tenantID string, startDate, endDate time.Time, realmNames []string) ([]*domain.OperatorMetricsSummary, error) {
	return a.service.GetAllOperatorsMetrics(ctx, tenantID, startDate, endDate, realmNames)
}

func provideChromePDFGenerator(log *logger.Logger) reports.PDFGenerator {
	return reports.NewChromePDFGenerator(log)
}

func provideReportsService(
	eventLister reports.EventLister,
	alertReader reports.AlertReader,
	operatorReader reports.OperatorMetricsReader,
	realmLister keycloak.Service,
	log *logger.Logger,
	pdfGenerator reports.PDFGenerator,
) reports.Service {
	return reports.NewService(eventLister, alertReader, operatorReader, realmLister, log, pdfGenerator)
}
