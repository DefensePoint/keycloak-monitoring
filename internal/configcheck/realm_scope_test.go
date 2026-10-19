package configcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// scopeKeycloak is a Keycloak as the monitoring client sees it: the realms it
// lists, and what reading one realm answers.
type scopeKeycloak struct {
	mu sync.Mutex
	// realms are listed with their settings, in this order.
	realms []*keycloakadmin.RealmRepresentation
	// stubs are listed without an id, as for a realm the client may not view.
	stubs []string
	// lostRoles are not listed, and read as a stub: the client lost its roles.
	lostRoles map[string]bool
	// readErr fails reading a realm, as a 500 would.
	readErr map[string]bool
	// down fails every call, as an unreachable Keycloak or a bad secret does.
	down bool
}

func (k *scopeKeycloak) GetAllRealms(context.Context) ([]*keycloakadmin.RealmRepresentation, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.down {
		return nil, errors.New("invalid_client")
	}
	out := append([]*keycloakadmin.RealmRepresentation{}, k.realms...)
	for _, name := range k.stubs {
		out = append(out, &keycloakadmin.RealmRepresentation{Realm: name})
	}
	return out, nil
}

func (k *scopeKeycloak) GetRealmInfo(_ context.Context, name string) (*keycloakadmin.RealmRepresentation, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	switch {
	case k.down:
		return nil, errors.New("invalid_client")
	case k.readErr[name]:
		return nil, errors.New("failed to get realm info, status: 500")
	case k.lostRoles[name]:
		return &keycloakadmin.RealmRepresentation{Realm: name}, nil
	}
	for _, stub := range k.stubs {
		if stub == name {
			return &keycloakadmin.RealmRepresentation{Realm: name}, nil
		}
	}
	for _, r := range k.realms {
		if r.Realm == name {
			return r, nil
		}
	}
	return nil, fmt.Errorf("failed to get realm info, status: 404: %w", keycloakadmin.ErrNotFound)
}

func (k *scopeKeycloak) GetClients(context.Context, string) ([]*keycloakadmin.ClientRepresentation, error) {
	return nil, nil
}

func (k *scopeKeycloak) GetIdentityProviders(context.Context, string) ([]*keycloakadmin.IdentityProviderRepresentation, error) {
	return nil, nil
}

func (k *scopeKeycloak) set(f func(k *scopeKeycloak)) {
	k.mu.Lock()
	defer k.mu.Unlock()
	f(k)
}

// realm is a realm with brute-force protection off, the one problem checked.
func realm(name string) *keycloakadmin.RealmRepresentation {
	return &keycloakadmin.RealmRepresentation{ID: name + "-id", Realm: name, Enabled: true}
}

// drop removes name from the listed realms.
func (k *scopeKeycloak) drop(name string) {
	k.set(func(k *scopeKeycloak) {
		kept := k.realms[:0]
		for _, r := range k.realms {
			if r.Realm != name {
				kept = append(kept, r)
			}
		}
		k.realms = kept
	})
}

func bruteForceCheck(kc KeycloakClient, tenantID string, realms ...string) *RealmSecurityCheck {
	return NewRealmSecurityCheck(kc, logger.NewNoop(), realms, tenantID, RealmSecurityCheckConfig{CheckBruteForce: true})
}

func TestRealmScope_AlertsOfARealmThatIsGoneResolve(t *testing.T) {
	for _, tc := range []struct {
		name string
		gone func(k *scopeKeycloak)
	}{
		{name: "deleted", gone: func(k *scopeKeycloak) { k.drop("r1") }},
		{name: "disabled", gone: func(k *scopeKeycloak) { k.set(func(k *scopeKeycloak) { k.realms[0].Enabled = false }) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kc := &scopeKeycloak{realms: []*keycloakadmin.RealmRepresentation{realm("r1")}}
			check := bruteForceCheck(kc, "t1")
			id := check.generateAlertID("r1", "brute_force_disabled")
			l := newAlertLife(check)
			l.run(t, 0)

			tc.gone(kc)
			l.run(t, 1)
			if got := l.status(t, id); got != domain.AlertStatusActive {
				t.Fatalf("one run after the realm went: status %s, want active", got)
			}
			l.run(t, 1)
			if got := l.status(t, id); got != domain.AlertStatusResolved {
				t.Fatalf("two runs after the realm went: status %s, want resolved", got)
			}
			if len(l.notifier.resolved) != 1 {
				t.Fatalf("%d resolution notifications, want 1", len(l.notifier.resolved))
			}
		})
	}
}

func TestRealmScope_ARealmEnabledAgainReopensItsAlert(t *testing.T) {
	kc := &scopeKeycloak{realms: []*keycloakadmin.RealmRepresentation{realm("r1")}}
	check := bruteForceCheck(kc, "t1")
	id := check.generateAlertID("r1", "brute_force_disabled")
	l := newAlertLife(check)
	l.run(t, 0)
	kc.set(func(k *scopeKeycloak) { k.realms[0].Enabled = false })
	l.run(t, 1)
	l.run(t, 1)
	kc.set(func(k *scopeKeycloak) { k.realms[0].Enabled = true })
	l.run(t, 1)
	if got := l.status(t, id); got != domain.AlertStatusActive || len(l.notifier.sent) != 2 {
		t.Fatalf("realm enabled again: status %s, %d notifications; want active, 2", got, len(l.notifier.sent))
	}
}

func TestRealmScope_AlertsOfARealmTheClientCannotReadStay(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured []string
		hide       func(k *scopeKeycloak)
	}{
		{name: "lost all roles: unlisted, read as a stub", hide: func(k *scopeKeycloak) {
			k.drop("r1")
			k.set(func(k *scopeKeycloak) { k.lostRoles = map[string]bool{"r1": true} })
		}},
		{name: "no view-realm: listed as a stub", hide: func(k *scopeKeycloak) {
			k.drop("r1")
			k.set(func(k *scopeKeycloak) { k.stubs = []string{"r1"} })
		}},
		{name: "unlisted, reading it fails", hide: func(k *scopeKeycloak) {
			k.drop("r1")
			k.set(func(k *scopeKeycloak) { k.readErr = map[string]bool{"r1": true} })
		}},
		{name: "configured realm read as a stub", configured: []string{"r1"}, hide: func(k *scopeKeycloak) {
			k.set(func(k *scopeKeycloak) { k.lostRoles = map[string]bool{"r1": true} })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kc := &scopeKeycloak{realms: []*keycloakadmin.RealmRepresentation{realm("r1")}}
			check := bruteForceCheck(kc, "t1", tc.configured...)
			id := check.generateAlertID("r1", "brute_force_disabled")
			l := newAlertLife(check)
			l.run(t, 0)

			tc.hide(kc)
			for range 3 {
				l.run(t, 1)
			}
			if got := l.status(t, id); got != domain.AlertStatusActive || len(l.notifier.resolved) != 0 {
				t.Fatalf("realm unreadable: status %s, %d resolutions; want active, none", got, len(l.notifier.resolved))
			}
		})
	}
}

func TestRealmScope_ARealmLeftOutOfTheConfiguredListResolves(t *testing.T) {
	kc := &scopeKeycloak{realms: []*keycloakadmin.RealmRepresentation{realm("r1"), realm("r2")}}
	both := bruteForceCheck(kc, "t1", "r1", "r2")
	id := both.generateAlertID("r2", "brute_force_disabled")
	l := newAlertLife(both)
	l.run(t, 0)

	l.check = bruteForceCheck(kc, "t1", "r1")
	l.run(t, 1)
	l.run(t, 1)
	if got := l.status(t, id); got != domain.AlertStatusResolved {
		t.Fatalf("r2 no longer configured: status %s, want resolved", got)
	}
}

// logLines captures what the service logs about realms it cannot read.
type logLines struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logLines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// take returns the realm lines logged since the last call, as
// "message tenant/realm", and forgets them.
func (l *logLines) take(t *testing.T) []string {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(l.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var e struct {
			Message  string `json:"message"`
			TenantID string `json:"tenant_id"`
			Realm    string `json:"realm"`
		}
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		switch e.Message {
		case cannotViewRealm:
			out = append(out, "cannot "+e.TenantID+"/"+e.Realm)
		case canViewRealm:
			out = append(out, "can "+e.TenantID+"/"+e.Realm)
		}
	}
	l.buf.Reset()
	return out
}

// tenants is one service checking two tenants that both have a realm r1.
type tenants struct {
	svc      *service
	store    *mockAlertStore
	log      *logLines
	kc1, kc2 *scopeKeycloak
}

func newTenants() *tenants {
	kc1 := &scopeKeycloak{realms: []*keycloakadmin.RealmRepresentation{realm("r1")}}
	kc2 := &scopeKeycloak{realms: []*keycloakadmin.RealmRepresentation{realm("r1")}}
	lines := &logLines{}
	store := newMockAlertStore()
	svc := NewService([]Check{bruteForceCheck(kc1, "t1"), bruteForceCheck(kc2, "t2")}, store, &recordingNotifier{},
		logger.New(zerolog.New(lines)), lifecyclePoll).(*service)
	return &tenants{svc: svc, store: store, log: lines, kc1: kc1, kc2: kc2}
}

func (ts *tenants) poll(t *testing.T, want ...string) {
	t.Helper()
	ts.svc.runChecks(context.Background())
	got := ts.log.take(t)
	if strings.Join(sorted(got), ", ") != strings.Join(sorted(want), ", ") {
		t.Fatalf("logged %v, want %v", got, want)
	}
}

func sorted(s []string) []string {
	out := append([]string{}, s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func hideR1(k *scopeKeycloak) {
	k.drop("r1")
	k.set(func(k *scopeKeycloak) { k.stubs = []string{"r1"} })
}

func showR1(k *scopeKeycloak) {
	k.set(func(k *scopeKeycloak) {
		k.stubs = nil
		k.lostRoles = nil
		k.realms = []*keycloakadmin.RealmRepresentation{realm("r1")}
	})
}

func TestRealmScope_AnUnreadableRealmIsLoggedOncePerTenant(t *testing.T) {
	ts := newTenants()
	ts.poll(t)

	hideR1(ts.kc2)
	ts.poll(t, "cannot t2/r1")
	ts.poll(t)
	ts.poll(t)

	hideR1(ts.kc1)
	ts.poll(t, "cannot t1/r1")

	showR1(ts.kc2)
	ts.poll(t, "can t2/r1")
	ts.poll(t)

	showR1(ts.kc1)
	ts.poll(t, "can t1/r1")
}

func TestRealmScope_ALostRolesRealmIsLoggedOnce(t *testing.T) {
	ts := newTenants()
	ts.poll(t)

	ts.kc1.drop("r1")
	ts.kc1.set(func(k *scopeKeycloak) { k.lostRoles = map[string]bool{"r1": true} })
	ts.poll(t, "cannot t1/r1")
	ts.poll(t)

	showR1(ts.kc1)
	ts.poll(t, "can t1/r1")
}

// One tenant's checks failing outright says nothing about its realms, and
// must not hold back what the other tenant's checks found.
func TestRealmScope_AFailingTenantKeepsOnlyItsOwnRealms(t *testing.T) {
	ts := newTenants()
	ts.poll(t)
	hideR1(ts.kc1)
	hideR1(ts.kc2)
	ts.poll(t, "cannot t1/r1", "cannot t2/r1")

	ts.kc2.set(func(k *scopeKeycloak) { k.down = true })
	showR1(ts.kc1)
	ts.poll(t, "can t1/r1")
	ts.poll(t)

	ts.kc2.set(func(k *scopeKeycloak) { k.down = false })
	showR1(ts.kc2)
	ts.poll(t, "can t2/r1")
}

// "Can view again" needs a check to have read the realm: a realm that only
// dropped out of sight is forgotten quietly.
func TestRealmScope_NoRecoveryLoggedForARealmThatOnlyDroppedOut(t *testing.T) {
	t.Run("its alerts resolved in the UI", func(t *testing.T) {
		ts := newTenants()
		ts.poll(t)
		ts.kc1.drop("r1")
		ts.kc1.set(func(k *scopeKeycloak) { k.lostRoles = map[string]bool{"r1": true} })
		ts.poll(t, "cannot t1/r1")

		for _, a := range ts.store.alerts {
			if a.TenantID == "t1" {
				a.Status = domain.AlertStatusResolved
			}
		}
		ts.poll(t)
		ts.poll(t)
	})

	t.Run("deleted while unreadable", func(t *testing.T) {
		ts := newTenants()
		ts.poll(t)
		ts.kc1.drop("r1")
		ts.kc1.set(func(k *scopeKeycloak) { k.lostRoles = map[string]bool{"r1": true} })
		ts.poll(t, "cannot t1/r1")

		ts.kc1.set(func(k *scopeKeycloak) { k.lostRoles = nil })
		ts.poll(t)
		ts.poll(t)
	})
}

// A stub has none of the realm's settings: read as settings, every check
// would see a problem that does not exist.
func TestRealmScope_AStubRaisesNothing(t *testing.T) {
	good := realm("r1")
	good.BruteForceProtected, good.SslRequired = true, "external"
	kc := &scopeKeycloak{realms: []*keycloakadmin.RealmRepresentation{good}}
	check := NewRealmSecurityCheck(kc, logger.NewNoop(), []string{"r1"}, "t1",
		RealmSecurityCheckConfig{CheckBruteForce: true, CheckSSLRequired: true})
	l := newAlertLife(check)
	l.run(t, 0)
	if len(l.store.alerts) != 0 {
		t.Fatalf("well-configured realm raised %d alerts", len(l.store.alerts))
	}

	kc.set(func(k *scopeKeycloak) { k.lostRoles = map[string]bool{"r1": true} })
	l.run(t, 1)
	if len(l.store.alerts) != 0 || len(l.notifier.sent) != 0 {
		t.Fatalf("realm read as a stub raised %d alerts, %d notifications; want none", len(l.store.alerts), len(l.notifier.sent))
	}
}
