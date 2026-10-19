package testdb

import (
	"fmt"
	"strings"
	"testing"
)

// outcomeTB records whether a helper skipped or failed the test. Both end the
// test in the real testing package, so here each panics to stop the helper.
type outcomeTB struct {
	testing.TB
	skipped, failed string
}

func (o *outcomeTB) Helper() {}

func (o *outcomeTB) Skipf(format string, args ...any) {
	o.skipped = fmt.Sprintf(format, args...)
	panic("skip")
}

func (o *outcomeTB) Fatalf(format string, args ...any) {
	o.failed = fmt.Sprintf(format, args...)
	panic("fatal")
}

func runDSN(name string) (dsn string, o *outcomeTB) {
	o = &outcomeTB{}
	func() {
		defer func() { _ = recover() }()
		dsn = DSN(o, name)
	}()
	return dsn, o
}

// testDSNVar is a real DSN variable, overridden per test with t.Setenv.
const testDSNVar = "KMT_DESTRUCTIVE_TEST_DATABASE_DSN"

func TestDSN_ReturnsASetVariable(t *testing.T) {
	t.Setenv(testDSNVar, "host=db dbname=kmt_test")
	t.Setenv(RequiredDSNsEnv, testDSNVar)

	dsn, o := runDSN(testDSNVar)
	if dsn != "host=db dbname=kmt_test" || o.skipped != "" || o.failed != "" {
		t.Errorf("DSN = %q (skipped %q, failed %q), want the value and neither", dsn, o.skipped, o.failed)
	}
}

func TestDSN_SkipsWhenUnsetAndNotRequired(t *testing.T) {
	t.Setenv(testDSNVar, "")
	t.Setenv(RequiredDSNsEnv, "KMT_TEST_DATABASE_DSN")

	_, o := runDSN(testDSNVar)
	if o.skipped == "" || o.failed != "" {
		t.Errorf("skipped %q, failed %q; want a skip", o.skipped, o.failed)
	}
}

// The point of the helper: a run that says it exercises a database must not
// pass without one.
func TestDSN_FailsWhenUnsetAndRequired(t *testing.T) {
	t.Setenv(testDSNVar, "")
	for _, list := range []string{
		testDSNVar,
		"KMT_TEST_DATABASE_DSN," + testDSNVar,
		"KMT_TEST_DATABASE_DSN " + testDSNVar,
		" " + testDSNVar + " ,",
	} {
		t.Setenv(RequiredDSNsEnv, list)
		_, o := runDSN(testDSNVar)
		if o.failed == "" {
			t.Errorf("%s=%q: the test skipped (%q) instead of failing", RequiredDSNsEnv, list, o.skipped)
			continue
		}
		if !strings.Contains(o.failed, testDSNVar) || !strings.Contains(o.failed, RequiredDSNsEnv) {
			t.Errorf("failure %q should name the missing variable and %s", o.failed, RequiredDSNsEnv)
		}
	}
}

// A misspelled or unknown entry would require nothing, so it fails every
// database test instead, naming the entry, even one whose own DSN is set.
func TestDSN_AnUnknownRequiredNameFails(t *testing.T) {
	for _, bad := range []string{
		"KMT_DESTRUCTIVE_TEST_DATABSE_DSN", // misspelled
		testDSNVar + "_EXTRA",              // a near miss
		"kmt_test_database_dsn",            // wrong case
	} {
		for _, set := range []string{"", "host=db dbname=kmt_test"} {
			t.Setenv(testDSNVar, set)
			t.Setenv(RequiredDSNsEnv, "KMT_TEST_DATABASE_DSN,"+bad)
			_, o := runDSN(testDSNVar)
			if o.failed == "" || !strings.Contains(o.failed, bad) {
				t.Errorf("%s lists %q (own DSN set: %v): failed %q, skipped %q; want a failure naming it",
					RequiredDSNsEnv, bad, set != "", o.failed, o.skipped)
			}
		}
	}
}
