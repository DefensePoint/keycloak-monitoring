package fx

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspg "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

type recordingEventNotifier struct {
	mu   sync.Mutex
	sent []domain.AlertStatus
}

func (n *recordingEventNotifier) NotifyAlert(_ context.Context, a *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, a.Status)
	return nil
}

func (n *recordingEventNotifier) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.sent)
}

type eventAlertHarness struct {
	t        *testing.T
	repo     alerts.Repository
	conv     *eventAlertConverterImpl
	notifier *recordingEventNotifier
	tenant   string
	at       time.Time
}

func newEventAlertHarness(t *testing.T) *eventAlertHarness {
	t.Helper()
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	tenant := "t45-" + t.Name()
	db := client.DB()
	clean := func() {
		db.Exec(`DELETE FROM configuration_alerts WHERE tenant_id = ?`, tenant)
	}
	clean()
	t.Cleanup(clean)
	repo := alertspg.NewRepository(db)
	notifier := &recordingEventNotifier{}
	return &eventAlertHarness{
		t:        t,
		repo:     repo,
		conv:     &eventAlertConverterImpl{alertRepo: repo, notifier: notifier, log: logger.NewNoop()},
		notifier: notifier,
		tenant:   tenant,
		at:       time.Now().Add(-time.Hour).Truncate(time.Second),
	}
}

func (h *eventAlertHarness) event() *domain.Alert {
	h.t.Helper()
	h.at = h.at.Add(time.Minute)
	return h.eventAt(h.at, "user-"+h.at.Format("150405"))
}

// eventAt converts a LOGIN_ERROR event at the given time, as a poll that
// rereads its lookback window does, and returns the stored alert.
func (h *eventAlertHarness) eventAt(at time.Time, user string) *domain.Alert {
	h.t.Helper()
	ev := &domain.KeycloakEvent{
		TenantID: h.tenant, EventID: at.Format(time.RFC3339Nano), EventType: "LOGIN_ERROR",
		RealmName: "r1", ClientID: "app", UserID: user, Time: at,
	}
	if err := h.conv.ConvertEvent(context.Background(), ev); err != nil {
		h.t.Fatalf("ConvertEvent: %v", err)
	}
	a, err := h.repo.GetByAlertID(context.Background(), h.tenant, h.conv.generateAlertID(ev))
	if err != nil {
		h.t.Fatalf("GetByAlertID: %v", err)
	}
	return a
}

// A repeat event keeps the status an operator set, while the alert's details
// still follow the newest event.
func TestConvertEvent_RepeatKeepsTheOperatorsStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status domain.AlertStatus
	}{
		{"acknowledged", domain.AlertStatusAcknowled},
		{"ignored", domain.AlertStatusIgnored},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newEventAlertHarness(t)
			first := h.event()
			if err := h.repo.UpdateStatus(context.Background(), h.tenant, first.AlertID, tc.status, "operator@example.com"); err != nil {
				t.Fatalf("UpdateStatus: %v", err)
			}

			got := h.event()
			if got.Status != tc.status {
				t.Fatalf("status after a repeat event = %s, want %s", got.Status, tc.status)
			}
			if tc.status == domain.AlertStatusAcknowled && (got.AcknowledgedAt == nil || got.AcknowledgedBy != "operator@example.com") {
				t.Errorf("acknowledgement cleared: at %v by %q", got.AcknowledgedAt, got.AcknowledgedBy)
			}
			if !got.LastSeen.Equal(h.at) {
				t.Errorf("last seen = %v, want the repeat event's time %v", got.LastSeen, h.at)
			}
			if !got.FirstDetected.Equal(first.FirstDetected) {
				t.Errorf("first detected moved from %v to %v", first.FirstDetected, got.FirstDetected)
			}
			if n := h.notifier.count(); n != 1 {
				t.Errorf("notifications = %d, want only the first", n)
			}
		})
	}
}

// A resolved alert whose event happens again reopens and notifies once; later
// repeats of that occurrence do not notify again.
func TestConvertEvent_RecurrenceAfterResolvingNotifiesOnce(t *testing.T) {
	h := newEventAlertHarness(t)
	first := h.event()
	if err := h.repo.Resolve(context.Background(), h.tenant, first.AlertID); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	h.at = time.Now().Add(time.Second).Truncate(time.Second)
	got := h.event()
	if got.Status != domain.AlertStatusActive {
		t.Fatalf("status after the event recurred = %s, want active", got.Status)
	}
	if n := h.notifier.count(); n != 2 {
		t.Fatalf("notifications = %d, want 2: the first occurrence and the recurrence", n)
	}

	h.event()
	h.event()
	if n := h.notifier.count(); n != 2 {
		t.Errorf("notifications after more repeats = %d, want still 2", n)
	}
}

// The poller rereads its whole lookback window on every poll, so an alert's
// own events come back after an operator resolves it. Only an event newer
// than the resolution is a recurrence.
func TestConvertEvent_RereadEventsDoNotReopenAResolvedAlert(t *testing.T) {
	h := newEventAlertHarness(t)
	older := h.event()
	newest := h.event()
	if err := h.repo.Resolve(context.Background(), h.tenant, newest.AlertID); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for _, at := range []time.Time{newest.LastSeen, older.LastSeen} {
		got := h.eventAt(at, "reread")
		if got.Status != domain.AlertStatusResolved {
			t.Fatalf("status after rereading the event at %v = %s, want resolved", at, got.Status)
		}
	}
	if n := h.notifier.count(); n != 1 {
		t.Fatalf("notifications = %d, want only the first", n)
	}

	// Any event newer than last seen is new to the operator, even one from
	// before the resolution that is polled after it: Keycloak times are
	// only ever compared with Keycloak times, so clock skew between the two
	// servers cannot hide a recurrence.
	late := h.eventAt(newest.LastSeen.Add(30*time.Second), "late")
	if late.Status != domain.AlertStatusActive || !late.LastSeen.Equal(newest.LastSeen.Add(30*time.Second)) {
		t.Fatalf("late event: status %s, last seen %v; want active and moved", late.Status, late.LastSeen)
	}
	if n := h.notifier.count(); n != 2 {
		t.Fatalf("notifications after a late event = %d, want 2", n)
	}
}

// The alert's event and resource follow the newest event, as its details do.
func TestConvertEvent_EventFieldFollowsTheNewestEvent(t *testing.T) {
	h := newEventAlertHarness(t)
	first := h.event()
	second := h.event()
	wantEvent := h.at.Format(time.RFC3339Nano)
	if second.EventID == nil || *second.EventID != wantEvent || second.ResourceID != wantEvent {
		got := "<nil>"
		if second.EventID != nil {
			got = *second.EventID
		}
		t.Fatalf("event %q, resource %q after a newer event; want both %q (first was %q)",
			got, second.ResourceID, wantEvent, first.ResourceID)
	}
}

// Keycloak returns events newest first, so a poll applies older events after
// newer ones. Last seen only moves forward, and the details stay those of the
// newest event.
func TestConvertEvent_OlderEventsDoNotMoveLastSeenBack(t *testing.T) {
	h := newEventAlertHarness(t)
	base := h.at
	newest := h.eventAt(base.Add(2*time.Minute), "newest-user")
	got := h.eventAt(base.Add(time.Minute), "older-user")
	if !got.LastSeen.Equal(newest.LastSeen) {
		t.Fatalf("last seen = %v, want the newest event's %v", got.LastSeen, newest.LastSeen)
	}
	if !strings.Contains(got.Description, "newest-user") {
		t.Fatalf("description = %q, want the newest event's details", got.Description)
	}
}

// Rereading the event an alert already holds writes nothing.
func TestConvertEvent_RereadingTheSameEventWritesNothing(t *testing.T) {
	h := newEventAlertHarness(t)
	first := h.event()
	time.Sleep(10 * time.Millisecond)
	got := h.eventAt(first.LastSeen, "reread")
	if !got.UpdatedAt.Equal(first.UpdatedAt) || got.Description != first.Description {
		t.Fatalf("rereading the same event rewrote the alert: updated %v -> %v, description %q -> %q",
			first.UpdatedAt, got.UpdatedAt, first.Description, got.Description)
	}
}
