// Coverage for tenant_id as the isolation boundary on the events table.
// Shares the KMT_TEST_DATABASE_DSN-gated helper in
// rawdata_integration_test.go; skipped when that variable is unset.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/events"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// The case realm-name scoping could never handle: two tenants each monitoring
// a realm called "master", which every Keycloak has. Before tenant_id, both
// wrote source "keycloak:master" and no query could tell them apart.
func TestList_IsolatesTenantsSharingARealmName(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	idA := fmt.Sprintf("tid-a-%d", stamp)
	idB := fmt.Sprintf("tid-b-%d", stamp)
	tenantA := fmt.Sprintf("tenant-a-%d", stamp)
	tenantB := fmt.Sprintf("tenant-b-%d", stamp)
	t.Cleanup(func() { db.Exec(`DELETE FROM events WHERE event_id IN (?, ?)`, idA, idB) })

	now := time.Now().UTC().Truncate(time.Second)
	source := events.SourceForKeycloakRealm("master")

	for _, ev := range []*domain.Event{
		{TenantID: tenantA, EventID: idA, Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Username: "alice", Status: "processed", Timestamp: now},
		{TenantID: tenantB, EventID: idB, Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Username: "bob", Status: "processed", Timestamp: now},
	} {
		if err := repo.Save(ctx, ev); err != nil {
			t.Fatalf("save %s: %v", ev.EventID, err)
		}
	}

	rows, err := repo.List(ctx, &events.ListOptions{
		Limit: 500, TenantID: tenantA, Sources: events.SourcesForRealm("master"),
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var sawA, sawB bool
	for _, r := range rows {
		switch r.EventID {
		case idA:
			sawA = true
		case idB:
			sawB = true
		}
	}
	if !sawA {
		t.Error("tenant A did not see its own event")
	}
	if sawB {
		t.Error("LEAK: tenant A's scoped query returned tenant B's event")
	}
}

// Two tenants monitoring the same Keycloak see identical event ids. While
// event_id was globally unique they overwrote each other; both rows must now
// coexist.
func TestSave_SameEventIDAcrossTenantsCoexist(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	shared := fmt.Sprintf("tid-shared-%d", stamp)
	tenantA := fmt.Sprintf("tenant-a-%d", stamp)
	tenantB := fmt.Sprintf("tenant-b-%d", stamp)
	t.Cleanup(func() { db.Exec(`DELETE FROM events WHERE event_id = ?`, shared) })

	now := time.Now().UTC().Truncate(time.Second)
	for _, tenant := range []string{tenantA, tenantB} {
		ev := &domain.Event{
			TenantID: tenant, EventID: shared, Type: "LOGIN",
			Source: events.SourceForKeycloakRealm("master"), SourceSystem: "keycloak",
			Username: tenant, Status: "processed", Timestamp: now,
		}
		if err := repo.Save(ctx, ev); err != nil {
			t.Fatalf("save for %s: %v", tenant, err)
		}
	}

	var count int64
	if err := db.Raw(`SELECT count(*) FROM events WHERE event_id = ?`, shared).
		Scan(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected both tenants' rows to coexist, found %d; a global unique index "+
			"on event_id would have let one overwrite the other", count)
	}
}

// Rows predating tenant_id carry an empty tenant and must be invisible to every
// tenant-scoped query rather than leaking into an arbitrary tenant's view.
func TestList_UnattributedRowsAreHiddenFromTenants(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	legacyID := fmt.Sprintf("tid-legacy-%d", stamp)
	tenant := fmt.Sprintf("tenant-%d", stamp)
	t.Cleanup(func() { db.Exec(`DELETE FROM events WHERE event_id = ?`, legacyID) })

	// Written with no tenant, as the pre-migration writers did.
	if err := repo.Save(ctx, &domain.Event{
		EventID: legacyID, Type: "LOGIN", Source: events.SourceForKeycloakRealm("master"),
		SourceSystem: "keycloak", Status: "processed",
		Timestamp: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("save legacy row: %v", err)
	}

	rows, err := repo.List(ctx, &events.ListOptions{
		Limit: 500, TenantID: tenant, Sources: events.SourcesForRealm("master"),
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, r := range rows {
		if r.EventID == legacyID {
			t.Error("an unattributed row surfaced in a tenant-scoped query")
		}
	}
}
