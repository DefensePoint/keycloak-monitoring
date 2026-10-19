// Package testdb holds the helpers shared by the database tests: the guard the
// destructive tests run against a real server, and the statement recorder that
// lets the rest assert on generated SQL with no server at all.
package testdb

import (
	"regexp"
	"strings"
	"testing"

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
}
