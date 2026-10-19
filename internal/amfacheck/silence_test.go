package amfacheck

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// Silences configured under alert_silences must reach the AMFA checks too,
// matched against the alert each check really builds.

func amfaSilences(t *testing.T, m config.AlertSilenceMatchersConfig) alerts.SilenceManager {
	t.Helper()
	sm := alerts.NewSilenceManager(logger.NewNoop())
	if err := sm.LoadSilences(context.Background(), config.AlertSilencesConfig{
		{ID: "rule", Enabled: true, Matchers: m},
	}); err != nil {
		t.Fatalf("LoadSilences: %v", err)
	}
	return sm
}

func (s *fakeAlertStore) size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.existing)
}

// The per-client check records the client as "client" and as an amfa_client
// resource, next to counts that used to make its metadata unreadable to the
// matcher. A client_id rule must still find it.
func TestAMFASilences_AClientRuleSilencesThatClientOnly(t *testing.T) {
	repo := &checkRepoStub{clientCounts: []amfa.ClientEventCount{
		{Client: "test-app", Count: 9}, {Client: "prod-app", Count: 9},
	}}
	store, notifier := newFakeAlertStore(), newFakeNotifier()
	check := NewLoginErrorByClientCheck("t1", singleRealm("master"), repo, 1, 5*time.Minute, testLogger{})
	svc := NewService("t1", []Check{check}, store, notifier, testLogger{}, time.Hour,
		WithSilencer(amfaSilences(t, config.AlertSilenceMatchersConfig{ClientID: "test-*"})))

	if err := svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}
	if n := store.size(); n != 1 {
		t.Fatalf("store holds %d alerts, want only prod-app's", n)
	}
	for id, a := range store.existing {
		if a.ResourceID != "prod-app" {
			t.Errorf("stored alert is for %q, want prod-app", a.ResourceID)
		}
		if notifier.notifs(id) != 1 {
			t.Errorf("prod-app's alert was not notified")
		}
	}
}

func TestAMFASilences_ARealmRuleSilencesTheRealm(t *testing.T) {
	f := newAMFAResolutionFixture()
	f.svc = NewService("t1", []Check{NewLoginErrorCheck("t1", singleRealm("master"), f.repo, 1, 5*time.Minute, testLogger{})},
		f.store, f.notifier, testLogger{}, time.Hour, WithResolveAfter(testResolveAfter),
		WithSilencer(amfaSilences(t, config.AlertSilenceMatchersConfig{RealmName: "master"})))
	f.repo.typeCount = 5
	f.poll(t)
	if n := f.store.size(); n != 0 {
		t.Errorf("store holds %d alerts for a silenced realm", n)
	}
}

type amfaSwitchSilencer struct {
	mu sync.Mutex
	on bool
}

func (s *amfaSwitchSilencer) set(on bool) { s.mu.Lock(); s.on = on; s.mu.Unlock() }
func (s *amfaSwitchSilencer) ShouldSilence(*domain.Alert) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.on, "switch"
}

// A silence hides a condition; it does not clear it.
func TestAMFASilences_ASilenceDoesNotResolveAnOpenAlert(t *testing.T) {
	sw := &amfaSwitchSilencer{}
	f := newAMFAResolutionFixture()
	f.svc = NewService("t1", []Check{NewLoginErrorCheck("t1", singleRealm("master"), f.repo, 1, 5*time.Minute, testLogger{})},
		f.store, f.notifier, testLogger{}, time.Hour, WithResolveAfter(testResolveAfter), WithSilencer(sw))

	f.repo.typeCount = 5
	f.poll(t)
	f.only(t)
	sw.set(true)

	f.age(2 * testResolveAfter)
	f.poll(t)
	if a := f.only(t); a.Status != domain.AlertStatusActive {
		t.Fatalf("status while silenced but still detected = %s, want active", a.Status)
	}
	if n := f.notifier.notifs(f.alertID); n != 1 {
		t.Errorf("%d notifications, want only the one before the silence", n)
	}
	if n := f.notifier.resolutions(f.alertID); n != 0 {
		t.Errorf("%d resolution notifications while the condition still held", n)
	}

	f.repo.typeCount = 0
	f.poll(t)
	if a := f.only(t); a.Status != domain.AlertStatusResolved {
		t.Errorf("status after the condition cleared = %s, want resolved", a.Status)
	}
}
