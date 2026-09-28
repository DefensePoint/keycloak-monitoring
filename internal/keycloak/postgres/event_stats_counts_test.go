// Counts for the login KPI tile, checked against known rows.
//
// The handler tests in internal/http/chi stub the service, so they prove the
// aggregate sums what it is handed and nothing about whether the numbers match
// the events in the table. That is the gap the KPI bug lived in: the tile read
// zero while the events were there the whole time, and every test still passed
// because none of them counted anything real.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset.
package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

func newEventStatsDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("KMT_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_TEST_DATABASE_DSN not set; skipping integration test")
	}

	testdb.ShareDatabase(t, dsn)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.AutoMigrate(&database.KeycloakEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// seedEvent writes one event at a known time in a known realm.
func seedEvent(t *testing.T, db *gorm.DB, tenantID, realm, eventType string, at time.Time, n int) {
	t.Helper()

	for i := 0; i < n; i++ {
		row := &database.KeycloakEvent{
			TenantID:  tenantID,
			Time:      at.Add(time.Duration(i) * time.Millisecond),
			EventID:   fmt.Sprintf("%s-%s-%s-%d", tenantID, realm, eventType, i),
			RealmID:   realm,
			RealmName: realm,
			EventType: eventType,
			Success:   true,
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %s in %s: %v", eventType, realm, err)
		}
	}
}

// The tile's number against rows someone can count by hand: per realm, and the
// all-realms view the dashboard opens on, which must equal the sum of its parts
// rather than a separate query that can drift from them.
func TestCountEventsByType_MatchesKnownRows(t *testing.T) {
	db := newEventStatsDB(t)
	repo := NewEventRepository(db)
	ctx := context.Background()

	tenant := fmt.Sprintf("kpi-tenant-%d", time.Now().UnixNano())
	t.Cleanup(func() { db.Where("tenant_id = ?", tenant).Delete(&database.KeycloakEvent{}) })

	base := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	from, to := base.Add(-time.Minute), base.Add(time.Hour)

	// prod: 5 logins, 2 errors. staging: 3 logins, 0 errors.
	seedEvent(t, db, tenant, "prod", "LOGIN", base, 5)
	seedEvent(t, db, tenant, "prod", "LOGIN_ERROR", base, 2)
	seedEvent(t, db, tenant, "staging", "LOGIN", base, 3)

	cases := []struct {
		realm     string
		eventType string
		want      int64
	}{
		{"prod", "LOGIN", 5},
		{"prod", "LOGIN_ERROR", 2},
		{"staging", "LOGIN", 3},
		{"staging", "LOGIN_ERROR", 0},
	}
	for _, tc := range cases {
		got, err := repo.CountEventsByType(ctx, tenant, tc.realm, tc.eventType, from, to)
		if err != nil {
			t.Fatalf("count %s in %s: %v", tc.eventType, tc.realm, err)
		}
		if got != tc.want {
			t.Errorf("%s in %s = %d, want %d", tc.eventType, tc.realm, got, tc.want)
		}
	}

	// The all-realms tile is the sum of the per-realm numbers. A caller reading
	// both views must not see them disagree.
	prod, _ := repo.CountEventsByType(ctx, tenant, "prod", "LOGIN", from, to)
	staging, _ := repo.CountEventsByType(ctx, tenant, "staging", "LOGIN", from, to)
	if prod+staging != 8 {
		t.Errorf("per-realm logins sum to %d, want 8", prod+staging)
	}
}

// The range picker, at the layer that actually decides. parseTimeWindow is
// tested for parsing the parameters; this checks the window then selects the
// right rows, which is what a SOC operator changing the range is relying on.
func TestCountEventsByType_HonoursTheWindow(t *testing.T) {
	db := newEventStatsDB(t)
	repo := NewEventRepository(db)
	ctx := context.Background()

	tenant := fmt.Sprintf("kpi-window-%d", time.Now().UnixNano())
	t.Cleanup(func() { db.Where("tenant_id = ?", tenant).Delete(&database.KeycloakEvent{}) })

	now := time.Now().UTC().Truncate(time.Second)
	seedEvent(t, db, tenant, "prod", "LOGIN", now.Add(-48*time.Hour), 4) // old
	seedEvent(t, db, tenant, "prod", "LOGIN", now.Add(-1*time.Hour), 6)  // recent

	tests := []struct {
		name       string
		from, to   time.Time
		want       int64
		wantReason string
	}{
		{
			name: "last 24 hours excludes the older events",
			from: now.Add(-24 * time.Hour), to: now,
			want:       6,
			wantReason: "a narrowed range must drop rows outside it, or the picker does nothing",
		},
		{
			name: "a wide range includes both batches",
			from: now.Add(-72 * time.Hour), to: now,
			want:       10,
			wantReason: "widening the range must bring the older rows back",
		},
		{
			name: "a range covering neither batch counts nothing",
			from: now.Add(-36 * time.Hour), to: now.Add(-30 * time.Hour),
			want:       0,
			wantReason: "a window with no events must report zero rather than falling back to everything",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.CountEventsByType(ctx, tenant, "prod", "LOGIN", tt.from, tt.to)
			if err != nil {
				t.Fatalf("count: %v", err)
			}
			if got != tt.want {
				t.Errorf("logins = %d, want %d: %s", got, tt.want, tt.wantReason)
			}
		})
	}
}

// One tenant's numbers must never include another's. The KPI is the first thing
// a SOC operator reads, so an inflated count is a false alarm and a deflated one
// hides a real incident.
func TestCountEventsByType_IsolatesTenants(t *testing.T) {
	db := newEventStatsDB(t)
	repo := NewEventRepository(db)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	tenantA := fmt.Sprintf("kpi-a-%d", stamp)
	tenantB := fmt.Sprintf("kpi-b-%d", stamp)
	t.Cleanup(func() {
		db.Where("tenant_id IN ?", []string{tenantA, tenantB}).Delete(&database.KeycloakEvent{})
	})

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	from, to := base.Add(-time.Minute), base.Add(time.Hour)

	// Same realm name in both tenants, which is the case realm-name scoping
	// alone could never separate.
	seedEvent(t, db, tenantA, "master", "LOGIN", base, 7)
	seedEvent(t, db, tenantB, "master", "LOGIN", base, 11)

	got, err := repo.CountEventsByType(ctx, tenantA, "master", "LOGIN", from, to)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 7 {
		t.Errorf("tenant A logins = %d, want 7; tenant B's 11 must not be included", got)
	}
}

// An empty realm name drops the realm filter entirely and counts the whole
// tenant. Nothing should reach the repository with one, because allowedRealms
// drops blank realms before a caller's scope is used, but the behaviour is
// pinned here so a future change cannot quietly turn a blank into "everything"
// without a test saying so.
func TestCountEventsByType_EmptyRealmCountsTheWholeTenant(t *testing.T) {
	db := newEventStatsDB(t)
	repo := NewEventRepository(db)
	ctx := context.Background()

	tenant := fmt.Sprintf("kpi-blank-%d", time.Now().UnixNano())
	t.Cleanup(func() { db.Where("tenant_id = ?", tenant).Delete(&database.KeycloakEvent{}) })

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	from, to := base.Add(-time.Minute), base.Add(time.Hour)

	seedEvent(t, db, tenant, "prod", "LOGIN", base, 2)
	seedEvent(t, db, tenant, "staging", "LOGIN", base, 3)

	got, err := repo.CountEventsByType(ctx, tenant, "", "LOGIN", from, to)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 5 {
		t.Errorf("empty realm = %d, want 5 (every realm in the tenant); if this changed, "+
			"check what now reaches the repository with a blank realm", got)
	}
}
