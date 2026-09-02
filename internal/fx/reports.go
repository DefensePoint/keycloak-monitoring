package fx

import (
	"context"
	"time"

	"go.uber.org/fx"

	"repos.defensepoint.com/iam/oss-pmt/kmt/alerts"
	"repos.defensepoint.com/iam/oss-pmt/kmt/events"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/keycloak"
	"repos.defensepoint.com/iam/oss-pmt/kmt/operator"
	"repos.defensepoint.com/iam/oss-pmt/kmt/reports"
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
		Limit:  opts.Limit,
		Offset: opts.Offset,
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

func (a *operatorReaderAdapter) GetAllOperatorsSummary(ctx context.Context, tenantID string, startDate, endDate time.Time) ([]*domain.OperatorMetricsSummary, error) {
	return a.service.GetAllOperatorsMetrics(ctx, tenantID, startDate, endDate)
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
