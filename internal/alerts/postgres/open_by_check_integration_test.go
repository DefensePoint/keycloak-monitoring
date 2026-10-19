package postgres

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// The checker services resolve a check's open alerts whose condition cleared,
// so ListOpenByCheck must return exactly those: this tenant's, this check's,
// active or acknowledged, nothing resolved, ignored or deleted.

func newOpenByCheckRepo(t *testing.T) (*Repository, string) {
	t.Helper()
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)

	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	tenantID := fmt.Sprintf("open-by-check-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		client.DB().Unscoped().Where("tenant_id LIKE ?", tenantID+"%").Delete(&database.ConfigurationAlert{})
	})
	return &Repository{db: client.DB()}, tenantID
}

func seedCheckAlert(t *testing.T, r *Repository, tenantID, alertID, checkType string, status domain.AlertStatus) {
	t.Helper()
	now := time.Now().UTC()
	if err := r.Save(context.Background(), &domain.Alert{
		TenantID: tenantID, AlertID: alertID, CheckType: checkType, Status: status,
		Source: domain.AlertSourceEvent, Type: domain.AlertTypeSecurity, Severity: domain.AlertSeverityWarning,
		Title: alertID, ResourceType: "realm", ResourceID: "master", ResourceName: "master", RealmName: "master",
		FirstDetected: now, LastSeen: now,
	}); err != nil {
		t.Fatalf("seed %s: %v", alertID, err)
	}
}

func TestListOpenByCheck_ReturnsOnlyThisChecksOpenAlerts(t *testing.T) {
	r, tenant := newOpenByCheckRepo(t)

	seedCheckAlert(t, r, tenant, "active", "events-login-error", domain.AlertStatusActive)
	seedCheckAlert(t, r, tenant, "acknowledged", "events-login-error", domain.AlertStatusAcknowled)
	seedCheckAlert(t, r, tenant, "resolved", "events-login-error", domain.AlertStatusResolved)
	seedCheckAlert(t, r, tenant, "ignored", "events-login-error", domain.AlertStatusIgnored)
	seedCheckAlert(t, r, tenant, "other-check", "events-idp-error", domain.AlertStatusActive)
	seedCheckAlert(t, r, tenant+"-other", "other-tenant", "events-login-error", domain.AlertStatusActive)
	seedCheckAlert(t, r, tenant, "deleted", "events-login-error", domain.AlertStatusActive)
	if err := r.Delete(context.Background(), tenant, "deleted"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	open, err := r.ListOpenByCheck(context.Background(), tenant, "events-login-error")
	if err != nil {
		t.Fatalf("ListOpenByCheck: %v", err)
	}
	var got []string
	for _, a := range open {
		got = append(got, a.AlertID)
	}
	slices.Sort(got)
	if want := []string{"acknowledged", "active"}; !slices.Equal(got, want) {
		t.Errorf("open alerts = %v, want %v", got, want)
	}
}

// Resolving through the repository and saving the alert active again, as a
// reopened alert is, must start a new occurrence: that is what lets the
// notification service announce the recurrence rather than suppress it.
func TestListOpenByCheck_AReopenedAlertStartsANewOccurrence(t *testing.T) {
	r, tenant := newOpenByCheckRepo(t)
	ctx := context.Background()
	seedCheckAlert(t, r, tenant, "recurring", "events-login-error", domain.AlertStatusActive)
	occurrence := func() time.Time {
		t.Helper()
		var at time.Time
		if err := r.db.Raw(`SELECT occurrence_started_at FROM configuration_alerts WHERE tenant_id = ? AND alert_id = ?`,
			tenant, "recurring").Scan(&at).Error; err != nil {
			t.Fatalf("read occurrence_started_at: %v", err)
		}
		return at
	}
	first := occurrence()

	if err := r.Resolve(ctx, tenant, "recurring"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if open, _ := r.ListOpenByCheck(ctx, tenant, "events-login-error"); len(open) != 0 {
		t.Fatalf("a resolved alert is still listed as open")
	}

	time.Sleep(10 * time.Millisecond)
	seedCheckAlert(t, r, tenant, "recurring", "events-login-error", domain.AlertStatusActive)
	again, err := r.GetByAlertID(ctx, tenant, "recurring")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if again.Status != domain.AlertStatusActive {
		t.Errorf("status after reopening = %s, want active", again.Status)
	}
	if next := occurrence(); !next.After(first) {
		t.Errorf("occurrence started %v, want after the first occurrence %v", next, first)
	}
}

// A save keeps an acknowledgement, but a save that reopens a resolved alert
// starts an occurrence nobody has acknowledged.
func TestSave_KeepsAnAcknowledgementUntilANewOccurrence(t *testing.T) {
	r, tenant := newOpenByCheckRepo(t)
	ctx := context.Background()
	seedCheckAlert(t, r, tenant, "acked", "events-login-error", domain.AlertStatusActive)
	if err := r.UpdateStatus(ctx, tenant, "acked", domain.AlertStatusAcknowled, "alice@example.test"); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}

	seedCheckAlert(t, r, tenant, "acked", "events-login-error", domain.AlertStatusAcknowled)
	if a, _ := r.GetByAlertID(ctx, tenant, "acked"); a.AcknowledgedAt == nil || a.AcknowledgedBy != "alice@example.test" {
		t.Fatalf("after a re-save: acknowledged at %v by %q, want kept", a.AcknowledgedAt, a.AcknowledgedBy)
	}

	// A producer that saves the alert active again, as the event converter
	// does on every matching event, un-acknowledges it: status and
	// acknowledgement must agree.
	seedCheckAlert(t, r, tenant, "acked", "events-login-error", domain.AlertStatusActive)
	if a, _ := r.GetByAlertID(ctx, tenant, "acked"); a.Status != domain.AlertStatusActive || a.AcknowledgedAt != nil || a.AcknowledgedBy != "" {
		t.Fatalf("after a save as active: status %s, acknowledged at %v by %q; want active and no acknowledgement",
			a.Status, a.AcknowledgedAt, a.AcknowledgedBy)
	}
	if err := r.UpdateStatus(ctx, tenant, "acked", domain.AlertStatusAcknowled, "alice@example.test"); err != nil {
		t.Fatalf("acknowledge again: %v", err)
	}

	if err := r.Resolve(ctx, tenant, "acked"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	seedCheckAlert(t, r, tenant, "acked", "events-login-error", domain.AlertStatusActive)
	if a, _ := r.GetByAlertID(ctx, tenant, "acked"); a.AcknowledgedAt != nil || a.AcknowledgedBy != "" {
		t.Errorf("after reopening: acknowledged at %v by %q, want none", a.AcknowledgedAt, a.AcknowledgedBy)
	}
}

// Deleting a tenant closes every alert it still has open on the notification
// channels, so ListUnresolved must return all of them, ignored included, and
// nothing resolved, deleted or another tenant's.
func TestListUnresolved_ReturnsEveryAlertNotResolved(t *testing.T) {
	r, tenant := newOpenByCheckRepo(t)

	seedCheckAlert(t, r, tenant, "active", "health_check", domain.AlertStatusActive)
	seedCheckAlert(t, r, tenant, "acknowledged", "event_monitor", domain.AlertStatusAcknowled)
	seedCheckAlert(t, r, tenant, "ignored", "events-login-error", domain.AlertStatusIgnored)
	seedCheckAlert(t, r, tenant, "resolved", "health_check", domain.AlertStatusResolved)
	seedCheckAlert(t, r, tenant+"-other", "other-tenant", "health_check", domain.AlertStatusActive)
	seedCheckAlert(t, r, tenant, "deleted", "health_check", domain.AlertStatusActive)
	if err := r.Delete(context.Background(), tenant, "deleted"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	open, err := r.ListUnresolved(context.Background(), tenant)
	if err != nil {
		t.Fatalf("ListUnresolved: %v", err)
	}
	var got []string
	for _, a := range open {
		got = append(got, a.AlertID)
	}
	slices.Sort(got)
	if want := []string{"acknowledged", "active", "ignored"}; !slices.Equal(got, want) {
		t.Errorf("unresolved alerts = %v, want %v", got, want)
	}
}
