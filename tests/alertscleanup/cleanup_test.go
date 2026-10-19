package alertscleanup

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestCleanup_DeletesEventsAndHealthOnly(t *testing.T) {
	rec := &recorder{}
	db := openRecorder(t, rec)

	if err := Cleanup(db, "tenant-a", "test-abc-%"); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if len(rec.calls) != 2 {
		t.Fatalf("got %d statements, want events then health", len(rec.calls))
	}
	if !strings.Contains(rec.calls[0].query, "keycloak_events") || rec.calls[0].args[1] != "test-abc-%" {
		t.Errorf("events delete = %#v", rec.calls[0])
	}
	if !strings.Contains(rec.calls[1].query, "keycloak_health") || rec.calls[1].args[1] != HealthMarker {
		t.Errorf("health delete = %#v", rec.calls[1])
	}
	for _, c := range rec.calls {
		if strings.Contains(c.query, "configuration_alerts") {
			t.Errorf("cleanup must not delete alerts, ran %s", c.query)
		}
		upper := strings.ToUpper(c.query)
		if strings.Contains(upper, "INTERVAL") || strings.Contains(upper, "NOW()") {
			t.Errorf("cleanup still uses a time window: %s", c.query)
		}
	}
}

func TestCleanup_ReturnsExecError(t *testing.T) {
	rec := &recorder{failAt: 1}
	db := openRecorder(t, rec)

	err := Cleanup(db, "tenant-a", "test-abc-%")
	if err == nil {
		t.Fatal("expected an error from the events delete")
	}
	if len(rec.calls) != 1 {
		t.Fatalf("health delete ran after events failed: %d calls", len(rec.calls))
	}
}

type call struct {
	query string
	args  []any
}

type recorder struct {
	mu     sync.Mutex
	calls  []call
	failAt int
}

func openRecorder(t *testing.T, rec *recorder) *sql.DB {
	t.Helper()
	name := t.Name()
	recorders.Store(name, rec)
	t.Cleanup(func() { recorders.Delete(name) })
	db, err := sql.Open("alertscleanup-rec", name)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

var recorders sync.Map

func init() {
	sql.Register("alertscleanup-rec", recDriver{})
}

type recDriver struct{}

func (recDriver) Open(name string) (driver.Conn, error) {
	v, ok := recorders.Load(name)
	if !ok {
		return nil, errors.New("no recorder")
	}
	return &recConn{rec: v.(*recorder)}, nil
}

type recConn struct{ rec *recorder }

func (c *recConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *recConn) Close() error                        { return nil }
func (c *recConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (c *recConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.rec.mu.Lock()
	defer c.rec.mu.Unlock()
	got := make([]any, len(args))
	for i, a := range args {
		got[i] = a.Value
	}
	c.rec.calls = append(c.rec.calls, call{query: query, args: got})
	if c.rec.failAt > 0 && len(c.rec.calls) == c.rec.failAt {
		return nil, errors.New("exec failed")
	}
	return driver.ResultNoRows, nil
}
