package database

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// The unique indexes on users used to cover every row and every value,
// including rows the application had deleted and columns a whole class of
// account leaves blank. These tests state the behaviour in terms of what a
// person can do — be deleted and come back, exist without an email address —
// rather than in terms of index definitions, because that is what broke.

func newUniqueIndexTestClient(t *testing.T) (*Client, *gorm.DB) {
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

	c := &Client{db: db, logger: logger.New(zerolog.New(io.Discard))}
	if err := c.migrateUserUniqueIndexes(); err != nil {
		t.Fatalf("migrate unique indexes: %v", err)
	}

	clean := func() { db.Exec("DELETE FROM users WHERE email LIKE ? OR subject LIKE ?", "uix-%", "uix-%") }
	clean()
	t.Cleanup(clean)

	// Other suites seed users with explicit IDs, which leaves the identity
	// sequence behind the table's max and makes the first insert here collide
	// on the primary key. Resync it so these tests measure the indexes rather
	// than the order the suites happened to run in.
	db.Exec(`SELECT setval(pg_get_serial_sequence('users', 'id'),
		GREATEST((SELECT COALESCE(MAX(id), 0) FROM users), 1))`)

	return c, db
}

func mustCreate(t *testing.T, db *gorm.DB, u *User) {
	t.Helper()
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create %q: %v", u.Subject, err)
	}
}

// A deleted user returning is the case the whole item exists for: the row stays
// behind a soft delete, so their address was reserved by an account that no
// longer exists as far as anyone using the platform is concerned.
func TestUserUniqueIndexes_ADeletedUserCanBeAddedAgain(t *testing.T) {
	_, db := newUniqueIndexTestClient(t)

	original := &User{Subject: "uix-subject", Email: "uix-person@example.invalid", AuthMethod: "oauth", IsActive: true}
	mustCreate(t, db, original)

	if err := db.Delete(original).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	returning := &User{Subject: "uix-subject", Email: "uix-person@example.invalid", AuthMethod: "oauth", IsActive: true}
	if err := db.Create(returning).Error; err != nil {
		t.Fatalf("the same person could not be added back after being deleted, which is the "+
			"whole point of a soft delete being reversible: %v", err)
	}

	// And the live rows must still be unique between themselves.
	duplicate := &User{Subject: "uix-other", Email: "uix-person@example.invalid", AuthMethod: "oauth", IsActive: true}
	if err := db.Create(duplicate).Error; err == nil {
		t.Error("two live users share an email address; narrowing the index must not stop it " +
			"enforcing anything")
	}
}

// Keycloak accounts frequently carry no email, and role_sync copies that
// absence through verbatim.
func TestUserUniqueIndexes_UsersWithoutAnEmailCanCoexist(t *testing.T) {
	_, db := newUniqueIndexTestClient(t)

	for i := 0; i < 2; i++ {
		u := &User{Subject: fmt.Sprintf("uix-noemail-%d", i), Email: "", AuthMethod: "oauth", IsActive: true}
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("user %d without an email could not be created, so the second Keycloak "+
				"account lacking one can never be synced: %v", i, err)
		}
	}
}

// Simple auth derives the subject from the email address, so an account
// without one carries an empty subject too.
func TestUserUniqueIndexes_UsersWithoutASubjectCanCoexist(t *testing.T) {
	_, db := newUniqueIndexTestClient(t)

	for i := 0; i < 2; i++ {
		u := &User{
			Subject: "", Email: fmt.Sprintf("uix-nosubject-%d@example.invalid", i),
			Username: fmt.Sprintf("uix-nosubject-%d", i), AuthMethod: "simple", IsActive: true,
		}
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("user %d without a subject could not be created: %v", i, err)
		}
	}
}

// Narrowing an index must not weaken what it still covers.
func TestUserUniqueIndexes_LiveDuplicatesAreStillRejected(t *testing.T) {
	_, db := newUniqueIndexTestClient(t)

	for _, tc := range []struct {
		name  string
		first *User
		clash *User
	}{
		{
			"subject",
			&User{Subject: "uix-dupe", Email: "uix-a@example.invalid", AuthMethod: "oauth", IsActive: true},
			&User{Subject: "uix-dupe", Email: "uix-b@example.invalid", AuthMethod: "oauth", IsActive: true},
		},
		{
			"username",
			&User{Subject: "uix-u1", Email: "uix-u1@example.invalid", Username: "uix-taken", AuthMethod: "simple", IsActive: true},
			&User{Subject: "uix-u2", Email: "uix-u2@example.invalid", Username: "uix-taken", AuthMethod: "simple", IsActive: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db.Exec("DELETE FROM users WHERE subject LIKE ?", "uix-%")
			mustCreate(t, db, tc.first)
			if err := db.Create(tc.clash).Error; err == nil {
				t.Errorf("two live users share a %s", tc.name)
			}
		})
	}
}

// The migration runs on every boot, so it has to be a no-op once applied and
// must never destroy an index it already fixed.
func TestUserUniqueIndexes_MigrationIsIdempotent(t *testing.T) {
	c, db := newUniqueIndexTestClient(t)

	before := indexDefs(t, db)
	for i := 0; i < 3; i++ {
		if err := c.migrateUserUniqueIndexes(); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	after := indexDefs(t, db)

	for name, def := range before {
		if after[name] != def {
			t.Errorf("%s changed across re-runs:\n  before %s\n  after  %s", name, def, after[name])
		}
	}
	if len(after) != len(before) {
		t.Errorf("index count changed from %d to %d", len(before), len(after))
	}
}

func indexDefs(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	var rows []struct{ Indexname, Indexdef string }
	if err := db.Raw(
		`SELECT indexname, indexdef FROM pg_indexes WHERE tablename = 'users' AND indexdef LIKE '%UNIQUE%'`).
		Scan(&rows).Error; err != nil {
		t.Fatalf("read index definitions: %v", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Indexname] = r.Indexdef
	}
	return out
}

// A database that predates this change has plain indexes; the migration has to
// convert them in place without disturbing the rows already stored.
func TestUserUniqueIndexes_ConvertsPlainIndexesInPlace(t *testing.T) {
	c, db := newUniqueIndexTestClient(t)

	existing := &User{Subject: "uix-keep", Email: "uix-keep@example.invalid", AuthMethod: "oauth", IsActive: true}
	mustCreate(t, db, existing)

	for _, stmt := range []string{
		"DROP INDEX IF EXISTS idx_users_email",
		"CREATE UNIQUE INDEX idx_users_email ON users(email)",
		"DROP INDEX IF EXISTS idx_users_subject",
		"CREATE UNIQUE INDEX idx_users_subject ON users(subject)",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("install plain index: %v", err)
		}
	}

	if err := c.migrateUserUniqueIndexes(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := c.assertUserUniqueIndexesAreSafe(); err != nil {
		t.Fatalf("the migration left an index the guard rejects: %v", err)
	}

	var count int64
	db.Model(&User{}).Where("subject = ?", "uix-keep").Count(&count)
	if count != 1 {
		t.Errorf("the row present before the migration is gone: found %d", count)
	}
}

// migrateUsernameIndex runs earlier and predates this change. It gives the
// username index a predicate that excludes blank usernames but not deleted
// rows, so a migration that skipped any index already carrying a WHERE clause
// left that one half-fixed and a deleted user's username reserved forever.
// That is exactly what happened on the first run of this work.
func TestUserUniqueIndexes_UpgradesTheOlderUsernamePredicate(t *testing.T) {
	c, db := newUniqueIndexTestClient(t)

	// The index exactly as migrateUsernameIndex leaves it.
	db.Exec("DROP INDEX IF EXISTS idx_users_username")
	if err := db.Exec(`CREATE UNIQUE INDEX idx_users_username ON users(username)
		WHERE username IS NOT NULL AND username != ''`).Error; err != nil {
		t.Fatalf("install the older partial index: %v", err)
	}

	if err := c.migrateUserUniqueIndexes(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	defs := indexDefs(t, db)
	if !isLiveOnly(defs["idx_users_username"]) {
		t.Fatalf("the older predicate was left in place, so a deleted user still holds their "+
			"username: %s", defs["idx_users_username"])
	}

	// And prove it in behaviour, not only in the catalogue.
	original := &User{Subject: "uix-un-1", Email: "uix-un-1@example.invalid", Username: "uix-returning", AuthMethod: "simple", IsActive: true}
	mustCreate(t, db, original)
	if err := db.Delete(original).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	returning := &User{Subject: "uix-un-2", Email: "uix-un-2@example.invalid", Username: "uix-returning", AuthMethod: "simple", IsActive: true}
	if err := db.Create(returning).Error; err != nil {
		t.Errorf("a deleted user's username is still reserved: %v", err)
	}
}
