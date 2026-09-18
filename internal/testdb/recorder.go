package testdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Statement is one SQL statement, with the arguments bound to it, as GORM
// handed it to the database driver.
type Statement struct {
	SQL  string
	Args []any
}

// ArgsAfter returns the arguments bound to the run of placeholders that
// follows marker, so a caller can assert not only that a predicate survived
// but that the intended values are bound to it. It returns nothing when marker
// is absent.
func (s Statement) ArgsAfter(marker string) []any {
	i := strings.Index(s.SQL, marker)
	if i < 0 {
		return nil
	}

	rest := s.SQL[i+len(marker):]
	var bound []any
	for {
		rest = strings.TrimLeft(rest, " (")
		if !strings.HasPrefix(rest, "$") {
			return bound
		}
		rest = rest[1:]

		digits := 0
		for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
			digits++
		}
		n, err := strconv.Atoi(rest[:digits])
		if err != nil || n < 1 || n > len(s.Args) {
			return bound
		}
		bound = append(bound, s.Args[n-1])

		rest = strings.TrimLeft(rest[digits:], " ")
		if !strings.HasPrefix(rest, ",") {
			return bound
		}
		rest = rest[1:]
	}
}

// Recorder collects the statements a repository generated.
type Recorder struct {
	mu         sync.Mutex
	statements []Statement
}

var recorderSeq atomic.Uint64

// NewRecorder returns a *gorm.DB speaking the PostgreSQL dialect whose
// statements are recorded and answered with no rows instead of reaching a
// server. Pass it to a repository constructor to exercise the real query
// builder in a test that CI can run without a database.
func NewRecorder(t testing.TB) (*gorm.DB, *Recorder) {
	t.Helper()

	rec := &Recorder{}
	driverName := fmt.Sprintf("pmp-testdb-recorder-%d", recorderSeq.Add(1))
	sql.Register(driverName, recorderDriver{rec: rec})

	db, err := gorm.Open(
		postgres.New(postgres.Config{DriverName: driverName, DSN: ""}),
		&gorm.Config{
			Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
			SkipDefaultTransaction: true,
		},
	)
	if err != nil {
		t.Fatalf("open recording database: %v", err)
	}

	return db, rec
}

// Statements returns everything recorded so far.
func (r *Recorder) Statements() []Statement {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Statement(nil), r.statements...)
}

// Against returns the recorded statements naming the given table.
func (r *Recorder) Against(table string) []Statement {
	quoted := `"` + table + `"`

	var matched []Statement
	for _, s := range r.Statements() {
		if strings.Contains(s.SQL, quoted) {
			matched = append(matched, s)
		}
	}
	return matched
}

// Reset drops everything recorded so far.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statements = nil
}

func (r *Recorder) record(query string, args []driver.NamedValue) {
	values := make([]any, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.statements = append(r.statements, Statement{SQL: query, Args: values})
}

type recorderDriver struct{ rec *Recorder }

func (d recorderDriver) Open(string) (driver.Conn, error) {
	return recorderConn(d), nil
}

type recorderConn struct{ rec *Recorder }

func (c recorderConn) Prepare(query string) (driver.Stmt, error) {
	return recorderStmt{rec: c.rec, query: query}, nil
}

func (c recorderConn) Close() error { return nil }

func (c recorderConn) Begin() (driver.Tx, error) { return recorderTx{}, nil }

// CheckNamedValue accepts every Go value unconverted, so a recorded argument
// is the one the repository passed rather than database/sql's conversion of it.
func (c recorderConn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (c recorderConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.rec.record(query, args)
	return emptyRows{}, nil
}

func (c recorderConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.rec.record(query, args)
	return driver.RowsAffected(0), nil
}

type recorderStmt struct {
	rec   *Recorder
	query string
}

func (s recorderStmt) Close() error  { return nil }
func (s recorderStmt) NumInput() int { return -1 }

func (s recorderStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.rec.record(s.query, namedValues(args))
	return driver.RowsAffected(0), nil
}

func (s recorderStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.rec.record(s.query, namedValues(args))
	return emptyRows{}, nil
}

func namedValues(args []driver.Value) []driver.NamedValue {
	named := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: arg}
	}
	return named
}

type recorderTx struct{}

func (recorderTx) Commit() error   { return nil }
func (recorderTx) Rollback() error { return nil }

type emptyRows struct{}

func (emptyRows) Columns() []string         { return nil }
func (emptyRows) Close() error              { return nil }
func (emptyRows) Next([]driver.Value) error { return io.EOF }
