package amfa

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// stubAdmin is an in-memory implementation of the keycloakAdmin interface
// used to drive the adapter through every translation path without a real
// HTTP client.
type stubAdmin struct {
	user *keycloakadmin.UserRepresentation
	err  error

	// Capture of last-call arguments so we can assert plumbing.
	gotRealm  string
	gotUserID string
}

func (s *stubAdmin) GetUserByID(_ context.Context, realmName, userID string) (*keycloakadmin.UserRepresentation, error) {
	s.gotRealm, s.gotUserID = realmName, userID
	if s.err != nil {
		return nil, s.err
	}
	return s.user, nil
}

func TestKeycloakAdapter_PassesThroughUser(t *testing.T) {
	stub := &stubAdmin{
		user: &keycloakadmin.UserRepresentation{
			ID:       "u1",
			Username: "alice",
			Email:    "alice@example.com",
		},
	}
	adapter := NewKeycloakAdapter(stub)

	u, err := adapter.GetUser(context.Background(), "tA", "master", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u == nil {
		t.Fatal("expected non-nil user")
		return
	}
	if u.Username == nil || *u.Username != "alice" {
		t.Errorf("Username = %v, want alice", u.Username)
	}
	if u.Email == nil || *u.Email != "alice@example.com" {
		t.Errorf("Email = %v, want alice@example.com", u.Email)
	}
	if stub.gotRealm != "master" || stub.gotUserID != "u1" {
		t.Errorf("call args = (%q, %q), want (master, u1)", stub.gotRealm, stub.gotUserID)
	}
}

func TestKeycloakAdapter_TranslatesNotFound_FromSentinel(t *testing.T) {
	// Future-proof: even though pkg/keycloakadmin's GetUserByID does not
	// currently return ErrNotFound, the adapter must accept it if/when it
	// does (so the package can be cleaned up later without breaking us).
	stub := &stubAdmin{err: keycloakadmin.ErrNotFound}
	adapter := NewKeycloakAdapter(stub)

	_, err := adapter.GetUser(context.Background(), "tA", "master", "u1")
	if !errors.Is(err, ErrKeycloakUserNotFound) {
		t.Fatalf("err = %v, want ErrKeycloakUserNotFound", err)
	}
}

func TestKeycloakAdapter_TranslatesNotFound_FromStatusString(t *testing.T) {
	// Matches the actual error format used by pkg/keycloakadmin's
	// GetUserByID today: "failed to get user, status: 404, body: ...".
	stub := &stubAdmin{err: fmt.Errorf("failed to get user, status: 404, body: {}")}
	adapter := NewKeycloakAdapter(stub)

	_, err := adapter.GetUser(context.Background(), "tA", "master", "u1")
	if !errors.Is(err, ErrKeycloakUserNotFound) {
		t.Fatalf("err = %v, want ErrKeycloakUserNotFound", err)
	}
}

func TestKeycloakAdapter_PassesThroughOtherErrors(t *testing.T) {
	// A 500 / network error must NOT be translated to ErrKeycloakUserNotFound,
	// because Enrichment would then negative-cache a transient failure.
	transient := errors.New("connection refused")
	stub := &stubAdmin{err: transient}
	adapter := NewKeycloakAdapter(stub)

	_, err := adapter.GetUser(context.Background(), "tA", "master", "u1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrKeycloakUserNotFound) {
		t.Errorf("non-404 error was wrongly translated to ErrKeycloakUserNotFound: %v", err)
	}
	if !errors.Is(err, transient) {
		t.Errorf("err = %v, want wrap of %v", err, transient)
	}
}

func TestKeycloakAdapter_NullifiesEmptyStrings(t *testing.T) {
	stub := &stubAdmin{
		user: &keycloakadmin.UserRepresentation{
			ID:       "u1",
			Username: "",
			Email:    "",
		},
	}
	adapter := NewKeycloakAdapter(stub)

	u, err := adapter.GetUser(context.Background(), "tA", "master", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Username != nil {
		t.Errorf("Username = %v, want nil", *u.Username)
	}
	if u.Email != nil {
		t.Errorf("Email = %v, want nil", *u.Email)
	}
}

func TestKeycloakAdapter_NilUserBecomesNotFound(t *testing.T) {
	// Defensive: if the underlying client somehow returns (nil, nil),
	// the adapter should surface ErrKeycloakUserNotFound rather than
	// panicking on a nil dereference.
	stub := &stubAdmin{user: nil, err: nil}
	adapter := NewKeycloakAdapter(stub)

	_, err := adapter.GetUser(context.Background(), "tA", "master", "u1")
	if !errors.Is(err, ErrKeycloakUserNotFound) {
		t.Fatalf("err = %v, want ErrKeycloakUserNotFound", err)
	}
}
