// Package postgres provides the PostgreSQL implementation of the operator repository.
package postgres

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/operator"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// Repository implements operator.Repository using PostgreSQL via GORM.
type Repository struct {
	db operator.DB
}

// NewRepository creates a new PostgreSQL operator repository.
func NewRepository(db operator.DB) operator.Repository {
	return &Repository{db: db}
}

// RecordAction records a new action taken by an operator.
func (r *Repository) RecordAction(ctx context.Context, action *domain.OperatorAction) error {
	dbAction := convertToDBAction(action)
	if err := r.db.DB().WithContext(ctx).Create(dbAction).Error; err != nil {
		return fmt.Errorf("failed to record operator action: %w", err)
	}
	action.ID = dbAction.ID
	return nil
}

// GetMetricsSummary retrieves aggregated metrics for a specific operator,
// optionally restricted to a set of realms.
func (r *Repository) GetMetricsSummary(ctx context.Context, tenantID, operatorEmail string, startDate, endDate time.Time, realmNames []string) (*domain.OperatorMetricsSummary, error) {
	summary := &domain.OperatorMetricsSummary{
		OperatorEmail: operatorEmail,
		TenantID:      tenantID,
		PeriodStart:   startDate,
		PeriodEnd:     endDate,
		AlertsByType:  make(map[string]int),
	}

	// Get operator name
	var user database.User
	if err := r.db.DB().WithContext(ctx).Where("email = ?", operatorEmail).First(&user).Error; err == nil {
		summary.OperatorName = user.Username
	}

	actions := func() *gorm.DB {
		query := r.db.DB().WithContext(ctx).
			Model(&database.OperatorAction{}).
			Where("tenant_id = ? AND operator_email = ? AND action_time BETWEEN ? AND ?", tenantID, operatorEmail, startDate, endDate)
		if len(realmNames) > 0 {
			query = query.Where("realm_name IN ?", realmNames)
		}
		return query
	}

	// Query for action counts
	type ActionMetrics struct {
		ActionType           string
		Count                int64
		TotalWorkTimeSeconds int64
		MinResponseTime      int
		MaxResponseTime      int
	}

	var actionMetrics []ActionMetrics
	err := actions().
		Select(`
			action_type,
			COUNT(*) as count,
			SUM(CASE WHEN resolution_time_seconds > 0 THEN resolution_time_seconds ELSE response_time_seconds END) as total_work_time_seconds,
			MIN(response_time_seconds) as min_response_time,
			MAX(response_time_seconds) as max_response_time
		`).
		Group("action_type").
		Find(&actionMetrics).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get operator action metrics: %w", err)
	}

	// Aggregate metrics
	for _, am := range actionMetrics {
		switch am.ActionType {
		case "acknowledged":
			summary.AlertsAcknowledged = int(am.Count)
		case "resolved":
			summary.AlertsResolved = int(am.Count)
		case "ignored":
			summary.AlertsIgnored = int(am.Count)
		case "commented":
			summary.AlertsCommented = int(am.Count)
		}

		summary.TotalWorkTimeSeconds += int(am.TotalWorkTimeSeconds)
		if am.MinResponseTime < summary.MinResponseTime || summary.MinResponseTime == 0 {
			summary.MinResponseTime = am.MinResponseTime
		}
		if am.MaxResponseTime > summary.MaxResponseTime {
			summary.MaxResponseTime = am.MaxResponseTime
		}
	}

	// Calculate overall average metrics
	var overallMetrics struct {
		AvgResponseTime      float64
		MedianResponseTime   float64
		AvgResolutionTime    float64
		MedianResolutionTime float64
	}
	err = actions().
		Select(`
			AVG(response_time_seconds) as avg_response_time,
			PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY response_time_seconds) as median_response_time,
			AVG(CASE WHEN resolution_time_seconds > 0 THEN resolution_time_seconds END) as avg_resolution_time,
			PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY resolution_time_seconds) FILTER (WHERE resolution_time_seconds > 0) as median_resolution_time
		`).
		Scan(&overallMetrics).Error

	if err == nil {
		summary.AvgResponseTime = overallMetrics.AvgResponseTime
		summary.MedianResponseTime = overallMetrics.MedianResponseTime
		summary.AvgResolutionTime = overallMetrics.AvgResolutionTime
		summary.MedianResolutionTime = overallMetrics.MedianResolutionTime
	}

	// Get severity breakdown
	type SeverityCount struct {
		Severity string
		Count    int64
	}
	var severityCounts []SeverityCount
	err = actions().
		Select("alert_severity as severity, COUNT(*) as count").
		Group("alert_severity").
		Find(&severityCounts).Error

	if err == nil {
		for _, sc := range severityCounts {
			switch sc.Severity {
			case "critical":
				summary.CriticalAlertsHandled = int(sc.Count)
			case "high":
				summary.HighAlertsHandled = int(sc.Count)
			case "medium":
				summary.MediumAlertsHandled = int(sc.Count)
			case "low":
				summary.LowAlertsHandled = int(sc.Count)
			}
		}
	}

	// Get alert type breakdown
	type TypeCount struct {
		Type  string
		Count int64
	}
	var typeCounts []TypeCount
	err = actions().
		Select("alert_type as type, COUNT(*) as count").
		Group("alert_type").
		Find(&typeCounts).Error

	if err == nil {
		for _, tc := range typeCounts {
			summary.AlertsByType[tc.Type] = int(tc.Count)
		}
	}

	// Calculate action-specific time metrics
	var actionTimings struct {
		AvgTimeToAcknowledge float64
		AvgTimeToResolve     float64
		AvgTimeToIgnore      float64
	}
	err = actions().
		Select(`
			AVG(CASE WHEN action_type = 'acknowledged' THEN time_from_detection_seconds END) as avg_time_to_acknowledge,
			AVG(CASE WHEN action_type = 'resolved' THEN time_from_detection_seconds END) as avg_time_to_resolve,
			AVG(CASE WHEN action_type = 'ignored' THEN time_from_detection_seconds END) as avg_time_to_ignore
		`).
		Scan(&actionTimings).Error

	if err == nil {
		summary.AvgTimeToAcknowledge = actionTimings.AvgTimeToAcknowledge
		summary.AvgTimeToResolve = actionTimings.AvgTimeToResolve
		summary.AvgTimeToIgnore = actionTimings.AvgTimeToIgnore
	}

	// Calculate transition time metrics
	var transitionTimings struct {
		AvgAcknowledgeToResolve float64
		AvgAcknowledgeToIgnore  float64
		AvgActiveToAcknowledge  float64
		AvgActiveToResolve      float64
		AvgActiveToIgnore       float64
	}
	err = actions().
		Select(`
			AVG(CASE WHEN action_type = 'resolved' AND previous_status = 'acknowledged' THEN time_from_previous_action_seconds END) as avg_acknowledge_to_resolve,
			AVG(CASE WHEN action_type = 'ignored' AND previous_status = 'acknowledged' THEN time_from_previous_action_seconds END) as avg_acknowledge_to_ignore,
			AVG(CASE WHEN action_type = 'acknowledged' AND previous_status = 'active' THEN time_from_previous_action_seconds END) as avg_active_to_acknowledge,
			AVG(CASE WHEN action_type = 'resolved' AND previous_status = 'active' THEN time_from_previous_action_seconds END) as avg_active_to_resolve,
			AVG(CASE WHEN action_type = 'ignored' AND previous_status = 'active' THEN time_from_previous_action_seconds END) as avg_active_to_ignore
		`).
		Scan(&transitionTimings).Error

	if err == nil {
		summary.AvgAcknowledgeToResolveTime = transitionTimings.AvgAcknowledgeToResolve
		summary.AvgAcknowledgeToIgnoreTime = transitionTimings.AvgAcknowledgeToIgnore
		summary.AvgActiveToAcknowledgeTime = transitionTimings.AvgActiveToAcknowledge
		summary.AvgActiveToResolveTime = transitionTimings.AvgActiveToResolve
		summary.AvgActiveToIgnoreTime = transitionTimings.AvgActiveToIgnore
	}

	// Calculate total alerts handled
	var totalAlerts int64
	err = actions().
		Distinct("alert_id").
		Count(&totalAlerts).Error

	if err == nil {
		summary.TotalAlertsHandled = int(totalAlerts)
	}

	summary.TotalWorkTimeHours = float64(summary.TotalWorkTimeSeconds) / 3600.0

	return summary, nil
}

// GetAllOperatorsSummary retrieves metrics summaries for all operators,
// optionally restricted to a set of realms.
func (r *Repository) GetAllOperatorsSummary(ctx context.Context, tenantID string, startDate, endDate time.Time, realmNames []string) ([]*domain.OperatorMetricsSummary, error) {
	query := r.db.DB().WithContext(ctx).
		Model(&database.OperatorAction{}).
		Where("tenant_id = ? AND action_time BETWEEN ? AND ?", tenantID, startDate, endDate)
	if len(realmNames) > 0 {
		query = query.Where("realm_name IN ?", realmNames)
	}

	var operatorEmails []string
	err := query.
		Distinct("operator_email").
		Pluck("operator_email", &operatorEmails).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get operator list: %w", err)
	}

	summaries := make([]*domain.OperatorMetricsSummary, 0, len(operatorEmails))
	for _, email := range operatorEmails {
		summary, err := r.GetMetricsSummary(ctx, tenantID, email, startDate, endDate, realmNames)
		if err != nil {
			continue
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// GetActions retrieves individual operator actions, optionally restricted to a
// set of realms.
func (r *Repository) GetActions(ctx context.Context, tenantID, operatorEmail string, startDate, endDate time.Time, limit int, realmNames []string) ([]*domain.OperatorAction, error) {
	var dbActions []database.OperatorAction

	query := r.db.DB().WithContext(ctx).
		Preload("Alert").
		Preload("Operator").
		Where("tenant_id = ? AND action_time BETWEEN ? AND ?", tenantID, startDate, endDate)

	if operatorEmail != "" {
		query = query.Where("operator_email = ?", operatorEmail)
	}

	if len(realmNames) > 0 {
		query = query.Where("realm_name IN ?", realmNames)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Order("action_time DESC").Find(&dbActions).Error; err != nil {
		return nil, fmt.Errorf("failed to get operator actions: %w", err)
	}

	return convertActionList(dbActions), nil
}

// GetLastActionForAlert retrieves the most recent action for a specific alert.
func (r *Repository) GetLastActionForAlert(ctx context.Context, tenantID string, alertID uint) (*domain.OperatorAction, error) {
	var dbAction database.OperatorAction

	err := r.db.DB().WithContext(ctx).
		Where("tenant_id = ? AND alert_id = ?", tenantID, alertID).
		Order("action_time DESC").
		First(&dbAction).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get last action for alert: %w", err)
	}

	return convertAction(&dbAction), nil
}

// convertAction converts a database action to a domain action.
func convertAction(db *database.OperatorAction) *domain.OperatorAction {
	if db == nil {
		return nil
	}
	return &domain.OperatorAction{
		ID:                            db.ID,
		TenantID:                      db.TenantID,
		AlertID:                       db.AlertID,
		OperatorID:                    db.OperatorID,
		OperatorEmail:                 db.OperatorEmail,
		ActionType:                    db.ActionType,
		ActionTime:                    db.ActionTime,
		PreviousStatus:                db.PreviousStatus,
		PreviousActionTime:            db.PreviousActionTime,
		AlertSeverity:                 db.AlertSeverity,
		AlertType:                     db.AlertType,
		RealmName:                     db.RealmName,
		TimeFromDetectionSeconds:      db.TimeFromDetectionSeconds,
		TimeFromPreviousActionSeconds: db.TimeFromPreviousActionSeconds,
		ResponseTimeSeconds:           db.ResponseTimeSeconds,
		ResolutionTimeSeconds:         db.ResolutionTimeSeconds,
		Comment:                       db.Comment,
		Metadata:                      db.Metadata,
		CreatedAt:                     db.CreatedAt,
		UpdatedAt:                     db.UpdatedAt,
	}
}

// convertActionList converts a list of database actions to domain actions.
func convertActionList(dbActions []database.OperatorAction) []*domain.OperatorAction {
	result := make([]*domain.OperatorAction, len(dbActions))
	for i := range dbActions {
		result[i] = convertAction(&dbActions[i])
	}
	return result
}

// convertToDBAction converts a domain action to a database action.
func convertToDBAction(action *domain.OperatorAction) *database.OperatorAction {
	if action == nil {
		return nil
	}
	return &database.OperatorAction{
		ID:                            action.ID,
		TenantID:                      action.TenantID,
		AlertID:                       action.AlertID,
		OperatorID:                    action.OperatorID,
		OperatorEmail:                 action.OperatorEmail,
		ActionType:                    action.ActionType,
		ActionTime:                    action.ActionTime,
		PreviousStatus:                action.PreviousStatus,
		PreviousActionTime:            action.PreviousActionTime,
		AlertSeverity:                 action.AlertSeverity,
		AlertType:                     action.AlertType,
		RealmName:                     action.RealmName,
		TimeFromDetectionSeconds:      action.TimeFromDetectionSeconds,
		TimeFromPreviousActionSeconds: action.TimeFromPreviousActionSeconds,
		ResponseTimeSeconds:           action.ResponseTimeSeconds,
		ResolutionTimeSeconds:         action.ResolutionTimeSeconds,
		Comment:                       action.Comment,
		Metadata:                      action.Metadata,
		CreatedAt:                     action.CreatedAt,
		UpdatedAt:                     action.UpdatedAt,
	}
}

// Ensure Repository implements operator.Repository
var _ operator.Repository = (*Repository)(nil)
