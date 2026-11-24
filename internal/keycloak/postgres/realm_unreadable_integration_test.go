// The mark a realm the monitoring client cannot read gets, so the Realms
// page can say its stored data is not current.
//
// The database is opened through the startup migrations (newReconcileRepo):
// a realm's name-only row and a retired row of the same name can only both
// exist when the name index ignores retired rows, as it does in production.
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset.
package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// unreadableTenant returns a tenant id unique to this run and cleans it up.
func unreadableTenant(t *testing.T, db *gorm.DB, label string) string {
	t.Helper()
	tenantID := fmt.Sprintf("realm-unreadable-%s-%d", label, time.Now().UnixNano())
	t.Cleanup(func() {
		db.Unscoped().Where("tenant_id = ?", tenantID).Delete(&database.KeycloakRealmInfo{})
	})
	return tenantID
}

func readRealm(t *testing.T, repo *RealmRepository, tenantID, name string) *domain.KeycloakRealmInfo {
	t.Helper()
	realm, err := repo.GetRealmByName(context.Background(), tenantID, name)
	if err != nil {
		t.Fatalf("read %s/%s: %v", tenantID, name, err)
	}
	return realm
}

func TestMarkRealmsUnreadable(t *testing.T) {
	repo, db := newReconcileRepo(t)
	ctx := context.Background()
	tenantA := unreadableTenant(t, db, "a")
	tenantB := unreadableTenant(t, db, "b")
	lastRead := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)

	for _, tenantID := range []string{tenantA, tenantB} {
		for _, name := range []string{"alpha", "beta"} {
			if err := repo.SaveRealm(ctx, &domain.KeycloakRealmInfo{
				TenantID: tenantID, RealmID: tenantID + "-" + name, RealmName: name,
				Enabled: true, IsHealthy: true, LastChecked: lastRead,
			}); err != nil {
				t.Fatalf("save: %v", err)
			}
		}
	}

	first := time.Now().UTC().Truncate(time.Second)
	if err := repo.MarkRealmsUnreadable(ctx, tenantA, []string{"beta"}, first); err != nil {
		t.Fatalf("mark: %v", err)
	}

	t.Run("marks only that realm of that tenant", func(t *testing.T) {
		if got := readRealm(t, repo, tenantA, "beta").UnreadableSince; got == nil || !got.Equal(first) {
			t.Errorf("tenant A beta unreadable_since = %v, want %v", got, first)
		}
		if got := readRealm(t, repo, tenantA, "alpha").UnreadableSince; got != nil {
			t.Errorf("tenant A alpha unreadable_since = %v, want nil", got)
		}
		if got := readRealm(t, repo, tenantB, "beta").UnreadableSince; got != nil {
			t.Errorf("tenant B beta, same name, unreadable_since = %v, want nil", got)
		}
	})

	t.Run("keeps the stored settings and the last read time", func(t *testing.T) {
		beta := readRealm(t, repo, tenantA, "beta")
		if !beta.Enabled || !beta.IsHealthy || !beta.LastChecked.Equal(lastRead) {
			t.Errorf("beta enabled=%v healthy=%v last_checked=%v, want true true %v",
				beta.Enabled, beta.IsHealthy, beta.LastChecked, lastRead)
		}
	})

	t.Run("a later mark keeps when it was first found unreadable", func(t *testing.T) {
		if err := repo.MarkRealmsUnreadable(ctx, tenantA, []string{"beta"}, first.Add(time.Minute)); err != nil {
			t.Fatalf("mark again: %v", err)
		}
		if got := readRealm(t, repo, tenantA, "beta").UnreadableSince; got == nil || !got.Equal(first) {
			t.Errorf("unreadable_since = %v after a second mark, want the first %v", got, first)
		}
	})

	t.Run("a full read clears it", func(t *testing.T) {
		if err := repo.SaveRealm(ctx, &domain.KeycloakRealmInfo{
			TenantID: tenantA, RealmID: tenantA + "-beta", RealmName: "beta",
			Enabled: true, IsHealthy: true, LastChecked: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("save: %v", err)
		}
		if got := readRealm(t, repo, tenantA, "beta").UnreadableSince; got != nil {
			t.Errorf("unreadable_since = %v after the realm was read, want nil", got)
		}
	})

	t.Run("a retired realm is not marked", func(t *testing.T) {
		if _, err := repo.RetireRealmsNotIn(ctx, tenantB, []string{tenantB + "-alpha"}); err != nil {
			t.Fatalf("retire: %v", err)
		}
		if err := repo.MarkRealmsUnreadable(ctx, tenantB, []string{"beta"}, first); err != nil {
			t.Fatalf("mark: %v", err)
		}
		var row database.KeycloakRealmInfo
		if err := db.Unscoped().Where("tenant_id = ? AND realm_id = ?", tenantB, tenantB+"-beta").First(&row).Error; err != nil {
			t.Fatalf("read retired row: %v", err)
		}
		if row.UnreadableSince != nil {
			t.Errorf("retired row unreadable_since = %v, want nil", row.UnreadableSince)
		}
	})
}

// liveRealmsByName returns the tenant's live rows by name.
func liveRealmsByName(t *testing.T, repo *RealmRepository, tenantID string) map[string]*domain.KeycloakRealmInfo {
	t.Helper()
	rows, err := repo.GetRealms(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("list %s: %v", tenantID, err)
	}
	byName := map[string]*domain.KeycloakRealmInfo{}
	for _, row := range rows {
		if _, dup := byName[row.RealmName]; dup {
			t.Fatalf("two live rows named %s in %s", row.RealmName, tenantID)
		}
		byName[row.RealmName] = row
	}
	return byName
}

// A realm the client could never read had no row, so it was neither listed
// nor polled for events: a tenant whose client lacks view-realm showed no
// realms and no events at all. It now gets a row with its name only.
func TestMarkRealmsUnreadable_NeverReadRealm(t *testing.T) {
	repo, db := newReconcileRepo(t)
	ctx := context.Background()
	tenantA := unreadableTenant(t, db, "never-a")
	tenantB := unreadableTenant(t, db, "never-b")
	first := time.Now().UTC().Truncate(time.Second)

	if err := repo.MarkRealmsUnreadable(ctx, tenantA, []string{"gamma", "delta"}, first); err != nil {
		t.Fatalf("mark: %v", err)
	}

	t.Run("gets a live, enabled row marked unreadable and never read", func(t *testing.T) {
		rows := liveRealmsByName(t, repo, tenantA)
		if len(rows) != 2 {
			t.Fatalf("live rows = %d, want 2 (gamma and delta)", len(rows))
		}
		gamma := rows["gamma"]
		if gamma == nil {
			t.Fatal("gamma has no row")
		}
		if !gamma.Enabled {
			t.Error("gamma enabled = false, want true: the events poll skips a disabled realm")
		}
		if gamma.UnreadableSince == nil || !gamma.UnreadableSince.Equal(first) {
			t.Errorf("gamma unreadable_since = %v, want %v", gamma.UnreadableSince, first)
		}
		if !gamma.LastChecked.IsZero() {
			t.Errorf("gamma last_checked = %v, want zero: it was never read", gamma.LastChecked)
		}
		if gamma.IsHealthy {
			t.Error("gamma is_healthy = true, want false: nothing about it is known")
		}
	})

	t.Run("another tenant gets nothing", func(t *testing.T) {
		if rows := liveRealmsByName(t, repo, tenantB); len(rows) != 0 {
			t.Errorf("tenant B live rows = %d, want 0", len(rows))
		}
	})

	t.Run("a later mark keeps one row and the first time", func(t *testing.T) {
		if err := repo.MarkRealmsUnreadable(ctx, tenantA, []string{"gamma"}, first.Add(time.Minute)); err != nil {
			t.Fatalf("mark again: %v", err)
		}
		gamma := liveRealmsByName(t, repo, tenantA)["gamma"]
		if gamma == nil || gamma.UnreadableSince == nil || !gamma.UnreadableSince.Equal(first) {
			t.Errorf("gamma after a second mark = %+v, want unreadable since %v", gamma, first)
		}
	})

	t.Run("the first full read replaces it", func(t *testing.T) {
		readAt := first.Add(2 * time.Minute)
		if err := repo.SaveRealm(ctx, &domain.KeycloakRealmInfo{
			TenantID: tenantA, RealmID: "real-gamma", RealmName: "gamma",
			Enabled: true, IsHealthy: true, EventsEnabled: true, LastChecked: readAt,
		}); err != nil {
			t.Fatalf("save: %v", err)
		}
		gamma := liveRealmsByName(t, repo, tenantA)["gamma"]
		if gamma.RealmID != "real-gamma" || gamma.UnreadableSince != nil || !gamma.LastChecked.Equal(readAt) || !gamma.EventsEnabled {
			t.Errorf("gamma after a full read = id %q unreadable %v last_checked %v events %v, want real-gamma nil %v true",
				gamma.RealmID, gamma.UnreadableSince, gamma.LastChecked, gamma.EventsEnabled, readAt)
		}
	})

	t.Run("a stored row is kept, with no second one", func(t *testing.T) {
		if err := repo.MarkRealmsUnreadable(ctx, tenantA, []string{"gamma"}, first.Add(3*time.Minute)); err != nil {
			t.Fatalf("mark: %v", err)
		}
		gamma := liveRealmsByName(t, repo, tenantA)["gamma"]
		if gamma.RealmID != "real-gamma" || !gamma.EventsEnabled {
			t.Errorf("gamma = id %q events %v, want its stored row real-gamma with its settings", gamma.RealmID, gamma.EventsEnabled)
		}
	})

	t.Run("a realm deleted in Keycloak is retired", func(t *testing.T) {
		if _, err := repo.RetireRealmsNotIn(ctx, tenantA, []string{"real-gamma"}); err != nil {
			t.Fatalf("retire: %v", err)
		}
		rows := liveRealmsByName(t, repo, tenantA)
		if _, ok := rows["delta"]; ok || len(rows) != 1 {
			t.Errorf("live rows = %v, want gamma only: delta's name-only row is retired", rows)
		}
	})

	t.Run("a retired name-only row comes back when the realm is found again", func(t *testing.T) {
		again := first.Add(time.Hour)
		if err := repo.MarkRealmsUnreadable(ctx, tenantA, []string{"delta"}, again); err != nil {
			t.Fatalf("mark: %v", err)
		}
		delta := liveRealmsByName(t, repo, tenantA)["delta"]
		if delta == nil || delta.UnreadableSince == nil || !delta.UnreadableSince.Equal(again) {
			t.Errorf("delta = %+v, want a live row unreadable since %v", delta, again)
		}
	})
}
