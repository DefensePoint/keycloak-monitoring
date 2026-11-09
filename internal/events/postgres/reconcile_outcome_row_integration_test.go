package postgres

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// One login can leave several Keycloak rows carrying the same amfa_event_id:
// an SSO login stamps it on its IDENTITY_PROVIDER_POST_LOGIN events as well as
// on its LOGIN, and a wrong code followed by the right one leaves a LOGIN_ERROR
// and a LOGIN. The AMFA row belongs to the row of its own type, so the merge
// puts the AMFA data there only. Only when no such row has arrived within
// amfaMergeFallbackAfter does it fall back to the other rows with the id, as it
// did before for every row.

// saveKC saves a Keycloak row of the given type carrying the correlation id.
func saveKC(t *testing.T, repo *Repository, db *gorm.DB, corr, suffix, typ string) string {
	t.Helper()
	id := testEventID(t, suffix)
	cleanupEventID(t, db, id)
	if err := repo.Save(context.Background(), &domain.Event{
		TenantID: fixtureTenant, EventID: id, Type: typ, Category: "authentication",
		Severity: "info", Description: "Keycloak " + typ, Source: "keycloak:test-realm",
		SourceSystem: "keycloak", Status: "processed", Timestamp: time.Now().UTC(),
		AMFAEventID: &corr,
	}); err != nil {
		t.Fatalf("save keycloak %s: %v", typ, err)
	}
	return id
}

// saveAMFA saves the AMFA row of a login, created age ago.
func saveAMFA(t *testing.T, repo *Repository, db *gorm.DB, corr, typ string, age time.Duration) string {
	t.Helper()
	id := testEventID(t, "amfa-row")
	cleanupEventID(t, db, id)
	if err := repo.Save(context.Background(), &domain.Event{
		TenantID: fixtureTenant, EventID: id, Type: typ, Category: "authentication",
		Severity: "info", Description: "AMFA " + typ, Source: "amfa:test-realm",
		SourceSystem: "amfa", Status: "processed", Timestamp: time.Now().UTC(),
		AMFAEventID: &corr, FinalStatus: strPtr(typ), Country: strPtr("DE"),
	}); err != nil {
		t.Fatalf("save amfa %s: %v", typ, err)
	}
	if age > 0 {
		if err := db.Exec(`UPDATE events SET created_at = now() - make_interval(secs => ?) WHERE event_id = ?`,
			age.Seconds(), id).Error; err != nil {
			t.Fatalf("age amfa row: %v", err)
		}
	}
	return id
}

// enriched reports whether the row carries the AMFA data.
func enriched(t *testing.T, db *gorm.DB, eventID string) bool {
	t.Helper()
	var country *string
	if err := db.Raw(`SELECT country FROM events WHERE event_id = ?`, eventID).Scan(&country).Error; err != nil {
		t.Fatalf("read %s: %v", eventID, err)
	}
	return country != nil && *country == "DE"
}

func rowExists(t *testing.T, db *gorm.DB, eventID string) bool {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM events WHERE event_id = ?`, eventID).Scan(&n).Error; err != nil {
		t.Fatalf("count %s: %v", eventID, err)
	}
	return n > 0
}

func reconcile(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := repo.ReconcileAMFAMerges(context.Background()); err != nil {
		t.Fatalf("ReconcileAMFAMerges: %v", err)
	}
}

func TestReconcileAMFAMerges_PutsTheAmfaDataOnTheRowOfItsOwnTypeOnly(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	corr := testEventID(t, "corr")
	postBroker := saveKC(t, repo, db, corr, "kc-post-broker", "IDENTITY_PROVIDER_POST_LOGIN")
	login := saveKC(t, repo, db, corr, "kc-login", "LOGIN")
	amfa := saveAMFA(t, repo, db, corr, "LOGIN", 0)

	reconcile(t, repo)

	if !enriched(t, db, login) {
		t.Errorf("the Keycloak LOGIN did not get the AMFA data")
	}
	if enriched(t, db, postBroker) {
		t.Errorf("the post broker row got the AMFA data too; it belongs to the LOGIN only")
	}
	if rowExists(t, db, amfa) {
		t.Errorf("the absorbed AMFA row is still there")
	}
}

func TestReconcileAMFAMerges_ARightCodeAfterAWrongOneKeepsTheDataOnTheLogin(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	corr := testEventID(t, "corr")
	wrongCode := saveKC(t, repo, db, corr, "kc-login-error", "LOGIN_ERROR")
	login := saveKC(t, repo, db, corr, "kc-login", "LOGIN")
	saveAMFA(t, repo, db, corr, "LOGIN", 0)

	reconcile(t, repo)

	if !enriched(t, db, login) {
		t.Errorf("the Keycloak LOGIN did not get the AMFA data")
	}
	if enriched(t, db, wrongCode) {
		t.Errorf("the wrong code's LOGIN_ERROR got the data of the successful login")
	}
}

func TestReconcileAMFAMerges_WaitsForTheRowOfItsOwnType(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	corr := testEventID(t, "corr")
	postBroker := saveKC(t, repo, db, corr, "kc-post-broker", "IDENTITY_PROVIDER_POST_LOGIN")
	amfa := saveAMFA(t, repo, db, corr, "LOGIN", time.Minute)

	// The Keycloak mirror has not brought the LOGIN in yet.
	reconcile(t, repo)
	if enriched(t, db, postBroker) {
		t.Errorf("the post broker row was merged before the LOGIN had a chance to arrive")
	}
	if !rowExists(t, db, amfa) {
		t.Fatalf("the AMFA row was absorbed before the LOGIN arrived")
	}

	// It arrives on the next poll.
	login := saveKC(t, repo, db, corr, "kc-login", "LOGIN")
	reconcile(t, repo)
	if !enriched(t, db, login) {
		t.Errorf("the LOGIN that arrived late did not get the AMFA data")
	}
	if enriched(t, db, postBroker) {
		t.Errorf("the post broker row got the AMFA data too")
	}
	if rowExists(t, db, amfa) {
		t.Errorf("the absorbed AMFA row is still there")
	}
}

func TestReconcileAMFAMerges_FallsBackToTheOtherRowsWhenNoneOfItsTypeArrives(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	corr := testEventID(t, "corr")
	first := saveKC(t, repo, db, corr, "kc-post-broker-1", "IDENTITY_PROVIDER_POST_LOGIN")
	second := saveKC(t, repo, db, corr, "kc-post-broker-2", "IDENTITY_PROVIDER_POST_LOGIN")
	amfa := saveAMFA(t, repo, db, corr, "LOGIN", amfaMergeFallbackAfter+time.Minute)

	// A realm whose LOGIN carries no amfa_event_id: after the wait, the rows
	// that do carry it get the data, as every row with the id did before.
	reconcile(t, repo)

	if !enriched(t, db, first) || !enriched(t, db, second) {
		t.Errorf("after the wait the post broker rows should get the AMFA data (got %v, %v)",
			enriched(t, db, first), enriched(t, db, second))
	}
	if rowExists(t, db, amfa) {
		t.Errorf("the absorbed AMFA row is still there")
	}
}
