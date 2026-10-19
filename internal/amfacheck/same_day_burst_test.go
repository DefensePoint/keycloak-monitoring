package amfacheck

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// The rolling-window rules used to name their alert by day, or for the
// per-client rule by client alone, and never resolved it. A second burst the
// same day (or, per client, ever) mapped onto the first burst's alert, which a
// person had resolved or acknowledged, so it was stored under that status and
// announced to nobody. These run every such rule through the service.

// windowRule describes one rolling-window rule for these tests.
type windowRule struct {
	name string
	new  func(repo *checkRepoStub) Check
	// burst sets the repo so that the given subjects are over the threshold
	// (none clears the burst). Realm-wide rules ignore the subject names.
	burst func(repo *checkRepoStub, subjects ...string)
	// id is the alert ID for subject in realm master.
	id func(subject string) string
	// perSubject is true for the per-user and per-client rules.
	perSubject bool
}

func windowRules() []windowRule {
	master := singleRealm("master")
	users := func(subjects []string) []amfa.UserRiskyCount {
		var out []amfa.UserRiskyCount
		for _, s := range subjects {
			out = append(out, amfa.UserRiskyCount{UserID: s, Count: 9})
		}
		return out
	}
	return []windowRule{
		{
			name: "repeated-risky",
			new: func(r *checkRepoStub) Check {
				return NewRepeatedRiskyCheck("t1", master, r, 3, time.Hour, 2, 15*time.Minute, testLogger{})
			},
			burst:      func(r *checkRepoStub, s ...string) { r.userCounts = users(s) },
			id:         func(s string) string { return alertID("amfa-repeated-risky", "t1", "master", s) },
			perSubject: true,
		},
		{
			name: "login-error-by-account",
			new: func(r *checkRepoStub) Check {
				return NewLoginErrorByAccountCheck("t1", master, r, 3, time.Hour, testLogger{})
			},
			burst:      func(r *checkRepoStub, s ...string) { r.byUserCounts = users(s) },
			id:         func(s string) string { return alertID("amfa-login-error-by-account", "t1", "master", s) },
			perSubject: true,
		},
		{
			name: "login-error-by-client",
			new: func(r *checkRepoStub) Check {
				return NewLoginErrorByClientCheck("t1", master, r, 3, time.Hour, testLogger{})
			},
			burst: func(r *checkRepoStub, s ...string) {
				r.clientCounts = nil
				for _, c := range s {
					r.clientCounts = append(r.clientCounts, amfa.ClientEventCount{Client: c, Count: 9})
				}
			},
			id:         func(s string) string { return alertID("amfa-login-error-by-client", "t1", "master", s) },
			perSubject: true,
		},
		{
			name: "realm-reject-burst",
			new: func(r *checkRepoStub) Check {
				return NewRealmRejectBurstCheck("t1", master, r, 3, time.Hour, testLogger{})
			},
			burst: func(r *checkRepoStub, s ...string) {
				r.distinctUsers = 0
				if len(s) > 0 {
					r.distinctUsers = 9
				}
			},
			id: func(string) string { return alertID("amfa-realm-reject-burst", "t1", "master") },
		},
	}
}

type burstFixture struct {
	repo     *checkRepoStub
	store    *fakeAlertStore
	notifier *fakeNotifier
	svc      Service
}

func newBurstFixture(rule windowRule) *burstFixture {
	f := &burstFixture{repo: &checkRepoStub{}, store: newFakeAlertStore(), notifier: newFakeNotifier()}
	f.svc = NewService("t1", []Check{rule.new(f.repo)}, f.store, f.notifier, testLogger{}, time.Hour,
		WithResolveAfter(testResolveAfter))
	return f
}

func (f *burstFixture) poll(t *testing.T) {
	t.Helper()
	if err := f.svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}
}

func (f *burstFixture) status(t *testing.T, id string) domain.AlertStatus {
	t.Helper()
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	a, ok := f.store.existing[id]
	if !ok {
		t.Fatalf("no alert %s", id)
	}
	return a.Status
}

// age moves every alert's last detection into the past, as time passing would.
func (f *burstFixture) age(by time.Duration) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	for _, a := range f.store.existing {
		a.LastSeen = a.LastSeen.Add(-by)
	}
}

func (f *burstFixture) setStatus(id string, status domain.AlertStatus) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	a := f.store.existing[id]
	a.Status = status
	if status == domain.AlertStatusAcknowled {
		now := time.Now()
		a.AcknowledgedAt = &now
	}
}

// The task's case: a burst, quiet, and another burst within minutes, so on the
// same UTC day. The second must be announced.
func TestWindowRules_ASecondSameDayBurstIsAnnounced(t *testing.T) {
	for _, rule := range windowRules() {
		t.Run(rule.name, func(t *testing.T) {
			f := newBurstFixture(rule)
			id := rule.id("subject-a")

			rule.burst(f.repo, "subject-a")
			f.poll(t)
			if n := f.notifier.notifs(id); n != 1 {
				t.Fatalf("first burst: %d notifications, want 1", n)
			}

			rule.burst(f.repo)
			f.age(testResolveAfter)
			f.poll(t)
			if s := f.status(t, id); s != domain.AlertStatusResolved {
				t.Fatalf("status once the burst aged out = %s, want resolved", s)
			}
			if n := f.notifier.resolutions(id); n != 1 {
				t.Errorf("resolution notifications = %d, want 1", n)
			}

			rule.burst(f.repo, "subject-a")
			f.poll(t)
			if s := f.status(t, id); s != domain.AlertStatusActive {
				t.Errorf("status after the second burst = %s, want active", s)
			}
			if n := f.notifier.notifs(id); n != 2 {
				t.Errorf("second burst: %d notifications in total, want 2", n)
			}
		})
	}
}

// Acknowledging the first burst must not swallow the next one: once the first
// has cleared, a new burst is a new occurrence.
func TestWindowRules_AnAcknowledgedBurstDoesNotHideTheNext(t *testing.T) {
	for _, rule := range windowRules() {
		t.Run(rule.name, func(t *testing.T) {
			f := newBurstFixture(rule)
			id := rule.id("subject-a")

			rule.burst(f.repo, "subject-a")
			f.poll(t)
			f.setStatus(id, domain.AlertStatusAcknowled)

			// Still bursting while acknowledged: no new notification.
			f.poll(t)
			if n := f.notifier.notifs(id); n != 1 {
				t.Fatalf("%d notifications while acknowledged, want 1", n)
			}

			rule.burst(f.repo)
			f.age(testResolveAfter)
			f.poll(t)
			rule.burst(f.repo, "subject-a")
			f.poll(t)
			if n := f.notifier.notifs(id); n != 2 {
				t.Errorf("the burst after an acknowledged one: %d notifications in total, want 2", n)
			}
		})
	}
}

// The query only names subjects over the threshold. One dropping below it
// resolves, while another still over it stays open.
func TestWindowRules_OnlyTheSubjectThatCalmedDownResolves(t *testing.T) {
	for _, rule := range windowRules() {
		if !rule.perSubject {
			continue
		}
		t.Run(rule.name, func(t *testing.T) {
			f := newBurstFixture(rule)
			rule.burst(f.repo, "subject-a", "subject-b")
			f.poll(t)

			rule.burst(f.repo, "subject-b")
			f.age(testResolveAfter)
			f.poll(t)
			if s := f.status(t, rule.id("subject-a")); s != domain.AlertStatusResolved {
				t.Errorf("calmed subject: status %s, want resolved", s)
			}
			if s := f.status(t, rule.id("subject-b")); s != domain.AlertStatusActive {
				t.Errorf("still-bursting subject: status %s, want active", s)
			}
		})
	}
}

// Alerts raised under the old per-day IDs are open in deployments today. They
// resolve like any other once their realm is quiet, instead of staying open
// for good under an ID nothing produces any more.
func TestWindowRules_AnAlertUnderTheOldPerDayIDResolves(t *testing.T) {
	for _, rule := range windowRules() {
		t.Run(rule.name, func(t *testing.T) {
			f := newBurstFixture(rule)
			checkType := rule.new(f.repo).GetCheckType()
			f.store.existing["legacy"] = &domain.Alert{
				TenantID: "t1", AlertID: "legacy", CheckType: checkType, RealmName: "master",
				Status: domain.AlertStatusActive, LastSeen: time.Now().Add(-2 * testResolveAfter),
			}
			f.poll(t)
			if s := f.status(t, "legacy"); s != domain.AlertStatusResolved {
				t.Errorf("old per-day alert: status %s, want resolved", s)
			}
		})
	}
}

// A realm that could not be read was not seen to calm down.
func TestWindowRules_AnOutageResolvesNothing(t *testing.T) {
	for _, rule := range windowRules() {
		t.Run(rule.name, func(t *testing.T) {
			f := newBurstFixture(rule)
			id := rule.id("subject-a")
			rule.burst(f.repo, "subject-a")
			f.poll(t)

			outage := errors.New("amfa unreachable")
			f.repo.userErr, f.repo.byUserCountsErr = outage, outage
			f.repo.clientCountsErr, f.repo.distinctUsersErr = outage, outage
			f.age(time.Hour)
			if err := f.svc.RunCheckNow(context.Background()); err == nil {
				t.Fatal("a run during the outage reported no error")
			}
			if s := f.status(t, id); s != domain.AlertStatusActive {
				t.Errorf("status during an outage = %s, want still active", s)
			}
		})
	}
}

// riskyDayRepo answers rule 2's two questions separately, as the database
// does: who is over the threshold across the 24h window, and who made a risky
// attempt recently.
type riskyDayRepo struct {
	*checkRepoStub
	overDay   []string
	recent    []string
	recentErr error
}

func (r *riskyDayRepo) CountRepeatedRiskyByUser(_ context.Context, _ string, since time.Time, _, _ int) ([]amfa.UserRiskyCount, error) {
	users := r.overDay
	if time.Since(since) < 2*time.Hour {
		if r.recentErr != nil {
			return nil, r.recentErr
		}
		users = r.recent
	}
	var out []amfa.UserRiskyCount
	for _, u := range users {
		out = append(out, amfa.UserRiskyCount{UserID: u, Count: 4})
	}
	return out, nil
}

func newRiskyDayFixture() (*riskyDayRepo, *burstFixture) {
	repo := &riskyDayRepo{checkRepoStub: &checkRepoStub{}}
	f := &burstFixture{store: newFakeAlertStore(), notifier: newFakeNotifier()}
	check := NewRepeatedRiskyCheck("t1", singleRealm("master"), repo, 3, 24*time.Hour, 2, 15*time.Minute, testLogger{})
	f.svc = NewService("t1", []Check{check}, f.store, f.notifier, testLogger{}, time.Hour, WithResolveAfter(testResolveAfter))
	return repo, f
}

// The task's case with the real window: the morning's attempts keep the user
// over the threshold all day, so only renewed activity can tell the afternoon
// burst apart from the morning one.
func TestRepeatedRisky_TheAfternoonBurstIsAnnouncedWithinTheSameWindow(t *testing.T) {
	repo, f := newRiskyDayFixture()
	id := alertID("amfa-repeated-risky", "t1", "master", "alice")

	repo.overDay, repo.recent = []string{"alice"}, []string{"alice"}
	f.poll(t)
	f.setStatus(id, domain.AlertStatusAcknowled)

	// Quiet: still over the 24h threshold, no recent attempt.
	repo.recent = nil
	f.age(testResolveAfter)
	f.poll(t)
	if s := f.status(t, id); s != domain.AlertStatusResolved {
		t.Fatalf("status once the user went quiet = %s, want resolved", s)
	}

	// Active again, hours later, same window.
	repo.recent = []string{"alice"}
	f.poll(t)
	if s := f.status(t, id); s != domain.AlertStatusActive {
		t.Errorf("status after the afternoon burst = %s, want active", s)
	}
	if n := f.notifier.notifs(id); n != 2 {
		t.Errorf("%d notifications in total, want 2", n)
	}
}

// Over the threshold but quiet is not an alert on its own: the user stopped.
func TestRepeatedRisky_AQuietUserOverTheThresholdRaisesNothing(t *testing.T) {
	repo, f := newRiskyDayFixture()
	repo.overDay = []string{"bob"}
	f.poll(t)
	if n := f.store.size(); n != 0 {
		t.Errorf("store holds %d alerts for a user with no recent attempt", n)
	}
}

// Recent activity alone, under the threshold, is not an alert either.
func TestRepeatedRisky_RecentActivityUnderTheThresholdRaisesNothing(t *testing.T) {
	repo, f := newRiskyDayFixture()
	repo.recent = []string{"carol"}
	f.poll(t)
	if n := f.store.size(); n != 0 {
		t.Errorf("store holds %d alerts for a user under the threshold", n)
	}
}

// If the recent-activity question cannot be answered, the realm was not
// evaluated, so nothing resolves.
func TestRepeatedRisky_ARecentActivityOutageResolvesNothing(t *testing.T) {
	repo, f := newRiskyDayFixture()
	id := alertID("amfa-repeated-risky", "t1", "master", "alice")
	repo.overDay, repo.recent = []string{"alice"}, []string{"alice"}
	f.poll(t)

	repo.recentErr = errors.New("amfa unreachable")
	f.age(time.Hour)
	if err := f.svc.RunCheckNow(context.Background()); err == nil {
		t.Fatal("a run during the outage reported no error")
	}
	if s := f.status(t, id); s != domain.AlertStatusActive {
		t.Errorf("status during an outage = %s, want still active", s)
	}
}
