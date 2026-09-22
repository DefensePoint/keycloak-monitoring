// Coverage for the query extensions on the events repository: the
// Type/Severity/MinRisk filters, keyset pagination (ListKeyset), the
// tenant-scoped single lookup (GetByTenantAndEventID), and the Stats
// aggregates. Shares the KMT_TEST_DATABASE_DSN-gated helper in
// rawdata_integration_test.go; skipped when that variable is unset.
package postgres

import (
	"context"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
)

func saveAll(t *testing.T, repo *Repository, evs []*domain.Event) {
	t.Helper()
	for _, ev := range evs {
		if err := repo.Save(context.Background(), ev); err != nil {
			t.Fatalf("save %s: %v", ev.EventID, err)
		}
	}
}

func listedEventIDs(rows []*domain.Event) map[string]bool {
	ids := make(map[string]bool, len(rows))
	for _, r := range rows {
		ids[r.EventID] = true
	}
	return ids
}

func TestList_FiltersByTypeSeverityAndMinRisk(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	tenant := fmt.Sprintf("flt-tenant-%d", stamp)
	t.Cleanup(func() { db.Exec(`DELETE FROM events WHERE tenant_id = ?`, tenant) })

	now := time.Now().UTC().Truncate(time.Second)
	source := events.SourceForKeycloakRealm("master")
	loginLow := fmt.Sprintf("flt-login-low-%d", stamp)
	loginHigh := fmt.Sprintf("flt-login-high-%d", stamp)
	logoutNoRisk := fmt.Sprintf("flt-logout-norisk-%d", stamp)

	saveAll(t, repo, []*domain.Event{
		{TenantID: tenant, EventID: loginLow, Type: "LOGIN", Severity: "info",
			RiskLevel: intPtr(1), Source: source, SourceSystem: "keycloak",
			Status: "processed", Timestamp: now},
		{TenantID: tenant, EventID: loginHigh, Type: "LOGIN", Severity: "warning",
			RiskLevel: intPtr(3), Source: source, SourceSystem: "keycloak",
			Status: "processed", Timestamp: now},
		{TenantID: tenant, EventID: logoutNoRisk, Type: "LOGOUT", Severity: "info",
			Source: source, SourceSystem: "keycloak",
			Status: "processed", Timestamp: now},
	})

	cases := []struct {
		name    string
		mutate  func(*events.ListOptions)
		wantIDs []string
	}{
		{"type", func(o *events.ListOptions) { o.Type = "LOGOUT" },
			[]string{logoutNoRisk}},
		{"severity", func(o *events.ListOptions) { o.Severity = "warning" },
			[]string{loginHigh}},
		// A NULL risk_level (no AMFA counterpart) must never satisfy MinRisk,
		// so logoutNoRisk stays out even for MinRisk 0.
		{"min risk excludes null", func(o *events.ListOptions) { o.MinRisk = intPtr(0) },
			[]string{loginLow, loginHigh}},
		{"min risk threshold", func(o *events.ListOptions) { o.MinRisk = intPtr(2) },
			[]string{loginHigh}},
		{"combined", func(o *events.ListOptions) { o.Type = "LOGIN"; o.MinRisk = intPtr(1) },
			[]string{loginLow, loginHigh}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := &events.ListOptions{
				Limit: 500, TenantID: tenant, Sources: events.SourcesForRealm("master"),
			}
			tc.mutate(opts)

			rows, err := repo.List(ctx, opts)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			got := listedEventIDs(rows)
			if len(rows) != len(tc.wantIDs) {
				t.Errorf("List returned %d events, want %d (%v)", len(rows), len(tc.wantIDs), got)
			}
			for _, id := range tc.wantIDs {
				if !got[id] {
					t.Errorf("List is missing %s", id)
				}
			}

			count, err := repo.CountWithFilter(ctx, opts)
			if err != nil {
				t.Fatalf("CountWithFilter: %v", err)
			}
			if count != int64(len(tc.wantIDs)) {
				t.Errorf("CountWithFilter = %d, want %d; the count must mirror List's filters",
					count, len(tc.wantIDs))
			}
		})
	}
}

func TestListKeyset_OrdersByTimestampThenIDAndContinues(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	tenant := fmt.Sprintf("ks-tenant-%d", stamp)
	t.Cleanup(func() { db.Exec(`DELETE FROM events WHERE tenant_id = ?`, tenant) })

	base := time.Now().UTC().Truncate(time.Second)
	source := events.SourceForKeycloakRealm("master")
	id := func(n int) string { return fmt.Sprintf("ks-%d-%d", n, stamp) }

	// Saved oldest-first so primary keys ascend; e3 and e4 share a timestamp,
	// which is exactly the case offset pagination cannot order stably and the
	// id DESC tiebreaker exists for.
	seed := []*domain.Event{
		{TenantID: tenant, EventID: id(1), Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Status: "processed", Timestamp: base},
		{TenantID: tenant, EventID: id(2), Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Status: "processed", Timestamp: base.Add(1 * time.Second)},
		{TenantID: tenant, EventID: id(3), Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Status: "processed", Timestamp: base.Add(2 * time.Second)},
		{TenantID: tenant, EventID: id(4), Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Status: "processed", Timestamp: base.Add(2 * time.Second)},
		{TenantID: tenant, EventID: id(5), Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Status: "processed", Timestamp: base.Add(3 * time.Second)},
	}
	saveAll(t, repo, seed)

	opts := &events.ListOptions{
		Limit: 2, TenantID: tenant, Sources: events.SourcesForRealm("master"),
	}

	assertPage := func(page *events.KeysetPage, wantIDs ...string) {
		t.Helper()
		if len(page.Events) != len(wantIDs) {
			t.Fatalf("page has %d events, want %d (%v)", len(page.Events), len(wantIDs), listedEventIDs(page.Events))
		}
		for i, want := range wantIDs {
			if page.Events[i].EventID != want {
				t.Errorf("page[%d] = %s, want %s", i, page.Events[i].EventID, want)
			}
		}
	}

	page1, err := repo.ListKeyset(ctx, opts, nil)
	if err != nil {
		t.Fatalf("ListKeyset page 1: %v", err)
	}
	assertPage(page1, id(5), id(4))
	if page1.Last == nil {
		t.Fatal("page 1 returned no Last position")
	}
	if page1.Last.ID != seed[3].ID {
		t.Errorf("page 1 Last.ID = %d, want %d (the id of %s)", page1.Last.ID, seed[3].ID, id(4))
	}
	if !page1.Last.Timestamp.UTC().Equal(base.Add(2 * time.Second)) {
		t.Errorf("page 1 Last.Timestamp = %s, want %s",
			page1.Last.Timestamp.UTC(), base.Add(2*time.Second))
	}

	// Page 2 must resume inside the equal-timestamp pair: same timestamp as
	// id(4), lower primary key.
	page2, err := repo.ListKeyset(ctx, opts, page1.Last)
	if err != nil {
		t.Fatalf("ListKeyset page 2: %v", err)
	}
	assertPage(page2, id(3), id(2))

	page3, err := repo.ListKeyset(ctx, opts, page2.Last)
	if err != nil {
		t.Fatalf("ListKeyset page 3: %v", err)
	}
	assertPage(page3, id(1))

	page4, err := repo.ListKeyset(ctx, opts, page3.Last)
	if err != nil {
		t.Fatalf("ListKeyset page 4: %v", err)
	}
	if len(page4.Events) != 0 || page4.Last != nil {
		t.Errorf("expected an empty final page with nil Last, got %d events (Last %+v)",
			len(page4.Events), page4.Last)
	}
}

// A cursor timestamp can carry nanoseconds (RFC3339 input parses to nanosecond
// precision) while Postgres stores microseconds. The position must still line
// up with stored rows: the previous page's last row must not repeat and its
// equal-timestamp sibling must not be skipped.
func TestListKeyset_NanosecondCursorMatchesStoredMicroseconds(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	tenant := fmt.Sprintf("ksn-tenant-%d", stamp)
	t.Cleanup(func() { db.Exec(`DELETE FROM events WHERE tenant_id = ?`, tenant) })

	shared := time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.UTC)
	source := events.SourceForKeycloakRealm("master")
	older := fmt.Sprintf("ksn-older-%d", stamp)
	twinLow := fmt.Sprintf("ksn-twin-low-%d", stamp)
	twinHigh := fmt.Sprintf("ksn-twin-high-%d", stamp)

	seed := []*domain.Event{
		{TenantID: tenant, EventID: older, Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Status: "processed", Timestamp: shared.Add(-1 * time.Second)},
		{TenantID: tenant, EventID: twinLow, Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Status: "processed", Timestamp: shared},
		{TenantID: tenant, EventID: twinHigh, Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Status: "processed", Timestamp: shared},
	}
	saveAll(t, repo, seed)

	after := &events.KeysetPosition{
		Timestamp: shared.Add(400 * time.Nanosecond),
		ID:        seed[2].ID,
	}
	page, err := repo.ListKeyset(ctx, &events.ListOptions{
		Limit: 10, TenantID: tenant, Sources: events.SourcesForRealm("master"),
	}, after)
	if err != nil {
		t.Fatalf("ListKeyset: %v", err)
	}

	got := listedEventIDs(page.Events)
	if got[twinHigh] {
		t.Error("the cursor's own row came back: the nanosecond timestamp fell into the " +
			"strict < arm instead of the equality arm")
	}
	if !got[twinLow] {
		t.Error("the equal-timestamp sibling was skipped: the nanosecond cursor never " +
			"satisfied the equality arm")
	}
	if !got[older] {
		t.Error("the older row is missing")
	}
	if len(page.Events) != 2 {
		t.Errorf("page has %d events, want 2 (%v)", len(page.Events), got)
	}
}

func TestGetByTenantAndEventID_ScopesToTenant(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	shared := fmt.Sprintf("scoped-%d", stamp)
	tenantA := fmt.Sprintf("scoped-tenant-a-%d", stamp)
	tenantB := fmt.Sprintf("scoped-tenant-b-%d", stamp)
	t.Cleanup(func() { db.Exec(`DELETE FROM events WHERE event_id = ?`, shared) })

	now := time.Now().UTC().Truncate(time.Second)
	source := events.SourceForKeycloakRealm("master")

	saveAll(t, repo, []*domain.Event{
		{TenantID: tenantA, EventID: shared, Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Username: "alice", Status: "processed", Timestamp: now},
	})

	got, err := repo.GetByTenantAndEventID(ctx, tenantA, shared)
	if err != nil {
		t.Fatalf("GetByTenantAndEventID for tenant A: %v", err)
	}
	if got.TenantID != tenantA || got.Username != "alice" {
		t.Errorf("tenant A got tenant %s / user %s, want its own row", got.TenantID, got.Username)
	}

	if _, err := repo.GetByTenantAndEventID(ctx, tenantB, shared); err == nil {
		t.Error("LEAK: tenant B resolved an event id that only tenant A owns")
	}

	// Once tenant B has its own row under the same event id, each tenant must
	// still resolve only its own.
	saveAll(t, repo, []*domain.Event{
		{TenantID: tenantB, EventID: shared, Type: "LOGIN", Source: source,
			SourceSystem: "keycloak", Username: "bob", Status: "processed", Timestamp: now},
	})
	gotB, err := repo.GetByTenantAndEventID(ctx, tenantB, shared)
	if err != nil {
		t.Fatalf("GetByTenantAndEventID for tenant B: %v", err)
	}
	if gotB.TenantID != tenantB || gotB.Username != "bob" {
		t.Errorf("tenant B got tenant %s / user %s, want its own row", gotB.TenantID, gotB.Username)
	}
}

func TestStats_GroupsByTypeSeverityAndSource(t *testing.T) {
	repo, db := newRawDataTestRepo(t)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	tenant := fmt.Sprintf("st-tenant-%d", stamp)
	other := fmt.Sprintf("st-other-%d", stamp)
	t.Cleanup(func() { db.Exec(`DELETE FROM events WHERE tenant_id IN (?, ?)`, tenant, other) })

	now := time.Now().UTC().Truncate(time.Second)
	kcSource := events.SourceForKeycloakRealm("master")
	amfaSource := events.SourceForAmfaRealm("master")
	id := func(n int) string { return fmt.Sprintf("st-%d-%d", n, stamp) }

	saveAll(t, repo, []*domain.Event{
		{TenantID: tenant, EventID: id(1), Type: "LOGIN", Severity: "info",
			Source: kcSource, SourceSystem: "keycloak", Status: "processed", Timestamp: now},
		{TenantID: tenant, EventID: id(2), Type: "LOGIN", Severity: "info",
			Source: kcSource, SourceSystem: "keycloak", Status: "processed", Timestamp: now},
		{TenantID: tenant, EventID: id(3), Type: "LOGIN", Severity: "warning",
			Source: amfaSource, SourceSystem: "amfa", Status: "processed", Timestamp: now},
		{TenantID: tenant, EventID: id(4), Type: "LOGOUT", Severity: "info",
			Source: kcSource, SourceSystem: "keycloak", Status: "processed", Timestamp: now},
		// Same tenant but outside the queried window: must not count.
		{TenantID: tenant, EventID: id(5), Type: "LOGIN", Severity: "info",
			Source: kcSource, SourceSystem: "keycloak", Status: "processed",
			Timestamp: now.Add(-48 * time.Hour)},
		// Another tenant with the same realm name: must not count.
		{TenantID: other, EventID: id(6), Type: "LOGIN", Severity: "info",
			Source: kcSource, SourceSystem: "keycloak", Status: "processed", Timestamp: now},
	})

	start := now.Add(-1 * time.Hour).Format(time.RFC3339)
	stats, err := repo.Stats(ctx, &events.ListOptions{
		TenantID:  tenant,
		Sources:   events.SourcesForRealm("master"),
		StartTime: &start,
	})
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if want := map[string]int64{"LOGIN": 3, "LOGOUT": 1}; !maps.Equal(stats.ByType, want) {
		t.Errorf("ByType = %v, want %v", stats.ByType, want)
	}
	if want := map[string]int64{"info": 3, "warning": 1}; !maps.Equal(stats.BySeverity, want) {
		t.Errorf("BySeverity = %v, want %v", stats.BySeverity, want)
	}
	if want := map[string]int64{kcSource: 3, amfaSource: 1}; !maps.Equal(stats.BySource, want) {
		t.Errorf("BySource = %v, want %v", stats.BySource, want)
	}
}
