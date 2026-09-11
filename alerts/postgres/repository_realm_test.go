package postgres

import (
	"context"
	"reflect"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

var (
	realmScope    = []string{"realm-a", "realm-b"}
	wantRealmArgs = []any{"realm-a", "realm-b"}
)

func TestListScopesToRealmNames(t *testing.T) {
	db, recorder := testdb.NewRecorder(t)
	repo := NewRepository(db)

	opts := &alerts.ListOptions{Limit: 10, RealmNames: realmScope}
	if _, err := repo.List(context.Background(), "tenant-a", opts); err != nil {
		t.Fatalf("List: %v", err)
	}

	assertRealmScoped(t, recorder.Against("configuration_alerts"), 1)
}

func TestCountScopesToRealmNames(t *testing.T) {
	db, recorder := testdb.NewRecorder(t)
	repo := NewRepository(db)

	opts := &alerts.ListOptions{RealmNames: realmScope}
	if _, err := repo.Count(context.Background(), "tenant-a", opts); err != nil {
		t.Fatalf("Count: %v", err)
	}

	assertRealmScoped(t, recorder.Against("configuration_alerts"), 1)
}

func TestGetStatisticsScopesEverySubQueryToRealms(t *testing.T) {
	db, recorder := testdb.NewRecorder(t)
	repo := NewRepository(db)

	if _, err := repo.GetStatistics(context.Background(), "tenant-a", realmScope...); err != nil {
		t.Fatalf("GetStatistics: %v", err)
	}

	assertRealmScoped(t, recorder.Against("configuration_alerts"), 3)
}

func assertRealmScoped(t *testing.T, statements []testdb.Statement, want int) {
	t.Helper()

	if len(statements) != want {
		t.Fatalf("got %d statements against configuration_alerts, want %d; a new sub-query needs the realm filter too", len(statements), want)
	}

	for i, statement := range statements {
		bound := statement.ArgsAfter("realm_name IN ")
		if !reflect.DeepEqual(bound, wantRealmArgs) {
			t.Errorf("sub-query %d binds %v to realm_name, want %v\nSQL: %s", i, bound, wantRealmArgs, statement.SQL)
		}
	}
}
