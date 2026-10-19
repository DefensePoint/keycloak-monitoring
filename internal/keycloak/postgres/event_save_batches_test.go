package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

func keycloakEvents(tenantID, prefix string, n int, from time.Time) []*domain.KeycloakEvent {
	events := make([]*domain.KeycloakEvent, n)
	for i := range events {
		events[i] = &domain.KeycloakEvent{
			TenantID:  tenantID,
			EventID:   fmt.Sprintf("%s-%04d", prefix, i),
			Time:      from.Add(time.Duration(i) * time.Millisecond),
			RealmID:   "realm-1",
			RealmName: "realm-1",
			EventType: "LOGIN",
			Success:   true,
		}
	}
	return events
}

// The database client caches a prepared statement per SQL text, and an
// INSERT's text grows with its row count, so the row count per INSERT must be
// capped for the number of cached statements to stay bounded.
func TestSaveEventsCapsRowsPerInsert(t *testing.T) {
	db, recorder := testdb.NewRecorder(t)
	repo := NewEventRepository(db)

	if err := repo.SaveEvents(context.Background(), keycloakEvents("tenant-1", "evt", 250, time.Now())); err != nil {
		t.Fatalf("SaveEvents: %v", err)
	}

	inserts := recorder.Against("keycloak_events")
	if len(inserts) != 3 {
		t.Fatalf("got %d statements against keycloak_events, want 3 inserts of at most %d rows", len(inserts), saveEventsBatchSize)
	}
	seen := map[string]bool{}
	for _, insert := range inserts {
		rows := 0
		for _, arg := range insert.Args {
			if id, ok := arg.(string); ok && strings.HasPrefix(id, "evt-") {
				seen[id] = true
				rows++
			}
		}
		if rows > saveEventsBatchSize {
			t.Errorf("an insert carries %d rows, want at most %d", rows, saveEventsBatchSize)
		}
		if !strings.Contains(insert.SQL, "ON CONFLICT") {
			t.Errorf("an insert has no ON CONFLICT clause: %s", insert.SQL)
		}
	}
	if len(seen) != 250 {
		t.Errorf("inserts carry %d of the 250 events", len(seen))
	}
}
