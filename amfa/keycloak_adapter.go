package amfa

import (
	"context"
	"errors"
	"strings"

	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// keycloakAdmin is the slice of pkg/keycloakadmin this adapter actually uses.
//
// Declared as an interface so tests can stub it without spinning up a real
// HTTP client. The concrete *keycloakadmin.Client satisfies this interface
// directly (it already has a matching GetUserByID method).
//
// Note: pkg/keycloakadmin's client is single-tenant — it has no tenantID
// parameter. The tenantID argument on the outer KeycloakClient.GetUser is
// retained on the adapter's public API for forward compatibility but is
// ignored when calling the underlying client.
type keycloakAdmin interface {
	GetUserByID(ctx context.Context, realmName, userID string) (*keycloakadmin.UserRepresentation, error)
}

// keycloakAdapter adapts *keycloakadmin.Client to amfa.KeycloakClient.
type keycloakAdapter struct {
	admin keycloakAdmin
}

// NewKeycloakAdapter wraps a Keycloak admin client (or any compatible stub)
// and returns it as an amfa.KeycloakClient suitable for use by Enrichment.
//
// On 404 from the underlying client, the adapter translates the error to
// ErrKeycloakUserNotFound so Enrichment can populate its negative cache.
// All other errors propagate unchanged.
func NewKeycloakAdapter(admin keycloakAdmin) KeycloakClient {
	return &keycloakAdapter{admin: admin}
}

// Compile-time interface check.
var _ KeycloakClient = (*keycloakAdapter)(nil)

// GetUser fetches a user from Keycloak by realm + userID and returns the
// minimal KeycloakUser projection KMT needs for AMFA event enrichment.
// The tenantID arg is part of the KeycloakClient contract but is unused
// here because pkg/keycloakadmin is configured per-process, not per-tenant.
func (k *keycloakAdapter) GetUser(ctx context.Context, _ /*tenantID*/, realm, userID string) (*KeycloakUser, error) {
	return getUserViaAdmin(ctx, k.admin, realm, userID)
}

// getUserViaAdmin is the shared implementation used by both the single-tenant
// keycloakAdapter and the multi-tenant poolKeycloakAdapter. It centralises the
// 404 detection, nil-result handling, and KeycloakUser projection so both
// adapters stay in lock-step if any of that logic changes.
func getUserViaAdmin(ctx context.Context, admin keycloakAdmin, realm, userID string) (*KeycloakUser, error) {
	raw, err := admin.GetUserByID(ctx, realm, userID)
	if err != nil {
		if isKCNotFound(err) {
			return nil, ErrKeycloakUserNotFound
		}
		return nil, err
	}
	if raw == nil {
		return nil, ErrKeycloakUserNotFound
	}
	return &KeycloakUser{
		Username: nullIfEmpty(raw.Username),
		Email:    nullIfEmpty(raw.Email),
	}, nil
}

// isKCNotFound detects a 404-equivalent error from pkg/keycloakadmin.
//
// pkg/keycloakadmin exposes ErrNotFound as a sentinel but its current
// GetUserByID implementation returns a wrapped/formatted error containing
// "status: 404" rather than the sentinel. We accept either form so this
// adapter keeps working if/when the package is normalised.
func isKCNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, keycloakadmin.ErrNotFound) {
		return true
	}
	// Fallback: parse the error string. Brittle but matches the current
	// pkg/keycloakadmin.GetUserByID error format ("failed to get user,
	// status: 404, body: ..."). Centralised here so it can be replaced with
	// errors.Is once the package surfaces a typed 404.
	return strings.Contains(err.Error(), "status: 404")
}

// nullIfEmpty returns nil for empty strings so JSON-encoded users don't
// surface empty username/email fields when Keycloak returned no value.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
