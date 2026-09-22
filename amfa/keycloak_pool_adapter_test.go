package amfa

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// stubKeycloakAdmin is a minimal KeycloakAdmin implementation used to verify
// the pool adapter routes calls to the right per-tenant client. Captures the
// last realm/userID it was asked for so we can assert plumbing.
type stubKeycloakAdmin struct {
	id        string // identifier so tests can confirm *which* stub was hit
	user      *keycloakadmin.UserRepresentation
	err       error
	gotRealm  string
	gotUserID string
	callCount int
}

func (s *stubKeycloakAdmin) GetUserByID(_ context.Context, realmName, userID string) (*keycloakadmin.UserRepresentation, error) {
	s.callCount++
	s.gotRealm, s.gotUserID = realmName, userID
	if s.err != nil {
		return nil, s.err
	}
	return s.user, nil
}

// stubLookup is an in-memory KeycloakClientLookup. Returning nil for an
// unknown tenantID is the contract we expect the pool adapter to translate
// into a transient error.
type stubLookup struct {
	clients map[string]KeycloakAdmin
}

func (s *stubLookup) GetKeycloakAdmin(tenantID string) KeycloakAdmin {
	if s == nil || s.clients == nil {
		return nil
	}
	c, ok := s.clients[tenantID]
	if !ok {
		return nil
	}
	return c
}

func TestPoolKeycloakAdapter_DispatchesByTenant(t *testing.T) {
	t.Parallel()

	clientA := &stubKeycloakAdmin{
		id: "A",
		user: &keycloakadmin.UserRepresentation{
			ID:       "u1",
			Username: "alice",
			Email:    "alice@example.com",
		},
	}
	clientB := &stubKeycloakAdmin{
		id: "B",
		user: &keycloakadmin.UserRepresentation{
			ID:       "u2",
			Username: "bob",
			Email:    "bob@example.com",
		},
	}
	lookup := &stubLookup{clients: map[string]KeycloakAdmin{
		"tenantA": clientA,
		"tenantB": clientB,
	}}

	adapter := NewPoolKeycloakAdapter(lookup)

	// Call A: should hit clientA only
	uA, err := adapter.GetUser(context.Background(), "tenantA", "master", "u1")
	if err != nil {
		t.Fatalf("tenantA: unexpected error: %v", err)
	}
	if uA == nil || uA.Username == nil || *uA.Username != "alice" {
		t.Errorf("tenantA: got user %+v, want alice", uA)
	}
	if clientA.callCount != 1 || clientB.callCount != 0 {
		t.Errorf("tenantA call counts: A=%d B=%d, want A=1 B=0", clientA.callCount, clientB.callCount)
	}
	if clientA.gotRealm != "master" || clientA.gotUserID != "u1" {
		t.Errorf("clientA args = (%q, %q), want (master, u1)", clientA.gotRealm, clientA.gotUserID)
	}

	// Call B: should hit clientB only
	uB, err := adapter.GetUser(context.Background(), "tenantB", "realm-b", "u2")
	if err != nil {
		t.Fatalf("tenantB: unexpected error: %v", err)
	}
	if uB == nil || uB.Username == nil || *uB.Username != "bob" {
		t.Errorf("tenantB: got user %+v, want bob", uB)
	}
	if clientA.callCount != 1 || clientB.callCount != 1 {
		t.Errorf("tenantB call counts: A=%d B=%d, want A=1 B=1", clientA.callCount, clientB.callCount)
	}
	if clientB.gotRealm != "realm-b" || clientB.gotUserID != "u2" {
		t.Errorf("clientB args = (%q, %q), want (realm-b, u2)", clientB.gotRealm, clientB.gotUserID)
	}
}

func TestPoolKeycloakAdapter_ReturnsErrorWhenTenantUnknown(t *testing.T) {
	t.Parallel()

	lookup := &stubLookup{clients: map[string]KeycloakAdmin{
		// no entry for "ghost-tenant"
	}}
	adapter := NewPoolKeycloakAdapter(lookup)

	// Must not panic on the nil-client path.
	user, err := adapter.GetUser(context.Background(), "ghost-tenant", "master", "u1")
	if err == nil {
		t.Fatal("expected non-nil error when tenant has no admin client, got nil")
	}
	if user != nil {
		t.Errorf("expected nil user, got %+v", user)
	}

	// Must be a transient-style error, NOT ErrKeycloakUserNotFound — otherwise
	// Enrichment would negative-cache the gap and prevent retry once the
	// tenant's monitor starts.
	if errors.Is(err, ErrKeycloakUserNotFound) {
		t.Errorf("missing-tenant error wrongly translated to ErrKeycloakUserNotFound: %v", err)
	}

	// The error message should mention the tenantID so operators have a
	// chance of correlating logs to misconfiguration.
	if !strings.Contains(err.Error(), "ghost-tenant") {
		t.Errorf("error should mention tenantID, got: %v", err)
	}
}

func TestPoolKeycloakAdapter_TranslatesNotFoundFromUnderlyingClient(t *testing.T) {
	t.Parallel()

	// Per-tenant client returns the pkg/keycloakadmin sentinel. The pool
	// adapter must surface ErrKeycloakUserNotFound just like the single-
	// tenant adapter does (both share getUserViaAdmin).
	notFoundClient := &stubKeycloakAdmin{err: keycloakadmin.ErrNotFound}
	lookup := &stubLookup{clients: map[string]KeycloakAdmin{
		"tenantA": notFoundClient,
	}}
	adapter := NewPoolKeycloakAdapter(lookup)

	_, err := adapter.GetUser(context.Background(), "tenantA", "master", "u1")
	if !errors.Is(err, ErrKeycloakUserNotFound) {
		t.Fatalf("err = %v, want ErrKeycloakUserNotFound", err)
	}
}
