package testdb

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// lockTestKey is a key of the test's own, so exercising the wait does not
// compete with other packages for the shared lock.
const lockTestKey = sharedDBLockKey + 1

func TestLock_WaitsForAnotherHolderAndRecognisesItself(t *testing.T) {
	dsn := DSN(t, "KMT_TEST_DATABASE_DSN")
	Lock(t, dsn)

	ctx := context.Background()
	target, key, err := lockTarget(ctx, dsn, lockTestKey)
	if err != nil {
		t.Fatalf("lock target: %v", err)
	}
	other, err := pgx.ConnectConfig(ctx, target)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = other.Close(ctx) }()
	// Another process holding the lock, as far as Postgres can tell.
	if _, err := other.Exec(ctx, "SELECT pg_advisory_lock($1)", key); err != nil {
		t.Fatalf("take the lock elsewhere: %v", err)
	}
	t.Cleanup(func() {
		lockMu.Lock()
		defer lockMu.Unlock()
		for id, h := range held {
			if id.key == lockTestKey {
				_ = h.conn.Close(ctx)
				delete(held, id)
			}
		}
	})

	done := make(chan struct{})
	go func() {
		acquire(t, dsn, lockTestKey)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("acquired a lock another session holds")
	case <-time.After(time.Second):
	}

	if _, err := other.Exec(ctx, "SELECT pg_advisory_unlock($1)", key); err != nil {
		t.Fatalf("release the lock elsewhere: %v", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("still waiting after the other holder released the lock")
	}

	// The same database spelled differently, as when both DSN variables name
	// it: waiting would be waiting on ourselves.
	alias := WithDSNParam(dsn, "application_name", "pmp_lock_test")
	returned := make(chan struct{})
	go func() {
		acquire(t, alias, lockTestKey)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlocked on a lock this process already holds under another DSN spelling")
	}
}

var (
	// dsnRead matches a test reading a shared database variable directly,
	// which would skip silently where testdb.DSN can be made to fail.
	dsnRead = regexp.MustCompile(`os\.(Getenv|LookupEnv)\("KMT_(DESTRUCTIVE_)?TEST_DATABASE_DSN"\)`)
	// dsnUse matches a test reading one through testdb.DSN.
	dsnUse = regexp.MustCompile(`testdb\.DSN\(t[a-zA-Z]*, "KMT_(DESTRUCTIVE_)?TEST_DATABASE_DSN"\)`)
)

// Every test that reads a shared database variable must do so through
// testdb.DSN, so a run that requires the database cannot pass by skipping, and
// must take the lock, or the parallel-package breakage comes back with the next
// suite that forgets either.
func TestLock_EverySharedDatabaseSuiteTakesIt(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("expected the module root at %s: %v", root, err)
	}

	checked := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") || filepath.Dir(path) == filepath.Join(root, "internal", "testdb") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if dsnRead.Match(src) {
			t.Errorf("%s reads a shared test database DSN with os.Getenv; use testdb.DSN, which a run can make fail instead of skip", rel)
		}
		if !dsnUse.Match(src) {
			return nil
		}
		checked++
		if !strings.Contains(string(src), "testdb.Lock(") {
			t.Errorf("%s reads a shared test database DSN but never calls testdb.Lock", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("found no test reading a shared database DSN; the pattern no longer matches")
	}
}
