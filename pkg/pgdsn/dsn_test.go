package pgdsn

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestFormat_DefaultsEmptySSLModeToRequire(t *testing.T) {
	dsn := Format("db.internal", 5432, "monitoring", "app", "pw", "")
	if !strings.Contains(dsn, "sslmode=require") {
		t.Errorf("empty sslmode: got %q, want sslmode=require", dsn)
	}
	if strings.Contains(dsn, "sslmode=") && strings.Contains(dsn, "sslmode= ") {
		t.Errorf("empty sslmode produced a blank sslmode value: %q", dsn)
	}
}

func TestFormat_PreservesExplicitDisable(t *testing.T) {
	dsn := Format("db.internal", 5432, "monitoring", "app", "pw", "disable")
	if !strings.Contains(dsn, "sslmode=disable") {
		t.Errorf("explicit disable: got %q", dsn)
	}
	if strings.Contains(dsn, "sslmode=require") {
		t.Errorf("explicit disable must not also set require: %q", dsn)
	}
}

func TestFormat_EmptyPasswordDoesNotSwallowSSLMode(t *testing.T) {
	dsn := Format("db.internal", 5432, "monitoring", "app", "", "")
	if !strings.Contains(dsn, "password=''") {
		t.Fatalf("empty password was not quoted: %q", dsn)
	}
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Password != "" {
		t.Errorf("password = %q, want empty", cfg.Password)
	}
	if cfg.Database != "monitoring" || cfg.User != "app" {
		t.Errorf("parsed database=%q user=%q", cfg.Database, cfg.User)
	}
	// require has no non-TLS fallback. prefer, the default when sslmode is
	// missing, has one.
	if len(cfg.Fallbacks) != 0 {
		t.Errorf("sslmode fell back (%d fallbacks); empty password swallowed the keyword", len(cfg.Fallbacks))
	}
}

func TestFormat_QuotesPasswordThatWouldBreakTheParser(t *testing.T) {
	const password = "a b'\\x"
	dsn := Format("db.internal", 5432, "monitoring", "app", password, "require")
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse: %v\ndsn: %s", err, dsn)
	}
	if cfg.Password != password {
		t.Errorf("password = %q, want %q", cfg.Password, password)
	}
}

func TestFormat_IncludesConnectionFields(t *testing.T) {
	dsn := Format("db.internal", 6543, "monitoring", "app", "s3cret", "verify-full")
	for _, want := range []string{
		"host=db.internal",
		"port=6543",
		"dbname=monitoring",
		"user=app",
		"password=s3cret",
		"sslmode=verify-full",
	} {
		if !strings.Contains(dsn, want) {
			t.Errorf("missing %q in %q", want, dsn)
		}
	}
}
