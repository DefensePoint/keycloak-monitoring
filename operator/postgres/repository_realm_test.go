package postgres

import (
	"context"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

var (
	realmScopeStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	realmScopeEnd   = realmScopeStart.Add(24 * time.Hour)
	realmScope      = []string{"realm-a", "realm-b"}
	wantRealmArgs   = []any{"realm-a", "realm-b"}
)

type recordedDB struct{ db *gorm.DB }

func (r recordedDB) DB() *gorm.DB { return r.db }

func TestGetMetricsSummaryScopesEverySubQueryToRealms(t *testing.T) {
	db, recorder := testdb.NewRecorder(t)
	repo := NewRepository(recordedDB{db: db})

	if _, err := repo.GetMetricsSummary(context.Background(), "tenant-a", "operator@example.com", realmScopeStart, realmScopeEnd, realmScope); err != nil {
		t.Fatalf("GetMetricsSummary: %v", err)
	}

	assertRealmScoped(t, recorder.Against("operator_actions"), 7)
}

func TestGetAllOperatorsSummaryScopesToRealms(t *testing.T) {
	db, recorder := testdb.NewRecorder(t)
	repo := NewRepository(recordedDB{db: db})

	if _, err := repo.GetAllOperatorsSummary(context.Background(), "tenant-a", realmScopeStart, realmScopeEnd, realmScope); err != nil {
		t.Fatalf("GetAllOperatorsSummary: %v", err)
	}

	assertRealmScoped(t, recorder.Against("operator_actions"), 1)
}

func TestGetActionsScopesToRealms(t *testing.T) {
	db, recorder := testdb.NewRecorder(t)
	repo := NewRepository(recordedDB{db: db})

	if _, err := repo.GetActions(context.Background(), "tenant-a", "operator@example.com", realmScopeStart, realmScopeEnd, 25, realmScope); err != nil {
		t.Fatalf("GetActions: %v", err)
	}

	assertRealmScoped(t, recorder.Against("operator_actions"), 1)
}

func assertRealmScoped(t *testing.T, statements []testdb.Statement, want int) {
	t.Helper()

	if len(statements) != want {
		t.Fatalf("got %d statements against operator_actions, want %d; a new sub-query needs the realm filter too", len(statements), want)
	}

	for i, statement := range statements {
		bound := statement.ArgsAfter("realm_name IN ")
		if !reflect.DeepEqual(bound, wantRealmArgs) {
			t.Errorf("sub-query %d binds %v to realm_name, want %v\nSQL: %s", i, bound, wantRealmArgs, statement.SQL)
		}
	}
}
