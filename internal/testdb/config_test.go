package testdb

import (
	"fmt"
	"testing"
)

func TestDatabaseConfig_KeepsTheSSLModeKeyword(t *testing.T) {
	cases := []string{"disable", "require", "prefer", "verify-ca", "verify-full"}
	for _, mode := range cases {
		t.Run(mode, func(t *testing.T) {
			dsn := "host=localhost port=5432 dbname=kmt_test user=kmt password=kmt sslmode=" + mode
			cfg := DatabaseConfig(t, dsn)
			if cfg.SSLMode != mode {
				t.Errorf("SSLMode = %q, want %q", cfg.SSLMode, mode)
			}
		})
	}
}

func TestDatabaseConfig_OmittedSSLModeIsDisable(t *testing.T) {
	cfg := DatabaseConfig(t, "host=localhost port=5432 dbname=kmt_test user=kmt password=kmt")
	if cfg.SSLMode != "disable" {
		t.Errorf("omitted sslmode = %q, want disable", cfg.SSLMode)
	}
}

func TestDatabaseConfig_RequiresHostAndDBName(t *testing.T) {
	for _, dsn := range []string{
		"port=5432 dbname=kmt_test user=kmt password=kmt",
		"host=localhost port=5432 user=kmt password=kmt",
	} {
		tb := &fatalTB{}
		func() {
			defer func() { _ = recover() }()
			DatabaseConfig(tb, dsn)
		}()
		if tb.msg == "" {
			t.Errorf("DSN %q did not fail", dsn)
		}
	}
}

type fatalTB struct {
	testing.TB
	msg string
}

func (f *fatalTB) Helper() {}

func (f *fatalTB) Fatalf(format string, args ...any) {
	f.msg = fmt.Sprintf(format, args...)
	panic("fatal")
}

func (f *fatalTB) Fatal(args ...any) {
	f.msg = fmt.Sprint(args...)
	panic("fatal")
}

// The URL form is as common as the keyword form, and pgx accepts both, so the
// suites must too.

func TestDatabaseConfig_URLForm(t *testing.T) {
	cfg := DatabaseConfig(t, "postgres://pmp:pmp@db.example:6543/kmt_test?sslmode=require")
	if cfg.Host != "db.example" || cfg.Port != 6543 || cfg.Database != "kmt_test" ||
		cfg.User != "pmp" || cfg.Password != "pmp" || cfg.SSLMode != "require" {
		t.Errorf("config = %+v", cfg)
	}
	if got := DatabaseConfig(t, "postgresql://pmp:pmp@localhost/kmt_test").SSLMode; got != "disable" {
		t.Errorf("omitted sslmode in a URL = %q, want disable", got)
	}
	// libpq lets a URL carry host and dbname as query parameters too.
	cfg = DatabaseConfig(t, "postgres://pmp:pmp@/?host=localhost&dbname=kmt_test")
	if cfg.Host != "localhost" || cfg.Database != "kmt_test" {
		t.Errorf("host and dbname from query parameters: %+v", cfg)
	}
}

func TestDatabaseConfig_URLFormRequiresHostAndDBName(t *testing.T) {
	for _, dsn := range []string{
		"postgres://pmp:pmp@/kmt_test",       // no host
		"postgres://pmp:pmp@localhost:5432",  // no dbname
		"postgres://pmp:pmp@localhost:5432/", // empty dbname
	} {
		tb := &fatalTB{}
		func() {
			defer func() { _ = recover() }()
			DatabaseConfig(tb, dsn)
		}()
		if tb.msg == "" {
			t.Errorf("DSN %q did not fail", dsn)
		}
	}
}

func TestWithDSNParam(t *testing.T) {
	for _, tc := range []struct{ dsn, want string }{
		{"host=localhost dbname=kmt_test", "host=localhost dbname=kmt_test application_name=x"},
		{"postgres://pmp@localhost/kmt_test", "postgres://pmp@localhost/kmt_test?application_name=x"},
		{"postgres://pmp@localhost/kmt_test?sslmode=disable", "postgres://pmp@localhost/kmt_test?application_name=x&sslmode=disable"},
	} {
		if got := WithDSNParam(tc.dsn, "application_name", "x"); got != tc.want {
			t.Errorf("WithDSNParam(%q) = %q, want %q", tc.dsn, got, tc.want)
		}
	}
}
