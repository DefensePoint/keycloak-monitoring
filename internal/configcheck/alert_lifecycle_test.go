package configcheck

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
	"github.com/DefensePoint/keycloak-monitoring/pkg/saferequest"
)

// A configuration-check alert follows its problem: it resolves once two runs
// in a row have read its realm without finding the problem, and a resolved
// alert whose problem is found again reopens and notifies once. The service
// runs every hour here, and a run "an hour later" moves the stored sighting
// back an hour, which must make no difference: runs are counted, not time.

const lifecyclePoll = time.Hour

type alertLife struct {
	svc      *service
	check    Check
	store    *mockAlertStore
	notifier *recordingNotifier
}

func newAlertLife(check Check) *alertLife {
	store := newMockAlertStore()
	notifier := &recordingNotifier{}
	svc := NewService([]Check{check}, store, notifier, logger.NewNoop(), lifecyclePoll).(*service)
	return &alertLife{svc: svc, check: check, store: store, notifier: notifier}
}

// run runs the check after the given number of poll intervals, by moving the
// stored sightings that far into the past.
func (l *alertLife) run(t *testing.T, intervalsLater int) {
	t.Helper()
	for _, a := range l.store.alerts {
		a.LastSeen = a.LastSeen.Add(-time.Duration(intervalsLater) * lifecyclePoll)
	}
	if err := l.svc.executeCheck(context.Background(), l.check); err != nil {
		t.Fatalf("executeCheck: %v", err)
	}
}

func (l *alertLife) status(t *testing.T, alertID string) domain.AlertStatus {
	t.Helper()
	a := l.store.alerts[alertID]
	if a == nil {
		t.Fatalf("no alert %s", alertID)
	}
	return a.Status
}

// bruteForceRealm is a realm whose only checked problem is brute-force
// protection being off.
func bruteForceRealm() (*testKeycloakClient, *RealmSecurityCheck, string) {
	realm := &keycloakadmin.RealmRepresentation{ID: "r1-id", Realm: "r1", Enabled: true}
	kc := &testKeycloakClient{realms: []*keycloakadmin.RealmRepresentation{realm}}
	check := NewRealmSecurityCheck(kc, logger.NewNoop(), []string{"r1"}, "t1", RealmSecurityCheckConfig{CheckBruteForce: true})
	return kc, check, check.generateAlertID("r1", "brute_force_disabled")
}

func TestConfigAlert_ResolvesOnceTheProblemIsFixed(t *testing.T) {
	kc, check, id := bruteForceRealm()
	l := newAlertLife(check)

	l.run(t, 0)
	if got := l.status(t, id); got != domain.AlertStatusActive || len(l.notifier.sent) != 1 {
		t.Fatalf("problem found: status %s, %d notifications; want active, 1", got, len(l.notifier.sent))
	}

	kc.realms[0].BruteForceProtected = true
	l.run(t, 1)
	if got := l.status(t, id); got != domain.AlertStatusActive {
		t.Fatalf("one run without the problem resolved the alert: status %s", got)
	}
	l.run(t, 1)
	if got := l.status(t, id); got != domain.AlertStatusResolved {
		t.Fatalf("two runs without the problem: status %s, want resolved", got)
	}
	if len(l.notifier.resolved) != 1 || l.notifier.resolved[0].AlertID != id {
		t.Fatalf("resolution notifications %+v, want one for %s", l.notifier.resolved, id)
	}

	l.run(t, 1)
	if len(l.notifier.resolved) != 1 || len(l.notifier.sent) != 1 {
		t.Fatalf("a later run notified again: %d raised, %d resolved", len(l.notifier.sent), len(l.notifier.resolved))
	}
}

func TestConfigAlert_ReopensWhenFoundAgain(t *testing.T) {
	for _, tc := range []struct {
		name string
		// resolve leaves the alert resolved with the problem present again.
		resolve func(t *testing.T, l *alertLife, kc *testKeycloakClient, id string)
	}{
		{name: "resolved by hand while the problem persists", resolve: func(_ *testing.T, l *alertLife, _ *testKeycloakClient, id string) {
			l.store.alerts[id].Status = domain.AlertStatusResolved
		}},
		{name: "resolved by the checker, then the problem returns", resolve: func(t *testing.T, l *alertLife, kc *testKeycloakClient, id string) {
			kc.realms[0].BruteForceProtected = true
			l.run(t, 1)
			l.run(t, 1)
			if got := l.status(t, id); got != domain.AlertStatusResolved {
				t.Fatalf("fixed: status %s, want resolved", got)
			}
			kc.realms[0].BruteForceProtected = false
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kc, check, id := bruteForceRealm()
			l := newAlertLife(check)
			l.run(t, 0)
			firstDetected := l.store.alerts[id].FirstDetected

			tc.resolve(t, l, kc, id)
			l.run(t, 1)
			reopened := l.store.alerts[id]
			if reopened.Status != domain.AlertStatusActive {
				t.Fatalf("found again: status %s, want active", reopened.Status)
			}
			if !reopened.FirstDetected.Equal(firstDetected) {
				t.Fatalf("first detected moved from %v to %v", firstDetected, reopened.FirstDetected)
			}
			if len(l.notifier.sent) != 2 || l.notifier.sent[1].AlertID != id {
				t.Fatalf("notifications %d, want a second one for the reopened alert", len(l.notifier.sent))
			}

			l.run(t, 0)
			if len(l.notifier.sent) != 2 {
				t.Fatalf("the reopened alert notified again on the next run: %d notifications", len(l.notifier.sent))
			}
		})
	}
}

func TestConfigAlert_AcknowledgedAndIgnoredKeepTheOperatorsChoice(t *testing.T) {
	for _, tc := range []struct {
		status domain.AlertStatus
		// whenFixed is the status once the problem has been gone two runs.
		whenFixed domain.AlertStatus
	}{
		{status: domain.AlertStatusAcknowled, whenFixed: domain.AlertStatusResolved},
		{status: domain.AlertStatusIgnored, whenFixed: domain.AlertStatusIgnored},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			kc, check, id := bruteForceRealm()
			l := newAlertLife(check)
			l.run(t, 0)
			now := time.Now()
			l.store.alerts[id].Status, l.store.alerts[id].AcknowledgedAt = tc.status, &now

			for range 3 {
				l.run(t, 1)
				if got := l.status(t, id); got != tc.status {
					t.Fatalf("problem persists: status %s, want %s", got, tc.status)
				}
			}
			if len(l.notifier.sent) != 1 {
				t.Fatalf("%d notifications while %s, want only the first", len(l.notifier.sent), tc.status)
			}

			kc.realms[0].BruteForceProtected = true
			l.run(t, 1)
			l.run(t, 1)
			if got := l.status(t, id); got != tc.whenFixed {
				t.Fatalf("problem fixed: status %s, want %s", got, tc.whenFixed)
			}
		})
	}
}

func TestConfigAlert_NotResolvedWhenTheCheckCouldNotRead(t *testing.T) {
	t.Run("Keycloak unreachable", func(t *testing.T) {
		kc, _, id := bruteForceRealm()
		// No realm list: the check asks Keycloak for its realms, and the
		// whole run fails when it cannot.
		check := NewRealmSecurityCheck(kc, logger.NewNoop(), nil, "t1", RealmSecurityCheckConfig{CheckBruteForce: true})
		l := newAlertLife(check)
		l.run(t, 0)

		kc.err = errors.New("connection refused")
		for range 3 {
			l.store.alerts[id].LastSeen = l.store.alerts[id].LastSeen.Add(-lifecyclePoll)
			if err := l.svc.executeCheck(context.Background(), check); err == nil {
				t.Fatal("a run that could not reach Keycloak reported no error")
			}
		}
		if got := l.status(t, id); got != domain.AlertStatusActive || len(l.notifier.resolved) != 0 {
			t.Fatalf("unreachable Keycloak: status %s, %d resolutions; want active, none", got, len(l.notifier.resolved))
		}
	})

	t.Run("realm settings unreadable", func(t *testing.T) {
		kc, check, id := bruteForceRealm()
		l := newAlertLife(check)
		l.run(t, 0)

		kc.realms = nil // GetRealmInfo fails for r1; the check itself runs
		for range 3 {
			l.run(t, 1)
		}
		if got := l.status(t, id); got != domain.AlertStatusActive || len(l.notifier.resolved) != 0 {
			t.Fatalf("unreadable realm: status %s, %d resolutions; want active, none", got, len(l.notifier.resolved))
		}
	})

	t.Run("client list unreadable in one realm", func(t *testing.T) {
		kc := &testKeycloakClient{clients: map[string][]*keycloakadmin.ClientRepresentation{
			"r1": {{ID: "c1-id", ClientID: "app", DirectAccessGrantsEnabled: true}},
		}}
		check := NewClientSecurityCheck(kc, logger.NewNoop(), []string{"r1"}, "t1", ClientSecurityCheckConfig{CheckDirectAccessGrants: true})
		id := check.generateAlertID("r1", "app", "direct_access_grants_enabled")
		l := newAlertLife(check)
		l.run(t, 0)

		failing := &failingClientsKeycloak{testKeycloakClient: kc}
		check.keycloakClient = failing
		for range 3 {
			l.run(t, 1)
		}
		if got := l.status(t, id); got != domain.AlertStatusActive {
			t.Fatalf("unreadable clients: status %s, want active", got)
		}

		// Read again with the grant turned off: the problem is fixed.
		check.keycloakClient = kc
		kc.clients["r1"][0].DirectAccessGrantsEnabled = false
		l.run(t, 1)
		l.run(t, 1)
		if got := l.status(t, id); got != domain.AlertStatusResolved {
			t.Fatalf("clients readable and fixed: status %s, want resolved", got)
		}
	})
}

type failingClientsKeycloak struct{ *testKeycloakClient }

func (failingClientsKeycloak) GetClients(context.Context, string) ([]*keycloakadmin.ClientRepresentation, error) {
	return nil, errors.New("503 from Keycloak")
}

// An identity provider whose metadata cannot be fetched keeps its certificate
// alerts: nobody saw the certificate. Its fetch-error alert resolves once the
// metadata can be read again.
func TestConfigAlert_CertificateAlertKeptWhileMetadataUnreachable(t *testing.T) {
	metadata := &metadataServer{serial: 4242}
	var down atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		metadata.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	kc := &testKeycloakClient{identityProviders: map[string][]*keycloakadmin.IdentityProviderRepresentation{
		"r1": {{Alias: "corp", ProviderID: "saml", InternalID: "idp-1", Config: map[string]string{"metadataDescriptorUrl": "http://metadata.example/idp"}}},
	}}
	check := NewIdentityProviderMetadataCheck(kc, logger.NewNoop(), []string{"r1"}, "t1", 30, 5*time.Second, saferequest.Policy{})
	check.metadataFetcher = fetcherAgainst(srv)
	warningID := check.generateCertAlertID("r1", "corp", "4242")
	fetchErrorID := check.generateCertAlertID("r1", "corp", "fetch-error")
	l := newAlertLife(check)

	metadata.set(time.Now().Add(10 * 24 * time.Hour))
	l.run(t, 0)
	if got := l.status(t, warningID); got != domain.AlertStatusActive {
		t.Fatalf("expiring certificate: status %s, want active", got)
	}

	down.Store(true)
	for range 3 {
		l.run(t, 1)
	}
	if got := l.status(t, warningID); got != domain.AlertStatusActive {
		t.Fatalf("metadata unreachable: certificate alert %s, want still active", got)
	}
	if got := l.status(t, fetchErrorID); got != domain.AlertStatusActive {
		t.Fatalf("metadata unreachable: fetch-error alert %s, want active", got)
	}

	// The metadata is back, with the certificate renewed.
	down.Store(false)
	metadata.set(time.Now().Add(365 * 24 * time.Hour))
	l.run(t, 1)
	l.run(t, 1)
	if got := l.status(t, warningID); got != domain.AlertStatusResolved {
		t.Fatalf("certificate renewed: status %s, want resolved", got)
	}
	if got := l.status(t, fetchErrorID); got != domain.AlertStatusResolved {
		t.Fatalf("metadata readable again: fetch-error alert %s, want resolved", got)
	}
}

// The superseded "expiring soon" warning is resolved by its replacement
// without a resolution notification, and the clearing pass does not send one
// either; the expired alert stays open while the certificate stays expired.
func TestConfigAlert_SupersededWarningIsNotResolvedTwice(t *testing.T) {
	c := newCertTransition(t)
	c.metadata.set(time.Now().Add(10 * 24 * time.Hour))
	c.poll(t)
	c.metadata.set(time.Now().Add(-24 * time.Hour))
	for range 3 {
		for _, a := range c.store.alerts {
			a.LastSeen = a.LastSeen.Add(-c.svc.pollInterval)
		}
		c.poll(t)
	}
	if got := c.store.alerts[c.warningID()].Status; got != domain.AlertStatusResolved {
		t.Fatalf("superseded warning: status %s, want resolved", got)
	}
	if got := c.store.alerts[c.expiredID()].Status; got != domain.AlertStatusActive {
		t.Fatalf("expired certificate: status %s, want active", got)
	}
	if len(c.notifier.resolved) != 0 {
		t.Fatalf("resolution notifications %+v, want none: the replacement carries the news", c.notifier.resolved)
	}
}

// Runs are counted, not time. After a restart or an outage the last sighting
// is long past, and one read that misses the problem must still not be
// enough; a run that could not read the realm neither counts nor starts the
// count over; and a new process counts from zero.
func TestConfigAlert_TwoReadsInARowNotTimeResolve(t *testing.T) {
	t.Run("long gap, one miss", func(t *testing.T) {
		kc, check, id := bruteForceRealm()
		l := newAlertLife(check)
		l.run(t, 0)

		kc.realms[0].BruteForceProtected = true
		l.run(t, 24)
		if got := l.status(t, id); got != domain.AlertStatusActive {
			t.Fatalf("one miss a day after the last sighting: status %s, want active", got)
		}
		l.run(t, 1)
		if got := l.status(t, id); got != domain.AlertStatusResolved {
			t.Fatalf("second miss: status %s, want resolved", got)
		}
	})

	t.Run("an unread run between two misses", func(t *testing.T) {
		kc, check, id := bruteForceRealm()
		l := newAlertLife(check)
		l.run(t, 0)

		kc.realms[0].BruteForceProtected = true
		l.run(t, 1) // miss 1
		fixedRealms := kc.realms
		kc.realms = nil // realm unreadable: neither a miss nor a sighting
		l.run(t, 1)
		if got := l.status(t, id); got != domain.AlertStatusActive {
			t.Fatalf("unread run counted as a miss: status %s", got)
		}
		kc.realms = fixedRealms
		l.run(t, 1) // miss 2
		if got := l.status(t, id); got != domain.AlertStatusResolved {
			t.Fatalf("two misses around an unread run: status %s, want resolved", got)
		}
	})

	t.Run("a sighting starts the count over", func(t *testing.T) {
		kc, check, id := bruteForceRealm()
		l := newAlertLife(check)
		l.run(t, 0)

		kc.realms[0].BruteForceProtected = true
		l.run(t, 1) // miss 1
		kc.realms[0].BruteForceProtected = false
		l.run(t, 1) // found again
		kc.realms[0].BruteForceProtected = true
		l.run(t, 1) // miss 1 again
		if got := l.status(t, id); got != domain.AlertStatusActive {
			t.Fatalf("misses either side of a sighting resolved the alert: status %s", got)
		}
	})

	t.Run("a new process counts from zero", func(t *testing.T) {
		kc, check, id := bruteForceRealm()
		l := newAlertLife(check)
		l.run(t, 0)

		kc.realms[0].BruteForceProtected = true
		l.run(t, 1) // miss 1, then the server restarts
		l.svc = NewService([]Check{check}, l.store, l.notifier, logger.NewNoop(), lifecyclePoll).(*service)
		l.run(t, 1)
		if got := l.status(t, id); got != domain.AlertStatusActive {
			t.Fatalf("first miss after a restart resolved the alert: status %s", got)
		}
		l.run(t, 1)
		if got := l.status(t, id); got != domain.AlertStatusResolved {
			t.Fatalf("second miss after a restart: status %s, want resolved", got)
		}
	})

	t.Run("tenants and checks count apart", func(t *testing.T) {
		// Two tenants with a same-named realm, one service.
		kc1, check1, id1 := bruteForceRealm()
		realm2 := &keycloakadmin.RealmRepresentation{ID: "r1-id", Realm: "r1", Enabled: true}
		kc2 := &testKeycloakClient{realms: []*keycloakadmin.RealmRepresentation{realm2}}
		check2 := NewRealmSecurityCheck(kc2, logger.NewNoop(), []string{"r1"}, "t2", RealmSecurityCheckConfig{CheckBruteForce: true})
		id2 := check2.generateAlertID("r1", "brute_force_disabled")
		store := newMockAlertStore()
		svc := NewService([]Check{check1, check2}, store, &recordingNotifier{}, logger.NewNoop(), lifecyclePoll).(*service)
		runAll := func() {
			for _, c := range []Check{check1, check2} {
				if err := svc.executeCheck(context.Background(), c); err != nil {
					t.Fatalf("executeCheck: %v", err)
				}
			}
		}
		runAll()
		kc1.realms[0].BruteForceProtected = true
		runAll()
		runAll()
		if store.alerts[id1].Status != domain.AlertStatusResolved || store.alerts[id2].Status != domain.AlertStatusActive {
			t.Fatalf("t1 fixed, t2 not: statuses %s and %s, want resolved and active",
				store.alerts[id1].Status, store.alerts[id2].Status)
		}
	})
}

// A silenced problem is not saved, but it is still there: its open alert must
// not resolve as fixed, nor send a resolution.
type silenceAll struct{}

func (silenceAll) ShouldSilence(*domain.Alert) (bool, string) { return true, "all" }

func TestConfigAlert_SilencedProblemIsNotFixed(t *testing.T) {
	_, check, id := bruteForceRealm()
	l := newAlertLife(check)
	l.run(t, 0)

	l.svc.silencer = silenceAll{}
	for range 3 {
		l.run(t, 1)
	}
	if got := l.status(t, id); got != domain.AlertStatusActive || len(l.notifier.resolved) != 0 {
		t.Fatalf("silenced and still present: status %s, %d resolutions; want active, none", got, len(l.notifier.resolved))
	}
}

// Turning a sub-check off stops it looking, it does not fix anything: the
// alerts it raised stay as they are, while the realm's other alerts still
// resolve when their problem is fixed.
func TestConfigAlert_DisabledSubCheckKeepsItsAlerts(t *testing.T) {
	t.Run("realm security", func(t *testing.T) {
		realm := &keycloakadmin.RealmRepresentation{ID: "r1-id", Realm: "r1", Enabled: true}
		kc := &testKeycloakClient{realms: []*keycloakadmin.RealmRepresentation{realm}}
		both := RealmSecurityCheckConfig{CheckBruteForce: true, CheckSSLRequired: true}
		check := NewRealmSecurityCheck(kc, logger.NewNoop(), []string{"r1"}, "t1", both)
		bruteForce := check.generateAlertID("r1", "brute_force_disabled")
		ssl := check.generateAlertID("r1", "ssl_not_required")
		l := newAlertLife(check)
		l.run(t, 0)
		if l.status(t, bruteForce) != domain.AlertStatusActive || l.status(t, ssl) != domain.AlertStatusActive {
			t.Fatal("want both alerts raised")
		}

		// Restarted with brute-force checking off; the SSL problem gets fixed.
		bruteForceOff := both
		bruteForceOff.CheckBruteForce = false
		l.check = NewRealmSecurityCheck(kc, logger.NewNoop(), []string{"r1"}, "t1", bruteForceOff)
		realm.SslRequired = "external"
		for range 3 {
			l.run(t, 1)
		}
		if got := l.status(t, bruteForce); got != domain.AlertStatusActive {
			t.Fatalf("brute-force check off: its alert is %s, want still active", got)
		}
		if got := l.status(t, ssl); got != domain.AlertStatusResolved {
			t.Fatalf("SSL fixed: alert %s, want resolved", got)
		}
		if len(l.notifier.resolved) != 1 || l.notifier.resolved[0].AlertID != ssl {
			t.Fatalf("resolutions %+v, want only the SSL alert's", l.notifier.resolved)
		}
	})

	t.Run("client security", func(t *testing.T) {
		client := &keycloakadmin.ClientRepresentation{ID: "c1-id", ClientID: "app", DirectAccessGrantsEnabled: true, ImplicitFlowEnabled: true}
		kc := &testKeycloakClient{clients: map[string][]*keycloakadmin.ClientRepresentation{"r1": {client}}}
		both := ClientSecurityCheckConfig{CheckDirectAccessGrants: true, CheckImplicitFlow: true}
		check := NewClientSecurityCheck(kc, logger.NewNoop(), []string{"r1"}, "t1", both)
		grants := check.generateAlertID("r1", "app", "direct_access_grants_enabled")
		implicit := check.generateAlertID("r1", "app", "implicit_flow_enabled")
		l := newAlertLife(check)
		l.run(t, 0)

		grantsOff := both
		grantsOff.CheckDirectAccessGrants = false
		l.check = NewClientSecurityCheck(kc, logger.NewNoop(), []string{"r1"}, "t1", grantsOff)
		client.ImplicitFlowEnabled = false
		for range 3 {
			l.run(t, 1)
		}
		if got := l.status(t, grants); got != domain.AlertStatusActive {
			t.Fatalf("direct-grants check off: its alert is %s, want still active", got)
		}
		if got := l.status(t, implicit); got != domain.AlertStatusResolved {
			t.Fatalf("implicit flow fixed: alert %s, want resolved", got)
		}
	})
}

// Every alert type the realm and client checks raise must name the switch of
// the sub-check that raises it; a type looksFor does not know counts as looked
// for, and its alerts would resolve when that sub-check is switched off.
func TestLooksFor_KnowsEveryAlertTypeRaised(t *testing.T) {
	typesOf := func(alerts []*domain.Alert) map[string]bool {
		types := map[string]bool{}
		for _, a := range alerts {
			var meta struct {
				AlertType string `json:"alert_type"`
			}
			if err := json.Unmarshal([]byte(a.Metadata), &meta); err != nil || meta.AlertType == "" {
				t.Fatalf("alert %q has no alert_type in its metadata", a.Title)
			}
			types[meta.AlertType] = true
		}
		return types
	}

	t.Run("realm security", func(t *testing.T) {
		kc := &testKeycloakClient{realms: []*keycloakadmin.RealmRepresentation{
			{ID: "a", Realm: "a", SslRequired: "none", PasswordPolicy: "length(4)", RegistrationAllowed: true, DuplicateEmailsAllowed: true},
			{ID: "b", Realm: "b", SslRequired: "external", EventsEnabled: true},
		}}
		on := RealmSecurityCheckConfig{CheckSSLRequired: true, CheckBruteForce: true, CheckPasswordPolicy: true,
			CheckEmailVerification: true, CheckAdminEvents: true, CheckUserEvents: true, CheckDuplicateEmails: true,
			RequirePasswordComplexity: true}
		alerts, err := NewRealmSecurityCheck(kc, logger.NewNoop(), []string{"a", "b"}, "t1", on).Execute(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		types := typesOf(alerts)
		if len(types) != 10 {
			t.Fatalf("raised %d alert types %v, want all 10: update this test and looksFor together", len(types), types)
		}
		off := NewRealmSecurityCheck(kc, logger.NewNoop(), []string{"a"}, "t1", RealmSecurityCheckConfig{})
		for alertType := range types {
			if off.looksFor(alertType) {
				t.Errorf("looksFor(%q) is true with every sub-check off", alertType)
			}
		}
	})

	t.Run("client security", func(t *testing.T) {
		kc := &testKeycloakClient{clients: map[string][]*keycloakadmin.ClientRepresentation{"r1": {
			{ID: "1", ClientID: "spa", PublicClient: true, ServiceAccountsEnabled: true, DirectAccessGrantsEnabled: true, ImplicitFlowEnabled: true,
				RedirectUris: []string{"https://app.example.com/*", "https://localhost/cb", "http://app.example.com/cb"}},
			{ID: "2", ClientID: "backend"},
		}}}
		on := ClientSecurityCheckConfig{CheckRedirectURIs: true, CheckPublicClients: true, CheckDirectAccessGrants: true,
			CheckClientSecret: true, CheckImplicitFlow: true, CheckStandardFlow: true, ProductionEnvironment: true}
		alerts, err := NewClientSecurityCheck(kc, logger.NewNoop(), []string{"r1"}, "t1", on).Execute(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		types := typesOf(alerts)
		if len(types) != 7 {
			t.Fatalf("raised %d alert types %v, want all 7: update this test and looksFor together", len(types), types)
		}
		off := NewClientSecurityCheck(kc, logger.NewNoop(), []string{"r1"}, "t1", ClientSecurityCheckConfig{})
		for alertType := range types {
			if off.looksFor(alertType) {
				t.Errorf("looksFor(%q) is true with every sub-check off", alertType)
			}
		}
	})
}

// Runs take turns: when the first run outlasts the poll interval, the first
// tick waits for it instead of reading alongside it, which would count two
// misses in one round.
type slowCheck struct {
	inFlight, maxInFlight, runs atomic.Int32
}

func (c *slowCheck) GetCheckType() string   { return "slow" }
func (c *slowCheck) GetDescription() string { return "slow" }
func (c *slowCheck) Execute(context.Context) ([]*domain.Alert, error) {
	n := c.inFlight.Add(1)
	for {
		m := c.maxInFlight.Load()
		if n <= m || c.maxInFlight.CompareAndSwap(m, n) {
			break
		}
	}
	time.Sleep(60 * time.Millisecond)
	c.inFlight.Add(-1)
	c.runs.Add(1)
	return nil, nil
}

func TestService_RunsTakeTurns(t *testing.T) {
	check := &slowCheck{}
	svc := NewService([]Check{check}, newMockAlertStore(), nil, logger.NewNoop(), 10*time.Millisecond)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	_ = svc.Stop()
	if check.runs.Load() < 2 {
		t.Fatalf("only %d runs; the test needs runs that outlast the interval", check.runs.Load())
	}
	if got := check.maxInFlight.Load(); got != 1 {
		t.Fatalf("%d runs at once, want them to take turns", got)
	}
}
