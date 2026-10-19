package eventscheck

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// Silences configured under alert_silences must reach the threshold checks
// too. These run the real login error check and the real silence manager, so
// a rule is matched against the alert the check actually builds.

func newSilencedFixture(t *testing.T, silencer Silencer, realms ...string) *resolutionFixture {
	t.Helper()
	f := &resolutionFixture{
		counts:   &realmCounts{counts: map[string]int64{}, errs: map[string]error{}},
		store:    newMemStore(),
		notifier: &countingNotifier{},
	}
	check := NewLoginErrorCheck(f.counts, discardLogger{}, "t1", staticRealms(realms...), 1, 5*time.Minute)
	f.svc = NewService([]Check{check}, f.store, f.notifier, discardLogger{}, time.Hour,
		WithResolveAfter(testResolveAfter), WithSilencer(silencer))
	return f
}

func silencesFrom(t *testing.T, rules ...config.AlertSilenceConfig) alerts.SilenceManager {
	t.Helper()
	sm := alerts.NewSilenceManager(logger.NewNoop())
	for i := range rules {
		rules[i].Enabled = true
		if rules[i].ID == "" {
			rules[i].ID = "rule"
		}
	}
	if err := sm.LoadSilences(context.Background(), config.AlertSilencesConfig(rules)); err != nil {
		t.Fatalf("LoadSilences: %v", err)
	}
	return sm
}

func (m *memStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.alerts)
}

func TestSilences_ASilencedRealmIsNeitherStoredNorNotified(t *testing.T) {
	sm := silencesFrom(t, config.AlertSilenceConfig{
		Matchers: config.AlertSilenceMatchersConfig{RealmName: "quiet-*"},
	})
	f := newSilencedFixture(t, sm, "quiet-test", "loud")

	f.counts.set("quiet-test", 5)
	f.counts.set("loud", 5)
	f.poll(t)

	if n := f.store.count(); n != 1 {
		t.Fatalf("store holds %d alerts, want only the unsilenced realm's", n)
	}
	f.store.only(t, "loud")
	if raised, _ := f.notifier.counts(); raised != 1 {
		t.Errorf("%d notifications, want 1 for the unsilenced realm", raised)
	}
}

// The check stores numbers next to the names a rule matches on. Those numbers
// used to make the whole metadata unreadable to the matcher, so a rule on
// event type or any metadata field never matched a threshold alert.
func TestSilences_RulesOnTheEventTypeMatchThresholdAlerts(t *testing.T) {
	for name, m := range map[string]config.AlertSilenceMatchersConfig{
		"alert_type":    {AlertType: "LOGIN_ERROR"},
		"field_matches": {FieldMatches: map[string]string{"event_type": "LOGIN_*", "threshold": "1"}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSilencedFixture(t, silencesFrom(t, config.AlertSilenceConfig{Matchers: m}), "master")
			f.counts.set("master", 5)
			f.poll(t)
			if n := f.store.count(); n != 0 {
				t.Errorf("a %s rule naming the event did not silence the alert", name)
			}
		})
	}
}

func TestSilences_ARuleForAnotherTenantSilencesNothing(t *testing.T) {
	sm := silencesFrom(t, config.AlertSilenceConfig{
		TenantID: "other", Matchers: config.AlertSilenceMatchersConfig{RealmName: "*"},
	})
	f := newSilencedFixture(t, sm, "master")
	f.counts.set("master", 5)
	f.poll(t)
	f.store.only(t, "master")
}

// switchSilencer silences everything while on.
type switchSilencer struct {
	mu sync.Mutex
	on bool
}

func (s *switchSilencer) set(on bool) { s.mu.Lock(); s.on = on; s.mu.Unlock() }
func (s *switchSilencer) ShouldSilence(*domain.Alert) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.on, "switch"
}

// A silence that starts while an alert is open hides the condition; it does
// not mean the condition cleared. The alert must stay open and unannounced,
// and resolve only once the condition really goes away.
func TestSilences_ASilenceDoesNotResolveAnOpenAlert(t *testing.T) {
	sw := &switchSilencer{}
	f := newSilencedFixture(t, sw, "master")

	f.counts.set("master", 5)
	f.poll(t)
	sw.set(true)

	// Long past the quiet period, still detected, silenced.
	f.store.age("master", 2*testResolveAfter)
	f.poll(t)
	if a := f.store.only(t, "master"); a.Status != domain.AlertStatusActive {
		t.Fatalf("status while silenced but still detected = %s, want active", a.Status)
	}
	if raised, resolutions := f.notifier.counts(); raised != 1 || resolutions != 0 {
		t.Errorf("notifications while silenced: %d raised, %d resolved; want only the first", raised, resolutions)
	}

	// The condition clears for real: the alert resolves as usual.
	f.counts.set("master", 0)
	f.poll(t)
	if a := f.store.only(t, "master"); a.Status != domain.AlertStatusResolved {
		t.Errorf("status after the condition cleared = %s, want resolved", a.Status)
	}
}

// Once the silence ends, a condition that is still there is handled as it
// would have been: stored again and announced again.
func TestSilences_AnAlertReturnsWhenTheSilenceEnds(t *testing.T) {
	sw := &switchSilencer{on: true}
	f := newSilencedFixture(t, sw, "master")

	f.counts.set("master", 5)
	f.poll(t)
	if n := f.store.count(); n != 0 {
		t.Fatalf("store holds %d alerts during the silence", n)
	}

	sw.set(false)
	f.poll(t)
	f.store.only(t, "master")
	if raised, _ := f.notifier.counts(); raised != 1 {
		t.Errorf("%d notifications after the silence ended, want 1", raised)
	}
}
