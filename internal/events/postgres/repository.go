// Package postgres provides PostgreSQL implementation of events repository interface.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
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
		// event_id is unique per tenant, so the conflict target must name
		// both columns or the upsert will not find the existing row.
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "event_id"}},
		// NOTE: the AMFA value columns (final_status, is_vpn, country, lat, long,
		// raw_data) are deliberately NOT in this update set. They are owned by
		// the AMFA side and written onto a Keycloak row by ReconcileAMFAMerges;
		// including them here would let the Keycloak monitor's periodic
		// re-upsert of the same event clobber that enrichment back to NULL
		// (the Keycloak mirror always saves an empty raw_data — see
		// keycloak/monitor.go).
		//
		// This applies to amfa_event_id and risk_level too. They used to be
		// updatable on the assumption that the Keycloak event always carries
		// them, which only holds where a Keycloak extension stamps them onto the
		// login event. The open-source Adaptive MFA SPI does not, so there the
		// values come from the merge and a re-upsert would erase them. Leaving
		// them out costs nothing when the event does carry them: both are
		// immutable for a given event, so the INSERT already stores the value.
		DoUpdates: clause.AssignmentColumns([]string{
			"type", "category", "severity", "description",
			"source", "source_ip", "source_system",
			"user_id", "username", "email", "client_id",
			"location", "status",
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
		return nil, fmt.Errorf("%w: %s", events.ErrNotFound, eventID)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get event: %w", result.Error)
	}

	return toDomainEvent(&event), nil
}

// GetByTenantAndEventID retrieves one tenant's event by its event ID. event_id
// is only unique per tenant, so both columns must be filtered or a lookup could
// return another tenant's row for the same id.
func (r *Repository) GetByTenantAndEventID(ctx context.Context, tenantID, eventID string) (*domain.Event, error) {
	var event database.Event
	result := r.db.WithContext(ctx).
		Where("tenant_id = ? AND event_id = ?", tenantID, eventID).
		First(&event)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: %s", events.ErrNotFound, eventID)
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
	query = r.applyFilters(query, opts)

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

// ListKeyset retrieves one page of events in keyset order (timestamp DESC,
// id DESC), continuing strictly after the given position, or from the newest
// row when after is nil. Applies the same filters as List; opts.Offset is
// ignored, the position replaces it.
func (r *Repository) ListKeyset(ctx context.Context, opts *events.ListOptions, after *events.KeysetPosition) (*events.KeysetPage, error) {
	if opts == nil {
		opts = &events.ListOptions{Limit: 100}
	}

	query := r.db.WithContext(ctx).Order("timestamp DESC, id DESC")
	query = r.applyFilters(query, opts)
	if after != nil {
		// Postgres timestamps resolve to microseconds, so a cursor carrying
		// nanoseconds never satisfies the equality arm and rows sharing the
		// cursor's timestamp would be skipped or repeated (same trap as the
		// purge cursor in pkg/database/tenant_purge.go).
		ts := after.Timestamp.UTC().Truncate(time.Microsecond)
		query = query.Where("timestamp < ? OR (timestamp = ? AND id < ?)", ts, ts, after.ID)
	}

	var dbEvents []*database.Event
	if err := query.Limit(opts.Limit).Find(&dbEvents).Error; err != nil {
		return nil, fmt.Errorf("failed to query events keyset page: %w", err)
	}

	page := &events.KeysetPage{Events: make([]*domain.Event, len(dbEvents))}
	for i, event := range dbEvents {
		page.Events[i] = toDomainEvent(event)
	}
	if n := len(dbEvents); n > 0 {
		page.Last = &events.KeysetPosition{
			Timestamp: dbEvents[n-1].Timestamp,
			ID:        dbEvents[n-1].ID,
		}
	}
	return page, nil
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
	query = r.applyFilters(query, opts)

	result := query.Count(&count)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to count events with filter: %w", result.Error)
	}

	return count, nil
}

// Stats returns event counts grouped by type, severity and source, all under
// the same filter scope as List.
func (r *Repository) Stats(ctx context.Context, opts *events.ListOptions) (*events.Stats, error) {
	if opts == nil {
		opts = &events.ListOptions{}
	}

	stats := &events.Stats{
		ByType:     map[string]int64{},
		BySeverity: map[string]int64{},
		BySource:   map[string]int64{},
	}
	for _, group := range []struct {
		column string
		counts map[string]int64
	}{
		{"type", stats.ByType},
		{"severity", stats.BySeverity},
		{"source", stats.BySource},
	} {
		var rows []struct {
			Key   string
			Count int64
		}
		query := r.applyFilters(r.db.WithContext(ctx).Model(&database.Event{}), opts)
		if err := query.
			Select(group.column + ` AS key, count(*) AS count`).
			Group(group.column).
			Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("failed to aggregate events by %s: %w", group.column, err)
		}
		for _, row := range rows {
			group.counts[row.Key] = row.Count
		}
	}
	return stats, nil
}

// applyFilters adds every ListOptions predicate except pagination: tenant and
// source scoping, the classification filters, and the time window. Shared by
// every read path (List, ListKeyset, CountWithFilter, Stats) so a page, its
// total and its aggregates always describe the same row set.
//
// A malformed time bound is warned about and skipped rather than rejected,
// which existing HTTP callers rely on.
func (r *Repository) applyFilters(query *gorm.DB, opts *events.ListOptions) *gorm.DB {
	query = applySourceFilter(query, opts)

	if opts.Type != "" {
		query = query.Where("type = ?", opts.Type)
	}
	if opts.Severity != "" {
		query = query.Where("severity = ?", opts.Severity)
	}
	if opts.MinRisk != nil {
		query = query.Where("risk_level >= ?", *opts.MinRisk)
	}

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
	return query
}

// applySourceFilter adds the source predicate from opts to the query.
// Sources (exact IN match) takes precedence over the single Source substring.
// Exact matching is required for tenant scoping: a substring match on a realm
// name would leak one tenant's events into another whose realm name is a prefix
// (for example "prod" matching "prod-staging").
func applySourceFilter(query *gorm.DB, opts *events.ListOptions) *gorm.DB {
	// Tenant first: it is the isolation boundary, and equality never matches
	// the '' carried by rows written before tenant_id existed.
	if opts.TenantID != "" {
		query = query.Where("tenant_id = ?", opts.TenantID)
	}
	if len(opts.Sources) > 0 {
		return query.Where("source IN ?", opts.Sources)
	}
	if opts.Source != "" {
		return query.Where("source LIKE ?", "%"+opts.Source+"%")
	}
	return query
}

// AmfaMirrorWatermark returns the AMFA events mirror's recorded read position
// for one (tenant, realm), or the zero time when none has been recorded yet.
//
// This replaced a MAX(timestamp) over the events table filtered by
// source = 'amfa:<realm>'. That query could not distinguish tenants, because
// events rows carry no tenant and realm names are unique per-tenant rather
// than globally, so two tenants monitoring a realm of the same name shared one
// position and the one that polled second silently skipped its own older
// events. It also read from rows ReconcileAMFAMerges deletes on merge, so the
// value could regress. The position is now stored explicitly per tenant.
func (r *Repository) AmfaMirrorWatermark(ctx context.Context, tenantID, realm string) (time.Time, error) {
	var row database.AmfaMirrorWatermark
	result := r.db.WithContext(ctx).
		Where("tenant_id = ? AND realm = ?", tenantID, realm).
		Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return time.Time{}, nil
	}
	if result.Error != nil {
		return time.Time{}, fmt.Errorf("failed to read amfa mirror watermark for tenant %s realm %s: %w",
			tenantID, realm, result.Error)
	}
	return row.EventTime, nil
}

// SaveAmfaMirrorWatermark records the mirror's read position for one
// (tenant, realm). The update is monotonic: an older position never overwrites
// a newer one, so a stale writer can at worst cause a re-read (harmless, since
// saves upsert on event_id) rather than skipping events.
func (r *Repository) SaveAmfaMirrorWatermark(ctx context.Context, tenantID, realm string, eventTime time.Time) error {
	const upsert = `
		INSERT INTO amfa_mirror_watermarks (tenant_id, realm, event_time, updated_at)
		VALUES (?, ?, ?, now())
		ON CONFLICT (tenant_id, realm) DO UPDATE
		SET event_time = EXCLUDED.event_time, updated_at = now()
		WHERE amfa_mirror_watermarks.event_time < EXCLUDED.event_time`
	if err := r.db.WithContext(ctx).Exec(upsert, tenantID, realm, eventTime).Error; err != nil {
		return fmt.Errorf("failed to save amfa mirror watermark for tenant %s realm %s: %w",
			tenantID, realm, err)
	}
	return nil
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
	//    risk_level (from the event details) when present. raw_data is
	//    similarly COALESCEd rather than overwritten unconditionally: today
	//    the Keycloak mirror always saves an empty raw_data (see
	//    keycloak/monitor.go), so this always falls through to the AMFA
	//    payload, but the merge shouldn't silently clobber a populated
	//    Keycloak raw_data if that ever changes.
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
		    raw_data = COALESCE(k.raw_data, a.raw_data),
		    -- The AMFA mirror resolves these against Keycloak when it writes
		    -- its row; the Keycloak monitor takes them from the event payload
		    -- and leaves them empty for the event types that carry no username
		    -- (REFRESH_TOKEN, CODE_TO_TOKEN and friends). Copying them here
		    -- costs no Keycloak call: the value is already on the row being
		    -- absorbed, and without this it is discarded when that row is.
		    --
		    -- NULLIF, because these columns are NOT NULL with an empty default:
		    -- a plain COALESCE would see '' as a value and keep it, which is a
		    -- silent no-op. Keycloak still wins where it has something.
		    username = COALESCE(NULLIF(k.username, ''), a.username),
		    email = COALESCE(NULLIF(k.email, ''), a.email),
		    updated_at = now()
		FROM events AS a
		WHERE a.source_system = 'amfa' AND a.deleted_at IS NULL
		  AND k.source_system = 'keycloak' AND k.deleted_at IS NULL
		  AND k.tenant_id = a.tenant_id
		  AND k.amfa_event_id = a.amfa_event_id`
	if err := r.db.WithContext(ctx).Exec(enrich).Error; err != nil {
		return 0, fmt.Errorf("amfa reconcile enrich: %w", err)
	}

	// 2. Hard-delete the absorbed AMFA rows: their data now lives on the
	//    Keycloak twin, so keeping a tombstone would just be a confusing second
	//    row. Safe to remove — if the mirror ever re-reads that AMFA event it
	//    re-inserts the row and the next sweep re-merges it.
	//
	//    The predicates here must match step 1's exactly, `a.deleted_at IS NULL`
	//    included. Step 1 skips soft-deleted AMFA rows, so without the same
	//    filter here a soft-deleted row would be hard-deleted having never been
	//    folded onto its twin: its AMFA fields would be destroyed rather than
	//    merged. Delete only what was actually absorbed.
	del := `
		DELETE FROM events AS a
		USING events AS k
		WHERE a.source_system = 'amfa' AND a.deleted_at IS NULL
		  AND k.source_system = 'keycloak'
		  AND k.deleted_at IS NULL
		  AND k.tenant_id = a.tenant_id
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
		TenantID:         event.TenantID,
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
		TenantID:         event.TenantID,
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
