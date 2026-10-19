package fx

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

func alertIDFor(clientID, eventError string) string {
	c := &eventAlertConverterImpl{}
	return c.generateAlertID(&domain.KeycloakEvent{
		TenantID:   "tenant-a",
		EventType:  "CLIENT_LOGIN_ERROR",
		RealmName:  "master",
		ClientID:   clientID,
		EventError: eventError,
	})
}

// Anyone can send a token request naming any client_id without
// authenticating, and Keycloak records the name. Keyed on it, each made-up
// name opened its own alert and sent its own notification.
func TestGenerateAlertID_UnknownClientsShareOneAlert(t *testing.T) {
	first := alertIDFor("made-up-1", keycloakClientNotFound)
	for _, client := range []string{"made-up-2", "<!channel>", "", "unknown-client"} {
		if got := alertIDFor(client, keycloakClientNotFound); got != first {
			t.Errorf("client %q not found got its own alert %s, want the shared %s", client, got, first)
		}
	}
}

func TestGenerateAlertID_RealClientsKeepTheirOwnAlert(t *testing.T) {
	a := alertIDFor("frontend", "invalid_client_credentials")
	b := alertIDFor("backend", "invalid_client_credentials")
	if a == b {
		t.Fatal("two real clients share one alert")
	}
	if a == alertIDFor("frontend", keycloakClientNotFound) {
		t.Fatal("a real client shares the unknown-client alert")
	}
	// A real client that happens to be named like the bucket stays separate.
	if alertIDFor("unknown-client", "invalid_client_credentials") == alertIDFor("x", keycloakClientNotFound) {
		t.Fatal("a client named unknown-client was folded into the unknown-client alert")
	}
}

// Real clients keep the IDs they had before, so open alerts are not reopened
// under a new ID after an upgrade.
func TestGenerateAlertID_RealClientIDUnchanged(t *testing.T) {
	// The format every existing event alert was stored under.
	sum := sha256.Sum256([]byte("tenant-a:CLIENT_LOGIN_ERROR:master:frontend"))
	want := fmt.Sprintf("%x", sum[:16])
	if got := alertIDFor("frontend", "invalid_client_credentials"); got != want {
		t.Fatalf("real client alert ID = %s, want the unchanged %s", got, want)
	}
}

func TestGenerateAlertID_UnknownClientsStaySeparatePerRealmAndType(t *testing.T) {
	c := &eventAlertConverterImpl{}
	base := domain.KeycloakEvent{TenantID: "tenant-a", EventType: "CLIENT_LOGIN_ERROR", RealmName: "master", ClientID: "x", EventError: keycloakClientNotFound}
	otherRealm, otherType, otherTenant := base, base, base
	otherRealm.RealmName = "customers"
	otherType.EventType = "LOGIN_ERROR"
	otherTenant.TenantID = "tenant-b"
	ids := map[string]bool{}
	for _, e := range []domain.KeycloakEvent{base, otherRealm, otherType, otherTenant} {
		ids[c.generateAlertID(&e)] = true
	}
	if len(ids) != 4 {
		t.Fatalf("got %d distinct alerts for four tenant/realm/type combinations, want 4", len(ids))
	}
}
