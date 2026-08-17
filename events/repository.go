// Package events provides event repository interfaces and implementations.
package events

import (
	"context"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// ListOptions defines filtering and pagination options for event queries.
type ListOptions struct {
	Limit  int
	Offset int
	// Source filters rows whose source contains this substring.
	Source string
	// Sources, when non-empty, takes precedence over Source and matches rows
	// whose source contains ANY of the given substrings (OR of LIKEs). Used by
	// the unified Events view to select keycloak:<realm> and amfa:<realm>.
	Sources   []string
	StartTime *string
	EndTime   *string
}

// Repository defines the interface for event persistence operations.
type Repository interface {
	// Save inserts or updates an event in the database.
	Save(ctx context.Context, event *domain.Event) error

	// GetByID retrieves an event by its event ID.
	GetByID(ctx context.Context, eventID string) (*domain.Event, error)

	// List retrieves events with pagination and filtering.
	List(ctx context.Context, opts *ListOptions) ([]*domain.Event, error)

	// Count returns the total number of events.
	Count(ctx context.Context) (int64, error)

	// CountWithFilter returns the count of events matching the filter criteria.
	CountWithFilter(ctx context.Context, opts *ListOptions) (int64, error)

	// LatestTimestampBySource returns the newest event timestamp among rows with
	// an exactly matching source (e.g. "amfa:MyRealm"). Returns the zero time
	// when no such rows exist. Used by mirrors to derive their watermark.
	LatestTimestampBySource(ctx context.Context, source string) (time.Time, error)

	// MergeKeycloakEvent saves a Keycloak-sourced event, first absorbing a
	// pre-existing standalone AMFA row that shares the same amfa_event_id
	// (AMFA arrived first): its AMFA fields are folded onto this event and the
	// standalone row is removed, so one canonical Keycloak row remains.
	// Falls back to a normal upsert when there is no such AMFA row.
	MergeKeycloakEvent(ctx context.Context, event *domain.Event) error

	// MergeAmfaEvent enriches an existing Keycloak row that shares the
	// amfa_event_id (AMFA arrived second), writing the AMFA fields onto it and
	// returning without inserting. When no Keycloak twin exists (AMFA-only, or
	// AMFA arrived first) it inserts a standalone AMFA row.
	MergeAmfaEvent(ctx context.Context, event *domain.Event) error

	// ReconcileAMFAMerges converges standalone AMFA rows onto their Keycloak
	// twin (matched by amfa_event_id) and soft-deletes the absorbed AMFA rows.
	// Idempotent and order-independent; returns the number of rows absorbed.
	ReconcileAMFAMerges(ctx context.Context) (int64, error)
}
