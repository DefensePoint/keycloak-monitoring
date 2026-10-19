package fx

import (
	"context"
	"fmt"
	"testing"
	"time"

	alertspg "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// Keycloak returns events newest first, and the poller converts the whole
// window again on every poll. The alert must still show the newest occurrence
// as last seen.
func TestConvertEvent_LastSeenIsTheNewestEventOfThePoll(t *testing.T) {
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	db := client.DB()
	tenant := fmt.Sprintf("last-seen-%d", time.Now().UnixNano())
	t.Cleanup(func() { db.Exec(`DELETE FROM configuration_alerts WHERE tenant_id = ?`, tenant) })

	repo := alertspg.NewRepository(db)
	c := &eventAlertConverterImpl{alertRepo: repo, log: logger.NewNoop()}
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	newest := base
	event := func(id string, at time.Time) *domain.KeycloakEvent {
		return &domain.KeycloakEvent{TenantID: tenant, EventID: id, EventType: "LOGIN_ERROR", RealmName: "master",
			ClientID: "admin-cli", Username: "intruder", Time: at}
	}
	// Two polls over the same window, each newest first.
	for poll := 0; poll < 2; poll++ {
		for i, at := range []time.Time{newest, base.Add(-time.Minute), base.Add(-2 * time.Minute)} {
			if err := c.ConvertEvent(ctx, event(fmt.Sprintf("ev-%d", i), at)); err != nil {
				t.Fatalf("poll %d, event %d: %v", poll, i, err)
			}
		}
	}

	alerts, err := repo.List(ctx, tenant, nil)
	if err != nil || len(alerts) != 1 {
		t.Fatalf("List: %d alerts, %v; want one", len(alerts), err)
	}
	if !alerts[0].LastSeen.Equal(newest) {
		t.Errorf("last_seen = %s, want the newest event %s", alerts[0].LastSeen, newest)
	}
}
