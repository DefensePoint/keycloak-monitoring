// Coverage for the queries that decide which alerts share one tracked issue
// and when that issue may be closed.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset. Only rows under a notify-share- tenant or alert are touched.
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

type shareEnv struct {
	notifs notifications.Repository
	alerts alerts.Repository
	db     *gorm.DB
	tenant string
}

func newShareEnv(t *testing.T) shareEnv {
	t.Helper()

	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	db := client.DB()
	t.Cleanup(func() {
		db.Unscoped().Where("tenant_id LIKE ?", "notify-share-%").Delete(&database.Alert{})
		db.Where("alert_id LIKE ?", "notify-share-%").Delete(&database.NotificationLog{})
	})
	return shareEnv{
		notifs: NewRepository(client),
		alerts: alertspostgres.NewRepository(db),
		db:     db,
		tenant: fmt.Sprintf("notify-share-%d", time.Now().UnixNano()),
	}
}

// raise saves an identity provider alert, then waits a moment so a send logged
// next is unambiguously after the occurrence start.
func (e shareEnv) raise(t *testing.T, alertID, provider, realm string) *domain.Alert {
	t.Helper()
	return e.raiseResource(t, alertID, "identity_provider", provider, realm)
}

func (e shareEnv) raiseResource(t *testing.T, alertID, resourceType, name, realm string) *domain.Alert {
	t.Helper()
	now := time.Now()
	alert := &domain.Alert{
		TenantID: e.tenant, AlertID: "notify-share-" + alertID,
		Source: domain.AlertSourceConfiguration, Type: domain.AlertTypeIdentityProvider,
		Severity: domain.AlertSeverityWarning, Status: domain.AlertStatusActive,
		Title:        fmt.Sprintf("Resource %q needs attention", name),
		ResourceType: resourceType, ResourceID: name + "-internal",
		ResourceName: name, RealmName: realm,
		FirstDetected: now, LastSeen: now,
	}
	if err := e.alerts.Save(context.Background(), alert); err != nil {
		t.Fatalf("save alert: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	return alert
}

func (e shareEnv) logSent(t *testing.T, alert *domain.Alert, externalID string) {
	t.Helper()
	if err := e.notifs.LogNotification(context.Background(), &domain.NotificationLog{
		AlertID: alert.AlertID, Channel: "gitlab", ExternalID: externalID, Status: "sent",
	}); err != nil {
		t.Fatalf("log notification: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
}

func (e shareEnv) shared(t *testing.T, alert *domain.Alert) string {
	t.Helper()
	got, err := e.notifs.FindSharedExternalID(context.Background(), notifications.GroupOf(alert), "gitlab")
	if err != nil {
		t.Fatalf("FindSharedExternalID: %v", err)
	}
	return got
}

func (e shareEnv) sharing(t *testing.T, externalID string, alert *domain.Alert) int {
	t.Helper()
	got, err := e.notifs.CountSharingAlerts(context.Background(), e.tenant, "gitlab", externalID, alert.AlertID)
	if err != nil {
		t.Fatalf("CountSharingAlerts: %v", err)
	}
	return got
}

func TestFindSharedExternalID_GivesOneProvidersAlertsTheSameIssue(t *testing.T) {
	e := newShareEnv(t)

	missing := e.raise(t, "missing", "okta", "prod")
	e.logSent(t, missing, "42")
	expiring := e.raise(t, "expiring", "okta", "prod")

	if got := e.shared(t, expiring); got != "42" {
		t.Errorf("shared issue = %q, want the one the provider's open alert holds, 42", got)
	}
}

func TestFindSharedExternalID_KeepsOverlappingNamesAndRealmsApart(t *testing.T) {
	e := newShareEnv(t)

	eu := e.raise(t, "eu", "okta-eu", "prod")
	e.logSent(t, eu, "42")

	cases := []struct {
		name  string
		alert *domain.Alert
	}{
		{"a provider whose name the open one merely contains", e.raise(t, "okta", "okta", "prod")},
		{"the same alias in another realm", e.raise(t, "staging", "okta-eu", "staging")},
		{"another resource of the same name", e.raiseResource(t, "realm", "realm", "okta-eu", "prod")},
	}

	for _, c := range cases {
		if got := e.shared(t, c.alert); got != "" {
			t.Errorf("%s was given issue %q, want an issue of its own", c.name, got)
		}
	}
}

func TestFindSharedExternalID_IgnoresIssuesNoOpenAlertHolds(t *testing.T) {
	e := newShareEnv(t)
	ctx := context.Background()

	held := e.raise(t, "held", "okta", "prod")
	e.logSent(t, held, "42")
	joining := e.raise(t, "joining", "okta", "prod")

	if err := e.alerts.Resolve(ctx, e.tenant, held.AlertID); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := e.shared(t, joining); got != "" {
		t.Errorf("shared issue = %q, want none: the alert that held it resolved and its issue closed", got)
	}

	// Acknowledged is still open: the problem stands and so does its issue.
	if err := e.alerts.UpdateStatus(ctx, e.tenant, held.AlertID, domain.AlertStatusAcknowled, "op"); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if got := e.shared(t, joining); got != "42" {
		t.Errorf("shared issue = %q, want 42: an acknowledged alert still holds its issue", got)
	}
}

// A send from a previous occurrence names an issue that was closed when that
// occurrence resolved, so the recurrence must open a new one.
func TestFindSharedExternalID_IgnoresSendsFromAnEarlierOccurrence(t *testing.T) {
	e := newShareEnv(t)
	ctx := context.Background()

	held := e.raise(t, "held", "okta", "prod")
	e.logSent(t, held, "42")
	if err := e.alerts.Resolve(ctx, e.tenant, held.AlertID); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	e.raise(t, "held", "okta", "prod")

	joining := e.raise(t, "joining", "okta", "prod")
	if got := e.shared(t, joining); got != "" {
		t.Errorf("shared issue = %q, want none: 42 belongs to the occurrence that resolved", got)
	}
}

// The alert an expiry supersedes is resolved without a notice of its own, so
// the issue it holds is still open and the expiry takes it over.
func TestFindHeldExternalID_FindsTheIssueOfAnAlertResolvedWithoutANotice(t *testing.T) {
	e := newShareEnv(t)
	ctx := context.Background()

	expiring := e.raise(t, "expiring", "okta", "prod")
	e.logSent(t, expiring, "42")
	if err := e.alerts.Resolve(ctx, e.tenant, expiring.AlertID); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	got, title, err := e.notifs.FindHeldExternalID(ctx, e.tenant, "gitlab", []string{expiring.AlertID})
	if err != nil {
		t.Fatalf("FindHeldExternalID: %v", err)
	}
	if got != "42" {
		t.Errorf("held issue = %q, want the one the superseded alert holds, 42", got)
	}
	if title != expiring.Title || title == "" {
		t.Errorf("holder title = %q, want the superseded alert's %q", title, expiring.Title)
	}

	for name, ids := range map[string][]string{
		"an alert that was never notified": {"notify-share-unknown"},
		"nothing superseded":               nil,
	} {
		got, _, err := e.notifs.FindHeldExternalID(ctx, e.tenant, "gitlab", ids)
		if err != nil {
			t.Fatalf("FindHeldExternalID: %v", err)
		}
		if got != "" {
			t.Errorf("%s gave issue %q, want none", name, got)
		}
	}
}

func TestFindHeldExternalID_IgnoresSendsFromAnEarlierOccurrence(t *testing.T) {
	e := newShareEnv(t)
	ctx := context.Background()

	expiring := e.raise(t, "expiring", "okta", "prod")
	e.logSent(t, expiring, "42")
	if err := e.alerts.Resolve(ctx, e.tenant, expiring.AlertID); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	e.raise(t, "expiring", "okta", "prod")

	got, _, err := e.notifs.FindHeldExternalID(ctx, e.tenant, "gitlab", []string{expiring.AlertID})
	if err != nil {
		t.Fatalf("FindHeldExternalID: %v", err)
	}
	if got != "" {
		t.Errorf("held issue = %q, want none: 42 belongs to the occurrence that resolved", got)
	}
}

func TestCountSharingAlerts_CountsOnlyTheOtherOpenHolders(t *testing.T) {
	e := newShareEnv(t)
	ctx := context.Background()

	missing := e.raise(t, "missing", "okta", "prod")
	e.logSent(t, missing, "42")
	expiring := e.raise(t, "expiring", "okta", "prod")
	e.logSent(t, expiring, "42")
	elsewhere := e.raise(t, "elsewhere", "okta", "staging")
	e.logSent(t, elsewhere, "43")

	if got := e.sharing(t, "42", missing); got != 1 {
		t.Errorf("alerts sharing 42 besides the first = %d, want 1", got)
	}
	if got := e.sharing(t, "43", elsewhere); got != 0 {
		t.Errorf("alerts sharing 43 besides its own = %d, want 0", got)
	}

	if err := e.alerts.Resolve(ctx, e.tenant, expiring.AlertID); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := e.sharing(t, "42", missing); got != 0 {
		t.Errorf("alerts sharing 42 after the last other one resolved = %d, want 0", got)
	}
}

// Repeated sends of one alert, from a retry or a reopen, are one holder.
func TestCountSharingAlerts_CountsAnAlertOnce(t *testing.T) {
	e := newShareEnv(t)

	missing := e.raise(t, "missing", "okta", "prod")
	e.logSent(t, missing, "42")
	expiring := e.raise(t, "expiring", "okta", "prod")
	e.logSent(t, expiring, "42")
	e.logSent(t, expiring, "42")

	if got := e.sharing(t, "42", missing); got != 1 {
		t.Errorf("alerts sharing 42 = %d, want 1: two sends of one alert are one holder", got)
	}
}

// notification_logs carries no tenant, so the holders are read through the
// alert rows of the tenant that asks.
func TestCountSharingAlerts_StaysWithinTheTenant(t *testing.T) {
	e := newShareEnv(t)

	missing := e.raise(t, "missing", "okta", "prod")
	e.logSent(t, missing, "42")

	now := time.Now()
	other := &domain.Alert{
		TenantID: e.tenant + "-b", AlertID: "notify-share-other-tenant",
		Source: domain.AlertSourceConfiguration, Type: domain.AlertTypeIdentityProvider,
		Severity: domain.AlertSeverityWarning, Status: domain.AlertStatusActive,
		Title: "Another tenant's provider", ResourceType: "identity_provider",
		ResourceID: "okta-internal", ResourceName: "okta", RealmName: "prod",
		FirstDetected: now, LastSeen: now,
	}
	if err := e.alerts.Save(context.Background(), other); err != nil {
		t.Fatalf("save alert: %v", err)
	}
	e.logSent(t, other, "42")

	if got := e.sharing(t, "42", missing); got != 0 {
		t.Errorf("alerts sharing 42 = %d, want 0: the other holder belongs to another tenant", got)
	}
}

func TestCountIgnoredSharingAlerts_CountsOnlyIgnoredHolders(t *testing.T) {
	e := newShareEnv(t)
	ctx := context.Background()

	ignored := e.raise(t, "ignored", "okta", "prod")
	active := e.raise(t, "active", "okta", "prod")
	e.logSent(t, ignored, "7")
	e.logSent(t, active, "7")
	if err := e.alerts.UpdateStatus(ctx, e.tenant, ignored.AlertID, domain.AlertStatusIgnored, ""); err != nil {
		t.Fatalf("ignore: %v", err)
	}

	got, err := e.notifs.CountIgnoredSharingAlerts(ctx, e.tenant, "gitlab", "7")
	if err != nil {
		t.Fatalf("CountIgnoredSharingAlerts: %v", err)
	}
	if got != 1 {
		t.Errorf("ignored holders of issue 7 = %d, want 1", got)
	}
}
