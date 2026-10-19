// The mark a realm the monitoring client cannot read gets, so the Realms
// page can say its stored data is not current.
//
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
	db := newRealmFlagsDB(t)
	repo := NewRealmRepository(db)
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
		if err := db.Unscoped().Where("tenant_id = ? AND realm_name = ?", tenantB, "beta").First(&row).Error; err != nil {
			t.Fatalf("read retired row: %v", err)
		}
		if row.UnreadableSince != nil {
			t.Errorf("retired row unreadable_since = %v, want nil", row.UnreadableSince)
		}
	})
}
