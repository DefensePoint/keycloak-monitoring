package testdb

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

func DatabaseConfig(t testing.TB, dsn string) *config.DatabaseConfig {
	t.Helper()
	if _, ok := dsnKeyword(dsn, "host"); !ok {
		t.Fatal("test DSN must set host")
	}
	if _, ok := dsnKeyword(dsn, "dbname"); !ok {
		t.Fatal("test DSN must set dbname")
	}
	parsed, err := pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test DSN: %v", err)
	}
	// An omitted sslmode means local Postgres with no TLS. Do not infer this
	// from parsed.TLSConfig: ParseConfig returns a non-nil config for an
	// omitted mode, for prefer, and for verify-full. Only disable is nil.
	sslMode := "disable"
	if mode, ok := dsnKeyword(dsn, "sslmode"); ok {
		sslMode = mode
	}
	return &config.DatabaseConfig{
		Host: parsed.Host, Port: int(parsed.Port), Database: parsed.Database,
		User: parsed.User, Password: parsed.Password, SSLMode: sslMode,
		MaxConns: 5, MinConns: 1, Timeout: 30 * time.Second,
	}
}

// isURLDSN reports whether dsn is in the postgres:// URL form rather than the
// keyword=value form. pgx accepts both, and so do the helpers here.
func isURLDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

// WithDSNParam returns dsn with key set to value, in whichever form dsn is
// written: a query parameter for a URL, a keyword=value pair otherwise.
func WithDSNParam(dsn, key, value string) string {
	if isURLDSN(dsn) {
		if u, err := url.Parse(dsn); err == nil {
			q := u.Query()
			q.Set(key, value)
			u.RawQuery = q.Encode()
			return u.String()
		}
	}
	return dsn + " " + key + "=" + value
}

// dsnKeyword reports the value dsn gives key, and whether it gives one at all.
// Only an explicit setting counts: ParseConfig would fill in defaults, and the
// guards here exist precisely so that nothing is inferred.
func dsnKeyword(dsn, key string) (string, bool) {
	if isURLDSN(dsn) {
		return urlDSNKeyword(dsn, key)
	}
	rest := dsn
	for rest != "" {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			break
		}
		eq := strings.IndexByte(rest, '=')
		if eq <= 0 {
			return "", false
		}
		k := rest[:eq]
		rest = rest[eq+1:]
		var val string
		if strings.HasPrefix(rest, "'") {
			end := 1
			for end < len(rest) {
				if rest[end] == '\\' && end+1 < len(rest) {
					end += 2
					continue
				}
				if rest[end] == '\'' {
					break
				}
				end++
			}
			if end >= len(rest) {
				return "", false
			}
			val = rest[1:end]
			rest = rest[end+1:]
		} else {
			sp := strings.IndexAny(rest, " \t")
			if sp < 0 {
				val, rest = rest, ""
			} else {
				val, rest = rest[:sp], rest[sp:]
			}
		}
		if k == key {
			return val, true
		}
	}
	return "", false
}

// urlDSNKeyword is dsnKeyword for the URL form, where host and dbname live in
// the authority and the path, and libpq also accepts either as a query
// parameter, which then wins.
func urlDSNKeyword(dsn, key string) (string, bool) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", false
	}
	if v := u.Query(); v.Has(key) {
		return v.Get(key), v.Get(key) != ""
	}
	switch key {
	case "host":
		h := u.Hostname()
		return h, h != ""
	case "dbname":
		db := strings.TrimPrefix(u.Path, "/")
		return db, db != ""
	}
	return "", false
}
