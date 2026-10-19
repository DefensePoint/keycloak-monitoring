package database

import (
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// The amfa:read grant ran on every startup, so a grant taken away from a
// default role came back at the next restart. It now runs once per database.

func seedDefaultRoles(t *testing.T, c *Client) {
	t.Helper()
	for _, name := range []string{"admin", "operator", "viewer"} {
		if err := c.db.Create(&Role{Name: name, DisplayName: name, IsSystem: true, IsActive: true}).Error; err != nil {
			t.Fatalf("seed role %q: %v", name, err)
		}
	}
}

func amfaReadHolders(t *testing.T, c *Client) []string {
	t.Helper()
	var names []string
	if err := c.db.Raw(`SELECT r.name FROM role_permissions rp
		JOIN roles r ON r.id = rp.role_id JOIN permissions p ON p.id = rp.permission_id
		WHERE p.name = 'amfa:read' ORDER BY r.name`).Scan(&names).Error; err != nil {
		t.Fatalf("list amfa:read holders: %v", err)
	}
	return names
}

func amfaReadMarkers(t *testing.T, c *Client) int64 {
	t.Helper()
	var n int64
	if err := c.db.Model(&DataMigration{}).Where("key = ?", amfaReadGrantKey).Count(&n).Error; err != nil {
		t.Fatalf("count marker: %v", err)
	}
	return n
}

func TestMigrateAmfaReadPermission_ARemovedGrantStaysRemoved(t *testing.T) {
	c := newTestClient(t)
	seedDefaultRoles(t, c)

	if err := c.migrateAmfaReadPermission(); err != nil {
		t.Fatalf("first startup: %v", err)
	}
	if got := amfaReadHolders(t, c); len(got) != 3 {
		t.Fatalf("after the first startup amfa:read is held by %v, want admin, operator and viewer", got)
	}

	if err := c.db.Exec(`DELETE FROM role_permissions WHERE role_id IN (SELECT id FROM roles WHERE name IN ('operator', 'viewer'))
		AND permission_id = (SELECT id FROM permissions WHERE name = 'amfa:read')`).Error; err != nil {
		t.Fatalf("take the grant away: %v", err)
	}

	if err := c.migrateAmfaReadPermission(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if got := amfaReadHolders(t, c); len(got) != 1 || got[0] != "admin" {
		t.Errorf("after a restart amfa:read is held by %v, want only admin", got)
	}
	if n := amfaReadMarkers(t, c); n != 1 {
		t.Errorf("marker count = %d, want 1", n)
	}
}

func TestMigrateAmfaReadPermission_FailedRunLeavesNoMarker(t *testing.T) {
	c := newTestClient(t)
	seedDefaultRoles(t, c)

	if err := c.db.Exec(`ALTER TABLE role_permissions RENAME TO role_permissions_away`).Error; err != nil {
		t.Fatalf("hide role_permissions: %v", err)
	}
	restored := false
	restore := func() {
		if !restored {
			restored = true
			if err := c.db.Exec(`ALTER TABLE role_permissions_away RENAME TO role_permissions`).Error; err != nil {
				t.Fatalf("restore role_permissions: %v", err)
			}
		}
	}
	t.Cleanup(restore)

	if err := c.migrateAmfaReadPermission(); err == nil {
		t.Fatal("the grant reported success without a role_permissions table")
	}
	restore()
	if n := amfaReadMarkers(t, c); n != 0 {
		t.Fatalf("a failed run left %d marker(s); the next startup would not retry", n)
	}

	if err := c.migrateAmfaReadPermission(); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := amfaReadHolders(t, c); len(got) != 3 {
		t.Errorf("after the retry amfa:read is held by %v, want admin, operator and viewer", got)
	}
}

func TestMigrateAmfaReadPermission_ConcurrentStartupsGrantOnce(t *testing.T) {
	c := newTestClient(t)
	seedDefaultRoles(t, c)

	dsn := testdb.DSN(t, "KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	raw, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	closeOnCleanup(t, raw)
	other := &Client{db: raw, logger: logger.NewNoop()}

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, client := range []*Client{c, other} {
		wg.Add(1)
		go func(i int, client *Client) {
			defer wg.Done()
			<-start
			errs[i] = client.migrateAmfaReadPermission()
		}(i, client)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("startup %d: %v", i, err)
		}
	}
	if got := amfaReadHolders(t, c); len(got) != 3 {
		t.Errorf("amfa:read is held by %v, want admin, operator and viewer once each", got)
	}
	if n := amfaReadMarkers(t, c); n != 1 {
		t.Errorf("marker count = %d, want 1", n)
	}
}

// On a fresh database the default roles do not exist yet. The run still
// counts: the seeder creates them with amfa:read.
func TestMigrateAmfaReadPermission_FreshDatabaseIsMarked(t *testing.T) {
	c := newTestClient(t)

	if err := c.migrateAmfaReadPermission(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n := amfaReadMarkers(t, c); n != 1 {
		t.Errorf("marker count = %d, want 1", n)
	}
	var perms int64
	c.db.Model(&Permission{}).Where("name = ?", "amfa:read").Count(&perms)
	if perms != 1 {
		t.Errorf("amfa:read permission rows = %d, want 1", perms)
	}
}
