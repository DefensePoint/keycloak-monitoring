package configcheck

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// A realm_name silence keeps the realm and client security alerts of that
// realm from being stored or notified, as in the event and AMFA checkers.
func TestConfigChecks_SilencedRealmGetsNothing(t *testing.T) {
	silences := alerts.NewSilenceManager(logger.NewNoop())
	if err := silences.LoadSilences(context.Background(), config.AlertSilencesConfig{
		{ID: "quiet-r1", Enabled: true, Matchers: config.AlertSilenceMatchersConfig{RealmName: "r1"}},
	}); err != nil {
		t.Fatalf("LoadSilences: %v", err)
	}

	client := &keycloakadmin.ClientRepresentation{ClientID: "web", Enabled: true, DirectAccessGrantsEnabled: true}
	kc := &testKeycloakClient{
		realms: []*keycloakadmin.RealmRepresentation{
			{ID: "r1-id", Realm: "r1", Enabled: true, SslRequired: "none"},
			{ID: "r2-id", Realm: "r2", Enabled: true, SslRequired: "none"},
		},
		clients: map[string][]*keycloakadmin.ClientRepresentation{"r1": {client}, "r2": {client}},
	}
	log := logger.NewNoop()
	checks := []Check{
		NewRealmSecurityCheck(kc, log, nil, "t1", RealmSecurityCheckConfig{CheckSSLRequired: true}),
		NewClientSecurityCheck(kc, log, nil, "t1", ClientSecurityCheckConfig{CheckDirectAccessGrants: true}),
	}
	store := newMockAlertStore()
	notifier := &recordingNotifier{}
	svc := NewService(checks, store, notifier, log, time.Hour, WithSilencer(silences)).(*service)

	for _, check := range checks {
		if err := svc.executeCheck(context.Background(), check); err != nil {
			t.Fatalf("executeCheck %s: %v", check.GetCheckType(), err)
		}
	}

	stored := map[string]int{}
	for _, a := range store.alerts {
		stored[a.RealmName]++
	}
	notified := map[string]int{}
	for _, a := range notifier.sent {
		notified[a.RealmName]++
	}
	if stored["r1"] != 0 || notified["r1"] != 0 {
		t.Fatalf("silenced realm r1: %d stored, %d notified; want none", stored["r1"], notified["r1"])
	}
	if stored["r2"] != 2 || notified["r2"] != 2 {
		t.Fatalf("realm r2: %d stored, %d notified; want its realm and client alert, each notified", stored["r2"], notified["r2"])
	}
}
