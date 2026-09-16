package database

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// A plain unique index on users(username) lets exactly one OAuth user exist,
// because they all have an empty username and PostgreSQL treats the empty
// string as a value. migrateUsernameIndex converts it, but its failure is
// tolerated so an unrelated hiccup cannot keep the platform down — which means
// something else has to stop a boot that tolerated it. These tests are that
// something.

func newIndexGuardClient(t *testing.T) (*Client, *gorm.DB) {
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
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("auto-migrate users: %v", err)
	}
	t.Cleanup(func() { db.Exec("DROP INDEX IF EXISTS idx_users_username") })

	return &Client{db: db, logger: logger.New(zerolog.New(io.Discard))}, db
}

func TestAssertUsernameIndexIsSafe_RefusesAPlainUniqueIndex(t *testing.T) {
	c, db := newIndexGuardClient(t)

	db.Exec("DROP INDEX IF EXISTS idx_users_username")
	if err := db.Exec("CREATE UNIQUE INDEX idx_users_username ON users(username)").Error; err != nil {
		t.Fatalf("create plain index: %v", err)
	}

	err := c.assertUsernameIndexIsSafe()
	if err == nil {
		t.Fatal("startup was allowed with a plain unique index; the second person to sign in " +
			"through SSO would fail on a constraint violation")
	}
	// The operator reading this has a broken index, not a broken login, and the
	// message is the only thing connecting the two.
	if !strings.Contains(err.Error(), "CREATE UNIQUE INDEX") {
		t.Errorf("the refusal should carry the statement that fixes it, got: %v", err)
	}
}

func TestAssertUsernameIndexIsSafe_AcceptsAPartialIndex(t *testing.T) {
	c, db := newIndexGuardClient(t)

	db.Exec("DROP INDEX IF EXISTS idx_users_username")
	if err := db.Exec(`CREATE UNIQUE INDEX idx_users_username ON users(username)
		WHERE username IS NOT NULL AND username != ''`).Error; err != nil {
		t.Fatalf("create partial index: %v", err)
	}

	if err := c.assertUsernameIndexIsSafe(); err != nil {
		t.Errorf("a partial index is the state the migration produces and must be accepted: %v", err)
	}
}

// Every path through migrateUsernameIndex ends with the index in place, so a
// missing one means the migration failed — in practice because CREATE UNIQUE
// INDEX rejected duplicate usernames after the old index was already dropped.
// Starting from there enforces username uniqueness nowhere at all, which is
// weaker than the state the boot began in.
func TestAssertUsernameIndexIsSafe_RefusesAMissingIndex(t *testing.T) {
	c, db := newIndexGuardClient(t)

	if err := db.Exec("DROP INDEX IF EXISTS idx_users_username").Error; err != nil {
		t.Fatalf("drop index: %v", err)
	}

	err := c.assertUsernameIndexIsSafe()
	if err == nil {
		t.Fatal("startup was allowed with nothing enforcing username uniqueness")
	}
	if !strings.Contains(err.Error(), "duplicated") {
		t.Errorf("the refusal should name the usual cause so an operator knows where to "+
			"look, got: %v", err)
	}
}
