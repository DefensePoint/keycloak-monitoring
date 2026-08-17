//go:build integration
// +build integration

package amfacheck_test

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	amfapg "repos.defensepoint.com/iam/oss-pmt/kmt/amfa/postgres"
	"repos.defensepoint.com/iam/oss-pmt/kmt/amfacheck"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
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
