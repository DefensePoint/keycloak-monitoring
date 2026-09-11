// Package amfamirror mirrors a tenant's AMFA login events into the platform
// events table so the unified Events view can serve one interleaved,
// correctly-paginated list of Keycloak and AMFA events. It is the AMFA
// counterpart of the Keycloak event mirror in keycloak/monitor.go.
package amfamirror

import (
	"context"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// RealmsFunc returns the realm names to mirror for the tenant. It matches the
// amfacheck realms contract: Keycloak realm names, which equal the AMFA
// realm_id values recorded in auth_context_json.
type RealmsFunc func(ctx context.Context) ([]string, error)

// EventStore is the subset of events.Repository the mirror needs.
type EventStore interface {
	// MergeAmfaEvent stores an AMFA event (its own row); ReconcileAMFAMerges
	// folds it into the Keycloak twin once both are present.
	MergeAmfaEvent(ctx context.Context, event *domain.Event) error
	// AmfaMirrorWatermark / SaveAmfaMirrorWatermark persist how far this
	// tenant has read for a realm, so a restart resumes in the right place.
	// Keyed by (tenant, realm): realm names are unique per-tenant, not
	// globally, so a realm-only key lets two tenants share one position.
	AmfaMirrorWatermark(ctx context.Context, tenantID, realm string) (time.Time, error)
	SaveAmfaMirrorWatermark(ctx context.Context, tenantID, realm string, eventTime time.Time) error
	// ReconcileAMFAMerges converges AMFA rows onto their Keycloak twin.
	ReconcileAMFAMerges(ctx context.Context) (int64, error)
}

// Enricher resolves AMFA user IDs to Keycloak usernames/emails. Satisfied by
// *amfa.Enrichment. Best-effort: failures leave the identity fields empty.
type Enricher interface {
	BatchEnrich(ctx context.Context, tenantID, realm string, userIDs []string) (map[string]amfa.KeycloakUser, error)
}

// Logger matches the amfacheck.Logger shape so the fx layer can reuse its
// adapter pattern.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
	Debug(msg string, args ...any)
	WithComponent(name string) Logger
}
