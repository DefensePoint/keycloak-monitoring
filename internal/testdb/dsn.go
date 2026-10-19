package testdb

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// RequiredDSNsEnv lists the DSN variables a run must provide, separated by
// commas or spaces. A database test whose variable is listed but unset fails
// instead of skipping.
//
// Skipping is right for a plain `go test ./...` on a machine without a
// database, but a skip is invisible: without -v, a package whose every test
// skipped prints the same "ok" as one whose every test passed. So a CI job
// with a missing or misspelled DSN went green having tested nothing, and a
// manual run with one variable set and the other not read as a pass for both.
// Listing the variables a run is meant to exercise makes their absence loud.
//
// It is a list rather than a switch because a run may provide only some
// variables on purpose: a run against a shared database must still skip the
// destructive suites.
const RequiredDSNsEnv = "KMT_REQUIRE_TEST_DSNS"

// KnownDSNs are the DSN variables the database suites read. RequiredDSNsEnv
// may only list these: a misspelled entry would otherwise require nothing,
// which is the silent green this exists to prevent.
var KnownDSNs = []string{"KMT_TEST_DATABASE_DSN", "KMT_DESTRUCTIVE_TEST_DATABASE_DSN"}

// DSN returns the value of the DSN variable name. When it is unset the test
// skips, or fails when RequiredDSNsEnv lists name.
func DSN(t testing.TB, name string) string {
	t.Helper()
	for _, listed := range requiredDSNs() {
		if !slices.Contains(KnownDSNs, listed) {
			t.Fatalf("%s lists %q, which is not a test DSN variable (known: %s): a misspelling would require nothing",
				RequiredDSNsEnv, listed, strings.Join(KnownDSNs, ", "))
		}
	}
	if dsn := os.Getenv(name); dsn != "" {
		return dsn
	}
	if dsnRequired(name) {
		t.Fatalf("%s is not set, but %s lists it: without it this test would skip and the run would look green", name, RequiredDSNsEnv)
	}
	t.Skipf("%s not set; skipping integration test", name)
	return ""
}

func dsnRequired(name string) bool {
	return slices.Contains(requiredDSNs(), name)
}

func requiredDSNs() []string {
	return strings.FieldsFunc(os.Getenv(RequiredDSNsEnv), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
}
