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
	// whose source is exactly one of the given values (SQL IN). Used to scope a
	// query to a closed set of sources such as a tenant's keycloak:<realm> and
	// amfa:<realm> feeds; exact matching keeps one tenant's realms from matching
	// another tenant's prefix-sharing realm names.
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
