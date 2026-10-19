package fx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspg "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
	notificationspg "github.com/DefensePoint/keycloak-monitoring/internal/notifications/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// flakySlack answers the first failures webhook calls with 500 and the rest
// with 200, counting every call.
type flakySlack struct {
	failures int32
	calls    atomic.Int32
}

func (s *flakySlack) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	if s.calls.Add(1) <= s.failures {
		http.Error(w, "unavailable", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

type retryHarness struct {
	t       *testing.T
	db      *database.Client
	alerts  alerts.Repository
	notes   notifications.Repository
	svc     notifications.Service
	slack   *flakySlack
	health  *healthAlertManagerImpl
	tenant  string
	alertID string
}

func newRetryHarness(t *testing.T, failures int32) *retryHarness {
	t.Helper()
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	slack := &flakySlack{failures: failures}
	srv := httptest.NewServer(slack)
	t.Cleanup(srv.Close)

	tenant := "t45-retry-" + t.Name()
	alertID := "tenant-health-down-" + tenant
	clean := func() {
		client.DB().Exec(`DELETE FROM notification_logs WHERE alert_id = ?`, alertID)
		client.DB().Exec(`DELETE FROM configuration_alerts WHERE tenant_id = ?`, tenant)
	}
	clean()
	t.Cleanup(clean)

	notes := notificationspg.NewRepository(client)
	svc := notifications.NewFullService(notes, &config.NotificationsConfig{
		Slack: config.SlackConfig{Enabled: true, WebhookURL: srv.URL + "/hook", MinSeverity: "info"},
	}, logger.NewNoop())
	repo := alertspg.NewRepository(client.DB())
	return &retryHarness{
		t: t, db: client, alerts: repo, notes: notes, svc: svc, slack: slack,
		health: &healthAlertManagerImpl{alertRepo: repo, notifier: svc, log: logger.NewNoop(), previousStatus: map[string]string{}},
		tenant: tenant, alertID: alertID,
	}
}

func (h *retryHarness) tenantDown() {
	h.t.Helper()
	ctx := context.Background()
	if err := h.health.ProcessHealthStatus(ctx, h.tenant, "t45", "UP", "", 10); err != nil {
		h.t.Fatalf("ProcessHealthStatus UP: %v", err)
	}
	if err := h.health.ProcessHealthStatus(ctx, h.tenant, "t45", "DOWN", "connection refused", 0); err != nil {
		h.t.Fatalf("ProcessHealthStatus DOWN: %v", err)
	}
}

func (h *retryHarness) retrier() *notificationRetrier {
	return &notificationRetrier{undelivered: h.notes, alerts: h.alerts, notifier: h.svc, interval: time.Hour, log: logger.NewNoop()}
}

func (h *retryHarness) expectCalls(want int32, when string) {
	h.t.Helper()
	if got := h.slack.calls.Load(); got != want {
		h.t.Fatalf("%s: Slack called %d times, want %d", when, got, want)
	}
}

// A tenant-down alert whose Slack send failed when it was raised is sent again
// on the next retry, and not after that.
func TestNotificationRetry_AFailedSendIsRetriedOnce(t *testing.T) {
	h := newRetryHarness(t, 1)
	h.tenantDown()
	h.expectCalls(1, "raised, send failed")

	if err := h.health.ProcessHealthStatus(context.Background(), h.tenant, "t45", "DOWN", "connection refused", 0); err != nil {
		t.Fatalf("ProcessHealthStatus: %v", err)
	}
	h.expectCalls(1, "health poll while still down")

	if n := h.retrier().retryOnce(context.Background()); n != 1 {
		t.Fatalf("retried %d alerts, want 1", n)
	}
	h.expectCalls(2, "first retry")

	if n := h.retrier().retryOnce(context.Background()); n != 0 {
		t.Fatalf("retried %d alerts after delivery, want 0", n)
	}
	h.expectCalls(2, "retry after delivery, as after a restart")
}

// A channel that keeps failing is called once per retry, not more.
func TestNotificationRetry_AFailingChannelIsCalledOncePerRetry(t *testing.T) {
	h := newRetryHarness(t, 100)
	h.tenantDown()
	for i := int32(2); i <= 4; i++ {
		h.retrier().retryOnce(context.Background())
		h.expectCalls(i, "retry")
	}
}

// Acknowledged and resolved alerts are not sent again.
func TestNotificationRetry_SkipsAcknowledgedAndResolvedAlerts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		close func(h *retryHarness) error
	}{
		{"acknowledged", func(h *retryHarness) error {
			return h.alerts.UpdateStatus(context.Background(), h.tenant, h.alertID, domain.AlertStatusAcknowled, "operator@example.com")
		}},
		{"resolved", func(h *retryHarness) error {
			return h.alerts.Resolve(context.Background(), h.tenant, h.alertID)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRetryHarness(t, 1)
			h.tenantDown()
			if err := tc.close(h); err != nil {
				t.Fatalf("close: %v", err)
			}
			if n := h.retrier().retryOnce(context.Background()); n != 0 {
				t.Fatalf("retried %d alerts, want 0", n)
			}
			h.expectCalls(1, "after retry")
		})
	}
}

// Only the channel whose send failed is retried; one that already received the
// alert during this occurrence is not.
func TestNotificationRetry_OnlyUndeliveredChannels(t *testing.T) {
	h := newRetryHarness(t, 1)
	h.tenantDown()
	if err := h.notes.LogNotification(context.Background(), &domain.NotificationLog{
		AlertID: h.alertID, Channel: "gitlab", Status: "sent", ExternalID: "7",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("LogNotification: %v", err)
	}
	channels, err := h.notes.UndeliveredChannels(context.Background(), h.tenant, h.alertID)
	if err != nil {
		t.Fatalf("UndeliveredChannels: %v", err)
	}
	if len(channels) != 1 || channels[0] != "slack" {
		t.Fatalf("undelivered channels = %v, want [slack]", channels)
	}
}

// A Keycloak event alert whose send failed is retried the same way.
func TestNotificationRetry_EventAlert(t *testing.T) {
	h := newRetryHarness(t, 1)
	conv := &eventAlertConverterImpl{alertRepo: h.alerts, notifier: h.svc, log: logger.NewNoop()}
	ev := &domain.KeycloakEvent{
		TenantID: h.tenant, EventID: "e1", EventType: "LOGIN_ERROR",
		RealmName: "r1", ClientID: "app", Time: time.Now(),
	}
	alertID := conv.generateAlertID(ev)
	t.Cleanup(func() { h.db.DB().Exec(`DELETE FROM notification_logs WHERE alert_id = ?`, alertID) })
	if err := conv.ConvertEvent(context.Background(), ev); err != nil {
		t.Fatalf("ConvertEvent: %v", err)
	}
	h.expectCalls(1, "raised, send failed")

	h.retrier().retryOnce(context.Background())
	h.expectCalls(2, "retry")
}

// An event storage alert whose send failed is retried the same way.
func TestNotificationRetry_EventStorageAlert(t *testing.T) {
	h := newRetryHarness(t, 1)
	m := &eventStorageAlertManager{alertRepo: h.alerts, notifier: h.svc, log: logger.NewNoop(), state: map[string]*eventStorageState{}}
	alertID := eventStorageAlertID(h.tenant)
	t.Cleanup(func() { h.db.DB().Exec(`DELETE FROM notification_logs WHERE alert_id = ?`, alertID) })
	m.raise(context.Background(), h.tenant, "t45", keycloak.EventStorageOutcome{Attempted: 3, Failed: 3, FirstError: errors.New("insert failed")})
	h.expectCalls(1, "raised, send failed")

	h.retrier().retryOnce(context.Background())
	h.expectCalls(2, "retry")
}

// A failure from an earlier occurrence, before the alert resolved and came
// back, is not retried once the current occurrence was delivered.
func TestNotificationRetry_IgnoresEarlierOccurrences(t *testing.T) {
	h := newRetryHarness(t, 0)
	h.tenantDown()
	h.expectCalls(1, "raised and delivered")
	if err := h.notes.LogNotification(context.Background(), &domain.NotificationLog{
		AlertID: h.alertID, Channel: "email", Status: "failed", ErrorMessage: "earlier occurrence",
		CreatedAt: time.Now().Add(-24 * time.Hour), UpdatedAt: time.Now().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("LogNotification: %v", err)
	}
	h.db.DB().Exec(`UPDATE notification_logs SET created_at = now() - interval '1 day' WHERE alert_id = ? AND error_message = 'earlier occurrence'`, h.alertID)

	keys, err := h.notes.ListUndelivered(context.Background(), retriedCheckTypes)
	if err != nil {
		t.Fatalf("ListUndelivered: %v", err)
	}
	for _, k := range keys {
		if k.AlertID == h.alertID {
			t.Fatalf("alert listed as undelivered because of an earlier occurrence's failure")
		}
	}
}

type staticUndelivered []notifications.AlertKey

func (s staticUndelivered) ListUndelivered(context.Context, []string) ([]notifications.AlertKey, error) {
	return s, nil
}

// An alert acknowledged between the listing and the retry is not sent.
func TestNotificationRetry_RechecksTheAlertBeforeSending(t *testing.T) {
	h := newRetryHarness(t, 1)
	h.tenantDown()
	if err := h.alerts.UpdateStatus(context.Background(), h.tenant, h.alertID, domain.AlertStatusAcknowled, "operator@example.com"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	r := h.retrier()
	r.undelivered = staticUndelivered{{TenantID: h.tenant, AlertID: h.alertID}}
	if n := r.retryOnce(context.Background()); n != 0 {
		t.Fatalf("retried %d alerts, want 0", n)
	}
	h.expectCalls(1, "after retry")
}
