package fx

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspg "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/operator"
	operatorpg "github.com/DefensePoint/keycloak-monitoring/internal/operator/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/reports"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// A report is the one place a tenant's alerts and its operators' work leave the
// platform as a document, so a caller limited to some realms must get a report
// of exactly those realms. These tests drive the report data step through the
// adapters the application wires, over real rows: the PDF is only a rendering
// of this data, and rendering it needs a browser CI does not have.

const (
	reportRealmAllowed = "realmB"
	reportRealmDenied  = "realmA"
)

// reportRealms is every realm the seeded tenant holds alerts in, including
// names a careless filter would confuse with the allowed one.
var reportRealms = []string{reportRealmAllowed, reportRealmDenied, reportRealmAllowed + "x", strings.ToLower(reportRealmAllowed)}

// reportAlertsPerRealm is how many active alerts each realm gets.
const reportAlertsPerRealm = 2

type reportScopeFixture struct {
	service  reports.Service
	tenantID string
	// operators maps each realm to the email of the operator who acted on an
	// alert there, and only there.
	operators map[string]string
}

func newReportScopeFixture(t *testing.T) *reportScopeFixture {
	t.Helper()

	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)

	log := logger.NewNoop()
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), log)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	db := client.DB()

	run := time.Now().UnixNano()
	f := &reportScopeFixture{tenantID: fmt.Sprintf("report-scope-%d", run), operators: map[string]string{}}
	t.Cleanup(func() {
		db.Exec("DELETE FROM operator_actions WHERE tenant_id = ?", f.tenantID)
		db.Exec("DELETE FROM configuration_alerts WHERE tenant_id = ?", f.tenantID)
		db.Exec("DELETE FROM users WHERE email LIKE ?", fmt.Sprintf("report-scope-%d-%%", run))
	})

	now := time.Now().UTC()
	for _, realm := range reportRealms {
		var alertIDs []uint
		for n := 1; n <= reportAlertsPerRealm; n++ {
			var id uint
			err := db.Raw(`INSERT INTO configuration_alerts
				(created_at, updated_at, tenant_id, alert_id, source, type, severity, status, title, description,
				 resource_type, resource_id, resource_name, realm_name, first_detected, last_seen)
				VALUES (now(), now(), ?, ?, 'keycloak', 'config', 'high', 'active', ?, 'seeded', 'realm', ?, ?, ?, ?, ?)
				RETURNING id`,
				f.tenantID, fmt.Sprintf("alert-%s-%d", realm, n), fmt.Sprintf("alert in %s number %d", realm, n),
				realm, realm, realm, now.Add(-time.Hour), now).Scan(&id).Error
			if err != nil {
				t.Fatalf("seed alert in %s: %v", realm, err)
			}
			alertIDs = append(alertIDs, id)
		}

		email := fmt.Sprintf("report-scope-%d-op-%s@example.invalid", run, strings.ToLower(realm)+fmt.Sprint(len(f.operators)))
		user := &database.User{Subject: email, Email: email, Username: email, IsActive: true}
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("seed operator for %s: %v", realm, err)
		}
		f.operators[realm] = email

		if err := operatorpg.NewRepository(client).RecordAction(context.Background(), &domain.OperatorAction{
			TenantID: f.tenantID, AlertID: alertIDs[0], OperatorID: user.ID, OperatorEmail: email,
			ActionType: "acknowledged", ActionTime: now.Add(-time.Minute), PreviousStatus: "active",
			AlertSeverity: "high", AlertType: "config", RealmName: realm, ResponseTimeSeconds: 60,
		}); err != nil {
			t.Fatalf("seed operator action in %s: %v", realm, err)
		}
	}

	// The adapters the application wires, over the real repositories. Events
	// are left out: the report gathers them but never prints them.
	f.service = reports.NewService(
		nil,
		provideReportsAlertReader(alerts.NewService(alertspg.NewRepository(db), log)),
		provideReportsOperatorReader(operator.NewService(operatorpg.NewRepository(client))),
		nil,
		log,
		nil,
	)
	return f
}

func (f *reportScopeFixture) aggregate(t *testing.T, scope reports.RealmScope) *reports.Data {
	t.Helper()
	data, err := f.service.AggregateData(context.Background(), &reports.GenerateRequest{
		TenantID: f.tenantID, TenantName: "report scope",
		StartDate: time.Now().Add(-24 * time.Hour), EndDate: time.Now().Add(24 * time.Hour),
		Scope: scope,
		// Every section, so the realm scope is what decides what is left.
		Access: reports.SectionAccess{Alerts: true, Events: true, Operators: true},
	})
	if err != nil {
		t.Fatalf("AggregateData: %v", err)
	}
	return data
}

// alertRealms and operatorEmails are what the report would print.
func alertRealms(d *reports.Data) []string {
	var out []string
	for _, a := range d.AlertList {
		if !slices.Contains(out, a.RealmName) {
			out = append(out, a.RealmName)
		}
	}
	slices.Sort(out)
	return out
}

func operatorEmails(d *reports.Data) []string {
	var out []string
	for _, m := range d.OperatorMetrics {
		out = append(out, m.OperatorEmail)
	}
	slices.Sort(out)
	return out
}

func TestReportRealmScope_ARestrictedCallerGetsOnlyTheirRealm(t *testing.T) {
	f := newReportScopeFixture(t)

	d := f.aggregate(t, reports.RealmScope{Realms: []string{reportRealmAllowed}})

	if got := alertRealms(d); !slices.Equal(got, []string{reportRealmAllowed}) {
		t.Errorf("report lists alerts from %v, want only %s", got, reportRealmAllowed)
	}
	if d.TotalAlerts != reportAlertsPerRealm {
		t.Errorf("total alerts = %d, want %d: the count includes other realms", d.TotalAlerts, reportAlertsPerRealm)
	}
	if got, want := operatorEmails(d), []string{f.operators[reportRealmAllowed]}; !slices.Equal(got, want) {
		t.Errorf("operator metrics name %v, want only %v: work done in other realms is reported", got, want)
	}
	for _, m := range d.OperatorMetrics {
		if m.TotalAlertsHandled != 1 {
			t.Errorf("%s handled %d alerts, want 1", m.OperatorEmail, m.TotalAlertsHandled)
		}
	}
}

func TestReportRealmScope_AnUnrestrictedCallerGetsEveryRealm(t *testing.T) {
	f := newReportScopeFixture(t)

	d := f.aggregate(t, reports.RealmScope{All: true})

	want := slices.Clone(reportRealms)
	slices.Sort(want)
	if got := alertRealms(d); !slices.Equal(got, want) {
		t.Errorf("report lists alerts from %v, want every realm %v", got, want)
	}
	if d.TotalAlerts != reportAlertsPerRealm*len(reportRealms) {
		t.Errorf("total alerts = %d, want %d", d.TotalAlerts, reportAlertsPerRealm*len(reportRealms))
	}
	if got := operatorEmails(d); len(got) != len(reportRealms) {
		t.Errorf("operator metrics name %v, want one operator per realm", got)
	}
}

// A policy naming no realm grants nothing. The repositories read an empty realm
// list as "no filter", so the report must not hand them one.
func TestReportRealmScope_ACallerAllowedNoRealmGetsNothing(t *testing.T) {
	f := newReportScopeFixture(t)

	d := f.aggregate(t, reports.RealmScope{Realms: []string{}})

	if d.TotalAlerts != 0 || len(d.AlertList) != 0 {
		t.Errorf("report carries %d alerts (%v) for a caller allowed no realm", d.TotalAlerts, alertRealms(d))
	}
	if len(d.OperatorMetrics) != 0 {
		t.Errorf("report carries operator metrics %v for a caller allowed no realm", operatorEmails(d))
	}
}

// A caller allowed a look-alike realm gets that realm, not the one it resembles.
func TestReportRealmScope_ALookalikeRealmIsItsOwnRealm(t *testing.T) {
	f := newReportScopeFixture(t)

	lookalike := reportRealmAllowed + "x"
	d := f.aggregate(t, reports.RealmScope{Realms: []string{lookalike}})

	if got := alertRealms(d); !slices.Equal(got, []string{lookalike}) {
		t.Errorf("report lists alerts from %v, want only %s", got, lookalike)
	}
	if got, want := operatorEmails(d), []string{f.operators[lookalike]}; !slices.Equal(got, want) {
		t.Errorf("operator metrics name %v, want only %v", got, want)
	}
}
