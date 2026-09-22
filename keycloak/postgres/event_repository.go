// Package postgres provides PostgreSQL implementations of keycloak repository interfaces.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/DefensePoint/keycloak-monitoring/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// EventRepository implements keycloak.EventRepository using GORM.
type EventRepository struct {
	db *gorm.DB
}

// NewEventRepository creates a new EventRepository.
func NewEventRepository(db *gorm.DB) *EventRepository {
	return &EventRepository{db: db}
}

// GetEvents returns events for a tenant.
func (r *EventRepository) GetEvents(ctx context.Context, tenantID string, limit, offset int) ([]*domain.KeycloakEvent, error) {
	var dbEvents []*database.KeycloakEvent

	query := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("time DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&dbEvents).Error; err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	return convertDBEventsToDomain(dbEvents), nil
}

// GetEventsByRealm returns events for a specific realm.
func (r *EventRepository) GetEventsByRealm(ctx context.Context, tenantID, realmName string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error) {
	var dbEvents []*database.KeycloakEvent

	query := r.db.WithContext(ctx).
		Where("tenant_id = ? AND realm_name = ? AND time >= ? AND time <= ?", tenantID, realmName, from, to).
		Order("time DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&dbEvents).Error; err != nil {
		return nil, fmt.Errorf("failed to get events by realm: %w", err)
	}

	return convertDBEventsToDomain(dbEvents), nil
}

// GetEventsByType returns events of a specific type within one realm.
func (r *EventRepository) GetEventsByType(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error) {
	var dbEvents []*database.KeycloakEvent

	query := r.db.WithContext(ctx).
		Where("tenant_id = ? AND realm_name = ? AND event_type = ? AND time >= ? AND time <= ?", tenantID, realmName, eventType, from, to).
		Order("time DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&dbEvents).Error; err != nil {
		return nil, fmt.Errorf("failed to get events by type: %w", err)
	}

	return convertDBEventsToDomain(dbEvents), nil
}

// GetEventsByTimeRange returns events within a time range.
func (r *EventRepository) GetEventsByTimeRange(ctx context.Context, tenantID string, from, to time.Time, limit, offset int) ([]*domain.KeycloakEvent, error) {
	var dbEvents []*database.KeycloakEvent

	query := r.db.WithContext(ctx).
		Where("tenant_id = ? AND time >= ? AND time <= ?", tenantID, from, to).
		Order("time DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&dbEvents).Error; err != nil {
		return nil, fmt.Errorf("failed to get events by time range: %w", err)
	}

	return convertDBEventsToDomain(dbEvents), nil
}

// CountEvents returns the total count of events.
func (r *EventRepository) CountEvents(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&database.KeycloakEvent{}).
		Where("tenant_id = ?", tenantID).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count events: %w", err)
	}

	return count, nil
}

// CountEventsByType counts events by type within a time range.
func (r *EventRepository) CountEventsByType(ctx context.Context, tenantID, realmName, eventType string, from, to time.Time) (int64, error) {
	var count int64

	query := r.db.WithContext(ctx).Model(&database.KeycloakEvent{}).
		Where("tenant_id = ?", tenantID)

	if realmName != "" {
		query = query.Where("realm_name = ?", realmName)
	}
	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	if !from.IsZero() {
		query = query.Where("time >= ?", from)
	}
	if !to.IsZero() {
		query = query.Where("time <= ?", to)
	}

	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count events by type: %w", err)
	}

	return count, nil
}

// SaveEvent saves a single event.
func (r *EventRepository) SaveEvent(ctx context.Context, event *domain.KeycloakEvent) error {
	dbEvent := convertDomainEventToDB(event)

	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "time"}, {Name: "event_id"}},
		DoNothing: true,
	}).Create(dbEvent)

	if result.Error != nil {
		return fmt.Errorf("failed to save event: %w", result.Error)
	}

	return nil
}

// SaveEvents saves multiple events.
func (r *EventRepository) SaveEvents(ctx context.Context, events []*domain.KeycloakEvent) error {
	if len(events) == 0 {
		return nil
	}

	dbEvents := make([]*database.KeycloakEvent, len(events))
	for i, e := range events {
		dbEvents[i] = convertDomainEventToDB(e)
	}

	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "time"}, {Name: "event_id"}},
		DoNothing: true,
	}).Create(&dbEvents)

	if result.Error != nil {
		return fmt.Errorf("failed to save events: %w", result.Error)
	}

	return nil
}

// DeleteEventsBefore deletes events before a given time.
func (r *EventRepository) DeleteEventsBefore(ctx context.Context, tenantID string, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("tenant_id = ? AND time < ?", tenantID, before).
		Delete(&database.KeycloakEvent{})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to delete events: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// convertDBEventsToDomain converts database events to domain events.
func convertDBEventsToDomain(dbEvents []*database.KeycloakEvent) []*domain.KeycloakEvent {
	events := make([]*domain.KeycloakEvent, len(dbEvents))
	for i, e := range dbEvents {
		events[i] = convertDBEventToDomain(e)
	}
	return events
}

// convertDBEventToDomain converts a single database event to domain event.
func convertDBEventToDomain(e *database.KeycloakEvent) *domain.KeycloakEvent {
	var details map[string]string
	if len(e.Details) > 0 {
		_ = json.Unmarshal(e.Details, &details)
	}

	return &domain.KeycloakEvent{
		TenantID:   e.TenantID,
		EventID:    e.EventID,
		Time:       e.Time,
		RealmID:    e.RealmID,
		RealmName:  e.RealmName,
		ClientID:   e.ClientID,
		SessionID:  e.SessionID,
		IPAddress:  e.IPAddress,
		EventType:  e.EventType,
		EventError: e.EventError,
		UserID:     e.UserID,
		Username:   e.Username,
		Email:      e.Email,
		Details:    details,
		Success:    e.Success,
	}
}

// convertDomainEventToDB converts a domain event to database event.
func convertDomainEventToDB(e *domain.KeycloakEvent) *database.KeycloakEvent {
	var detailsJSON []byte
	if e.Details != nil {
		detailsJSON, _ = json.Marshal(e.Details)
	}

	return &database.KeycloakEvent{
		TenantID:   e.TenantID,
		Time:       e.Time,
		EventID:    e.EventID,
		RealmID:    e.RealmID,
		RealmName:  e.RealmName,
		ClientID:   e.ClientID,
		SessionID:  e.SessionID,
		IPAddress:  e.IPAddress,
		EventType:  e.EventType,
		EventError: e.EventError,
		UserID:     e.UserID,
		Username:   e.Username,
		Email:      e.Email,
		Details:    detailsJSON,
		Success:    e.Success,
	}
}

// Ensure EventRepository implements keycloak.EventRepository
var _ keycloak.EventRepository = (*EventRepository)(nil)
