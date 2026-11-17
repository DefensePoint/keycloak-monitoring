package configcheck

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// secretToday is the day the tests run on, at an hour that is not midnight.
var secretToday = time.Date(2027, 3, 1, 15, 30, 0, 0, time.UTC)

func secretDay(daysFromToday int) time.Time {
	return time.Date(2027, 3, 1+daysFromToday, 0, 0, 0, 0, time.UTC)
}

func secretKeycloak() *testKeycloakClient {
	return &testKeycloakClient{
		realms: []*keycloakadmin.RealmRepresentation{
			{ID: "r1-id", Realm: "customers", Enabled: true},
			{ID: "r2-id", Realm: "archive", Enabled: false},
		},
		identityProviders: map[string][]*keycloakadmin.IdentityProviderRepresentation{
			"customers": {{Alias: "entra", DisplayName: "Entra ID", ProviderID: "oidc", InternalID: "idp-entra"}},
		},
	}
}

func newSecretCheck(kc KeycloakClient, warningDays int, secrets ...IdentityProviderSecretExpiry) *IdentityProviderSecretExpiryCheck {
	c := NewIdentityProviderSecretExpiryCheck(kc, logger.NewNoop(), "t1", warningDays, secrets)
	c.now = func() time.Time { return secretToday }
	return c
}

func entra(expiresOn time.Time) IdentityProviderSecretExpiry {
	return IdentityProviderSecretExpiry{Realm: "customers", IdentityProvider: "entra", ExpiresOn: expiresOn}
}

func evaluate(t *testing.T, c *IdentityProviderSecretExpiryCheck) Evaluation {
	t.Helper()
	eval, err := c.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return eval
}

func TestSecretExpiry_NoAlertOutsideTheWarningWindow(t *testing.T) {
	eval := evaluate(t, newSecretCheck(secretKeycloak(), 30, entra(secretDay(31))))

	if len(eval.Alerts) != 0 {
		t.Fatalf("alerts = %+v, want none 31 days before expiry", eval.Alerts)
	}
	if !eval.EvaluatedRealms["customers"] {
		t.Fatal("the realm was not marked as read, so a warning raised earlier would never resolve")
	}
}

func TestSecretExpiry_WarnsInsideTheWindow(t *testing.T) {
	for _, days := range []int{30, 10, 0} {
		eval := evaluate(t, newSecretCheck(secretKeycloak(), 30, entra(secretDay(days))))

		if len(eval.Alerts) != 1 {
			t.Fatalf("%d days left: alerts = %d, want 1", days, len(eval.Alerts))
		}
		a := eval.Alerts[0]
		if a.Severity != domain.AlertSeverityWarning || a.Type != domain.AlertTypeIdentityProvider ||
			a.ResourceType != "identity_provider" || a.ResourceID != "idp-entra" || a.ResourceName != "entra" ||
			a.RealmName != "customers" || a.CheckType != "identity_provider_secret_expiry" || a.TenantID != "t1" {
			t.Fatalf("%d days left: alert %+v", days, a)
		}
		date := secretDay(days).Format(time.DateOnly)
		if !strings.Contains(a.Title, "Entra ID") || !strings.Contains(a.Description, date) {
			t.Fatalf("%d days left: title %q / description %q do not name the provider and %s", days, a.Title, a.Description, date)
		}
		if !strings.Contains(a.Metadata, `"days_until_expiry":`+strconv.Itoa(days)) {
			t.Fatalf("%d days left: metadata %s", days, a.Metadata)
		}
	}
}

// expires_on is the last day the secret is valid: on that day it still only
// warns, and from the next day on it has expired.
func TestSecretExpiry_ExpiredFromTheDayAfter(t *testing.T) {
	eval := evaluate(t, newSecretCheck(secretKeycloak(), 30, entra(secretDay(-1))))

	if len(eval.Alerts) != 1 {
		t.Fatalf("alerts = %d, want 1", len(eval.Alerts))
	}
	a := eval.Alerts[0]
	if a.Severity != domain.AlertSeverityCritical || !strings.Contains(a.Title, "has expired") {
		t.Fatalf("alert severity %s title %q, want a critical expired alert", a.Severity, a.Title)
	}
	warning := newSecretCheck(secretKeycloak(), 30).warningAlertID("customers", "entra", secretDay(-1))
	if len(a.Supersedes) != 1 || a.Supersedes[0] != warning || a.AlertID == warning {
		t.Fatalf("expired alert %s supersedes %v, want its own ID replacing the warning %s", a.AlertID, a.Supersedes, warning)
	}
}

func TestSecretExpiry_AlertsWhenTheProviderOrRealmDoesNotExist(t *testing.T) {
	for _, tc := range []struct{ name, realm, alias string }{
		{name: "provider renamed or deleted", realm: "customers", alias: "okta"},
		{name: "realm that does not exist", realm: "nowhere", alias: "entra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kc := &realmProbeClient{testKeycloakClient: secretKeycloak(), probe: keycloakadmin.ErrNotFound}
			eval := evaluate(t, newSecretCheck(kc, 30,
				IdentityProviderSecretExpiry{Realm: tc.realm, IdentityProvider: tc.alias, ExpiresOn: secretDay(200)}))

			if len(eval.Alerts) != 1 {
				t.Fatalf("alerts = %d, want 1", len(eval.Alerts))
			}
			a := eval.Alerts[0]
			if a.Severity != domain.AlertSeverityWarning || !strings.Contains(a.Title, "not found") ||
				a.RealmName != tc.realm || a.ResourceName != tc.alias {
				t.Fatalf("alert %+v, want a not-found warning for %s/%s", a, tc.realm, tc.alias)
			}
		})
	}
}

// A disabled realm is not checked, as with every other check: its alerts
// resolve with it.
func TestSecretExpiry_SkipsADisabledRealm(t *testing.T) {
	eval := evaluate(t, newSecretCheck(secretKeycloak(), 30,
		IdentityProviderSecretExpiry{Realm: "archive", IdentityProvider: "entra", ExpiresOn: secretDay(-5)}))

	if len(eval.Alerts) != 0 {
		t.Fatalf("alerts = %+v, want none for a disabled realm", eval.Alerts)
	}
}

// A realm whose providers could not be listed is not marked as read, so the
// alerts raised for it before stay open.
func TestSecretExpiry_UnreadableRealmKeepsItsAlerts(t *testing.T) {
	kc := &failingProvidersClient{testKeycloakClient: secretKeycloak()}
	eval := evaluate(t, newSecretCheck(kc, 30, entra(secretDay(5))))

	if len(eval.Alerts) != 0 || eval.EvaluatedRealms["customers"] {
		t.Fatalf("alerts %d, realm read %v; want none and not read", len(eval.Alerts), eval.EvaluatedRealms["customers"])
	}
}

func TestSecretExpiry_KeycloakUnreachableFailsTheRun(t *testing.T) {
	kc := secretKeycloak()
	kc.err = errors.New("connection refused")
	if _, err := newSecretCheck(kc, 30, entra(secretDay(5))).Evaluate(context.Background()); err == nil {
		t.Fatal("Evaluate succeeded without reaching Keycloak")
	}
}

// Through the real service: a renewed secret, its new date written in the
// configuration, resolves the warning, and expiry replaces the warning.
func TestSecretExpiry_Lifecycle(t *testing.T) {
	store := newMockAlertStore()
	notifier := &recordingNotifier{}
	run := func(c *IdentityProviderSecretExpiryCheck) {
		t.Helper()
		svc := NewService([]Check{c}, store, notifier, logger.NewNoop(), time.Hour).(*service)
		if err := svc.executeCheck(context.Background(), c); err != nil {
			t.Fatalf("executeCheck: %v", err)
		}
	}
	soon := newSecretCheck(secretKeycloak(), 30, entra(secretDay(10)))
	warningID := soon.warningAlertID("customers", "entra", secretDay(10))

	run(soon)
	if a := store.alerts[warningID]; a == nil || a.Status != domain.AlertStatusActive || len(notifier.sent) != 1 {
		t.Fatalf("warning %+v, notifications %d; want one active warning, notified once", a, len(notifier.sent))
	}

	// The date passes without a renewal.
	expired := newSecretCheck(secretKeycloak(), 30, entra(secretDay(10)))
	expired.now = func() time.Time { return secretDay(11).Add(time.Hour) }
	run(expired)
	expiredID := expired.expiredAlertID("customers", "entra", secretDay(10))
	if a := store.alerts[expiredID]; a == nil || a.Severity != domain.AlertSeverityCritical || a.Status != domain.AlertStatusActive {
		t.Fatalf("expired alert %+v, want active critical", a)
	}
	if got := store.alerts[warningID].Status; got != domain.AlertStatusResolved {
		t.Fatalf("superseded warning status %s, want resolved", got)
	}

	// Renewed: the operator writes the new date, and two runs later the
	// expired alert resolves.
	renewed := newSecretCheck(secretKeycloak(), 30, entra(secretDay(400)))
	renewed.now = expired.now
	svc := NewService([]Check{renewed}, store, notifier, logger.NewNoop(), time.Hour).(*service)
	for range 2 {
		if err := svc.executeCheck(context.Background(), renewed); err != nil {
			t.Fatalf("executeCheck: %v", err)
		}
	}
	if got := store.alerts[expiredID].Status; got != domain.AlertStatusResolved {
		t.Fatalf("expired alert status %s after the renewal, want resolved", got)
	}
}

type failingProvidersClient struct{ *testKeycloakClient }

func (c *failingProvidersClient) GetIdentityProviders(context.Context, string) ([]*keycloakadmin.IdentityProviderRepresentation, error) {
	return nil, errors.New("403 forbidden")
}

// A realm with no listed secret has nothing to warn about: it counts as read,
// so an alert left from an entry since removed resolves.
func TestSecretExpiry_RealmWithoutEntriesCountsAsRead(t *testing.T) {
	kc := secretKeycloak()
	kc.realms = append(kc.realms, &keycloakadmin.RealmRepresentation{ID: "r3-id", Realm: "staff", Enabled: true})

	eval := evaluate(t, newSecretCheck(kc, 30, entra(secretDay(100))))
	if !eval.EvaluatedRealms["customers"] || !eval.EvaluatedRealms["staff"] {
		t.Fatalf("realms read = %v, want customers and staff", eval.EvaluatedRealms)
	}

	eval = evaluate(t, newSecretCheck(kc, 30))
	if !eval.EvaluatedRealms["customers"] || !eval.EvaluatedRealms["staff"] || eval.EvaluatedRealms["archive"] {
		t.Fatalf("no entries: realms read = %v, want the enabled ones", eval.EvaluatedRealms)
	}
}

func TestSecretExpiry_RemovedEntryResolvesItsAlert(t *testing.T) {
	store := newMockAlertStore()
	notifier := &recordingNotifier{}
	listed := newSecretCheck(secretKeycloak(), 30, entra(secretDay(10)))
	svc := NewService([]Check{listed}, store, notifier, logger.NewNoop(), time.Hour).(*service)
	if err := svc.executeCheck(context.Background(), listed); err != nil {
		t.Fatalf("executeCheck: %v", err)
	}
	warningID := listed.warningAlertID("customers", "entra", secretDay(10))
	if store.alerts[warningID] == nil {
		t.Fatal("no warning raised")
	}

	removed := newSecretCheck(secretKeycloak(), 30)
	svc = NewService([]Check{removed}, store, notifier, logger.NewNoop(), time.Hour).(*service)
	for range 2 {
		if err := svc.executeCheck(context.Background(), removed); err != nil {
			t.Fatalf("executeCheck: %v", err)
		}
	}
	if got := store.alerts[warningID].Status; got != domain.AlertStatusResolved {
		t.Fatalf("warning status %s after its entry was removed, want resolved", got)
	}
}

// Keycloak lists only the realms the monitoring client can see. A realm left
// out of the list that still answers exists: it is unreadable, not missing,
// and an error other than 404 says nothing either way.
func TestSecretExpiry_RealmTheClientCannotSeeIsNotReportedMissing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		probe error
	}{
		{name: "realm answers", probe: nil},
		{name: "probe fails", probe: errors.New("503 service unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kc := &realmProbeClient{testKeycloakClient: secretKeycloak(), probe: tc.probe}
			eval := evaluate(t, newSecretCheck(kc, 30,
				IdentityProviderSecretExpiry{Realm: "hidden", IdentityProvider: "entra", ExpiresOn: secretDay(-5)}))

			if len(eval.Alerts) != 0 || eval.EvaluatedRealms["hidden"] {
				t.Fatalf("alerts %+v, realm read %v; want none and not read", eval.Alerts, eval.EvaluatedRealms["hidden"])
			}
		})
	}
}

// realmProbeClient answers GetRealmInfo for an unlisted realm with probe: nil
// for a realm that exists, keycloakadmin.ErrNotFound for one that does not.
type realmProbeClient struct {
	*testKeycloakClient
	probe error
}

func (c *realmProbeClient) GetRealmInfo(_ context.Context, realm string) (*keycloakadmin.RealmRepresentation, error) {
	if c.probe != nil {
		return nil, c.probe
	}
	return &keycloakadmin.RealmRepresentation{Realm: realm}, nil
}
