package database

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

func TestBackfillEventTenantIDs_SharedRealmStaysUnattributedAfterTenantGoesAway(t *testing.T) {
	c := newPurgeTestClient(t)
	clearEventTenantBackfill(t, c)

	seedBackfillTenant(t, c, "tenant-a", "master")
	seedBackfillTenant(t, c, "tenant-b", "master")
	seedUnattributedEvent(t, c, "legacy-master", "keycloak:master")

	var buf bytes.Buffer
	c.logger = logger.New(zerolog.New(&buf)).PinLevel("warn")

	if err := c.backfillEventTenantIDs(); err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	if got := eventTenant(t, c, "legacy-master"); got != "" {
		t.Fatalf("shared-realm row attributed to %q on the first run", got)
	}
	if n := backfillMarkerCount(t, c); n != 1 {
		t.Fatalf("marker count = %d, want 1", n)
	}
	if !strings.Contains(buf.String(), "Events remain unattributed") {
		t.Fatalf("first run did not log the unattributed rows: %s", buf.String())
	}

	if err := c.DB().Exec(`DELETE FROM keycloak_realms WHERE tenant_id = ?`, "tenant-b").Error; err != nil {
		t.Fatalf("delete realm: %v", err)
	}

	buf.Reset()
	if err := c.backfillEventTenantIDs(); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if got := eventTenant(t, c, "legacy-master"); got != "" {
		t.Fatalf("later startup attributed the leftover row to %q", got)
	}
	if buf.Len() != 0 {
		t.Fatalf("later startup logged again: %s", buf.String())
	}
}

func TestBackfillEventTenantIDs_UniqueRealmAttributesOnce(t *testing.T) {
	c := newPurgeTestClient(t)
	clearEventTenantBackfill(t, c)

	seedBackfillTenant(t, c, "tenant-a", "customer")
	seedUnattributedEvent(t, c, "legacy-customer", "keycloak:customer")

	if err := c.backfillEventTenantIDs(); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if got := eventTenant(t, c, "legacy-customer"); got != "tenant-a" {
		t.Fatalf("unique-realm row tenant_id = %q, want tenant-a", got)
	}

	seedUnattributedEvent(t, c, "later-customer", "keycloak:customer")
	if err := c.backfillEventTenantIDs(); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if got := eventTenant(t, c, "later-customer"); got != "" {
		t.Fatalf("row inserted after the marker was attributed to %q", got)
	}
}

func TestBackfillEventTenantIDs_FailedRunLeavesNoMarker(t *testing.T) {
	c := newPurgeTestClient(t)
	clearEventTenantBackfill(t, c)
	seedBackfillTenant(t, c, "tenant-a", "customer")
	seedUnattributedEvent(t, c, "legacy-customer", "keycloak:customer")

	if err := c.DB().Exec(`ALTER TABLE events RENAME COLUMN source TO source_hidden`).Error; err != nil {
		t.Fatalf("rename source: %v", err)
	}
	if err := c.backfillEventTenantIDs(); err == nil {
		t.Fatal("backfill succeeded against a missing source column")
	}
	if n := backfillMarkerCount(t, c); n != 0 {
		t.Fatalf("failed run wrote %d marker rows", n)
	}

	if err := c.DB().Exec(`ALTER TABLE events RENAME COLUMN source_hidden TO source`).Error; err != nil {
		t.Fatalf("restore source: %v", err)
	}
	if err := c.backfillEventTenantIDs(); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := eventTenant(t, c, "legacy-customer"); got != "tenant-a" {
		t.Fatalf("retry tenant_id = %q, want tenant-a", got)
	}
	if n := backfillMarkerCount(t, c); n != 1 {
		t.Fatalf("marker count after retry = %d, want 1", n)
	}
}

func TestNewReadOnlyClientDoesNotWriteBackfill(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dsn := testdb.DSN(t, "KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	testdb.RequireDisposable(t, dsn)
	testdb.Lock(t, dsn)

	raw, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := raw.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	sqlDB, err := raw.DB()
	if err != nil {
		t.Fatalf("sql DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	ro, err := NewReadOnlyClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewReadOnlyClient: %v", err)
	}
	t.Cleanup(func() { _ = ro.Close() })

	var n int64
	if err := ro.DB().Raw(
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name = 'data_migrations'`).
		Scan(&n).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 0 {
		t.Fatalf("read-only client created data_migrations")
	}
}

func TestBackfillEventTenantIDs_ConcurrentStartupsOneMarker(t *testing.T) {
	c := newPurgeTestClient(t)
	clearEventTenantBackfill(t, c)
	seedBackfillTenant(t, c, "tenant-a", "customer")
	seedUnattributedEvent(t, c, "legacy-customer", "keycloak:customer")

	dsn := testdb.DSN(t, "KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	raw, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	sqlDB, err := raw.DB()
	if err != nil {
		t.Fatalf("second sql DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	other := &Client{db: raw, logger: logger.NewNoop()}

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, client := range []*Client{c, other} {
		wg.Add(1)
		go func(i int, client *Client) {
			defer wg.Done()
			<-start
			errs[i] = client.backfillEventTenantIDs()
		}(i, client)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("startup %d: %v", i, err)
		}
	}
	if n := backfillMarkerCount(t, c); n != 1 {
		t.Fatalf("marker count = %d, want 1", n)
	}
	if got := eventTenant(t, c, "legacy-customer"); got != "tenant-a" {
		t.Fatalf("concurrent backfill tenant_id = %q, want tenant-a", got)
	}
}

func clearEventTenantBackfill(t *testing.T, c *Client) {
	t.Helper()
	if err := c.DB().Exec(`DELETE FROM data_migrations WHERE key = ?`, eventTenantIDBackfillKey).Error; err != nil {
		t.Fatalf("clear marker: %v", err)
	}
}

func backfillMarkerCount(t *testing.T, c *Client) int64 {
	t.Helper()
	var n int64
	if err := c.DB().Model(&DataMigration{}).Where("key = ?", eventTenantIDBackfillKey).Count(&n).Error; err != nil {
		t.Fatalf("count marker: %v", err)
	}
	return n
}

func seedBackfillTenant(t *testing.T, c *Client, tenantID, realmName string) {
	t.Helper()
	if err := c.DB().Create(&KeycloakTenant{
		TenantID: tenantID, Name: tenantID, ServerURL: "https://kc.example.com",
		AdminRealm: "master", ClientID: "monitoring-service", ClientSecret: "s", Enabled: true,
	}).Error; err != nil {
		t.Fatalf("seed tenant %s: %v", tenantID, err)
	}
	if err := c.DB().Create(&KeycloakRealmInfo{
		TenantID: tenantID, RealmID: tenantID + "-" + realmName, RealmName: realmName,
	}).Error; err != nil {
		t.Fatalf("seed realm %s/%s: %v", tenantID, realmName, err)
	}
}

func seedUnattributedEvent(t *testing.T, c *Client, eventID, source string) {
	t.Helper()
	ev := &Event{
		EventID:   eventID,
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Source:    source,
		TenantID:  "",
		Status:    "active",
	}
	if err := c.DB().Create(ev).Error; err != nil {
		t.Fatalf("seed event %s: %v", eventID, err)
	}
}

func eventTenant(t *testing.T, c *Client, eventID string) string {
	t.Helper()
	var tenantID string
	if err := c.DB().Raw(`SELECT tenant_id FROM events WHERE event_id = ?`, eventID).Scan(&tenantID).Error; err != nil {
		t.Fatalf("read event %s: %v", eventID, err)
	}
	return tenantID
}

func TestNewClient_EmptyDatabaseConcurrentAutoMigrate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dsn := testdb.DSN(t, "KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	testdb.RequireDisposable(t, dsn)
	testdb.Lock(t, dsn)

	raw, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	sqlDB, err := raw.DB()
	if err != nil {
		t.Fatalf("sql DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := raw.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}

	cfg := testdb.DatabaseConfig(t, dsn)
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	clients := make([]*Client, 2)
	for i := range clients {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			clients[i], errs[i] = NewClient(cfg, logger.NewNoop())
		}(i)
	}
	close(start)
	wg.Wait()
	for _, c := range clients {
		if c != nil {
			_ = c.Close()
		}
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("startup %d: %v", i, err)
		}
	}
}
