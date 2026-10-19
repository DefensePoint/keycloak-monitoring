// Package postgres provides the PostgreSQL implementation of the operator repository.
package postgres

import (
	"context"
	"fmt"
	"math"
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
	if action.OccurrenceStartedAt == nil {
		// The occurrence the action belongs to, read when it is recorded: the
		// alert's occurrence start moves when the alert comes back after being
		// resolved. Left unset if the alert cannot be read; the row then keeps
		// the old figures rather than failing the recording.
		var started []time.Time
		if err := r.db.DB().WithContext(ctx).Raw(
			`SELECT COALESCE(occurrence_started_at, first_detected) FROM configuration_alerts WHERE id = ?`,
			action.AlertID).Scan(&started).Error; err == nil && len(started) == 1 {
			action.OccurrenceStartedAt = &started[0]
		}
	}
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
		ActionType string
		Count      int64
	}

	var actionMetrics []ActionMetrics
	err := actions().
		Select("action_type, COUNT(*) as count").
		Group("action_type").
		Find(&actionMetrics).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get operator action metrics: %w", err)
	}

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
	}

	t, err := r.timings(ctx, tenantID, operatorEmail, startDate, endDate, realmNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get operator time metrics: %w", err)
	}
	summary.TotalWorkTimeSeconds = int(math.Round(t.WorkSeconds))
	summary.AvgResponseTime = t.AvgResponse
	summary.MedianResponseTime = t.MedianResponse
	summary.MinResponseTime = int(math.Round(t.MinResponse))
	summary.MaxResponseTime = int(math.Round(t.MaxResponse))
	summary.AvgResolutionTime = t.AvgResolution
	summary.MedianResolutionTime = t.MedianResolution
	summary.AvgTimeToAcknowledge = t.AvgTimeToAcknowledge
	summary.AvgTimeToResolve = t.AvgTimeToResolve
	summary.AvgTimeToIgnore = t.AvgTimeToIgnore
	summary.AvgAcknowledgeToResolveTime = t.AvgAcknowledgeToResolve
	summary.AvgAcknowledgeToIgnoreTime = t.AvgAcknowledgeToIgnore
	summary.AvgActiveToAcknowledgeTime = t.AvgActiveToAcknowledge
	summary.AvgActiveToResolveTime = t.AvgActiveToResolve
	summary.AvgActiveToIgnoreTime = t.AvgActiveToIgnore

	// Get severity breakdown. Each alert counts once, like the total, under the
	// severity it had at the latest action: a re-detection can change an
	// alert's severity between two actions on it.
	type SeverityCount struct {
		Severity string
		Count    int64
	}
	var severityCounts []SeverityCount
	latest := actions().
		Select("DISTINCT ON (alert_id) alert_id, alert_severity").
		Order("alert_id, action_time DESC, id DESC")
	err = r.db.DB().WithContext(ctx).
		Table("(?) AS latest", latest).
		Select("alert_severity as severity, COUNT(*) as count").
		Group("alert_severity").
		Find(&severityCounts).Error

	if err == nil {
		for _, sc := range severityCounts {
			switch domain.AlertSeverity(sc.Severity) {
			case domain.AlertSeverityCritical:
				summary.CriticalAlertsHandled = int(sc.Count)
			case domain.AlertSeverityError:
				summary.ErrorAlertsHandled = int(sc.Count)
			case domain.AlertSeverityWarning:
				summary.WarningAlertsHandled = int(sc.Count)
			case domain.AlertSeverityInfo:
				summary.InfoAlertsHandled = int(sc.Count)
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
		OccurrenceStartedAt:           db.OccurrenceStartedAt,
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
		OccurrenceStartedAt:           action.OccurrenceStartedAt,
		Comment:                       action.Comment,
		Metadata:                      action.Metadata,
		CreatedAt:                     action.CreatedAt,
		UpdatedAt:                     action.UpdatedAt,
	}
}

// Ensure Repository implements operator.Repository
var _ operator.Repository = (*Repository)(nil)

// operatorTimings are an operator's time metrics over a period, in seconds.
type operatorTimings struct {
	WorkSeconds             float64
	AvgResponse             float64
	MedianResponse          float64
	MinResponse             float64
	MaxResponse             float64
	AvgResolution           float64
	MedianResolution        float64
	AvgTimeToAcknowledge    float64
	AvgTimeToResolve        float64
	AvgTimeToIgnore         float64
	AvgAcknowledgeToResolve float64
	AvgAcknowledgeToIgnore  float64
	AvgActiveToAcknowledge  float64
	AvgActiveToResolve      float64
	AvgActiveToIgnore       float64
}

// timingsSQL measures each action against the occurrence of the alert it
// belongs to. For actions that carry an occurrence:
//   - response is the operator's first acknowledge, resolve or ignore of the
//     occurrence, from the occurrence's start; later actions are not responses
//   - resolution is a resolve, from the occurrence's start
//   - work is a resolve or ignore, from the operator's first action on the
//     occurrence: how long they had it in hand, counted once
//   - a previous action from an earlier occurrence is not a previous action
//
// first_own also reads the operator's actions from before the period, so
// handling that began before the period counts in full where it ends. Only
// occurrences that began before the period can have such actions, so only
// those are looked up there. Actions recorded before occurrences
// were stored (occ IS NULL) keep the figures they always produced.
const timingsSQL = `
WITH period AS (
	SELECT alert_id, action_type, action_time, previous_status, previous_action_time,
		time_from_detection_seconds, time_from_previous_action_seconds,
		response_time_seconds, resolution_time_seconds,
		occurrence_started_at AS occ
	FROM "operator_actions"
	WHERE tenant_id = ? AND operator_email = ?%s AND action_time BETWEEN ? AND ?
), firsts AS (
	SELECT alert_id, occ, MIN(action_time) AS first_own
	FROM (
		SELECT alert_id, occ, action_time FROM period
		WHERE occ IS NOT NULL AND action_type IN ('acknowledged', 'resolved', 'ignored')
		UNION ALL
		SELECT alert_id, occurrence_started_at, action_time FROM "operator_actions"
		WHERE tenant_id = ? AND operator_email = ? AND action_time < ?
			AND action_type IN ('acknowledged', 'resolved', 'ignored')
			AND (alert_id, occurrence_started_at) IN (SELECT alert_id, occ FROM period WHERE occ < ?)
	) acted
	GROUP BY alert_id, occ
), own AS (
	SELECT p.*, f.first_own
	FROM period p LEFT JOIN firsts f ON f.alert_id = p.alert_id AND f.occ = p.occ
), m AS (
	SELECT action_type, action_time,
		CASE WHEN occ IS NULL THEN response_time_seconds::float8
			WHEN action_type IN ('acknowledged', 'resolved', 'ignored') AND action_time = first_own
				THEN GREATEST(EXTRACT(EPOCH FROM action_time - occ), 0) END AS response,
		CASE WHEN occ IS NULL THEN NULLIF(resolution_time_seconds, 0)::float8
			WHEN action_type = 'resolved' THEN GREATEST(EXTRACT(EPOCH FROM action_time - occ), 0) END AS resolution,
		CASE WHEN occ IS NULL
				THEN (CASE WHEN resolution_time_seconds > 0 THEN resolution_time_seconds ELSE response_time_seconds END)::float8
			WHEN action_type IN ('resolved', 'ignored') THEN GREATEST(EXTRACT(EPOCH FROM action_time - first_own), 0)
			ELSE 0 END AS work,
		CASE WHEN occ IS NULL THEN time_from_detection_seconds::float8
			ELSE GREATEST(EXTRACT(EPOCH FROM action_time - occ), 0) END AS since_start,
		CASE WHEN occ IS NULL THEN previous_status
			WHEN previous_action_time IS NULL OR previous_action_time < occ THEN 'active'
			ELSE previous_status END AS prev_status,
		CASE WHEN occ IS NULL THEN time_from_previous_action_seconds::float8
			WHEN previous_action_time IS NULL OR previous_action_time < occ
				THEN GREATEST(EXTRACT(EPOCH FROM action_time - occ), 0)
			ELSE time_from_previous_action_seconds::float8 END AS since_previous
	FROM own
)
SELECT
	COALESCE(SUM(work), 0) AS work_seconds,
	COALESCE(AVG(response), 0) AS avg_response,
	COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY response), 0) AS median_response,
	COALESCE(MIN(response), 0) AS min_response,
	COALESCE(MAX(response), 0) AS max_response,
	COALESCE(AVG(resolution), 0) AS avg_resolution,
	COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY resolution), 0) AS median_resolution,
	COALESCE(AVG(since_start) FILTER (WHERE action_type = 'acknowledged'), 0) AS avg_time_to_acknowledge,
	COALESCE(AVG(since_start) FILTER (WHERE action_type = 'resolved'), 0) AS avg_time_to_resolve,
	COALESCE(AVG(since_start) FILTER (WHERE action_type = 'ignored'), 0) AS avg_time_to_ignore,
	COALESCE(AVG(since_previous) FILTER (WHERE action_type = 'resolved' AND prev_status = 'acknowledged'), 0) AS avg_acknowledge_to_resolve,
	COALESCE(AVG(since_previous) FILTER (WHERE action_type = 'ignored' AND prev_status = 'acknowledged'), 0) AS avg_acknowledge_to_ignore,
	COALESCE(AVG(since_previous) FILTER (WHERE action_type = 'acknowledged' AND prev_status = 'active'), 0) AS avg_active_to_acknowledge,
	COALESCE(AVG(since_previous) FILTER (WHERE action_type = 'resolved' AND prev_status = 'active'), 0) AS avg_active_to_resolve,
	COALESCE(AVG(since_previous) FILTER (WHERE action_type = 'ignored' AND prev_status = 'active'), 0) AS avg_active_to_ignore
FROM m`

// timings reads an operator's time metrics for the period.
func (r *Repository) timings(ctx context.Context, tenantID, operatorEmail string, startDate, endDate time.Time, realmNames []string) (*operatorTimings, error) {
	realmClause := ""
	args := []any{tenantID, operatorEmail}
	if len(realmNames) > 0 {
		realmClause = " AND realm_name IN ?"
		args = append(args, realmNames)
	}
	args = append(args, startDate, endDate, tenantID, operatorEmail, startDate, startDate)

	var t operatorTimings
	if err := r.db.DB().WithContext(ctx).Raw(fmt.Sprintf(timingsSQL, realmClause), args...).Scan(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}
