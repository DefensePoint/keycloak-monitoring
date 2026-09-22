package amfa

import (
	"context"
	"fmt"

	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// KeycloakAdmin is the exported, multi-tenant counterpart of the package-
// private keycloakAdmin interface. It exposes the single method the AMFA
// enrichment path needs from pkg/keycloakadmin. Defining it as an exported
// type lets the fx wiring (in internal/fx) return a value that satisfies
// this interface without depending on package-private symbols, while still
// reusing getUserViaAdmin's translation logic via the implicit interface
// match (KeycloakAdmin's method set is a superset of keycloakAdmin's).
type KeycloakAdmin interface {
	GetUserByID(ctx context.Context, realmName, userID string) (*keycloakadmin.UserRepresentation, error)
}

// KeycloakClientLookup is the small dependency the pool adapter needs.
// It is intentionally defined in the amfa package (rather than internal/fx
// or tenant) so the adapter can stay free of upstream import cycles, and
// so tests can stub it without pulling in the real monitor pool.
//
// Implementations resolve a tenantID to a per-tenant Keycloak admin client.
// Returning nil signals "no admin client available for this tenant" (for
// example, the tenant is disabled or its monitor hasn't started yet). The
// pool adapter turns that into a transient error which Enrichment will
// skip without negative-caching.
type KeycloakClientLookup interface {
	GetKeycloakAdmin(tenantID string) KeycloakAdmin
}

// poolKeycloakAdapter implements KeycloakClient by looking up the right
// per-tenant admin client at call time. Use this when the underlying
// admin clients are owned by a pool (e.g., KMT's MonitorPoolManager)
// rather than constructed per-request. Because resolution happens on every
// call, changes to the underlying pool (tenants added or removed) are
// picked up automatically without re-wiring the adapter.
type poolKeycloakAdapter struct {
	lookup KeycloakClientLookup
}

// NewPoolKeycloakAdapter wraps a tenant-keyed Keycloak admin lookup as an
// amfa.KeycloakClient. The returned adapter resolves the underlying admin
// client at call time.
func NewPoolKeycloakAdapter(lookup KeycloakClientLookup) KeycloakClient {
	return &poolKeycloakAdapter{lookup: lookup}
}

// Compile-time interface check.
var _ KeycloakClient = (*poolKeycloakAdapter)(nil)

// GetUser resolves the admin client for tenantID and delegates to the shared
// getUserViaAdmin helper. When no client is available for the tenant we
// return a non-sentinel error so Enrichment treats it as transient (no
// negative cache); the next request will retry.
func (p *poolKeycloakAdapter) GetUser(ctx context.Context, tenantID, realm, userID string) (*KeycloakUser, error) {
	admin := p.lookup.GetKeycloakAdmin(tenantID)
	if admin == nil {
		// Not ErrKeycloakUserNotFound: this is "we have nothing to ask",
		// not "user definitely does not exist". Treating it as transient
		// keeps the door open for retry once the tenant's monitor starts.
		return nil, fmt.Errorf("amfa pool adapter: no keycloak admin client for tenant %q", tenantID)
	}
	// KeycloakAdmin's method set is a superset of the package-private
	// keycloakAdmin interface getUserViaAdmin accepts, so it implements
	// that interface implicitly.
	return getUserViaAdmin(ctx, admin, realm, userID)
}
