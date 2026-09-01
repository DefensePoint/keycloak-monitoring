package reports

import (
	"context"
	"sort"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
)

// Service defines the interface for report operations.
type Service interface {
	// AggregateData collects all data needed for a report.
	AggregateData(ctx context.Context, req *GenerateRequest) (*Data, error)

	// GeneratePDF generates a PDF report from the aggregated data.
	GeneratePDF(data *Data) ([]byte, error)
}

// service implements the Service interface.
type service struct {
	eventLister    EventLister
	alertReader    AlertReader
	operatorReader OperatorMetricsReader
	realmLister    RealmLister
	logger         *logger.Logger
	pdfGenerator   PDFGenerator
}

// PDFGenerator generates PDFs from report data.
type PDFGenerator interface {
	Generate(data *Data) ([]byte, error)
}

// NewService creates a new report service.
func NewService(
	eventLister EventLister,
	alertReader AlertReader,
	operatorReader OperatorMetricsReader,
	realmLister RealmLister,
	log *logger.Logger,
	pdfGenerator PDFGenerator,
) Service {
	return &service{
		eventLister:    eventLister,
		alertReader:    alertReader,
		operatorReader: operatorReader,
		realmLister:    realmLister,
		logger:         log,
		pdfGenerator:   pdfGenerator,
	}
}

// tenantEventSources returns the exact event sources that belong to a tenant
// (keycloak:<realm> and amfa:<realm> for each of its realms). An empty result
// means the report must include no events (deny-by-default) rather than fall
// back to an unscoped query across every tenant.
func (s *service) tenantEventSources(ctx context.Context, tenantID string) ([]string, error) {
	if s.realmLister == nil || tenantID == "" {
		return nil, nil
	}
	realms, err := s.realmLister.ListRealms(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	sources := make([]string, 0, len(realms)*2)
	for _, realm := range realms {
		if realm == nil || realm.RealmName == "" {
			continue
		}
		sources = append(sources, "keycloak:"+realm.RealmName, "amfa:"+realm.RealmName)
	}
	return sources, nil
}

// AggregateData collects all data needed for a report.
func (s *service) AggregateData(ctx context.Context, req *GenerateRequest) (*Data, error) {
	data := &Data{
		TenantID:    req.TenantID,
		TenantName:  req.TenantName,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		GeneratedAt: time.Now(),
	}

	alertsByType := make(map[string]int)
	alertsByRealm := make(map[string]int)
	eventsByType := make(map[string]int)
	eventsByRealm := make(map[string]int)
	realmAlertsBySeverity := make(map[string]map[string]int)

	// Fetch alerts
	if s.alertReader != nil {
		alertList, err := s.alertReader.ListActiveAlerts(ctx, req.TenantID, &AlertListOptions{
			Limit:  1000,
			Offset: 0,
		})
		if err != nil {
			s.logger.Error("Failed to fetch alerts", logger.Err(err))
		} else {
			data.TotalAlerts = len(alertList)
			for i, alert := range alertList {
				// Count by severity
				switch alert.Severity {
				case domain.AlertSeverityCritical:
					data.CriticalAlerts++
				case domain.AlertSeverityError:
					data.HighAlerts++
				case domain.AlertSeverityWarning:
					data.MediumAlerts++
				case domain.AlertSeverityInfo:
					data.InfoAlerts++
				}

				// Count open alerts
				if alert.Status == domain.AlertStatusActive {
					data.OpenAlerts++
				}

				// Count by type and realm
				alertsByType[string(alert.Type)]++
				if alert.RealmName != "" {
					alertsByRealm[alert.RealmName]++

					// Track severity by realm
					if realmAlertsBySeverity[alert.RealmName] == nil {
						realmAlertsBySeverity[alert.RealmName] = make(map[string]int)
					}
					realmAlertsBySeverity[alert.RealmName][string(alert.Severity)]++
				}

				// Add to detailed list (limit to first 20)
				if i < 20 {
					data.AlertList = append(data.AlertList, AlertDetail{
						ID:          i + 1,
						Title:       alert.Title,
						Severity:    string(alert.Severity),
						Status:      string(alert.Status),
						Description: alert.Description,
						Remediation: alert.Recommendation,
						RealmName:   alert.RealmName,
					})
				}
			}
		}
	}

	// Fetch events, scoped to the report tenant's own sources. If the tenant has
	// no known sources, leave the events section empty rather than run an
	// unscoped query that would include every tenant's events.
	sources, srcErr := s.tenantEventSources(ctx, req.TenantID)
	if srcErr != nil {
		s.logger.Error("Failed to resolve tenant event sources for report",
			logger.Str("tenant_id", req.TenantID), logger.Err(srcErr))
	}
	if s.eventLister != nil && srcErr == nil && len(sources) > 0 {
		startStr := req.StartDate.Format(time.RFC3339)
		endStr := req.EndDate.Format(time.RFC3339)

		events, err := s.eventLister.List(ctx, &EventListOptions{
			Limit:     10000,
			Offset:    0,
			StartTime: &startStr,
			EndTime:   &endStr,
			Sources:   sources,
		})
		if err != nil {
			s.logger.Error("Failed to fetch events", logger.Err(err))
		} else {
			data.TotalEvents = len(events)
			for _, event := range events {
				eventsByType[event.Type]++
				if event.Source != "" {
					eventsByRealm[event.Source]++
				}
			}
		}
	}

	// Process alert types
	for typ, count := range alertsByType {
		data.AlertsByType = append(data.AlertsByType, TypeStats{Type: typ, Count: count})
	}
	sortTypeStatsByCount(data.AlertsByType)

	// Process event types
	for typ, count := range eventsByType {
		data.TopEventTypes = append(data.TopEventTypes, TypeStats{Type: typ, Count: count})
	}
	sortTypeStatsByCount(data.TopEventTypes)

	// Process realms with severity breakdown
	realmMap := make(map[string]*RealmStats)
	for realm, count := range alertsByRealm {
		realmMap[realm] = &RealmStats{
			Name:       realm,
			AlertCount: count,
		}
		if severityMap, ok := realmAlertsBySeverity[realm]; ok {
			realmMap[realm].CriticalCount = severityMap["critical"]
			realmMap[realm].ErrorCount = severityMap["error"]
			realmMap[realm].WarningCount = severityMap["warning"]
			realmMap[realm].InfoCount = severityMap["info"]
		}
	}
	for realm, count := range eventsByRealm {
		if realmMap[realm] == nil {
			realmMap[realm] = &RealmStats{Name: realm}
		}
		realmMap[realm].EventCount = count
	}
	for _, stats := range realmMap {
		data.TopAlertedRealms = append(data.TopAlertedRealms, *stats)
	}
	sortRealmStatsByAlertCount(data.TopAlertedRealms)

	data.TotalRealms = len(realmMap)

	// Fetch operator metrics if reader is available
	if s.operatorReader != nil {
		operatorMetrics, err := s.operatorReader.GetAllOperatorsSummary(ctx, req.TenantID, req.StartDate, req.EndDate)
		if err != nil {
			s.logger.Warn("Failed to fetch operator metrics for report", logger.Err(err))
		} else {
			data.OperatorMetrics = operatorMetrics
			data.TotalOperators = len(operatorMetrics)

			// Calculate totals
			var totalHours float64
			var totalResponseTime float64
			var countWithResponse int

			for _, op := range operatorMetrics {
				totalHours += op.TotalWorkTimeHours
				if op.AvgResponseTime > 0 {
					totalResponseTime += op.AvgResponseTime
					countWithResponse++
				}
			}

			data.TotalOperatorHours = totalHours
			if countWithResponse > 0 {
				// Convert seconds to minutes for better readability
				data.AvgResponseTimeMin = (totalResponseTime / float64(countWithResponse)) / 60.0
			}
		}
	}

	// Calculate risk level
	data.RiskLevel, data.RiskLevelText = calculateRiskLevel(data)

	return data, nil
}

// GeneratePDF generates a PDF report from the aggregated data.
func (s *service) GeneratePDF(data *Data) ([]byte, error) {
	if s.pdfGenerator == nil {
		return nil, nil
	}
	return s.pdfGenerator.Generate(data)
}

// calculateRiskLevel determines the risk level based on alert counts.
func calculateRiskLevel(data *Data) (level, text string) {
	if data.CriticalAlerts > 0 {
		return "critical", "CRITICAL"
	} else if data.HighAlerts > 0 {
		return "high", "HIGH"
	} else if data.MediumAlerts > 0 {
		return "medium", "MEDIUM"
	} else if data.LowAlerts > 0 {
		return "low", "LOW"
	}
	return "info", "INFORMATIONAL"
}

// sortTypeStatsByCount sorts TypeStats by count in descending order.
func sortTypeStatsByCount(stats []TypeStats) {
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})
}

// sortRealmStatsByAlertCount sorts RealmStats by alert count in descending order.
func sortRealmStatsByAlertCount(stats []RealmStats) {
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].AlertCount > stats[j].AlertCount
	})
}
