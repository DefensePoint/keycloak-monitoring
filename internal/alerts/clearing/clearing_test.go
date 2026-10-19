package clearing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

type fakeStore struct {
	open       []*domain.Alert
	listErr    error
	resolveErr error
	resolved   []string
}

func (s *fakeStore) ListOpenAlertsByCheck(context.Context, string, string) ([]*domain.Alert, error) {
	return s.open, s.listErr
}

func (s *fakeStore) ResolveAlert(_ context.Context, _, alertID string) error {
	if s.resolveErr != nil {
		return s.resolveErr
	}
	s.resolved = append(s.resolved, alertID)
	return nil
}

type fakeNotifier struct{ sent []*domain.Alert }

func (n *fakeNotifier) NotifyAlertResolution(_ context.Context, a *domain.Alert) error {
	n.sent = append(n.sent, a)
	return nil
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func openAlert(id string, lastSeenAgo time.Duration, status domain.AlertStatus) *domain.Alert {
	return &domain.Alert{TenantID: "t1", AlertID: id, Status: status, LastSeen: now.Add(-lastSeenAgo), Description: "seen"}
}

func evaluated(ids ...string) Evaluation {
	ev := Evaluation{TenantID: "t1"}
	for _, id := range ids {
		ev.MarkEvaluated(id)
	}
	return ev
}

func TestResolveCleared_ResolvesOnlyEvaluatedQuietUndetectedAlerts(t *testing.T) {
	store := &fakeStore{open: []*domain.Alert{
		openAlert("cleared", 20*time.Minute, domain.AlertStatusActive),
		openAlert("cleared-acknowledged", 20*time.Minute, domain.AlertStatusAcknowled),
		openAlert("still-detected", 20*time.Minute, domain.AlertStatusActive),
		openAlert("not-evaluated", 20*time.Minute, domain.AlertStatusActive),
		openAlert("too-recent", 10*time.Minute, domain.AlertStatusActive),
	}}
	ev := evaluated("cleared", "cleared-acknowledged", "still-detected", "too-recent")
	ev.Alerts = []*domain.Alert{{AlertID: "still-detected"}}
	notifier := &fakeNotifier{}

	n := ResolveCleared(context.Background(), store, notifier, nopLogger{}, "check", ev, now, 15*time.Minute)

	want := []string{"cleared", "cleared-acknowledged"}
	if n != len(want) || strings.Join(store.resolved, ",") != strings.Join(want, ",") {
		t.Fatalf("resolved %v (count %d), want %v", store.resolved, n, want)
	}
	if len(notifier.sent) != 2 {
		t.Fatalf("sent %d resolution notifications, want 2", len(notifier.sent))
	}
	for _, a := range notifier.sent {
		if a.Status != domain.AlertStatusResolved || a.ResolvedAt == nil || !a.ResolvedAt.Equal(now) {
			t.Errorf("%s notified as %s resolved at %v, want resolved at %v", a.AlertID, a.Status, a.ResolvedAt, now)
		}
		if !strings.Contains(a.Description, "Cleared: not detected since") {
			t.Errorf("%s description %q does not say it cleared", a.AlertID, a.Description)
		}
	}
}

func TestResolveCleared_ExactlyTheQuietPeriodResolves(t *testing.T) {
	store := &fakeStore{open: []*domain.Alert{openAlert("a", 15*time.Minute, domain.AlertStatusActive)}}
	if n := ResolveCleared(context.Background(), store, nil, nopLogger{}, "check", evaluated("a"), now, 15*time.Minute); n != 1 {
		t.Errorf("resolved %d at exactly the quiet period, want 1", n)
	}
}

func TestResolveCleared_FailuresResolveAndNotifyNothing(t *testing.T) {
	notifier := &fakeNotifier{}
	listFails := &fakeStore{listErr: errors.New("db down")}
	if n := ResolveCleared(context.Background(), listFails, notifier, nopLogger{}, "check", evaluated("a"), now, time.Minute); n != 0 {
		t.Errorf("resolved %d when listing failed", n)
	}

	resolveFails := &fakeStore{open: []*domain.Alert{openAlert("a", time.Hour, domain.AlertStatusActive)}, resolveErr: errors.New("db down")}
	if n := ResolveCleared(context.Background(), resolveFails, notifier, nopLogger{}, "check", evaluated("a"), now, time.Minute); n != 0 {
		t.Errorf("resolved %d when resolving failed", n)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("sent %d resolution notifications for alerts that did not resolve", len(notifier.sent))
	}
}

func TestReopens(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing *domain.Alert
		want     bool
	}{
		{"resolved and quiet for the period", openAlert("a", 15*time.Minute, domain.AlertStatusResolved), true},
		{"resolved but seen within the period", openAlert("a", 5*time.Minute, domain.AlertStatusResolved), false},
		{"still active", openAlert("a", time.Hour, domain.AlertStatusActive), false},
		{"acknowledged", openAlert("a", time.Hour, domain.AlertStatusAcknowled), false},
		{"ignored", openAlert("a", time.Hour, domain.AlertStatusIgnored), false},
		{"no existing alert", nil, false},
	} {
		if got := Reopens(tc.existing, now, 15*time.Minute); got != tc.want {
			t.Errorf("%s: Reopens = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A realm marked evaluated covers every open alert of the check in that realm,
// and nothing outside it: not another realm's, and not one with no realm.
func TestResolveCleared_ARealmMarkCoversOnlyThatRealm(t *testing.T) {
	inRealm := func(id, realm string) *domain.Alert {
		a := openAlert(id, time.Hour, domain.AlertStatusActive)
		a.RealmName = realm
		return a
	}
	store := &fakeStore{open: []*domain.Alert{
		inRealm("master-user", "master"),
		inRealm("master-detected", "master"),
		inRealm("other-user", "other"),
		inRealm("no-realm", ""),
	}}
	ev := Evaluation{TenantID: "t1"}
	ev.MarkRealmEvaluated("master")
	ev.Alerts = []*domain.Alert{{AlertID: "master-detected"}}

	ResolveCleared(context.Background(), store, nil, nopLogger{}, "check", ev, now, 15*time.Minute)

	if got := strings.Join(store.resolved, ","); got != "master-user" {
		t.Errorf("resolved %q, want only master-user", got)
	}
}

func TestResolveCleared_AnUnreadResourceKeepsItsAlerts(t *testing.T) {
	onResource := func(id, realm, resource string) *domain.Alert {
		a := openAlert(id, time.Hour, domain.AlertStatusActive)
		a.RealmName, a.ResourceName = realm, resource
		return a
	}
	store := &fakeStore{open: []*domain.Alert{
		onResource("corp-cert", "master", "corp"),
		onResource("partner-cert", "master", "partner"),
		onResource("other-corp-cert", "other", "corp"),
	}}
	ev := Evaluation{TenantID: "t1"}
	ev.MarkRealmEvaluated("master")
	ev.MarkRealmEvaluated("other")
	ev.MarkEvaluated("corp-cert")
	ev.MarkUnread("master", "corp")

	ResolveCleared(context.Background(), store, nil, nopLogger{}, "check", ev, now, 15*time.Minute)

	if got := strings.Join(store.resolved, ","); got != "partner-cert,other-corp-cert" {
		t.Errorf("resolved %q, want partner-cert,other-corp-cert: an unread resource keeps its alerts, even one marked evaluated", got)
	}
}
