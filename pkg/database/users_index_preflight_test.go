package database

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// deployments/postgres/users-index-preflight.sql tells an operator, before an
// upgrade, which unique indexes on users are missing and whether the
// statement it gives for one would fail. These tests hold the script to the
// migration it prepares for: the statements must be the ones the server
// itself would create, and the report must match what the server then does.

const preflightScriptPath = "../../deployments/postgres/users-index-preflight.sql"

func readPreflightScript(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(preflightScriptPath)
	if err != nil {
		t.Fatalf("read pre-flight script: %v", err)
	}
	return string(body)
}

// preflightCreateStatements returns the CREATE UNIQUE INDEX statements the
// script's header gives for a missing index, keyed by index name.
func preflightCreateStatements(t *testing.T) map[string]string {
	t.Helper()
	stmts := map[string]string{}
	var current []string
	for _, line := range strings.Split(readPreflightScript(t), "\n") {
		text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "--"))
		if strings.HasPrefix(text, "CREATE UNIQUE INDEX") {
			current = []string{text}
		} else if current != nil {
			current = append(current, text)
		}
		if current != nil && strings.HasSuffix(text, ";") {
			stmt := strings.TrimSuffix(strings.Join(current, " "), ";")
			stmts[strings.Fields(stmt)[3]] = stmt
			current = nil
		}
	}
	return stmts
}

// preflightQueries returns the script's executable statements: everything
// outside the comments.
func preflightQueries(t *testing.T) []string {
	t.Helper()
	var code []string
	for _, line := range strings.Split(readPreflightScript(t), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			code = append(code, line)
		}
	}
	var queries []string
	for _, q := range strings.Split(strings.Join(code, "\n"), ";") {
		if q = strings.TrimSpace(q); q != "" {
			queries = append(queries, q)
		}
	}
	if len(queries) != 3 {
		t.Fatalf("pre-flight script has %d queries, want the shape report, the duplicate count "+
			"and the duplicate list", len(queries))
	}
	return queries
}

type preflightReport struct {
	shapes     map[string]string
	duplicates map[string]int
	// groups lists, per column, the user ids of each set of live accounts
	// sharing a value, as the script prints them.
	groups map[string][]string
}

func runPreflight(t *testing.T, db *gorm.DB) preflightReport {
	t.Helper()
	queries := preflightQueries(t)
	report := preflightReport{shapes: map[string]string{}, duplicates: map[string]int{}, groups: map[string][]string{}}

	rows, err := db.Raw(queries[0]).Rows()
	if err != nil {
		t.Fatalf("shape report: %v", err)
	}
	for rows.Next() {
		var name, shape string
		if err := rows.Scan(&name, &shape); err != nil {
			t.Fatalf("scan shape: %v", err)
		}
		report.shapes[name] = shape
	}
	_ = rows.Close()

	rows, err = db.Raw(queries[1]).Rows()
	if err != nil {
		t.Fatalf("duplicate count: %v", err)
	}
	for rows.Next() {
		var column string
		var count int
		if err := rows.Scan(&column, &count); err != nil {
			t.Fatalf("scan duplicates: %v", err)
		}
		report.duplicates[column] = count
	}
	_ = rows.Close()

	rows, err = db.Raw(queries[2]).Rows()
	if err != nil {
		t.Fatalf("duplicate list: %v", err)
	}
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("duplicate list columns: %v", err)
	}
	if strings.Join(cols, ",") != "column_name,user_ids" {
		t.Fatalf("duplicate list columns = %v, want only column_name and user_ids, never the values", cols)
	}
	for rows.Next() {
		var column, ids string
		if err := rows.Scan(&column, &ids); err != nil {
			t.Fatalf("scan duplicate list: %v", err)
		}
		report.groups[column] = append(report.groups[column], ids)
	}
	_ = rows.Close()
	return report
}

func assertPreflight(t *testing.T, got preflightReport, shapes map[string]string, duplicates map[string]int) {
	t.Helper()
	for name, want := range shapes {
		if got.shapes[name] != want {
			t.Errorf("%s shape = %q, want %q (report %v)", name, got.shapes[name], want, got.shapes)
		}
	}
	for column, want := range duplicates {
		if got.duplicates[column] != want {
			t.Errorf("%s duplicated values = %d, want %d (report %v)", column, got.duplicates[column], want, got.duplicates)
		}
	}
}

// An operator who creates a missing index by hand must end up with exactly
// the index the server would have built, or the server rewrites it or
// refuses to start.
func TestUsersIndexPreflight_StatementsAreTheServersOwn(t *testing.T) {
	stmts := preflightCreateStatements(t)
	if len(stmts) != len(userUniqueIndexes) {
		t.Fatalf("script gives %d CREATE statements, the server has %d unique indexes on users", len(stmts), len(userUniqueIndexes))
	}
	for _, idx := range userUniqueIndexes {
		if got := strings.Join(strings.Fields(stmts[idx.name]), " "); got != idx.createSQL() {
			t.Errorf("script statement for %s:\n  %s\nserver creates:\n  %s", idx.name, got, idx.createSQL())
		}
	}
}

func TestUsersIndexPreflight_ReportMatchesWhatTheServerDoes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	dsn := testdb.DSN(t, "KMT_DESTRUCTIVE_TEST_DATABASE_DSN")
	testdb.RequireDisposable(t, dsn)
	testdb.Lock(t, dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	closeOnCleanup(t, db)
	// The script reports on the whole users table, so it needs one holding
	// only this test's rows.
	if err := db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	// It ends with an index missing and two live accounts sharing an
	// address, which the next test's AutoMigrate cannot build on.
	t.Cleanup(func() {
		if err := db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public").Error; err != nil {
			t.Errorf("reset schema after the test: %v", err)
		}
	})
	c := &Client{db: db, logger: logger.NewNoop()}
	all := func(shape string) map[string]string {
		return map[string]string{"idx_users_email": shape, "idx_users_subject": shape, "idx_users_username": shape}
	}
	none := map[string]int{"email": 0, "subject": 0, "username": 0}

	// An older release's AutoMigrate built the indexes over the bare column.
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("auto-migrate users: %v", err)
	}
	assertPreflight(t, runPreflight(t, db), all("plain"), none)

	if err := c.migrateUserUniqueIndexes(); err != nil {
		t.Fatalf("migrate unique indexes: %v", err)
	}
	assertPreflight(t, runPreflight(t, db), all("live-only"), none)

	// A missing index, on rows the final index allows: two accounts with no
	// email, and an address reused after its first account was deleted.
	if err := db.Exec("DROP INDEX idx_users_email").Error; err != nil {
		t.Fatalf("drop email index: %v", err)
	}
	mustCreate(t, db, &User{Subject: "pf-a", Email: "", AuthMethod: "oauth", IsActive: true})
	mustCreate(t, db, &User{Subject: "pf-b", Email: "", AuthMethod: "oauth", IsActive: true})
	deleted := &User{Subject: "pf-c", Email: "pf-shared@example.invalid", AuthMethod: "oauth", IsActive: true}
	mustCreate(t, db, deleted)
	if err := db.Delete(deleted).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	mustCreate(t, db, &User{Subject: "pf-d", Email: "pf-shared@example.invalid", AuthMethod: "oauth", IsActive: true})

	shapes := all("live-only")
	shapes["idx_users_email"] = "missing"
	assertPreflight(t, runPreflight(t, db), shapes, none)

	// GORM's AutoMigrate alone cannot start on that database...
	if err := db.AutoMigrate(&User{}); !isUniqueViolation(err) {
		t.Fatalf("auto-migrate with idx_users_email missing = %v, want a unique violation", err)
	}

	// ...so the server creates the missing index first, with its final
	// predicate, and AutoMigrate then leaves it alone.
	if err := c.createMissingUserUniqueIndexes(); err != nil {
		t.Fatalf("server creating the missing idx_users_email: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("auto-migrate after the server created the index: %v", err)
	}
	if err := c.migrateUserUniqueIndexes(); err != nil {
		t.Fatalf("migrate unique indexes after the script statement: %v", err)
	}
	if err := c.assertUserUniqueIndexesAreSafe(); err != nil {
		t.Fatalf("server refuses the index the script created: %v", err)
	}
	assertPreflight(t, runPreflight(t, db), all("live-only"), none)

	// Two live accounts sharing an address: the count says so, and the
	// statement fails until one of them is resolved.
	if err := db.Exec("DROP INDEX idx_users_email").Error; err != nil {
		t.Fatalf("drop email index: %v", err)
	}
	e := &User{Subject: "pf-e", Email: "pf-shared@example.invalid", AuthMethod: "oauth", IsActive: true}
	mustCreate(t, db, e)
	report := runPreflight(t, db)
	assertPreflight(t, report, shapes, map[string]int{"email": 1, "subject": 0, "username": 0})

	// The list names the two accounts by id, so the operator can resolve
	// them without the script printing the address.
	var d User
	if err := db.Where("subject = ?", "pf-d").First(&d).Error; err != nil {
		t.Fatalf("load pf-d: %v", err)
	}
	want := fmt.Sprintf("%d,%d", d.ID, e.ID)
	if got := report.groups["email"]; len(got) != 1 || got[0] != want {
		t.Errorf("email duplicate groups = %v, want [%s]", got, want)
	}
	if len(report.groups["subject"])+len(report.groups["username"]) != 0 {
		t.Errorf("duplicate groups = %v, want only the email one", report.groups)
	}

	// Neither the server nor the script's statement can create the index
	// until they are resolved.
	if err := c.createMissingUserUniqueIndexes(); !isUniqueViolation(err) {
		t.Fatalf("server with a live duplicate = %v, want a unique violation", err)
	}
	if err := db.Exec(preflightCreateStatements(t)["idx_users_email"]).Error; !isUniqueViolation(err) {
		t.Fatalf("script statement with a live duplicate = %v, want a unique violation", err)
	}
}
