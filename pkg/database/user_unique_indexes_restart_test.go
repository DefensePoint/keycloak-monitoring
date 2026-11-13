package database

import (
	"fmt"
	"strings"
	"testing"
)

// A unique index on users can go missing: an operator drops it by hand, or the
// process dies between the DROP and the CREATE of a rewrite. GORM's AutoMigrate
// then builds it over the whole column, which fails as soon as two accounts are
// blank in it, and every later start fails the same way. These tests state what
// a restart must do instead.

// blankUsers returns two live users who are both blank in column, the case a
// whole-column index rejects and the final index allows.
func blankUsers(column string) []*User {
	users := make([]*User, 0, 2)
	for i := 0; i < 2; i++ {
		u := &User{
			Subject:    fmt.Sprintf("uix-restart-%s-%d", column, i),
			Email:      fmt.Sprintf("uix-restart-%s-%d@example.invalid", column, i),
			Username:   fmt.Sprintf("uix-restart-%s-%d", column, i),
			AuthMethod: "oauth",
			IsActive:   true,
		}
		switch column {
		case "email":
			u.Email = ""
		case "subject":
			u.Subject = ""
			u.AuthMethod = "simple"
		case "username":
			u.Username = ""
		}
		users = append(users, u)
	}
	return users
}

func indexDef(t *testing.T, c *Client, name string) string {
	t.Helper()
	var def string
	if err := c.DB().Raw(`SELECT indexdef FROM pg_indexes WHERE indexname = ?`, name).Scan(&def).Error; err != nil {
		t.Fatalf("inspect %s: %v", name, err)
	}
	return def
}

func TestUserUniqueIndexes_ARestartRebuildsAMissingIndex(t *testing.T) {
	for _, idx := range userUniqueIndexes {
		t.Run(idx.column, func(t *testing.T) {
			c := newPurgeTestClient(t)
			for _, u := range blankUsers(idx.column) {
				mustCreate(t, c.DB(), u)
			}
			if err := c.DB().Exec("DROP INDEX " + idx.name).Error; err != nil {
				t.Fatalf("drop %s: %v", idx.name, err)
			}

			if err := c.AutoMigrate(); err != nil {
				t.Fatalf("restart with %s missing and two users blank in %s: %v", idx.name, idx.column, err)
			}
			if def := indexDef(t, c, idx.name); !isLiveOnly(def) {
				t.Errorf("%s after the restart = %q, want the live-only index", idx.name, def)
			}
		})
	}
}

// Only real duplicates stop a restart, and the error tells the operator how to
// find them.
func TestUserUniqueIndexes_ARestartNamesLiveDuplicates(t *testing.T) {
	c := newPurgeTestClient(t)
	if err := c.DB().Exec("DROP INDEX idx_users_email").Error; err != nil {
		t.Fatalf("drop idx_users_email: %v", err)
	}
	for i := 0; i < 2; i++ {
		mustCreate(t, c.DB(), &User{
			Subject: fmt.Sprintf("uix-dup-%d", i), Email: "uix-dup@example.invalid",
			AuthMethod: "oauth", IsActive: true,
		})
	}

	err := c.AutoMigrate()
	if err == nil {
		t.Fatal("restart with two live users sharing an email succeeded, want it refused")
	}
	for _, want := range []string{"idx_users_email", "users-index-preflight.sql"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("restart error = %q, want it to mention %q", err, want)
		}
	}
}

// failNextCreateIndex makes every CREATE INDEX fail until the test ends, the
// way a process killed between the DROP and the CREATE leaves things.
func failNextCreateIndex(t *testing.T, c *Client) {
	t.Helper()
	db := c.DB()
	for _, stmt := range []string{
		`CREATE OR REPLACE FUNCTION uix_fail_create_index() RETURNS event_trigger
		 LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'uix: create index refused'; END $$`,
		`CREATE EVENT TRIGGER uix_fail_create_index ON ddl_command_start
		 WHEN TAG IN ('CREATE INDEX') EXECUTE FUNCTION uix_fail_create_index()`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("install the failing trigger: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Exec(`DROP EVENT TRIGGER IF EXISTS uix_fail_create_index`)
		db.Exec(`DROP FUNCTION IF EXISTS uix_fail_create_index()`)
	})
}

func TestUserUniqueIndexes_AFailedRewriteKeepsTheOldIndex(t *testing.T) {
	c := newPurgeTestClient(t)
	plain := "CREATE UNIQUE INDEX idx_users_email ON users(email)"
	if err := c.DB().Exec("DROP INDEX idx_users_email").Exec(plain).Error; err != nil {
		t.Fatalf("set up the plain index: %v", err)
	}
	failNextCreateIndex(t, c)

	if err := c.migrateUserUniqueIndexes(); err == nil {
		t.Fatal("rewrite succeeded although CREATE INDEX was refused")
	}
	if def := indexDef(t, c, "idx_users_email"); def == "" {
		t.Error("the failed rewrite left idx_users_email missing, want the old index kept")
	}
}

func TestUsernameIndex_AFailedRewriteKeepsTheOldIndex(t *testing.T) {
	c := newPurgeTestClient(t)
	plain := "CREATE UNIQUE INDEX idx_users_username ON users(username)"
	if err := c.DB().Exec("DROP INDEX idx_users_username").Exec(plain).Error; err != nil {
		t.Fatalf("set up the plain index: %v", err)
	}
	failNextCreateIndex(t, c)

	if err := c.migrateUsernameIndex(); err == nil {
		t.Fatal("rewrite succeeded although CREATE INDEX was refused")
	}
	if def := indexDef(t, c, "idx_users_username"); def == "" {
		t.Error("the failed rewrite left idx_users_username missing, want the old index kept")
	}
}
