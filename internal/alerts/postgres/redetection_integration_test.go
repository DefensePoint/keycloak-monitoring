package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

func saveDetection(t *testing.T, r *Repository, tenantID, alertID string, status domain.AlertStatus, severity domain.AlertSeverity, description, metadata string) {
	t.Helper()
	now := time.Now().UTC()
	if err := r.Save(context.Background(), &domain.Alert{
		TenantID: tenantID, AlertID: alertID, CheckType: "events-login-error", Status: status,
		Source: domain.AlertSourceEvent, Type: domain.AlertTypeSecurity, Severity: severity,
		Title: alertID, Description: description, Metadata: metadata,
		ResourceType: "realm", ResourceID: "master", ResourceName: "master", RealmName: "master",
		FirstDetected: now, LastSeen: now,
	}); err != nil {
		t.Fatalf("save %s: %v", alertID, err)
	}
}

func timesOf(t *testing.T, r *Repository, tenantID, alertID string) (firstDetected, occurrenceStarted time.Time, resolvedAt *time.Time) {
	t.Helper()
	row := struct {
		FirstDetected       time.Time
		OccurrenceStartedAt time.Time
		ResolvedAt          *time.Time
	}{}
	if err := r.db.Raw(`SELECT first_detected, occurrence_started_at, resolved_at FROM configuration_alerts
		WHERE tenant_id = ? AND alert_id = ? AND deleted_at IS NULL`, tenantID, alertID).Scan(&row).Error; err != nil {
		t.Fatalf("read times: %v", err)
	}
	return row.FirstDetected, row.OccurrenceStartedAt, row.ResolvedAt
}

func metadataOf(t *testing.T, a *domain.Alert) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(a.Metadata), &m); err != nil {
		t.Fatalf("metadata %q: %v", a.Metadata, err)
	}
	return m
}

// A re-detection must show what was detected now: a Warning that became
// Critical with a higher count reads as Critical with that count, while the
// first detection, the occurrence and the notification record stay.
func TestSave_ARedetectionWritesTheCurrentSeverityAndCount(t *testing.T) {
	r, tenant := newOpenByCheckRepo(t)
	ctx := context.Background()
	notified := "2026-10-02T06:00:00Z"
	saveDetection(t, r, tenant, "burst", domain.AlertStatusActive, domain.AlertSeverityWarning,
		"3 failed logins", `{"count":3,"last_notified_at":"`+notified+`"}`)
	firstDetected, occurrence, _ := timesOf(t, r, tenant, "burst")

	time.Sleep(10 * time.Millisecond)
	saveDetection(t, r, tenant, "burst", domain.AlertStatusActive, domain.AlertSeverityCritical,
		"8 failed logins", `{"count":8}`)

	a, err := r.GetByAlertID(ctx, tenant, "burst")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if a.Severity != domain.AlertSeverityCritical {
		t.Errorf("severity = %s, want critical", a.Severity)
	}
	if a.Description != "8 failed logins" {
		t.Errorf("description = %q, want the latest count", a.Description)
	}
	m := metadataOf(t, a)
	if m["count"] != float64(8) {
		t.Errorf("metadata count = %v, want 8", m["count"])
	}
	if m["last_notified_at"] != notified {
		t.Errorf("metadata last_notified_at = %v, want %s carried over", m["last_notified_at"], notified)
	}
	gotFirst, gotOccurrence, _ := timesOf(t, r, tenant, "burst")
	if !gotFirst.Equal(firstDetected) {
		t.Errorf("first_detected = %v, want %v unchanged", gotFirst, firstDetected)
	}
	if !gotOccurrence.Equal(occurrence) {
		t.Errorf("occurrence_started_at = %v, want %v unchanged", gotOccurrence, occurrence)
	}
}

// An operator's acknowledgement survives a re-detection that changes the
// severity and the count.
func TestSave_ARedetectionKeepsTheAcknowledgement(t *testing.T) {
	r, tenant := newOpenByCheckRepo(t)
	ctx := context.Background()
	saveDetection(t, r, tenant, "acked", domain.AlertStatusActive, domain.AlertSeverityWarning, "3 failed logins", `{"count":3}`)
	if err := r.UpdateStatus(ctx, tenant, "acked", domain.AlertStatusAcknowled, "alice@example.test"); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	before, _ := r.GetByAlertID(ctx, tenant, "acked")

	saveDetection(t, r, tenant, "acked", domain.AlertStatusAcknowled, domain.AlertSeverityCritical, "8 failed logins", `{"count":8}`)

	a, _ := r.GetByAlertID(ctx, tenant, "acked")
	if a.Status != domain.AlertStatusAcknowled || a.AcknowledgedBy != "alice@example.test" ||
		a.AcknowledgedAt == nil || before.AcknowledgedAt == nil || !a.AcknowledgedAt.Equal(*before.AcknowledgedAt) {
		t.Errorf("status %s, acknowledged at %v by %q; want acknowledged, at %v by alice@example.test",
			a.Status, a.AcknowledgedAt, a.AcknowledgedBy, before.AcknowledgedAt)
	}
	if a.Severity != domain.AlertSeverityCritical || a.Description != "8 failed logins" {
		t.Errorf("severity %s, description %q; want the re-detection's", a.Severity, a.Description)
	}
}

// An alert resolved by hand while its condition still holds is saved again as
// resolved on the next poll, with no resolution time of its own. The time the
// operator resolved it must stay; a reopen still clears it.
func TestSave_KeepsTheResolutionTimeWhileTheAlertStaysResolved(t *testing.T) {
	r, tenant := newOpenByCheckRepo(t)
	ctx := context.Background()
	saveDetection(t, r, tenant, "resolved", domain.AlertStatusActive, domain.AlertSeverityWarning, "3 failed logins", `{"count":3}`)
	if err := r.Resolve(ctx, tenant, "resolved"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	_, occurrence, resolvedAt := timesOf(t, r, tenant, "resolved")
	if resolvedAt == nil {
		t.Fatal("Resolve set no resolved_at")
	}

	saveDetection(t, r, tenant, "resolved", domain.AlertStatusResolved, domain.AlertSeverityWarning, "5 failed logins", `{"count":5}`)
	a, _ := r.GetByAlertID(ctx, tenant, "resolved")
	_, _, kept := timesOf(t, r, tenant, "resolved")
	if a.Status != domain.AlertStatusResolved {
		t.Fatalf("status = %s, want resolved", a.Status)
	}
	if kept == nil || !kept.Equal(*resolvedAt) {
		t.Errorf("resolved_at = %v after the next poll, want %v kept", kept, *resolvedAt)
	}
	if a.Description != "5 failed logins" {
		t.Errorf("description = %q, want the latest count", a.Description)
	}

	time.Sleep(10 * time.Millisecond)
	saveDetection(t, r, tenant, "resolved", domain.AlertStatusActive, domain.AlertSeverityWarning, "6 failed logins", `{"count":6}`)
	_, reopened, cleared := timesOf(t, r, tenant, "resolved")
	if cleared != nil {
		t.Errorf("resolved_at = %v after reopening, want none", *cleared)
	}
	if !reopened.After(occurrence) {
		t.Errorf("occurrence started %v after reopening, want after %v", reopened, occurrence)
	}
}
