// Package testdb holds the helpers shared by the database tests: the guard the
// destructive tests run against a real server, and the statement recorder that
// lets the rest assert on generated SQL with no server at all.
package testdb

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// disposableName matches a database name carrying a test, tmp or scratch
// component. The database that must never be reached is called "monitoring" in
// all three deployments, and no spelling of "monitoring" can match this.
var disposableName = regexp.MustCompile(`(^|[_-])(test|tmp|scratch)([_-]|$)`)

// RequireDisposable fails the test unless dsn names a database whose name marks
// it as throwaway. Call it before the first destructive statement.
//
// This is a backstop, not the control: the control is that destructive helpers
// read a DSN variable with "DESTRUCTIVE" in its name. Failures are fatal rather
// than skips, so a misdirected DSN is reported instead of silently passing.
func RequireDisposable(t testing.TB, dsn string) {
	t.Helper()

	// Never widen this to report the DSN: it carries the password.
	parsed, err := pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse destructive test DSN: %v", err)
	}
	if !disposableName.MatchString(strings.ToLower(parsed.Database)) {
		t.Fatalf("refusing to run a destructive test against database %q: the name needs a test, tmp or scratch component (e.g. kmt_test)", parsed.Database)
	}

	serialise(t, dsn)
}

// destructiveLockKey identifies the advisory lock. The value is arbitrary.
const destructiveLockKey int64 = 0x6b6d745f64657374

// destructiveLockWait bounds the wait so a deadlock fails rather than hangs.
const destructiveLockWait = 4 * time.Minute

// serialise gives a test exclusive use of the database.
//
// These packages wipe the schema and re-run AutoMigrate, and go test runs
// packages in parallel processes against the one database the DSN names. A
// session-level Postgres advisory lock orders them: it is held across
// processes and released if a test binary dies.
func serialise(t testing.TB, dsn string) {
	t.Helper()
	lock(t, dsn, "SELECT pg_advisory_lock($1)", "SELECT pg_advisory_unlock($1)")
}

// ShareDatabase claims the database for a test that reads and writes rows.
//
// It takes the same exclusive lock as serialise rather than a shared one: these
// tests write to the same tables, so running two together is enough for one to
// see the other's rows. Call it once the DSN is known to be set, before the
// first query.
func ShareDatabase(t testing.TB, dsn string) {
	t.Helper()
	lock(t, dsn, "SELECT pg_advisory_lock($1)", "SELECT pg_advisory_unlock($1)")
}

func lock(t testing.TB, dsn, acquire, release string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), destructiveLockWait)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to take the destructive test lock: %v", err)
	}

	if _, err := conn.Exec(ctx, acquire, destructiveLockKey); err != nil {
		_ = conn.Close(context.Background())
		t.Fatalf("waited %s for the destructive test lock: %v", destructiveLockWait, err)
	}

	t.Cleanup(func() {
		// Closing the session would drop the lock anyway; this returns it sooner.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = conn.Exec(ctx, release, destructiveLockKey)
		_ = conn.Close(ctx)
	})
}
