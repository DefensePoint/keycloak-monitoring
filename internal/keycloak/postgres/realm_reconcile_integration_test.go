// Regression coverage for realms deleted, or deleted and recreated under the
// same name, in Keycloak.
//
// A recreated realm gets a new id. While the old row stayed live the
// (tenant_id, realm_name) index refused the new one, so every sync of that
// realm failed and KMT kept showing the old realm's state. A deleted realm
// stayed listed, healthy and polled forever.
//
// The repository is opened through database.NewClient so the startup
// migrations run: the name index must ignore retired rows for a recreated
// realm to be saved. Requires a real PostgreSQL instance via
// KMT_TEST_DATABASE_DSN; skipped when unset. Only rows under a realm-rec-
// tenant are touched.
package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

func newReconcileRepo(t *testing.T) (*RealmRepository, *gorm.DB) {
	t.Helper()

	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	// testdb.DatabaseConfig rather than a local reading: inferring sslmode from
	// the parsed TLS config asked for TLS whenever the DSN left sslmode out.
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	db := client.DB()
	t.Cleanup(func() {
		db.Unscoped().Where("tenant_id LIKE ?", "realm-rec-%").Delete(&database.KeycloakRealmInfo{})
	})
	return NewRealmRepository(db), db
}

func reconcileTenant(label string) string {
	return fmt.Sprintf("realm-rec-%s-%d", label, time.Now().UnixNano())
}

func reconcileRealm(tenantID, id, name string, enabled bool) *domain.KeycloakRealmInfo {
	return &domain.KeycloakRealmInfo{
		TenantID: tenantID, RealmID: id, RealmName: name,
		Enabled: enabled, LoginWithEmailAllowed: enabled, IsHealthy: enabled,
		LastChecked: time.Now().UTC(),
	}
}

// liveRealms returns realm_id -> enabled for the tenant's live rows.
func liveRealms(t *testing.T, repo *RealmRepository, tenantID string) map[string]bool {
	t.Helper()
	realms, err := repo.GetRealms(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("GetRealms: %v", err)
	}
	out := map[string]bool{}
	for _, r := range realms {
		out[r.RealmID] = r.Enabled
	}
	return out
}

func isRetired(t *testing.T, db *gorm.DB, tenantID, realmID string) bool {
	t.Helper()
	var row database.KeycloakRealmInfo
	if err := db.Unscoped().Where("tenant_id = ? AND realm_id = ?", tenantID, realmID).
		First(&row).Error; err != nil {
		t.Fatalf("read realm %s: %v", realmID, err)
	}
	return row.DeletedAt.Valid
}

func TestSaveRealm_ARecreatedRealmReplacesTheOldOne(t *testing.T) {
	repo, db := newReconcileRepo(t)
	ctx := context.Background()
	tenant := reconcileTenant("recreate")

	if err := repo.SaveRealm(ctx, reconcileRealm(tenant, "old-id", "shop", true)); err != nil {
		t.Fatalf("save original realm: %v", err)
	}
	// Deleted and recreated in Keycloak: same name, new id, and here disabled
	// so a stale row would show.
	if err := repo.SaveRealm(ctx, reconcileRealm(tenant, "new-id", "shop", false)); err != nil {
		t.Fatalf("save recreated realm: %v", err)
	}

	live := liveRealms(t, repo, tenant)
	if len(live) != 1 || live["new-id"] != false {
		t.Fatalf("live realms = %v, want only new-id, disabled", live)
	}
	if _, ok := live["new-id"]; !ok {
		t.Fatalf("recreated realm is not live: %v", live)
	}
	if !isRetired(t, db, tenant, "old-id") {
		t.Error("the old realm's row is still live")
	}
}

func TestRetireRealmsNotIn_RetiresOnlyTheRealmsKeycloakDropped(t *testing.T) {
	repo, db := newReconcileRepo(t)
	ctx := context.Background()
	tenant := reconcileTenant("retire")
	other := reconcileTenant("other")

	for _, r := range []*domain.KeycloakRealmInfo{
		reconcileRealm(tenant, "a", "alpha", true),
		reconcileRealm(tenant, "b", "beta", true),
		reconcileRealm(tenant, "c", "gamma", true),
		// Another tenant's realm with an id Keycloak did not list for this one.
		reconcileRealm(other, "b", "beta", true),
	} {
		if err := repo.SaveRealm(ctx, r); err != nil {
			t.Fatalf("seed %s/%s: %v", r.TenantID, r.RealmID, err)
		}
	}

	retired, err := repo.RetireRealmsNotIn(ctx, tenant, []string{"a", "c"})
	if err != nil {
		t.Fatalf("RetireRealmsNotIn: %v", err)
	}
	if retired != 1 {
		t.Errorf("retired = %d, want 1", retired)
	}
	live := liveRealms(t, repo, tenant)
	if _, ok := live["b"]; ok || len(live) != 2 {
		t.Errorf("live realms = %v, want a and c", live)
	}
	if !isRetired(t, db, tenant, "b") {
		t.Error("realm b is still live")
	}
	if _, ok := liveRealms(t, repo, other)["b"]; !ok {
		t.Error("another tenant's realm was retired")
	}

	// An empty list is what a failed or empty fetch would pass; it must not
	// wipe the tenant.
	if n, err := repo.RetireRealmsNotIn(ctx, tenant, nil); err != nil || n != 0 {
		t.Errorf("RetireRealmsNotIn(nil) = %d, %v; want 0, nil", n, err)
	}
	if len(liveRealms(t, repo, tenant)) != 2 {
		t.Error("an empty list retired realms")
	}
}

func TestSaveRealm_BringsARetiredRealmBack(t *testing.T) {
	repo, _ := newReconcileRepo(t)
	ctx := context.Background()
	tenant := reconcileTenant("revive")

	realm := reconcileRealm(tenant, "back-id", "back", true)
	if err := repo.SaveRealm(ctx, realm); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := repo.RetireRealmsNotIn(ctx, tenant, []string{"some-other-id"}); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if len(liveRealms(t, repo, tenant)) != 0 {
		t.Fatal("realm was not retired")
	}

	if err := repo.SaveRealm(ctx, realm); err != nil {
		t.Fatalf("save again: %v", err)
	}
	if _, ok := liveRealms(t, repo, tenant)["back-id"]; !ok {
		t.Error("a realm Keycloak lists again stayed retired")
	}
}

// The startup migration, not the model tag, decides the index shape; check
// what a database opened through NewClient actually has.
func TestRealmNameIndex_IgnoresRetiredRowsAfterStartup(t *testing.T) {
	_, db := newReconcileRepo(t)

	var def string
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_keycloak_realms_tenant_realm_name'`).
		Scan(&def).Error; err != nil {
		t.Fatalf("read index definition: %v", err)
	}
	if !strings.Contains(def, "tenant_id") || !strings.Contains(def, "deleted_at IS NULL") {
		t.Errorf("index = %q, want tenant-scoped over live rows", def)
	}
}
