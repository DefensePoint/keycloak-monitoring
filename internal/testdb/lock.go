package testdb

import (
	"context"
	"hash/fnv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// The database-backed suites share one database and clean up by deleting,
// dropping and migrating, so two packages running at once break each other:
// one drops the schema while the other is mid-migration, and what surfaces is
// `relation "users" does not exist`, or a security posture test reporting a
// tenant-isolation failure that is not there. go test runs packages in
// parallel by default, and a -p 1 in the command line is not something a test
// can check for.
//
// Lock makes them take turns instead. The first call in a test process takes a
// session-level advisory lock on a connection it keeps open, so the lock lasts
// until the process exits and releases it with the connection. Every package's
// test binary is its own process, so packages sharing a database run one at a
// time whatever -p says, and so do two checkouts or two people pointed at the
// same database. Tests inside one package are unaffected: none of the
// database-backed ones call t.Parallel.
//
// The lock is taken in the server's postgres maintenance database, not the
// database under test, keyed by the test database's name, so a process queued
// for its turn never has a session in the database being tested. On
// TimescaleDB a new session reads the extension's catalog, and a waiter doing
// that every second deadlocks the running package's DROP SCHEMA. When the
// maintenance database cannot be reached, the lock falls back to the test
// database itself.

// sharedDBLockKey identifies the lock. Advisory locks are per database, so it
// only has to be unique among locks taken in a test database.
const sharedDBLockKey int64 = 0x504d_505f_5445_5354 // "PMP_TEST"

// lockPoll is how often a waiting process retries, lockReport how often it
// says it is still waiting, so a queued package does not look hung, and
// lockErrorBudget how long it tolerates a failing retry before giving up.
const (
	lockPoll        = time.Second
	lockReport      = 10 * time.Second
	lockErrorBudget = time.Minute
)

// lockID is one lock in one database, as named by a DSN.
type lockID struct {
	dsn string
	key int64
}

var (
	lockMu sync.Mutex
	// held are the connections holding a lock, one per database and key this
	// process has locked. Never closed: closing is how the lock is released.
	held = map[lockID]heldLock{}
)

// heldLock is a connection holding the lock under its effective key.
type heldLock struct {
	conn *pgx.Conn
	key  int64
}

// Lock takes the shared test-database lock for dsn, waiting while another
// process holds it, and keeps it for the rest of this test process. Call it
// before the test touches the database.
func Lock(t testing.TB, dsn string) {
	t.Helper()
	acquire(t, dsn, sharedDBLockKey)
}

// acquire is Lock for any key, so the lock's own test can exercise waiting
// without competing with other packages for the shared one.
//
// A waiting process holds no connection between attempts. Idle waiters would
// add one connection each on top of whatever the running package uses, which
// is enough to exhaust max_connections when several invocations queue at once.
// A fresh session per attempt also matters on TimescaleDB: the running package
// may drop the schema and the extension with it, after which a session that
// had loaded the extension fails every query.
func acquire(t testing.TB, dsn string, key int64) {
	t.Helper()

	lockMu.Lock()
	defer lockMu.Unlock()
	id := lockID{dsn: dsn, key: key}
	if _, ok := held[id]; ok {
		return
	}

	ctx := context.Background()
	target, lockKey, err := lockTarget(ctx, dsn, key)
	if err != nil {
		// Never report the DSN: it carries the password.
		t.Fatalf("shared test database lock: %v", err)
	}
	var waitingSince, lastReport, failingSince time.Time
	for first := true; ; first = false {
		conn, holder, err := tryLock(ctx, target, lockKey)
		switch {
		case err != nil && first:
			t.Fatalf("shared test database lock: %v", err)
		case err != nil:
			// While waiting, the running package may be dropping the schema or
			// using every connection. Retry for a while rather than fail a test
			// for being queued behind a busy one.
			if failingSince.IsZero() {
				failingSince = time.Now()
				t.Logf("shared test database lock: retrying after: %v", err)
			}
			if time.Since(failingSince) > lockErrorBudget {
				t.Fatalf("shared test database lock: still failing after %s: %v", lockErrorBudget, err)
			}
		case conn != nil:
			held[id] = heldLock{conn: conn, key: lockKey}
			if !waitingSince.IsZero() {
				t.Logf("shared test database lock acquired after %s", time.Since(waitingSince).Round(time.Second))
			}
			return
		case ownsConnection(holder, lockKey):
			// The holder is this process under a different spelling of the same
			// database, say the DESTRUCTIVE and plain DSNs both aimed at one.
			// Waiting on ourselves would never end, and we already hold it.
			return
		default:
			failingSince = time.Time{}
		}

		now := time.Now()
		if waitingSince.IsZero() {
			waitingSince = now
		}
		if err == nil && now.Sub(lastReport) >= lockReport {
			t.Logf("waiting for the shared test database: another test process holds it (backend pid %d)", holder)
			lastReport = now
		}
		time.Sleep(lockPoll)
	}
}

// tryLock makes one attempt on a fresh connection. On success it returns that
// connection, which now holds the lock and must stay open. Otherwise it closes
// it and returns the backend pid holding the lock, 0 if nobody does any more.
func tryLock(ctx context.Context, target *pgx.ConnConfig, key int64) (*pgx.Conn, int32, error) {
	conn, err := pgx.ConnectConfig(ctx, target.Copy())
	if err != nil {
		return nil, 0, err
	}
	var got bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&got); err != nil {
		_ = conn.Close(ctx)
		return nil, 0, err
	}
	if got {
		return conn, 0, nil
	}
	defer func() { _ = conn.Close(ctx) }()

	var pid int32
	err = conn.QueryRow(ctx, `
		SELECT pid FROM pg_locks
		WHERE locktype = 'advisory' AND granted
		  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
		  AND classid = (($1::bigint >> 32) & 4294967295)::oid
		  AND objid = ($1::bigint & 4294967295)::oid
		  AND objsubid = 1`, key).Scan(&pid)
	if err == pgx.ErrNoRows {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return nil, pid, nil
}

// lockTarget is where the lock for dsn is taken and under which key: the
// maintenance database with key mixed with the test database's name, or,
// when the maintenance database refuses the connection, the test database
// itself with key unchanged.
func lockTarget(ctx context.Context, dsn string, key int64) (*pgx.ConnConfig, int64, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, 0, err
	}
	maint := cfg.Copy()
	maint.Database = "postgres"
	if cfg.Database != maint.Database {
		if conn, err := pgx.ConnectConfig(ctx, maint.Copy()); err == nil {
			_ = conn.Close(ctx)
			h := fnv.New64a()
			_, _ = h.Write([]byte(cfg.Database))
			return maint, key ^ int64(h.Sum64()), nil
		}
	}
	return cfg, key, nil
}

// ownsConnection reports whether pid is the backend of a connection on which
// this process already holds key.
func ownsConnection(pid int32, key int64) bool {
	if pid == 0 {
		return false
	}
	for _, h := range held {
		if h.key == key && h.conn.PgConn().PID() == uint32(pid) {
			return true
		}
	}
	return false
}
