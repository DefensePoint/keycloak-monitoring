// Package postgres provides the PostgreSQL implementation of the alerts repository.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// Repository implements alerts.Repository using PostgreSQL via GORM.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new PostgreSQL alerts repository.
func NewRepository(db *gorm.DB) alerts.Repository {
	return &Repository{db: db}
}

// GetByID retrieves an alert by its database ID.
func (r *Repository) GetByID(ctx context.Context, id uint) (*domain.Alert, error) {
	var dbAlert database.ConfigurationAlert
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&dbAlert).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("alert with id %d not found", id)
		}
		return nil, fmt.Errorf("failed to get alert: %w", err)
	}
	return convertAlert(&dbAlert), nil
}

// GetByAlertID retrieves an alert by its unique alert_id. A missing alert is
// reported as a wrapped alerts.ErrNotFound.
func (r *Repository) GetByAlertID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	var dbAlert database.ConfigurationAlert
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND alert_id = ?", tenantID, alertID).
		First(&dbAlert).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("%w: %s", alerts.ErrNotFound, alertID)
		}
		return nil, fmt.Errorf("failed to get alert: %w", err)
	}
	return convertAlert(&dbAlert), nil
}

// GetByAlertIDGlobal retrieves an alert by its alert_id across all tenants.
func (r *Repository) GetByAlertIDGlobal(ctx context.Context, alertID string) (*domain.Alert, error) {
	var dbAlert database.ConfigurationAlert
	if err := r.db.WithContext(ctx).
		Where("alert_id = ?", alertID).
		First(&dbAlert).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("alert not found: %s", alertID)
		}
		return nil, fmt.Errorf("failed to get alert: %w", err)
	}
	return convertAlert(&dbAlert), nil
}

// List retrieves alerts with optional filtering.
func (r *Repository) List(ctx context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, error) {
	if opts == nil {
		opts = &alerts.ListOptions{Limit: 100, Offset: 0}
	}

	// The id tiebreaker keeps pagination stable across rows sharing the same
	// first_detected timestamp.
	var dbAlerts []*database.ConfigurationAlert
	query := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("first_detected DESC, id DESC")

	query = applyFilters(query, opts)

	if err := query.Limit(opts.Limit).Offset(opts.Offset).Find(&dbAlerts).Error; err != nil {
		return nil, fmt.Errorf("failed to list alerts: %w", err)
	}

	return convertAlertList(dbAlerts), nil
}

// ListByRealm retrieves alerts for a specific realm.
func (r *Repository) ListByRealm(ctx context.Context, tenantID, realmName string, opts *alerts.ListOptions) ([]*domain.Alert, error) {
	if opts == nil {
		opts = &alerts.ListOptions{Limit: 100, Offset: 0}
	}
	opts.RealmName = realmName
	return r.List(ctx, tenantID, opts)
}

// ListOpenByCheck returns the tenant's open alerts, active or acknowledged,
// raised by checkType. Unpaged: a check's open alerts are bounded by the
// realms and keys it evaluates.
// ListUnresolved returns every alert of the tenant that is not resolved.
func (r *Repository) ListUnresolved(ctx context.Context, tenantID string) ([]*domain.Alert, error) {
	var dbAlerts []*database.ConfigurationAlert
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND status <> ?", tenantID, string(domain.AlertStatusResolved)).
		Order("id ASC").
		Find(&dbAlerts).Error; err != nil {
		return nil, fmt.Errorf("failed to list unresolved alerts: %w", err)
	}
	return convertAlertList(dbAlerts), nil
}

func (r *Repository) ListOpenByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error) {
	var dbAlerts []*database.ConfigurationAlert
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND check_type = ? AND status IN ?", tenantID, checkType,
			[]string{string(domain.AlertStatusActive), string(domain.AlertStatusAcknowled)}).
		Order("id ASC").
		Find(&dbAlerts).Error; err != nil {
		return nil, fmt.Errorf("failed to list open alerts for check %s: %w", checkType, err)
	}
	return convertAlertList(dbAlerts), nil
}

// LatestFirstDetected implements alerts.Repository. Unscoped, so a deleted
// alert still marks its event as handled.
func (r *Repository) LatestFirstDetected(ctx context.Context, tenantID, checkType, realmName string) (time.Time, bool, error) {
	var latest sql.NullTime
	if err := r.db.WithContext(ctx).Unscoped().Model(&database.ConfigurationAlert{}).
		Where("tenant_id = ? AND check_type = ? AND realm_name = ?", tenantID, checkType, realmName).
		Select("MAX(first_detected)").Scan(&latest).Error; err != nil {
		return time.Time{}, false, fmt.Errorf("failed to find latest %s alert: %w", checkType, err)
	}
	return latest.Time, latest.Valid, nil
}

// ExistsIncludingDeleted implements alerts.Repository.
func (r *Repository) ExistsIncludingDeleted(ctx context.Context, tenantID, alertID string) (bool, error) {
	var found bool
	if err := r.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM configuration_alerts WHERE tenant_id = ? AND alert_id = ?)`,
		tenantID, alertID).Scan(&found).Error; err != nil {
		return false, fmt.Errorf("failed to check alert %s: %w", alertID, err)
	}
	return found, nil
}

// CountActive returns the total number of active alerts.
func (r *Repository) CountActive(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&database.ConfigurationAlert{}).
		Where("tenant_id = ? AND status = ?", tenantID, "active").
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count alerts: %w", err)
	}
	return count, nil
}

// Count returns the number of alerts matching the given filter options.
func (r *Repository) Count(ctx context.Context, tenantID string, opts *alerts.ListOptions) (int64, error) {
	if opts == nil {
		opts = &alerts.ListOptions{}
	}

	var count int64
	query := r.db.WithContext(ctx).
		Model(&database.ConfigurationAlert{}).
		Where("tenant_id = ?", tenantID)

	query = applyFilters(query, opts)

	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count alerts: %w", err)
	}
	return count, nil
}

// GetStatistics returns aggregated alert statistics, optionally scoped to a set
// of realms. Passing no realm counts every realm of the tenant.
func (r *Repository) GetStatistics(ctx context.Context, tenantID string, realmName ...string) (*alerts.Statistics, error) {
	stats := &alerts.Statistics{
		BySeverity: make(map[string]int),
		ByType:     make(map[string]int),
	}

	realms := make([]string, 0, len(realmName))
	for _, realm := range realmName {
		if realm != "" {
			realms = append(realms, realm)
		}
	}

	scoped := func() *gorm.DB {
		query := r.db.WithContext(ctx).
			Model(&database.ConfigurationAlert{}).
			Where("tenant_id = ? AND status = ?", tenantID, "active")
		if len(realms) > 0 {
			query = query.Where("realm_name IN ?", realms)
		}
		return query
	}

	var totalActive int64
	if err := scoped().Count(&totalActive).Error; err != nil {
		return nil, fmt.Errorf("failed to count active alerts: %w", err)
	}
	stats.TotalActive = int(totalActive)

	type severityCount struct {
		Severity string
		Count    int64
	}
	var severityCounts []severityCount
	if err := scoped().
		Select("severity, COUNT(*) as count").
		Group("severity").
		Scan(&severityCounts).Error; err != nil {
		return nil, fmt.Errorf("failed to get severity statistics: %w", err)
	}
	for _, sc := range severityCounts {
		stats.BySeverity[sc.Severity] = int(sc.Count)
	}

	type typeCount struct {
		Type  string
		Count int64
	}
	var typeCounts []typeCount
	if err := scoped().
		Select("type, COUNT(*) as count").
		Group("type").
		Scan(&typeCounts).Error; err != nil {
		return nil, fmt.Errorf("failed to get type statistics: %w", err)
	}
	for _, tc := range typeCounts {
		stats.ByType[tc.Type] = int(tc.Count)
	}

	return stats, nil
}

// Save inserts or updates an alert. The conflict target matches
// idx_alert_tenant's (tenant_id, alert_id) scope — alert_id alone is not
// globally unique, only unique within a tenant, so two tenants with the same
// alert_id (e.g. a same-named realm) must never collide into one row.
//
// idx_alert_tenant is a partial unique index (WHERE deleted_at IS NULL), so
// Postgres requires ON CONFLICT to repeat that exact predicate via
// TargetWhere — a plain ON CONFLICT (tenant_id, alert_id) doesn't match a
// partial index and errors with "no unique or exclusion constraint matching
// the ON CONFLICT specification".
//
// A resolved alert saved as active again starts a new occurrence, so
// occurrence_started_at moves to now. In ON CONFLICT DO UPDATE the unqualified
// column names below refer to the row as it was before this save, which is
// what lets one CASE see the transition. An alert that stays open keeps its
// occurrence; a row from before the column existed falls back to first_detected.
//
// A save of an existing alert is a re-detection, so it also writes what was
// detected now: the severity, the description, the metadata and updated_at. It
// used to write only the status columns, which left every notification
// describing the first detection, and left metadata.last_notified_at at the
// first notification, so from an hour later every poll re-notified. The
// metadata is the detection's, but last_notified_at carries over when this save
// does not set it: a checker records a notification only on the save that sends
// one.
//
// last_seen moves to the later of the stored and the saved time, never back:
// a save can describe an older sighting than one already recorded, as when the
// event poller re-reads its window. first_detected keeps the first save's.
//
// The acknowledgement carries over too, while the alert stays acknowledged (or
// resolved). A re-detection is built fresh and carries none, and writing that
// through wiped who acknowledged the alert and when on the next poll, though
// the checkers keep it acknowledged. It is replaced when the save brings its
// own, and cleared when the save makes the alert active: a reopened alert is a
// new occurrence nobody has acknowledged, and a producer that saves every
// re-detection as active, as the event converter does, un-acknowledges it.
// The resolution time carries over the same way while the alert stays resolved.
func (r *Repository) Save(ctx context.Context, alert *domain.Alert) error {
	dbAlert := convertToDBAlert(alert)
	now := time.Now()
	dbAlert.OccurrenceStartedAt = &now

	updates := clause.AssignmentColumns([]string{
		"status", "severity", "description", "event_id", "resource_id", "updated_at",
	})
	becomesActive := `EXCLUDED.status = '` + string(domain.AlertStatusActive) + `'`
	updates = append(updates,
		// last_seen only moves forward. The event poller re-reads its whole
		// window on every poll, newest first, so the oldest event is saved
		// last; taking its time would turn back the alert's last sighting.
		clause.Assignment{
			Column: clause.Column{Name: "last_seen"},
			Value:  gorm.Expr(`GREATEST(configuration_alerts.last_seen, EXCLUDED.last_seen)`),
		},
		clause.Assignment{
			Column: clause.Column{Name: "metadata"},
			Value: gorm.Expr(`CASE
				WHEN EXCLUDED.metadata IS NULL THEN configuration_alerts.metadata
				WHEN jsonb_typeof(EXCLUDED.metadata) = 'object' AND NOT (EXCLUDED.metadata ? 'last_notified_at')
				     AND jsonb_typeof(configuration_alerts.metadata) = 'object' AND configuration_alerts.metadata ? 'last_notified_at'
				THEN EXCLUDED.metadata || jsonb_build_object('last_notified_at', configuration_alerts.metadata -> 'last_notified_at')
				ELSE EXCLUDED.metadata END`),
		},
		clause.Assignment{
			Column: clause.Column{Name: "acknowledged_at"},
			Value: gorm.Expr(`CASE WHEN ` + becomesActive + ` THEN EXCLUDED.acknowledged_at
				ELSE COALESCE(EXCLUDED.acknowledged_at, configuration_alerts.acknowledged_at) END`),
		},
		clause.Assignment{
			Column: clause.Column{Name: "acknowledged_by"},
			Value: gorm.Expr(`CASE WHEN ` + becomesActive + ` THEN EXCLUDED.acknowledged_by
				ELSE COALESCE(NULLIF(EXCLUDED.acknowledged_by, ''), configuration_alerts.acknowledged_by) END`),
		},
		clause.Assignment{
			Column: clause.Column{Name: "resolved_at"},
			Value: gorm.Expr(`CASE WHEN EXCLUDED.status = '` + string(domain.AlertStatusResolved) + `'
				THEN COALESCE(EXCLUDED.resolved_at, configuration_alerts.resolved_at)
				ELSE EXCLUDED.resolved_at END`),
		},
	)
	updates = append(updates, clause.Assignment{
		Column: clause.Column{Name: "occurrence_started_at"},
		Value: gorm.Expr(`CASE WHEN configuration_alerts.status = ? AND EXCLUDED.status <> ?
			THEN EXCLUDED.occurrence_started_at
			ELSE COALESCE(configuration_alerts.occurrence_started_at, configuration_alerts.first_detected) END`,
			string(domain.AlertStatusResolved), string(domain.AlertStatusResolved)),
	})

	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "tenant_id"}, {Name: "alert_id"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
		DoUpdates:   updates,
	}).Create(dbAlert)

	if result.Error != nil {
		return fmt.Errorf("failed to save alert: %w", result.Error)
	}

	alert.ID = dbAlert.ID
	return nil
}

// UpdateStatus updates the status of an alert.
func (r *Repository) UpdateStatus(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) error {
	updates := map[string]interface{}{
		"status": string(status),
	}

	if status == domain.AlertStatusAcknowled {
		updates["acknowledged_at"] = gorm.Expr("NOW()")
		updates["acknowledged_by"] = acknowledgedBy
	}
	// Reopening a resolved alert starts a new occurrence, as in Save. The start
	// is the application's clock, not NOW(): notification logs are stamped by
	// the application too, and the dedup compares the two, so mixing in the
	// database's clock would miscount sends whenever the hosts' clocks differ.
	if status == domain.AlertStatusActive {
		updates["occurrence_started_at"] = gorm.Expr(
			"CASE WHEN status = ? THEN ?::timestamptz ELSE occurrence_started_at END",
			string(domain.AlertStatusResolved), time.Now())
	}

	if err := r.db.WithContext(ctx).
		Model(&database.ConfigurationAlert{}).
		Where("tenant_id = ? AND alert_id = ?", tenantID, alertID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update alert status: %w", err)
	}

	return nil
}

// Resolve marks an alert as resolved.
func (r *Repository) Resolve(ctx context.Context, tenantID, alertID string) error {
	updates := map[string]interface{}{
		"status":      "resolved",
		"resolved_at": gorm.Expr("NOW()"),
	}

	if err := r.db.WithContext(ctx).
		Model(&database.ConfigurationAlert{}).
		Where("tenant_id = ? AND alert_id = ?", tenantID, alertID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to resolve alert: %w", err)
	}

	return nil
}

// Delete removes an alert.
func (r *Repository) Delete(ctx context.Context, tenantID, alertID string) error {
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND alert_id = ?", tenantID, alertID).
		Delete(&database.ConfigurationAlert{}).Error; err != nil {
		return fmt.Errorf("failed to delete alert: %w", err)
	}
	return nil
}

// applyFilters applies filtering options to a query.
func applyFilters(query *gorm.DB, opts *alerts.ListOptions) *gorm.DB {
	if opts.Status != "" {
		query = query.Where("status = ?", string(opts.Status))
	}
	if opts.Severity != "" {
		query = query.Where("severity = ?", string(opts.Severity))
	}
	if opts.Type != "" {
		query = query.Where("type = ?", string(opts.Type))
	}
	if opts.RealmName != "" {
		query = query.Where("realm_name = ?", opts.RealmName)
	}
	if len(opts.RealmNames) > 0 {
		query = query.Where("realm_name IN ?", opts.RealmNames)
	}
	if opts.ResourceType != "" {
		query = query.Where("resource_type = ?", opts.ResourceType)
	}
	return query
}

// convertAlert converts a database alert to a domain alert.
func convertAlert(db *database.ConfigurationAlert) *domain.Alert {
	if db == nil {
		return nil
	}

	var metadata string
	if len(db.Metadata) > 0 {
		metadata = string(db.Metadata)
	}

	return &domain.Alert{
		ID:             db.ID,
		TenantID:       db.TenantID,
		AlertID:        db.AlertID,
		Source:         domain.AlertSource(db.Source),
		Type:           domain.AlertType(db.Type),
		Severity:       domain.AlertSeverity(db.Severity),
		Status:         domain.AlertStatus(db.Status),
		Title:          db.Title,
		Description:    db.Description,
		ResourceType:   db.ResourceType,
		ResourceID:     db.ResourceID,
		EventID:        db.EventID,
		ResourceName:   db.ResourceName,
		RealmName:      db.RealmName,
		CheckType:      db.CheckType,
		Recommendation: db.Recommendation,
		Metadata:       metadata,
		FirstDetected:  db.FirstDetected,
		LastSeen:       db.LastSeen,
		ResolvedAt:     db.ResolvedAt,
		AcknowledgedAt: db.AcknowledgedAt,
		AcknowledgedBy: db.AcknowledgedBy,
		CreatedAt:      db.CreatedAt,
		UpdatedAt:      db.UpdatedAt,
	}
}

// convertAlertList converts a list of database alerts to domain alerts.
func convertAlertList(dbAlerts []*database.ConfigurationAlert) []*domain.Alert {
	result := make([]*domain.Alert, len(dbAlerts))
	for i := range dbAlerts {
		result[i] = convertAlert(dbAlerts[i])
	}
	return result
}

// convertToDBAlert converts a domain alert to a database alert.
func convertToDBAlert(alert *domain.Alert) *database.ConfigurationAlert {
	if alert == nil {
		return nil
	}

	var metadata []byte
	if alert.Metadata != "" {
		metadata = []byte(alert.Metadata)
	}

	return &database.ConfigurationAlert{
		ID:             alert.ID,
		TenantID:       alert.TenantID,
		AlertID:        alert.AlertID,
		Source:         string(alert.Source),
		Type:           string(alert.Type),
		Severity:       string(alert.Severity),
		Status:         string(alert.Status),
		Title:          alert.Title,
		Description:    alert.Description,
		ResourceType:   alert.ResourceType,
		ResourceID:     alert.ResourceID,
		EventID:        alert.EventID,
		ResourceName:   alert.ResourceName,
		RealmName:      alert.RealmName,
		CheckType:      alert.CheckType,
		Recommendation: alert.Recommendation,
		Metadata:       metadata,
		FirstDetected:  alert.FirstDetected,
		LastSeen:       alert.LastSeen,
		ResolvedAt:     alert.ResolvedAt,
		AcknowledgedAt: alert.AcknowledgedAt,
		AcknowledgedBy: alert.AcknowledgedBy,
		CreatedAt:      alert.CreatedAt,
		UpdatedAt:      alert.UpdatedAt,
	}
}

// Ensure Repository implements alerts.Repository
var _ alerts.Repository = (*Repository)(nil)
