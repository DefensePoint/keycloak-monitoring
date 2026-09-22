package amfa

import (
	"context"
	"time"
)

// Repository abstracts the data layer for one tenant's AMFA database.
// Implementations should be read-only.
type Repository interface {
	ListEvents(ctx context.Context, opts ListEventsOptions) (*ListEventsResult, error)
	GetStats(ctx context.Context, opts StatsOptions) (Stats, error)
	GetGeoBuckets(ctx context.Context, opts GeoOptions) ([]GeoBucket, error)

	// ListRejectedEventsSince returns events whose pre_auth_risk_decision = 4
	// (rejected by the risk engine) with event_time strictly newer than `since`,
	// for the named realm, oldest first. Used by rule 1 (RiskRejectedCheck).
	ListRejectedEventsSince(ctx context.Context, realm string, since time.Time) ([]EventRow, error)

	// ListVPNRiskyEventsSince returns events with is_vpn = true AND
	// pre_auth_risk_decision >= minRisk (inclusive) and event_time strictly
	// newer than `since`, for the named realm, oldest first. Used by rule 3
	// (VPNRiskyCheck); callers pass minRisk=3 for the "risk > 2" semantics.
	ListVPNRiskyEventsSince(ctx context.Context, realm string, since time.Time, minRisk int) ([]EventRow, error)

	// CountRepeatedRiskyByUser returns, per user in the realm, the count of
	// events with pre_auth_risk_decision >= minRisk (inclusive) in [since, now),
	// keeping only users whose count >= threshold. Used by rule 2; callers pass
	// minRisk=2.
	CountRepeatedRiskyByUser(ctx context.Context, realm string, since time.Time, minRisk, threshold int) ([]UserRiskyCount, error)

	// CountByEventTypeInWindow returns the count of events of the given type in
	// [start, end] for the realm. Used by rules 4a and 4b.
	CountByEventTypeInWindow(ctx context.Context, realm, eventType string, start, end time.Time) (int64, error)

	// ListEventsSince returns all events with event_time strictly newer than
	// `since` for the realm, oldest first, capped at `limit` rows (a limit <= 0
	// selects an implementation default). Used by the unified-events mirror to
	// incrementally pull new AMFA events for copying into the platform events
	// table.
	ListEventsSince(ctx context.Context, realm string, since time.Time, limit int) ([]EventRow, error)

	// CountByEventTypeAndClientInWindow returns, per OAuth client in the realm,
	// the count of events of the given type in [start, end], keeping only
	// clients whose count >= threshold. Used by the per-client login-error
	// rule (amfa-login-error-by-client).
	CountByEventTypeAndClientInWindow(ctx context.Context, realm, eventType string, start, end time.Time, threshold int) ([]ClientEventCount, error)

	// CountDistinctRejectedUsersSince returns the number of distinct users with
	// at least one pre_auth_risk_decision=4 (rejected) event in the realm with
	// event_time >= since. Used by the realm-wide reject-burst rule
	// (amfa-realm-reject-burst).
	CountDistinctRejectedUsersSince(ctx context.Context, realm string, since time.Time) (int64, error)

	// CountByEventTypeByUser returns, per user in the realm, the count of
	// events of the given type in [since, now), keeping only users whose count
	// >= threshold. Used by the per-account login-error rule
	// (amfa-login-error-by-account).
	CountByEventTypeByUser(ctx context.Context, realm, eventType string, since time.Time, threshold int) ([]UserRiskyCount, error)
}

// Service is the orchestration layer used by HTTP handlers.
// It dispatches to the correct per-tenant repository and applies Keycloak
// user enrichment for events.
type Service interface {
	ListEvents(ctx context.Context, tenantID string, opts ListEventsOptions) ([]Event, int64, error)
	GetStats(ctx context.Context, tenantID string, opts StatsOptions) (Stats, error)
	GetGeoBuckets(ctx context.Context, tenantID string, opts GeoOptions) ([]GeoBucket, error)
}
