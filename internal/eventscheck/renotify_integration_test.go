package eventscheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspg "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
	notificationspg "github.com/DefensePoint/keycloak-monitoring/internal/notifications/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
	"gorm.io/gorm"
)

// What a re-detected alert looks like after it is saved again, through the
// real alerts repository: whether a notification is due, what the alert says,
// and who acknowledged it must all survive the save.

// repoStore adapts alerts.Repository to AlertStore, as the application does.
type repoStore struct{ repo alerts.Repository }

func (s repoStore) GetAlertByID(ctx context.Context, tenantID, id string) (*domain.Alert, error) {
	return s.repo.GetByAlertID(ctx, tenantID, id)
}
func (s repoStore) SaveAlert(ctx context.Context, a *domain.Alert) error { return s.repo.Save(ctx, a) }
func (s repoStore) ListOpenAlertsByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error) {
	return s.repo.ListOpenByCheck(ctx, tenantID, checkType)
}
func (s repoStore) ResolveAlert(ctx context.Context, tenantID, id string) error {
	return s.repo.Resolve(ctx, tenantID, id)
}

type renotifyFixture struct {
	client   *database.Client
	db       *gorm.DB
	repo     alerts.Repository
	counts   *realmCounts
	notifier *countingNotifier
	svc      Service
	tenantID string
	check    Check
}

func newRenotifyFixture(t *testing.T) *renotifyFixture {
	t.Helper()
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	f := &renotifyFixture{
		client:   client,
		db:       client.DB(),
		repo:     alertspg.NewRepository(client.DB()),
		counts:   &realmCounts{counts: map[string]int64{}, errs: map[string]error{}},
		notifier: &countingNotifier{},
		tenantID: fmt.Sprintf("renotify-%d", time.Now().UnixNano()),
	}
	t.Cleanup(func() {
		f.db.Unscoped().Where("tenant_id = ?", f.tenantID).Delete(&database.ConfigurationAlert{})
	})
	f.check = NewLoginErrorCheck(f.counts, discardLogger{}, f.tenantID, staticRealms("master"), 1, 5*time.Minute)
	f.svc = NewService([]Check{f.check}, repoStore{f.repo}, f.notifier, discardLogger{}, time.Hour)
	return f
}

func (f *renotifyFixture) poll(t *testing.T) {
	t.Helper()
	if err := f.svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}
}

// anHourPasses makes the stored alert's last notification, and its last save,
// 61 minutes old.
func (f *renotifyFixture) anHourPasses(t *testing.T) {
	t.Helper()
	if err := f.db.Exec(`UPDATE configuration_alerts
		SET metadata = jsonb_set(metadata, '{last_notified_at}', to_jsonb(to_char((now() - interval '61 minutes') AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'))),
		    updated_at = now() - interval '61 minutes'
		WHERE tenant_id = ?`, f.tenantID).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}
}

func (f *renotifyFixture) stored(t *testing.T) *domain.Alert {
	t.Helper()
	list, err := f.repo.ListOpenByCheck(context.Background(), f.tenantID, "login_error_events")
	if err != nil || len(list) != 1 {
		t.Fatalf("open alerts: %v (%d)", err, len(list))
	}
	return list[0]
}

// The reported storm: once an alert's first notification is an hour old, every
// poll re-notified, because the save never wrote down that it had.
func TestRenotify_AnHourLaterItNotifiesOnceNotEveryPoll(t *testing.T) {
	f := newRenotifyFixture(t)
	f.counts.set("master", 1)
	f.poll(t)

	// An hour passes with the condition still holding.
	f.anHourPasses(t)

	for i := 0; i < 5; i++ {
		f.poll(t)
	}
	if raised, _ := f.notifier.counts(); raised != 2 {
		t.Errorf("notifications = %d over the first detection and five polls an hour later, want 2: one re-notification, not one per poll", raised)
	}
}

// The hourly request is how a failed delivery is retried: the notification
// service counts only successful sends, and nothing else retries a failed one.
// Through the real notification service and a Slack webhook that fails the
// first post: no retry within the hour, one an hour later, then no repeats.
func TestRenotify_AFailedDeliveryIsRetriedAnHourLater(t *testing.T) {
	f := newRenotifyFixture(t)
	var posts, accepted atomic.Int32
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if posts.Add(1) == 1 {
			http.Error(w, "unavailable", http.StatusInternalServerError)
			return
		}
		accepted.Add(1)
	}))
	t.Cleanup(webhook.Close)
	t.Cleanup(func() {
		f.db.Exec(`DELETE FROM notification_logs WHERE alert_id IN (SELECT alert_id FROM configuration_alerts WHERE tenant_id = ?)`, f.tenantID)
	})

	cfg := &config.NotificationsConfig{Slack: config.SlackConfig{Enabled: true, WebhookURL: webhook.URL}}
	notifier := notifications.NewFullService(notificationspg.NewRepository(f.client), cfg, logger.NewNoop())
	svc := NewService([]Check{f.check}, repoStore{f.repo}, notifier, discardLogger{}, time.Hour)
	poll := func() {
		t.Helper()
		if err := svc.RunCheckNow(context.Background()); err != nil {
			t.Fatalf("RunCheckNow: %v", err)
		}
	}

	f.counts.set("master", 1)
	poll()
	poll()
	if p, a := posts.Load(), accepted.Load(); p != 1 || a != 0 {
		t.Fatalf("webhook posts = %d, accepted = %d within the first hour, want the one failed post", p, a)
	}

	f.anHourPasses(t)
	poll()
	poll()
	poll()
	if a := accepted.Load(); a != 1 {
		t.Errorf("deliveries = %d an hour after the failed one, want 1: retried once", a)
	}
	if p := posts.Load(); p != 2 {
		t.Errorf("webhook posts = %d, want 2: the failed one and the retry, no repeats once delivered", p)
	}
}

// The reported stale body: a re-detected alert must describe what was
// detected now, not what was detected first, and keep its first detection and
// its single notification.
func TestRenotify_TheDescriptionTracksTheLatestCount(t *testing.T) {
	f := newRenotifyFixture(t)
	f.counts.set("master", 1)
	f.poll(t)
	first := f.stored(t)
	f.counts.set("master", 37)
	f.poll(t)

	got := f.stored(t)
	if !strings.Contains(got.Description, "37") {
		t.Errorf("description = %q, want it to report the latest count, 37", got.Description)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(got.Metadata), &m); err != nil || m["count"] != float64(37) {
		t.Errorf("metadata = %s (%v), want count 37", got.Metadata, err)
	}
	if !got.FirstDetected.Equal(first.FirstDetected) {
		t.Errorf("first detected = %v, want %v unchanged", got.FirstDetected, first.FirstDetected)
	}
	if raised, _ := f.notifier.counts(); raised != 1 {
		t.Errorf("notifications = %d, want 1: a higher count is not a new occurrence", raised)
	}
}

// severityCheck raises one alert whose severity the test changes between polls,
// as the realm SSL check does under one alert ID.
type severityCheck struct {
	tenantID string
	severity *domain.AlertSeverity
}

func (c severityCheck) GetCheckType() string   { return "severity-check" }
func (c severityCheck) GetDescription() string { return "severity" }
func (c severityCheck) Execute(context.Context) ([]*domain.Alert, error) {
	return []*domain.Alert{{TenantID: c.tenantID, AlertID: c.tenantID + "-severity", CheckType: "severity-check",
		Source: domain.AlertSourceEvent, Type: domain.AlertTypeSecurity, Severity: *c.severity,
		Title: "severity", Description: "severity is " + string(*c.severity), ResourceType: "realm",
		ResourceID: "master", ResourceName: "master", RealmName: "master"}}, nil
}

// A Warning that becomes Critical is stored as Critical. The change by itself
// sends no new notification: the occurrence was already announced.
func TestRenotify_ASeverityChangeIsStoredWithoutANewNotification(t *testing.T) {
	f := newRenotifyFixture(t)
	severity := domain.AlertSeverityWarning
	svc := NewService([]Check{severityCheck{f.tenantID, &severity}}, repoStore{f.repo}, f.notifier, discardLogger{}, time.Hour)
	run := func() {
		t.Helper()
		if err := svc.RunCheckNow(context.Background()); err != nil {
			t.Fatalf("RunCheckNow: %v", err)
		}
	}
	load := func() *domain.Alert {
		t.Helper()
		a, err := f.repo.GetByAlertID(context.Background(), f.tenantID, f.tenantID+"-severity")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		return a
	}

	run()
	first := load()
	severity = domain.AlertSeverityCritical
	run()

	got := load()
	if got.Severity != domain.AlertSeverityCritical || got.Description != "severity is critical" {
		t.Errorf("severity %s, description %q; want critical and its description", got.Severity, got.Description)
	}
	if !got.FirstDetected.Equal(first.FirstDetected) {
		t.Errorf("first detected = %v, want %v unchanged", got.FirstDetected, first.FirstDetected)
	}
	if raised, _ := f.notifier.counts(); raised != 1 {
		t.Errorf("notifications = %d, want 1: a severity change alone does not notify", raised)
	}
}

// An alert the operator resolved while the burst goes on stays resolved on the
// next poll, keeps the time it was resolved, and is not announced again.
func TestRenotify_AResolutionByHandKeepsItsTimeOnTheNextPoll(t *testing.T) {
	f := newRenotifyFixture(t)
	ctx := context.Background()
	f.counts.set("master", 1)
	f.poll(t)
	id := f.stored(t).AlertID
	if err := f.repo.Resolve(ctx, f.tenantID, id); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	resolved, err := f.repo.GetByAlertID(ctx, f.tenantID, id)
	if err != nil || resolved.ResolvedAt == nil {
		t.Fatalf("after Resolve: %v, resolved at %v", err, resolved.ResolvedAt)
	}

	f.counts.set("master", 5)
	f.poll(t)

	got, err := f.repo.GetByAlertID(ctx, f.tenantID, id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Status != domain.AlertStatusResolved {
		t.Fatalf("status = %s, want resolved", got.Status)
	}
	if got.ResolvedAt == nil || !got.ResolvedAt.Equal(*resolved.ResolvedAt) {
		t.Errorf("resolved at %v after the next poll, want %v kept", got.ResolvedAt, *resolved.ResolvedAt)
	}
	if raised, _ := f.notifier.counts(); raised != 1 {
		t.Errorf("notifications = %d, want 1: a hand-resolved alert is not announced again", raised)
	}
}

// Found with the above: the next save after an acknowledgement wiped who
// acknowledged the alert and when.
func TestRenotify_AnAcknowledgementSurvivesTheNextPoll(t *testing.T) {
	f := newRenotifyFixture(t)
	f.counts.set("master", 1)
	f.poll(t)
	a := f.stored(t)
	if err := f.repo.UpdateStatus(context.Background(), f.tenantID, a.AlertID, domain.AlertStatusAcknowled, "alice@example.test"); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}

	f.poll(t)
	got := f.stored(t)
	if got.Status != domain.AlertStatusAcknowled {
		t.Fatalf("status = %s, want acknowledged", got.Status)
	}
	if got.AcknowledgedAt == nil || got.AcknowledgedBy != "alice@example.test" {
		t.Errorf("acknowledged at %v by %q after the next poll, want it kept, by alice@example.test", got.AcknowledgedAt, got.AcknowledgedBy)
	}
}

// bareCheck raises one alert with no metadata at all.
type bareCheck struct{ tenantID string }

func (c bareCheck) GetCheckType() string   { return "bare-check" }
func (c bareCheck) GetDescription() string { return "bare" }
func (c bareCheck) Execute(context.Context) ([]*domain.Alert, error) {
	return []*domain.Alert{{TenantID: c.tenantID, AlertID: c.tenantID + "-bare", CheckType: "bare-check",
		Source: domain.AlertSourceEvent, Type: domain.AlertTypeSecurity, Severity: domain.AlertSeverityWarning,
		Title: "bare", Description: "bare", ResourceType: "realm", ResourceID: "master", ResourceName: "master", RealmName: "master"}}, nil
}

// An alert that carries no metadata must record its notifications too:
// otherwise the hourly check falls back to updated_at, which every save moves,
// and it never re-notifies at all.
func TestRenotify_AnAlertWithoutMetadataStillRenotifiesHourly(t *testing.T) {
	f := newRenotifyFixture(t)
	svc := NewService([]Check{bareCheck{f.tenantID}}, repoStore{f.repo}, f.notifier, discardLogger{}, time.Hour)
	run := func() {
		if err := svc.RunCheckNow(context.Background()); err != nil {
			t.Fatalf("RunCheckNow: %v", err)
		}
	}
	run()
	if err := f.db.Exec(`UPDATE configuration_alerts
		SET metadata = jsonb_set(metadata, '{last_notified_at}', to_jsonb(to_char((now() - interval '61 minutes') AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')))
		WHERE tenant_id = ?`, f.tenantID).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}
	for i := 0; i < 3; i++ {
		run()
	}
	if raised, _ := f.notifier.counts(); raised != 2 {
		t.Errorf("notifications = %d, want 2: the first, and one an hour later", raised)
	}
}
