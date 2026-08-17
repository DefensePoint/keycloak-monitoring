// Package postgres provides PostgreSQL implementation of events repository interface.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"repos.defensepoint.com/iam/oss-pmt/kmt/events"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/database"
)

// Repository implements events.Repository using GORM.
type Repository struct {
	db     *gorm.DB
	logger *logger.Logger
}

// NewRepository creates a new events Repository.
func NewRepository(db *gorm.DB, log *logger.Logger) events.Repository {
	return &Repository{
		db:     db,
		logger: log,
	}
}

// Save inserts or updates an event in the database using GORM upsert.
func (r *Repository) Save(ctx context.Context, event *domain.Event) error {
	// Convert domain event to database event
	dbEvent := toDBEvent(event)
	// Use GORM's Clauses for upsert (ON CONFLICT DO UPDATE)
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "event_id"}},
		// NOTE: the AMFA value columns (final_status, is_vpn, country, lat, long)
		// are deliberately NOT in this update set. They are owned by the AMFA
		// side and written onto a Keycloak row by ReconcileAMFAMerges; including
		// them here would let the Keycloak monitor's periodic re-upsert of the
		// same event clobber that enrichment back to NULL. amfa_event_id and
		// risk_level are safe: the Keycloak event itself carries them.
		DoUpdates: clause.AssignmentColumns([]string{
			"type", "category", "severity", "description",
			"source", "source_ip", "source_system",
			"user_id", "username", "email", "client_id",
			"location", "raw_data", "status",
			"amfa_event_id", "risk_level",
			"updated_at",
		}),
	}).Create(dbEvent)

	if result.Error != nil {
		return fmt.Errorf("failed to save event: %w", result.Error)
	}

	// Update the domain event with the saved ID
	event.ID = dbEvent.ID
	return nil
}

// GetByID retrieves an event by its event ID using GORM.
func (r *Repository) GetByID(ctx context.Context, eventID string) (*domain.Event, error) {
	var event database.Event
	result := r.db.WithContext(ctx).Where("event_id = ?", eventID).First(&event)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("event not found: %s", eventID)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get event: %w", result.Error)
	}

	return toDomainEvent(&event), nil
}

// List retrieves events with pagination using GORM.
func (r *Repository) List(ctx context.Context, opts *events.ListOptions) ([]*domain.Event, error) {
	if opts == nil {
		opts = &events.ListOptions{Limit: 100, Offset: 0}
	}

	var dbEvents []*database.Event
	query := r.db.WithContext(ctx).Order("timestamp DESC")
	query = applySourceFilter(query, opts)

	// Add time range filters if provided
	if opts.StartTime != nil && *opts.StartTime != "" {
		startTime, err := time.Parse(time.RFC3339, *opts.StartTime)
		if err == nil {
			query = query.Where("timestamp >= ?", startTime)
		} else {
			r.logger.Warn("Invalid start_time format", logger.Str("start_time", *opts.StartTime))
		}
	}
	if opts.EndTime != nil && *opts.EndTime != "" {
		endTime, err := time.Parse(time.RFC3339, *opts.EndTime)
		if err == nil {
			query = query.Where("timestamp <= ?", endTime)
		} else {
			r.logger.Warn("Invalid end_time format", logger.Str("end_time", *opts.EndTime))
		}
	}

	result := query.
		Limit(opts.Limit).
		Offset(opts.Offset).
		Find(&dbEvents)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to query events: %w", result.Error)
	}

	// Convert database events to domain events
	domainEvents := make([]*domain.Event, len(dbEvents))
	for i, event := range dbEvents {
		domainEvents[i] = toDomainEvent(event)
	}

	return domainEvents, nil
}

// Count returns the total number of events using GORM.
func (r *Repository) Count(ctx context.Context) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).Model(&database.Event{}).Count(&count)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to count events: %w", result.Error)
	}

	return count, nil
}

// CountWithFilter returns the count of events matching the filter criteria.
func (r *Repository) CountWithFilter(ctx context.Context, opts *events.ListOptions) (int64, error) {
	if opts == nil {
		opts = &events.ListOptions{}
	}

	var count int64
	query := r.db.WithContext(ctx).Model(&database.Event{})
	query = applySourceFilter(query, opts)

	// Add time range filters if provided
	if opts.StartTime != nil && *opts.StartTime != "" {
		startTime, err := time.Parse(time.RFC3339, *opts.StartTime)
		if err == nil {
			query = query.Where("timestamp >= ?", startTime)
		} else {
			r.logger.Warn("Invalid start_time format", logger.Str("start_time", *opts.StartTime))
		}
	}
	if opts.EndTime != nil && *opts.EndTime != "" {
		endTime, err := time.Parse(time.RFC3339, *opts.EndTime)
		if err == nil {
			query = query.Where("timestamp <= ?", endTime)
		} else {
			r.logger.Warn("Invalid end_time format", logger.Str("end_time", *opts.EndTime))
		}
	}

	result := query.Count(&count)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to count events with filter: %w", result.Error)
	}

	return count, nil
}

// applySourceFilter adds the source predicate from opts to the query.
// Sources (OR of LIKEs) takes precedence over the single Source substring.
func applySourceFilter(query *gorm.DB, opts *events.ListOptions) *gorm.DB {
	if len(opts.Sources) > 0 {
		or := query.Session(&gorm.Session{NewDB: true}).
			Where("source LIKE ?", "%"+opts.Sources[0]+"%")
		for _, s := range opts.Sources[1:] {
			or = or.Or("source LIKE ?", "%"+s+"%")
		}
		return query.Where(or)
	}
	if opts.Source != "" {
		return query.Where("source LIKE ?", "%"+opts.Source+"%")
	}
	return query
}

// LatestTimestampBySource returns the newest event timestamp among rows whose
// source matches exactly; zero time when none exist.
func (r *Repository) LatestTimestampBySource(ctx context.Context, source string) (time.Time, error) {
	var latest sql.NullTime
	result := r.db.WithContext(ctx).
		Model(&database.Event{}).
		Select("MAX(timestamp)").
		Where("source = ?", source).
		Scan(&latest)
	if result.Error != nil {
		return time.Time{}, fmt.Errorf("failed to query latest timestamp for source %s: %w", source, result.Error)
	}
	if !latest.Valid {
		return time.Time{}, nil
	}
	return latest.Time, nil
}

// MergeKeycloakEvent stores a Keycloak event. Convergence with its AMFA twin
// (when one exists) is handled by ReconcileAMFAMerges, not inline: the two
// writers run independently and a login's halves can arrive in any order, so a
// single idempotent SQL sweep is both simpler and race-free.
func (r *Repository) MergeKeycloakEvent(ctx context.Context, event *domain.Event) error {
	return r.Save(ctx, event)
}

// MergeAmfaEvent stores an AMFA event as its own row; ReconcileAMFAMerges folds
// it into the Keycloak twin once both are present.
func (r *Repository) MergeAmfaEvent(ctx context.Context, event *domain.Event) error {
	return r.Save(ctx, event)
}

// ReconcileAMFAMerges converges each login onto one row: it copies the AMFA
// fields from every standalone AMFA row onto its Keycloak twin (matched by
// amfa_event_id), then soft-deletes the now-absorbed AMFA rows. Idempotent and
// order-independent — safe to run every poll cycle. Returns the number of AMFA
// rows absorbed. AMFA-only events (no Keycloak twin) are left untouched.
func (r *Repository) ReconcileAMFAMerges(ctx context.Context) (int64, error) {
	// 1. Enrich Keycloak rows from their AMFA twin. Keycloak keeps its own
	//    risk_level (from the event details) when present.
	enrich := `
		UPDATE events AS k
		SET is_vpn = a.is_vpn,
		    country = a.country,
		    city = a.city,
		    lat = a.lat,
		    "long" = a."long",
		    final_status = a.final_status,
		    operating_system = a.operating_system,
		    browser = a.browser,
		    device = a.device,
		    system_language = a.system_language,
		    screen_resolution = a.screen_resolution,
		    risk_level = COALESCE(k.risk_level, a.risk_level),
		    updated_at = now()
		FROM events AS a
		WHERE a.source_system = 'amfa' AND a.deleted_at IS NULL
		  AND k.source_system = 'keycloak' AND k.deleted_at IS NULL
		  AND k.amfa_event_id = a.amfa_event_id`
	if err := r.db.WithContext(ctx).Exec(enrich).Error; err != nil {
		return 0, fmt.Errorf("amfa reconcile enrich: %w", err)
	}

	// 2. Hard-delete the absorbed AMFA rows: their data now lives on the
	//    Keycloak twin, so keeping a tombstone would just be a confusing second
	//    row. Safe to remove — if the mirror ever re-reads that AMFA event it
	//    re-inserts the row and the next sweep re-merges it.
	del := `
		DELETE FROM events AS a
		USING events AS k
		WHERE a.source_system = 'amfa'
		  AND k.source_system = 'keycloak'
		  AND k.deleted_at IS NULL
		  AND k.amfa_event_id = a.amfa_event_id`
	res := r.db.WithContext(ctx).Exec(del)
	if res.Error != nil {
		return 0, fmt.Errorf("amfa reconcile delete: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// toDomainEvent converts a database Event to a domain Event.
func toDomainEvent(event *database.Event) *domain.Event {
	if event == nil {
		return nil
	}
	// Convert RawData from []byte (JSONB) to string
	rawDataStr := ""
	if len(event.RawData) > 0 {
		rawDataStr = string(event.RawData)
	}

	return &domain.Event{
		ID:               event.ID,
		EventID:          event.EventID,
		Type:             event.Type,
		Category:         event.Category,
		Severity:         event.Severity,
		Description:      event.Description,
		Source:           event.Source,
		SourceIP:         event.SourceIP,
		SourceSystem:     event.SourceSystem,
		UserID:           event.UserID,
		Username:         event.Username,
		Email:            event.Email,
		ClientID:         event.ClientID,
		Location:         event.Location,
		RawData:          rawDataStr,
		Status:           event.Status,
		Timestamp:        event.Timestamp,
		CreatedAt:        event.CreatedAt,
		UpdatedAt:        event.UpdatedAt,
		AMFAEventID:      event.AMFAEventID,
		RiskLevel:        event.RiskLevel,
		FinalStatus:      event.FinalStatus,
		IsVPN:            event.IsVPN,
		Country:          event.Country,
		City:             event.City,
		Lat:              event.Lat,
		Long:             event.Long,
		OperatingSystem:  event.OperatingSystem,
		Browser:          event.Browser,
		Device:           event.Device,
		SystemLanguage:   event.SystemLanguage,
		ScreenResolution: event.ScreenResolution,
	}
}

// toDBEvent converts a domain Event to a database Event.
func toDBEvent(event *domain.Event) *database.Event {
	if event == nil {
		return nil
	}
	// Convert RawData from string to []byte (JSONB)
	var rawDataBytes []byte
	if event.RawData != "" {
		rawDataBytes = []byte(event.RawData)
	}

	return &database.Event{
		ID:               event.ID,
		EventID:          event.EventID,
		Type:             event.Type,
		Category:         event.Category,
		Severity:         event.Severity,
		Description:      event.Description,
		Source:           event.Source,
		SourceIP:         event.SourceIP,
		SourceSystem:     event.SourceSystem,
		UserID:           event.UserID,
		Username:         event.Username,
		Email:            event.Email,
		ClientID:         event.ClientID,
		Location:         event.Location,
		RawData:          rawDataBytes,
		Status:           event.Status,
		Timestamp:        event.Timestamp,
		CreatedAt:        event.CreatedAt,
		UpdatedAt:        event.UpdatedAt,
		AMFAEventID:      event.AMFAEventID,
		RiskLevel:        event.RiskLevel,
		FinalStatus:      event.FinalStatus,
		IsVPN:            event.IsVPN,
		Country:          event.Country,
		City:             event.City,
		Lat:              event.Lat,
		Long:             event.Long,
		OperatingSystem:  event.OperatingSystem,
		Browser:          event.Browser,
		Device:           event.Device,
		SystemLanguage:   event.SystemLanguage,
		ScreenResolution: event.ScreenResolution,
	}
}
