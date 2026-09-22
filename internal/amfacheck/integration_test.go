//go:build integration
// +build integration

package amfacheck_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	amfapg "github.com/DefensePoint/keycloak-monitoring/internal/amfa/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfacheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// memAlertStore is an in-memory amfacheck.AlertStore for the integration test.
type memAlertStore struct {
	saved map[string]*domain.Alert
}

func (m *memAlertStore) GetAlertByID(_ context.Context, _, alertID string) (*domain.Alert, error) {
	return m.saved[alertID], nil
}
func (m *memAlertStore) SaveAlert(_ context.Context, a *domain.Alert) error {
	cp := *a
	m.saved[a.AlertID] = &cp
	return nil
}

type nopNotifier struct{}

func (nopNotifier) NotifyAlert(_ context.Context, _ *domain.Alert) error { return nil }

type itLogger struct{}

func (itLogger) Info(string, ...any)                     {}
func (itLogger) Error(string, ...any)                    {}
func (itLogger) Warn(string, ...any)                     {}
func (itLogger) Debug(string, ...any)                    {}
func (l itLogger) WithComponent(string) amfacheck.Logger { return l }

func openAmfaDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AMFA_TEST_DSN")
	if dsn == "" {
		t.Skip("AMFA_TEST_DSN not set; skipping amfacheck integration test")
	}

	testdb.ShareDatabase(t, dsn)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect AMFA DB: %v", err)
	}
	return db
}

// TestAmfaCheck_OneTick_AgainstRealDB runs one tick of the risk-rejected check
// against a seeded AMFA DB and asserts the run completes and any produced
// alerts are well-formed. Seed data is expected to contain at least one
// pre_auth_risk_decision=4 event in realm "master" within the lookback window
// for a positive assertion; otherwise the test still validates the no-error
// path. Adjust the realm to match your seed.
func TestAmfaCheck_OneTick_AgainstRealDB(t *testing.T) {
	db := openAmfaDB(t)
	repo := amfapg.NewRepository(db)

	realmsFn := amfacheck.RealmsFunc(func(_ context.Context) ([]string, error) {
		return []string{"master"}, nil
	})
	check := amfacheck.NewRiskRejectedCheck("tenant-it", realmsFn, repo, time.Hour, itLogger{})

	store := &memAlertStore{saved: map[string]*domain.Alert{}}
	svc := amfacheck.NewService("tenant-it", []amfacheck.Check{check}, store, nopNotifier{}, itLogger{}, time.Hour)

	if err := svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}
	for id, a := range store.saved {
		if a.Severity != domain.AlertSeverityCritical {
			t.Errorf("alert %s severity = %q, want critical", id, a.Severity)
		}
		if a.Source != domain.AlertSourceEvent {
			t.Errorf("alert %s source = %q, want event", id, a.Source)
		}
	}
	t.Logf("integration tick produced %d alert(s)", len(store.saved))
}

// itSeedEvent inserts one auth_process + auth_event row directly (bypassing
// amfacheck entirely) so the new-rules test below seeds real rows a real
// tenant's AMFA instance would produce, not a stub. procID/eventID/userID
// must be valid UUID strings; ctxJSON must carry a realm_id equal to realm.
func itSeedEvent(t *testing.T, db *gorm.DB, procID, eventID, userID, eventType, ctxJSON string, risk int, deviceHash, netHash, configID string) {
	t.Helper()
	now := time.Now().UTC()
	ctxHash := eventID + "-noctx" // auth_context_hash is varchar(64); stay short and unique

	t.Cleanup(func() {
		db.Exec(`DELETE FROM auth_event WHERE id = ?`, eventID)
		db.Exec(`DELETE FROM auth_process WHERE id = ?`, procID)
	})

	if err := db.Exec(`
		INSERT INTO auth_process
		  (id, user_id, auth_context_hash, device_info_hash, network_location_hash,
		   auth_context_json, pre_auth_risk_decision, parameters_config_id,
		   final_status, started_at)
		VALUES (?, ?, ?, ?, ?, ?::json, ?, ?, ?, ?)`,
		procID, userID, ctxHash, deviceHash, netHash, ctxJSON, risk, configID, eventType, now,
	).Error; err != nil {
		t.Fatalf("seed auth_process %s: %v", procID, err)
	}
	if err := db.Exec(`
		INSERT INTO auth_event
		  (id, auth_process, user_id, event_type, event_time,
		   auth_context_hash, device_info_hash, network_location_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID, procID, userID, eventType, now, ctxHash, deviceHash, netHash,
	).Error; err != nil {
		t.Fatalf("seed auth_event %s: %v", eventID, err)
	}
}

// TestAmfaCheck_OneTick_NewRulesAgainstRealDB is the end-to-end proof for the
// three new rules (per-client login-error, realm reject-burst, per-account
// login-error): it seeds real rows in the live AMFA database matching each
// rule's trigger condition exactly, builds the real amfacheck.Service with
// the real amfa/postgres.Repository (the same wiring internal/fx uses in
// production), runs one tick, and asserts each rule produced exactly the
// alert it should - not against a stub, against the real SQL + real service
// + real dedup path together.
func TestAmfaCheck_OneTick_NewRulesAgainstRealDB(t *testing.T) {
	db := openAmfaDB(t)
	repo := amfapg.NewRepository(db)
	const realm = "amfacheck-it-newrules"

	var deviceHash, netHash, configID string
	row := db.Raw(`SELECT device_info_hash, network_location_hash, parameters_config_id::text
	               FROM auth_process LIMIT 1`).Row()
	if err := row.Scan(&deviceHash, &netHash, &configID); err != nil {
		t.Skipf("no existing auth_process to borrow FK values from: %v", err)
	}

	ctxJSON := `{"realm_id":"` + realm + `","client":"it-attacked-client"}`
	bareCtxJSON := `{"realm_id":"` + realm + `"}`

	// Rule A (per-client login-error): 5 LOGIN_ERROR events for one client,
	// each a different user, threshold is 5.
	for i := 0; i < 5; i++ {
		suffix := fmt.Sprintf("%012x", 0xa00+i)
		procID := fmt.Sprintf("9a9a9a01-0000-0000-0000-%s", suffix)
		eventID := fmt.Sprintf("9a9a9a02-0000-0000-0000-%s", suffix)
		userID := fmt.Sprintf("9a9a9a03-0000-0000-0000-%s", suffix)
		itSeedEvent(t, db, procID, eventID, userID, "LOGIN_ERROR", ctxJSON, 1, deviceHash, netHash, configID)
	}

	// Rule C (per-account login-error): 5 LOGIN_ERROR events for one user,
	// threshold is 5.
	bruteUserID := "9a9a9a04-0000-0000-0000-00000000000a"
	for i := 0; i < 5; i++ {
		suffix := fmt.Sprintf("%012x", 0xb00+i)
		procID := fmt.Sprintf("9a9a9a05-0000-0000-0000-%s", suffix)
		eventID := fmt.Sprintf("9a9a9a06-0000-0000-0000-%s", suffix)
		itSeedEvent(t, db, procID, eventID, bruteUserID, "LOGIN_ERROR", bareCtxJSON, 1, deviceHash, netHash, configID)
	}

	// Rule B (realm-wide reject-burst): 10 distinct users each rejected
	// (risk=4), threshold is 10 distinct users.
	for i := 0; i < 10; i++ {
		suffix := fmt.Sprintf("%012x", 0xc00+i)
		procID := fmt.Sprintf("9a9a9a07-0000-0000-0000-%s", suffix)
		eventID := fmt.Sprintf("9a9a9a08-0000-0000-0000-%s", suffix)
		userID := fmt.Sprintf("9a9a9a09-0000-0000-0000-%s", suffix)
		itSeedEvent(t, db, procID, eventID, userID, "LOGIN_ERROR", bareCtxJSON, 4, deviceHash, netHash, configID)
	}

	realmsFn := amfacheck.RealmsFunc(func(_ context.Context) ([]string, error) {
		return []string{realm}, nil
	})
	checks := []amfacheck.Check{
		amfacheck.NewLoginErrorByClientCheck("tenant-it", realmsFn, repo, 5, 5*time.Minute, itLogger{}),
		amfacheck.NewRealmRejectBurstCheck("tenant-it", realmsFn, repo, 10, 10*time.Minute, itLogger{}),
		amfacheck.NewLoginErrorByAccountCheck("tenant-it", realmsFn, repo, 5, 5*time.Minute, itLogger{}),
	}
	store := &memAlertStore{saved: map[string]*domain.Alert{}}
	svc := amfacheck.NewService("tenant-it", checks, store, nopNotifier{}, itLogger{}, time.Hour)

	if err := svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}

	var gotClient, gotBurst, gotAccount *domain.Alert
	for _, a := range store.saved {
		switch a.CheckType {
		case "amfa-login-error-by-client":
			gotClient = a
		case "amfa-realm-reject-burst":
			gotBurst = a
		case "amfa-login-error-by-account":
			gotAccount = a
		}
	}

	if gotClient == nil {
		t.Fatal("expected an amfa-login-error-by-client alert, got none")
	}
	if gotClient.Severity != domain.AlertSeverityWarning {
		t.Errorf("client alert severity = %q, want warning", gotClient.Severity)
	}
	if gotClient.ResourceType != "amfa_client" || gotClient.ResourceID != "it-attacked-client" {
		t.Errorf("client alert resource = %s/%s, want amfa_client/it-attacked-client", gotClient.ResourceType, gotClient.ResourceID)
	}

	if gotBurst == nil {
		t.Fatal("expected an amfa-realm-reject-burst alert, got none")
	}
	if gotBurst.Severity != domain.AlertSeverityCritical {
		t.Errorf("burst alert severity = %q, want critical", gotBurst.Severity)
	}
	if gotBurst.ResourceType != "realm" || gotBurst.ResourceID != realm {
		t.Errorf("burst alert resource = %s/%s, want realm/%s", gotBurst.ResourceType, gotBurst.ResourceID, realm)
	}

	if gotAccount == nil {
		t.Fatal("expected an amfa-login-error-by-account alert, got none")
	}
	if gotAccount.Severity != domain.AlertSeverityWarning {
		t.Errorf("account alert severity = %q, want warning", gotAccount.Severity)
	}
	if gotAccount.ResourceType != "amfa_user" || gotAccount.ResourceID != bruteUserID {
		t.Errorf("account alert resource = %s/%s, want amfa_user/%s", gotAccount.ResourceType, gotAccount.ResourceID, bruteUserID)
	}

	t.Logf("e2e tick produced %d alert(s) across 3 new rules", len(store.saved))
}
