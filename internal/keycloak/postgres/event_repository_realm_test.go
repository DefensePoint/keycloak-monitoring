package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

func TestGetEventsByTypeScopesToRealm(t *testing.T) {
	db, recorder := testdb.NewRecorder(t)
	repo := NewEventRepository(db)

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := repo.GetEventsByType(context.Background(), "tenant-a", "realm-a", "LOGIN_ERROR", from, from.Add(24*time.Hour), 50, 0); err != nil {
		t.Fatalf("GetEventsByType: %v", err)
	}

	statements := recorder.Against("keycloak_events")
	if len(statements) != 1 {
		t.Fatalf("got %d statements against keycloak_events, want 1", len(statements))
	}

	bound := statements[0].ArgsAfter("realm_name = ")
	if len(bound) != 1 || bound[0] != "realm-a" {
		t.Errorf("realm_name predicate binds %v, want [realm-a]\nSQL: %s", bound, statements[0].SQL)
	}
}
