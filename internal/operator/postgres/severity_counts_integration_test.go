package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// The summary counted handled alerts as critical, high, medium and low, but
// alerts are critical, error, warning or info, so three of the four counters
// never moved. And it counted actions, so an alert acknowledged and then
// resolved counted twice by severity but once in the total. This test acts
// twice on alerts of every severity and reads the counts back.

func TestGetMetricsSummary_CountsEverySeverity(t *testing.T) {
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	db := client.DB()

	run := time.Now().UnixNano()
	tenantID := fmt.Sprintf("severity-counts-%d", run)
	email := fmt.Sprintf("severity-counts-%d@example.invalid", run)
	now := time.Now().UTC()

	user := &database.User{Subject: email, Email: email, Username: email, IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM operator_actions WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM configuration_alerts WHERE tenant_id = ?", tenantID)
		db.Unscoped().Delete(user)
	})

	want := map[domain.AlertSeverity]int{
		domain.AlertSeverityCritical: 1,
		domain.AlertSeverityError:    2,
		domain.AlertSeverityWarning:  3,
		domain.AlertSeverityInfo:     4,
	}
	repo := NewRepository(client)
	for severity, n := range want {
		for i := 0; i < n; i++ {
			alert := &database.ConfigurationAlert{
				TenantID: tenantID, AlertID: fmt.Sprintf("%s-%d", severity, i), Type: "security",
				Severity: string(severity), Status: "resolved", Title: "t", ResourceType: "realm",
				ResourceID: "r", ResourceName: "r", RealmName: "r", FirstDetected: now, LastSeen: now,
			}
			if err := db.Create(alert).Error; err != nil {
				t.Fatalf("seed a %s alert: %v", severity, err)
			}
			for j, action := range []string{"acknowledged", "resolved"} {
				if err := repo.RecordAction(context.Background(), &domain.OperatorAction{
					TenantID: tenantID, AlertID: alert.ID, OperatorID: user.ID, OperatorEmail: email,
					ActionType: action, ActionTime: now.Add(time.Duration(j-2) * time.Minute), PreviousStatus: "active",
					AlertSeverity: string(severity), AlertType: "security", RealmName: "r", ResponseTimeSeconds: 60,
				}); err != nil {
					t.Fatalf("record %s on a %s alert: %v", action, severity, err)
				}
			}
		}
	}

	summaries, err := repo.GetAllOperatorsSummary(context.Background(), tenantID, now.Add(-time.Hour), now.Add(time.Hour), nil)
	if err != nil || len(summaries) != 1 {
		t.Fatalf("GetAllOperatorsSummary = %d summaries, %v; want 1", len(summaries), err)
	}
	s := summaries[0]
	got := map[domain.AlertSeverity]int{
		domain.AlertSeverityCritical: s.CriticalAlertsHandled,
		domain.AlertSeverityError:    s.ErrorAlertsHandled,
		domain.AlertSeverityWarning:  s.WarningAlertsHandled,
		domain.AlertSeverityInfo:     s.InfoAlertsHandled,
	}
	for severity, n := range want {
		if got[severity] != n {
			t.Errorf("%s alerts handled = %d, want %d", severity, got[severity], n)
		}
	}
	if s.TotalAlertsHandled != 10 {
		t.Errorf("total alerts handled = %d, want 10", s.TotalAlertsHandled)
	}
}

// A re-detection can change an alert's severity between two actions on it.
// The alert still counts once, under the severity of the latest action.
func TestGetMetricsSummary_CountsAnEscalatedAlertOnce(t *testing.T) {
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	db := client.DB()

	run := time.Now().UnixNano()
	tenantID := fmt.Sprintf("severity-escalated-%d", run)
	email := fmt.Sprintf("severity-escalated-%d@example.invalid", run)
	now := time.Now().UTC()

	user := &database.User{Subject: email, Email: email, Username: email, IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	alert := &database.ConfigurationAlert{
		TenantID: tenantID, AlertID: "ssl-not-required", Type: "security",
		Severity: string(domain.AlertSeverityCritical), Status: "resolved", Title: "t", ResourceType: "realm",
		ResourceID: "r", ResourceName: "r", RealmName: "r", FirstDetected: now, LastSeen: now,
	}
	if err := db.Create(alert).Error; err != nil {
		t.Fatalf("seed alert: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM operator_actions WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM configuration_alerts WHERE tenant_id = ?", tenantID)
		db.Unscoped().Delete(user)
	})

	repo := NewRepository(client)
	for i, step := range []struct {
		action   string
		severity domain.AlertSeverity
	}{
		{"acknowledged", domain.AlertSeverityWarning},
		{"resolved", domain.AlertSeverityCritical},
	} {
		if err := repo.RecordAction(context.Background(), &domain.OperatorAction{
			TenantID: tenantID, AlertID: alert.ID, OperatorID: user.ID, OperatorEmail: email,
			ActionType: step.action, ActionTime: now.Add(time.Duration(i-2) * time.Minute), PreviousStatus: "active",
			AlertSeverity: string(step.severity), AlertType: "security", RealmName: "r", ResponseTimeSeconds: 60,
		}); err != nil {
			t.Fatalf("record %s: %v", step.action, err)
		}
	}

	s, err := repo.GetMetricsSummary(context.Background(), tenantID, email, now.Add(-time.Hour), now.Add(time.Hour), nil)
	if err != nil {
		t.Fatalf("GetMetricsSummary: %v", err)
	}
	if s.CriticalAlertsHandled != 1 || s.WarningAlertsHandled != 0 || s.TotalAlertsHandled != 1 {
		t.Errorf("critical %d, warning %d, total %d; want the alert once, as critical: 1, 0, 1",
			s.CriticalAlertsHandled, s.WarningAlertsHandled, s.TotalAlertsHandled)
	}
}
