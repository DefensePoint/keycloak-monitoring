// Package events provides event repository interfaces and implementations.
package events

import (
	"context"
	"errors"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// ErrNotFound is returned when the requested event does not exist.
var ErrNotFound = errors.New("event not found")

// ListOptions defines filtering and pagination options for event queries.
type ListOptions struct {
	Limit  int
	Offset int
	// TenantID scopes the query to one tenant's rows. This is the isolation
	// boundary; Source/Sources only narrow within it. Empty means unscoped, so
	// every caller serving a tenant-facing surface must set it.
	TenantID string
	// Source filters rows whose source contains this substring.
	Source string
	// Sources, when non-empty, takes precedence over Source and matches rows
	// whose source is exactly one of the given values (SQL IN). Used to scope a
	// query to a closed set of sources such as a tenant's keycloak:<realm> and
	// amfa:<realm> feeds; exact matching keeps one tenant's realms from matching
	// another tenant's prefix-sharing realm names.
	Sources   []string
	StartTime *string
	EndTime   *string
	// Type filters rows by exact event type (e.g. "LOGIN").
	Type     string
	Severity string
	// MinRisk filters rows whose risk_level is at least this value. Rows with
	// a NULL risk_level (no AMFA counterpart) never match.
	MinRisk *int
}

// KeysetPosition identifies a row in the keyset ordering (timestamp DESC,
// id DESC). ID is the surrogate primary key, not the event_id string: event
// ids are only unique per tenant and carry no order, while the uint key gives
// rows with equal timestamps a stable tiebreak.
type KeysetPosition struct {
	Timestamp time.Time
	ID        uint
}

// KeysetPage is one page of a keyset-paginated listing. Last is the position
// of the final row (nil for an empty page); callers pass it back as the next
// call's after position to continue where this page ended.
type KeysetPage struct {
	Events []*domain.Event
	Last   *KeysetPosition
}

// Stats holds event counts grouped by classification column, all computed
// under one ListOptions filter scope. Keys are the raw column values,
// including "" for rows that never had the column set.
type Stats struct {
	ByType     map[string]int64
	BySeverity map[string]int64
	BySource   map[string]int64
}

// Repository defines the interface for event persistence operations.
type Repository interface {
	// Save inserts or updates an event in the database.
	Save(ctx context.Context, event *domain.Event) error

	// GetByID retrieves an event by its event ID.
	GetByID(ctx context.Context, eventID string) (*domain.Event, error)

	// GetByTenantAndEventID retrieves one tenant's event by its event ID.
	// Unlike GetByID it filters on both columns, so two tenants monitoring the
	// same Keycloak (and therefore seeing the same event ids) each get only
	// their own row.
	GetByTenantAndEventID(ctx context.Context, tenantID, eventID string) (*domain.Event, error)

	// List retrieves events with pagination and filtering.
	List(ctx context.Context, opts *ListOptions) ([]*domain.Event, error)

	// ListKeyset retrieves events ordered by (timestamp DESC, id DESC) using
	// keyset pagination: only rows strictly after the given position are
	// returned, or the newest rows when after is nil. Uses opts.Limit and the
	// same filters as List; opts.Offset is ignored.
	ListKeyset(ctx context.Context, opts *ListOptions, after *KeysetPosition) (*KeysetPage, error)

	// Stats returns event counts grouped by type, severity and source, all
	// under the same filter scope as List.
	Stats(ctx context.Context, opts *ListOptions) (*Stats, error)

	// Count returns the total number of events.
	Count(ctx context.Context) (int64, error)

	// CountWithFilter returns the count of events matching the filter criteria.
	CountWithFilter(ctx context.Context, opts *ListOptions) (int64, error)

	// AmfaMirrorWatermark returns the AMFA events mirror's recorded read
	// position for one (tenant, realm), or the zero time when none is recorded.
	// Scoped by tenant because realm names are unique per-tenant, not globally.
	AmfaMirrorWatermark(ctx context.Context, tenantID, realm string) (time.Time, error)

	// SaveAmfaMirrorWatermark records the mirror's read position for one
	// (tenant, realm). Monotonic: an older position never replaces a newer one.
	SaveAmfaMirrorWatermark(ctx context.Context, tenantID, realm string, eventTime time.Time) error

	// MergeKeycloakEvent saves a Keycloak-sourced event as its own row via a
	// plain upsert. It does NOT inline-absorb a pre-existing standalone AMFA
	// row that shares the same amfa_event_id — that convergence is handled
	// separately and asynchronously by ReconcileAMFAMerges, so a login whose
	// AMFA half arrived first may briefly exist as two rows until the next
	// reconcile sweep.
	MergeKeycloakEvent(ctx context.Context, event *domain.Event) error

	// MergeAmfaEvent saves an AMFA-sourced event as its own row via a plain
	// upsert. It does NOT check for or write onto an existing Keycloak twin —
	// that convergence is handled separately and asynchronously by
	// ReconcileAMFAMerges, so a login whose Keycloak half arrived first may
	// briefly exist as two rows until the next reconcile sweep.
	MergeAmfaEvent(ctx context.Context, event *domain.Event) error

	// ReconcileAMFAMerges converges standalone AMFA rows onto their Keycloak
	// twin (matched by amfa_event_id) by copying the AMFA-owned fields onto
	// the Keycloak row, then hard-deletes the absorbed AMFA rows. Idempotent
	// and order-independent; returns the number of rows absorbed. AMFA-only
	// events (no Keycloak twin) are left untouched.
	ReconcileAMFAMerges(ctx context.Context) (int64, error)
}
