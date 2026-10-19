package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// An alert's last_seen is the latest time its condition was seen. The event
// poller re-reads its whole window on every poll, newest first, so the oldest
// event in the window was saved last and its time overwrote the newer ones:
// a recurring alert showed the window's first occurrence as last seen.

func saveSeenAt(t *testing.T, r *Repository, tenantID string, firstDetected, lastSeen time.Time) {
	t.Helper()
	if err := r.Save(context.Background(), &domain.Alert{
		TenantID: tenantID, AlertID: "login-error", CheckType: "event_monitor", Status: domain.AlertStatusActive,
		Source: domain.AlertSourceEvent, Type: domain.AlertTypeEvent, Severity: domain.AlertSeverityError,
		Title: "LOGIN_ERROR in master", ResourceType: "event", ResourceID: "e", ResourceName: "LOGIN_ERROR", RealmName: "master",
		FirstDetected: firstDetected, LastSeen: lastSeen,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
}

func TestSave_LastSeenOnlyMovesForward(t *testing.T) {
	r, tenant := newOpenByCheckRepo(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)
	newest, middle, oldest := base, base.Add(-time.Minute), base.Add(-2*time.Minute)

	// The newest event first, as Keycloak returns them, then the older ones.
	saveSeenAt(t, r, tenant, newest, newest)
	saveSeenAt(t, r, tenant, middle, middle)
	saveSeenAt(t, r, tenant, oldest, oldest)

	got, err := r.GetByAlertID(ctx, tenant, "login-error")
	if err != nil || got == nil {
		t.Fatalf("GetByAlertID: %v", err)
	}
	if !got.LastSeen.Equal(newest) {
		t.Errorf("last_seen = %s, want the newest event %s", got.LastSeen, newest)
	}
	if !got.FirstDetected.Equal(newest) {
		t.Errorf("first_detected = %s, want it left as first saved (%s)", got.FirstDetected, newest)
	}

	// A newer sighting still moves it forward.
	later := base.Add(time.Minute)
	saveSeenAt(t, r, tenant, later, later)
	got, _ = r.GetByAlertID(ctx, tenant, "login-error")
	if !got.LastSeen.Equal(later) {
		t.Errorf("last_seen = %s after a newer sighting, want %s", got.LastSeen, later)
	}
}
