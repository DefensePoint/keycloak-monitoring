package postgres

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// A refused login reaches AMFA under the id of its own Keycloak event, not
// under the AMFA decision id that Keycloak stamps on the event as
// amfa_event_id: the plugins send only a LOGIN under the decision id, since
// the engine rejects a second event with the same id and would otherwise lose
// the LOGIN that follows a wrong code. So the AMFA row of a refusal names its
// Keycloak twin by event_id, and the merge has to look there too.

// saveAMFAOutcome saves the AMFA row of one login outcome, stored by AMFA
// under amfaID, with the country telling the rows apart.
func saveAMFAOutcome(t *testing.T, repo *Repository, db *gorm.DB, amfaID, suffix, typ, country string) string {
	t.Helper()
	id := testEventID(t, suffix)
	cleanupEventID(t, db, id)
	if err := repo.Save(context.Background(), &domain.Event{
		TenantID: fixtureTenant, EventID: id, Type: typ, Category: "authentication",
		Severity: "info", Description: "AMFA " + typ, Source: "amfa:test-realm",
		SourceSystem: "amfa", Status: "processed", Timestamp: time.Now().UTC(),
		AMFAEventID: &amfaID, FinalStatus: strPtr(typ), Country: strPtr(country),
	}); err != nil {
		t.Fatalf("save amfa %s: %v", typ, err)
	}
	return id
}

// outcome reads the AMFA data a Keycloak row carries.
func outcome(t *testing.T, db *gorm.DB, eventID string) (country, finalStatus string) {
	t.Helper()
	var row struct {
		Country     *string
		FinalStatus *string
	}
	if err := db.Raw(`SELECT country, final_status FROM events WHERE event_id = ? AND deleted_at IS NULL`, eventID).
		Scan(&row).Error; err != nil {
		t.Fatalf("read %s: %v", eventID, err)
	}
	if row.Country != nil {
		country = *row.Country
	}
	if row.FinalStatus != nil {
		finalStatus = *row.FinalStatus
	}
	return country, finalStatus
}

func TestReconcileAMFAMerges_ARefusedLoginJoinsItsKeycloakEvent(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	decision := testEventID(t, "decision")
	refused := saveKC(t, repo, db, decision, "kc-login-error", "LOGIN_ERROR")
	amfa := saveAMFAOutcome(t, repo, db, refused, "amfa-login-error", "LOGIN_ERROR", "DE")

	reconcile(t, repo)

	if country, status := outcome(t, db, refused); country != "DE" || status != "LOGIN_ERROR" {
		t.Errorf("the refused LOGIN_ERROR got country %q, final_status %q; want DE, LOGIN_ERROR", country, status)
	}
	if rowExists(t, db, amfa) {
		t.Errorf("the AMFA row of the refusal is still there next to its Keycloak event")
	}
}

func TestReconcileAMFAMerges_ARefusedSsoLoginJoinsItsPostBrokerError(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	decision := testEventID(t, "decision")
	postBroker := saveKC(t, repo, db, decision, "kc-post-broker", "IDENTITY_PROVIDER_POST_LOGIN")
	refused := saveKC(t, repo, db, decision, "kc-post-broker-error", "IDENTITY_PROVIDER_POST_LOGIN_ERROR")
	// AMFA records the refusal as a LOGIN_ERROR, under the id of Keycloak's
	// IDENTITY_PROVIDER_POST_LOGIN_ERROR.
	amfa := saveAMFAOutcome(t, repo, db, refused, "amfa-login-error", "LOGIN_ERROR", "DE")

	reconcile(t, repo)

	if country, status := outcome(t, db, refused); country != "DE" || status != "LOGIN_ERROR" {
		t.Errorf("the refused SSO login got country %q, final_status %q; want DE, LOGIN_ERROR", country, status)
	}
	if country, _ := outcome(t, db, postBroker); country != "" {
		t.Errorf("the post broker row got the refusal's AMFA data")
	}
	if rowExists(t, db, amfa) {
		t.Errorf("the AMFA row of the refusal is still there next to its Keycloak event")
	}
}

func TestReconcileAMFAMerges_AWrongCodeAndTheRightOneEachKeepTheirOwnData(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	decision := testEventID(t, "decision")
	wrongCode := saveKC(t, repo, db, decision, "kc-login-error", "LOGIN_ERROR")
	login := saveKC(t, repo, db, decision, "kc-login", "LOGIN")
	amfaWrong := saveAMFAOutcome(t, repo, db, wrongCode, "amfa-login-error", "LOGIN_ERROR", "FR")
	amfaLogin := saveAMFAOutcome(t, repo, db, decision, "amfa-login", "LOGIN", "DE")

	reconcile(t, repo)

	if country, status := outcome(t, db, wrongCode); country != "FR" || status != "LOGIN_ERROR" {
		t.Errorf("the wrong code's LOGIN_ERROR got country %q, final_status %q; want FR, LOGIN_ERROR", country, status)
	}
	if country, status := outcome(t, db, login); country != "DE" || status != "LOGIN" {
		t.Errorf("the LOGIN got country %q, final_status %q; want DE, LOGIN", country, status)
	}
	if rowExists(t, db, amfaWrong) || rowExists(t, db, amfaLogin) {
		t.Errorf("an absorbed AMFA row is still there (wrong code %v, login %v)",
			rowExists(t, db, amfaWrong), rowExists(t, db, amfaLogin))
	}
}

func TestReconcileAMFAMerges_TheFallbackLeavesAWrongCodesOwnDataAlone(t *testing.T) {
	repo, db := newReconcileTestRepo(t)
	decision := testEventID(t, "decision")
	wrongCode := saveKC(t, repo, db, decision, "kc-login-error", "LOGIN_ERROR")
	saveAMFAOutcome(t, repo, db, wrongCode, "amfa-login-error", "LOGIN_ERROR", "FR")
	// The LOGIN's AMFA row has waited past the fallback and no Keycloak LOGIN
	// carrying the decision id arrived (a realm whose LOGIN lacks it).
	amfaLogin := saveAMFA(t, repo, db, decision, "LOGIN", amfaMergeFallbackAfter+time.Minute)

	reconcile(t, repo)

	if country, status := outcome(t, db, wrongCode); country != "FR" || status != "LOGIN_ERROR" {
		t.Errorf("the wrong code's LOGIN_ERROR got country %q, final_status %q; want its own FR, LOGIN_ERROR", country, status)
	}
	if !rowExists(t, db, amfaLogin) {
		t.Errorf("the LOGIN's AMFA row was absorbed onto a row that already has its own outcome")
	}
}
