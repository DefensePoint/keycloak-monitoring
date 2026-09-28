package database

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// migrateUnprovenEmailVerified has to separate two rows that look identical in
// the column it rewrites: one whose true was hardcoded by the old OAuth path,
// and one whose true came from the IdP after the fix. Only last_login_at tells
// them apart, so the cutoff is the whole behaviour and these tests exercise it
// from both sides. The idempotency case is the one that matters in production:
// the migration runs on every boot, so a re-run that clobbered a real claim
// would quietly erase it on the next restart.

func newEmailVerifiedTestClient(t *testing.T) *Client {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	dsn := os.Getenv("KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_DESTRUCTIVE_TEST_DATABASE_DSN not set; skipping destructive integration test")
	}
	testdb.RequireDisposable(t, dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("failed to auto-migrate users: %v", err)
	}

	clean := func() { db.Exec("DELETE FROM users WHERE email LIKE ?", "ev-migration-%@example.com") }
	clean()
	t.Cleanup(clean)

	// Other suites seed users with explicit IDs, which leaves the identity
	// sequence behind the table's max and makes the first insert here collide
	// on the primary key. Resync it so this test measures the migration rather
	// than the order the suites happened to run in.
	db.Exec(`SELECT setval(pg_get_serial_sequence('users', 'id'),
		GREATEST((SELECT COALESCE(MAX(id), 0) FROM users), 1))`)

	return &Client{db: db, logger: logger.New(zerolog.New(io.Discard))}
}

// seedUser inserts one user row and returns its ID.
func seedUser(t *testing.T, c *Client, email, authMethod string, verified bool, lastLogin *time.Time) uint {
	t.Helper()
	u := &User{
		Subject:       email,
		Email:         email,
		EmailVerified: verified,
		AuthMethod:    authMethod,
		LastLoginAt:   lastLogin,
		IsActive:      true,
	}
	if err := c.db.Create(u).Error; err != nil {
		t.Fatalf("failed to seed user %s: %v", email, err)
	}
	return u.ID
}

func emailVerifiedOf(t *testing.T, c *Client, id uint) bool {
	t.Helper()
	var got User
	if err := c.db.First(&got, id).Error; err != nil {
		t.Fatalf("failed to reload user %d: %v", id, err)
	}
	return got.EmailVerified
}

func TestMigrateUnprovenEmailVerified_ClearsOnlyPreFixOAuthRows(t *testing.T) {
	c := newEmailVerifiedTestClient(t)

	before := emailVerifiedFixCutoff.Add(-24 * time.Hour)
	after := emailVerifiedFixCutoff.Add(24 * time.Hour)

	staleOAuth := seedUser(t, c, "ev-migration-stale@example.com", "oauth", true, &before)
	neverLoggedIn := seedUser(t, c, "ev-migration-never@example.com", "oauth", true, nil)
	postFixOAuth := seedUser(t, c, "ev-migration-postfix@example.com", "oauth", true, &after)
	simpleAuth := seedUser(t, c, "ev-migration-simple@example.com", "simple", true, &before)

	if err := c.migrateUnprovenEmailVerified(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if emailVerifiedOf(t, c, staleOAuth) {
		t.Error("a pre-cutoff OAuth row kept email_verified; its true was hardcoded, not asserted")
	}
	if emailVerifiedOf(t, c, neverLoggedIn) {
		t.Error("an OAuth row that never logged in kept email_verified; it cannot hold a real claim")
	}
	if !emailVerifiedOf(t, c, postFixOAuth) {
		t.Error("a post-cutoff OAuth row lost email_verified; that value came from the IdP")
	}
	if !emailVerifiedOf(t, c, simpleAuth) {
		t.Error("a simple-auth row was modified; this migration is scoped to OAuth users")
	}
}

func TestMigrateUnprovenEmailVerified_IsIdempotent(t *testing.T) {
	c := newEmailVerifiedTestClient(t)

	before := emailVerifiedFixCutoff.Add(-24 * time.Hour)
	stale := seedUser(t, c, "ev-migration-stale@example.com", "oauth", true, &before)

	if err := c.migrateUnprovenEmailVerified(); err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if emailVerifiedOf(t, c, stale) {
		t.Fatal("first run did not clear the stale row")
	}

	// A user logging in after the fix: the real claim is written, and
	// last_login_at moves past the cutoff.
	after := emailVerifiedFixCutoff.Add(48 * time.Hour)
	if err := c.db.Model(&User{}).Where("id = ?", stale).
		Updates(map[string]interface{}{"email_verified": true, "last_login_at": after}).Error; err != nil {
		t.Fatalf("failed to simulate a post-fix login: %v", err)
	}

	if err := c.migrateUnprovenEmailVerified(); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if !emailVerifiedOf(t, c, stale) {
		t.Error("a re-run erased a claim the IdP made after the fix; the migration is not idempotent " +
			"across a login, so every restart would undo real verification")
	}
}
