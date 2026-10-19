// Regression coverage for notifying once per occurrence of an alert.
//
// Alert IDs are stable per problem, and the dedup counted every send an alert
// ever had, so a problem that resolved and came back was never notified again.
// These tests drive the real alert upsert and the real dedup query together.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset. Only rows under a notify-occ- tenant or alert are touched.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspostgres "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

type occurrenceEnv struct {
	notifs notifications.Repository
	alerts alerts.Repository
	db     *gorm.DB
}

func newOccurrenceEnv(t *testing.T) occurrenceEnv {
	t.Helper()

	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	// testdb.DatabaseConfig rather than a local reading: inferring sslmode from
	// the parsed TLS config asked for TLS whenever the DSN left sslmode out.
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	db := client.DB()
	t.Cleanup(func() {
		db.Unscoped().Where("tenant_id LIKE ?", "notify-occ-%").Delete(&database.Alert{})
		db.Where("alert_id LIKE ?", "notify-occ-%").Delete(&database.NotificationLog{})
	})
	return occurrenceEnv{notifs: NewRepository(client), alerts: alertspostgres.NewRepository(db), db: db}
}

func occurrenceAlert(tenantID, alertID string) *domain.Alert {
	now := time.Now()
	return &domain.Alert{
		TenantID: tenantID, AlertID: alertID, Source: domain.AlertSourceMetric,
		Type: domain.AlertTypePerformance, Severity: domain.AlertSeverityCritical,
		Status: domain.AlertStatusActive, Title: "Tenant is DOWN",
		FirstDetected: now, LastSeen: now,
	}
}

// raise saves the alert as the checkers do, then waits a moment so a send
// logged next is unambiguously after the occurrence start.
func (e occurrenceEnv) raise(t *testing.T, a *domain.Alert) {
	t.Helper()
	a.Status = domain.AlertStatusActive
	a.LastSeen = time.Now()
	if err := e.alerts.Save(context.Background(), a); err != nil {
		t.Fatalf("save alert: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
}

func (e occurrenceEnv) logSent(t *testing.T, alertID, channel, externalID string) {
	t.Helper()
	if err := e.notifs.LogNotification(context.Background(), &domain.NotificationLog{
		AlertID: alertID, Channel: channel, ExternalID: externalID, Status: "sent",
	}); err != nil {
		t.Fatalf("log notification: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
}

func (e occurrenceEnv) alreadySent(t *testing.T, tenantID, alertID, channel string) bool {
	t.Helper()
	sent, err := e.notifs.CheckAlreadySent(context.Background(), tenantID, alertID, channel)
	if err != nil {
		t.Fatalf("CheckAlreadySent: %v", err)
	}
	return sent
}

func TestCheckAlreadySent_NotifiesOncePerOccurrence(t *testing.T) {
	e := newOccurrenceEnv(t)
	ctx := context.Background()
	tenant := fmt.Sprintf("notify-occ-t-%d", time.Now().UnixNano())
	alertID := "notify-occ-" + tenant
	a := occurrenceAlert(tenant, alertID)

	// Occurrence 1: not yet sent, then sent.
	e.raise(t, a)
	if e.alreadySent(t, tenant, alertID, "slack") {
		t.Fatal("reported sent before any send")
	}
	e.logSent(t, alertID, "slack", "first")
	if !e.alreadySent(t, tenant, alertID, "slack") {
		t.Fatal("not reported sent after the send")
	}

	// Re-detected while still active: same occurrence, no second message.
	e.raise(t, a)
	if !e.alreadySent(t, tenant, alertID, "slack") {
		t.Error("a re-detection while the alert stayed active counted as a new occurrence")
	}

	// Resolved and back: a new occurrence, which must be notified.
	if err := e.alerts.Resolve(ctx, tenant, alertID); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	e.raise(t, a)
	if e.alreadySent(t, tenant, alertID, "slack") {
		t.Fatal("the recurrence is suppressed by the previous occurrence's send")
	}
	e.logSent(t, alertID, "slack", "second")
	if !e.alreadySent(t, tenant, alertID, "slack") {
		t.Error("the second occurrence's send is not counted")
	}
}

// Rows saved before occurrence_started_at existed have it NULL; the dedup must
// then fall back to first_detected, which keeps today's behaviour for them.
func TestCheckAlreadySent_FallsBackToFirstDetectedForOlderRows(t *testing.T) {
	e := newOccurrenceEnv(t)
	tenant := fmt.Sprintf("notify-occ-legacy-%d", time.Now().UnixNano())
	alertID := "notify-occ-" + tenant
	e.raise(t, occurrenceAlert(tenant, alertID))
	e.logSent(t, alertID, "slack", "legacy")
	if err := e.db.Model(&database.Alert{}).Where("tenant_id = ? AND alert_id = ?", tenant, alertID).
		Update("occurrence_started_at", nil).Error; err != nil {
		t.Fatalf("clear occurrence_started_at: %v", err)
	}
	if !e.alreadySent(t, tenant, alertID, "slack") {
		t.Error("a send after first_detected is not counted for a row without occurrence_started_at")
	}
}

// notification_logs carries no tenant; the occurrence is read from the alert
// row of the tenant that asks.
func TestCheckAlreadySent_ReadsTheAskingTenantsAlert(t *testing.T) {
	e := newOccurrenceEnv(t)
	stamp := time.Now().UnixNano()
	alertID := fmt.Sprintf("notify-occ-shared-%d", stamp)
	tenantA := fmt.Sprintf("notify-occ-a-%d", stamp)
	tenantB := fmt.Sprintf("notify-occ-b-%d", stamp)

	e.raise(t, occurrenceAlert(tenantA, alertID))
	e.logSent(t, alertID, "slack", "for-a")
	// Tenant B's occurrence starts after that send, so it has not been notified.
	e.raise(t, occurrenceAlert(tenantB, alertID))
	if e.alreadySent(t, tenantB, alertID, "slack") {
		t.Error("tenant B's occurrence counted tenant A's earlier send")
	}
	if !e.alreadySent(t, tenantA, alertID, "slack") {
		t.Error("tenant A lost its send")
	}
}

// Resolving acts on the current occurrence's send, not the first one ever.
func TestGetByAlertAndChannel_ReturnsTheMostRecentSend(t *testing.T) {
	e := newOccurrenceEnv(t)
	alertID := fmt.Sprintf("notify-occ-latest-%d", time.Now().UnixNano())
	e.logSent(t, alertID, "gitlab", "11")
	e.logSent(t, alertID, "gitlab", "42")

	got, err := e.notifs.GetByAlertAndChannel(context.Background(), alertID, "gitlab", "sent")
	if err != nil || got == nil {
		t.Fatalf("GetByAlertAndChannel: %v, %v", got, err)
	}
	if got.ExternalID != "42" {
		t.Errorf("external id = %q, want the latest, 42", got.ExternalID)
	}
}

// An operator reopening a resolved alert (UpdateStatus to active) also starts
// a new occurrence; one that stays open does not.
func TestCheckAlreadySent_AReopenedAlertIsANewOccurrence(t *testing.T) {
	e := newOccurrenceEnv(t)
	ctx := context.Background()
	tenant := fmt.Sprintf("notify-occ-reopen-%d", time.Now().UnixNano())
	alertID := "notify-occ-" + tenant

	e.raise(t, occurrenceAlert(tenant, alertID))
	e.logSent(t, alertID, "email", "first")

	// Acknowledging keeps the occurrence.
	if err := e.alerts.UpdateStatus(ctx, tenant, alertID, domain.AlertStatusAcknowled, "op"); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if err := e.alerts.UpdateStatus(ctx, tenant, alertID, domain.AlertStatusActive, ""); err != nil {
		t.Fatalf("back to active from acknowledged: %v", err)
	}
	if !e.alreadySent(t, tenant, alertID, "email") {
		t.Error("acknowledged and back to active started a new occurrence")
	}

	// Resolving and reopening starts one.
	if err := e.alerts.Resolve(ctx, tenant, alertID); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := e.alerts.UpdateStatus(ctx, tenant, alertID, domain.AlertStatusActive, ""); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if e.alreadySent(t, tenant, alertID, "email") {
		t.Error("a reopened alert is suppressed by the previous occurrence's send")
	}
}
