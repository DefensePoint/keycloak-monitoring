package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// The AMFA mirror resolves a username against Keycloak when it writes its row.
// The Keycloak monitor takes the name from the event payload and leaves it
// empty for the event types Keycloak does not put one in — REFRESH_TOKEN,
// CODE_TO_TOKEN and similar. Where a login has both rows the name is sitting on
// the one about to be absorbed, so the merge carries it across rather than
// discarding it. This is why an events table full of bare UUIDs is not simply
// the price of that ingest decision.

func TestReconcileAMFAMerges_CopiesTheNameOntoAKeycloakTwinThatHasNone(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	ctx := context.Background()

	corr := testEventID(t, "amfa-name-corr")
	kcID := testEventID(t, "kc-name")
	amfaID := testEventID(t, "amfa-name")
	now := time.Now().UTC().Truncate(time.Millisecond)
	cleanupEventID(t, db, kcID)
	cleanupEventID(t, db, amfaID)

	// A Keycloak row as the monitor writes one for a token event: a user id and
	// nothing to call them.
	if err := repo.Save(ctx, &domain.Event{
		EventID: kcID, Type: "REFRESH_TOKEN", Category: "authentication",
		Severity: "info", Source: "keycloak:test-realm", SourceSystem: "keycloak",
		Status: "processed", Timestamp: now, AMFAEventID: &corr,
		UserID: "11111111-2222-3333-4444-555555555555",
	}); err != nil {
		t.Fatalf("save keycloak event: %v", err)
	}

	if err := repo.Save(ctx, &domain.Event{
		EventID: amfaID, Type: "LOGIN", Category: "authentication",
		Severity: "info", Source: "amfa:test-realm", SourceSystem: "amfa",
		Status: "processed", Timestamp: now, AMFAEventID: &corr,
		UserID:   "11111111-2222-3333-4444-555555555555",
		Username: "resolved-by-amfa", Email: "resolved@example.invalid",
	}); err != nil {
		t.Fatalf("save amfa event: %v", err)
	}

	if _, err := repo.ReconcileAMFAMerges(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got struct{ Username, Email string }
	if err := db.Raw(`SELECT username, email FROM events WHERE event_id = ?`, kcID).
		Scan(&got).Error; err != nil {
		t.Fatalf("reload keycloak twin: %v", err)
	}
	if got.Username != "resolved-by-amfa" {
		t.Errorf("username = %q, want the name AMFA had already resolved; without it the row "+
			"shows a bare UUID even though the platform knew who it was", got.Username)
	}
	if got.Email != "resolved@example.invalid" {
		t.Errorf("email = %q, want the address AMFA had already resolved", got.Email)
	}
}

// Keycloak wins where it has a name of its own, matching how risk_level and
// raw_data already behave on the same statement.
func TestReconcileAMFAMerges_KeycloakNameWinsOverAmfa(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	ctx := context.Background()

	corr := testEventID(t, "amfa-keep-corr")
	kcID := testEventID(t, "kc-keep")
	amfaID := testEventID(t, "amfa-keep")
	now := time.Now().UTC().Truncate(time.Millisecond)
	cleanupEventID(t, db, kcID)
	cleanupEventID(t, db, amfaID)

	if err := repo.Save(ctx, &domain.Event{
		EventID: kcID, Type: "LOGIN", Category: "authentication",
		Severity: "info", Source: "keycloak:test-realm", SourceSystem: "keycloak",
		Status: "processed", Timestamp: now, AMFAEventID: &corr,
		UserID: "aaaa", Username: "from-keycloak", Email: "kc@example.invalid",
	}); err != nil {
		t.Fatalf("save keycloak event: %v", err)
	}
	if err := repo.Save(ctx, &domain.Event{
		EventID: amfaID, Type: "LOGIN", Category: "authentication",
		Severity: "info", Source: "amfa:test-realm", SourceSystem: "amfa",
		Status: "processed", Timestamp: now, AMFAEventID: &corr,
		UserID: "aaaa", Username: "from-amfa", Email: "amfa@example.invalid",
	}); err != nil {
		t.Fatalf("save amfa event: %v", err)
	}

	if _, err := repo.ReconcileAMFAMerges(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got struct{ Username, Email string }
	if err := db.Raw(`SELECT username, email FROM events WHERE event_id = ?`, kcID).
		Scan(&got).Error; err != nil {
		t.Fatalf("reload keycloak twin: %v", err)
	}
	if got.Username != "from-keycloak" || got.Email != "kc@example.invalid" {
		t.Errorf("the Keycloak row's own name was overwritten: username=%q email=%q",
			got.Username, got.Email)
	}
}
