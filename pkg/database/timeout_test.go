package database

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

// database.timeout bounds opening a connection: it reaches the driver as
// connect_timeout, in whole seconds, rounded up so a sub-second value still
// sets a limit.
func TestConnectionString_CarriesTheConnectTimeout(t *testing.T) {
	base := config.DatabaseConfig{Host: "db", Port: 5432, Database: "monitoring", User: "u", Password: "p", SSLMode: "disable"}
	for _, tc := range []struct {
		timeout time.Duration
		want    string
	}{
		{30 * time.Second, "connect_timeout=30"},
		{1500 * time.Millisecond, "connect_timeout=2"},
		{200 * time.Millisecond, "connect_timeout=1"},
	} {
		cfg := base
		cfg.Timeout = tc.timeout
		if got := connectionString(&cfg); !strings.Contains(got, tc.want) {
			t.Errorf("timeout %v: connection string %q lacks %q", tc.timeout, got, tc.want)
		}
	}
	// mcp.database.timeout has no default, so an unset timeout must still
	// bound connecting, or startup can hang on a database that never answers.
	if got := connectionString(&base); !strings.Contains(got, "connect_timeout=10") {
		t.Errorf("no timeout configured: connection string %q lacks the 10s default", got)
	}
}

// A server that accepts the connection and never answers must not hold
// startup for longer than database.timeout.
func TestOpenClient_GivesUpAfterTheTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	cfg := &config.DatabaseConfig{Host: "127.0.0.1", Port: port, Database: "x", User: "x", Password: "x",
		SSLMode: "disable", MaxConns: 2, Timeout: time.Second}
	start := time.Now()
	client, err := openClient(cfg, logger.NewNoop())
	if err == nil {
		_ = client.Close()
		t.Fatal("connected to a server that never answers")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("gave up after %v, want about the 1s timeout", took)
	}
}

// database.timeout is not the connection lifetime: with a short timeout,
// connections stay open across many timeout periods instead of being closed
// and reopened, which also threw away their prepared statements.
func TestOpenClient_TimeoutIsNotTheConnectionLifetime(t *testing.T) {
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	cfg := testdb.DatabaseConfig(t, dsn)
	cfg.Timeout = 500 * time.Millisecond
	cfg.MaxConns = 2
	client, err := NewReadOnlyClient(cfg, logger.NewNoop())
	if err != nil {
		t.Fatalf("NewReadOnlyClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	sqlDB, err := client.DB().DB()
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if err := sqlDB.PingContext(context.Background()); err != nil {
			t.Fatalf("ping: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if closed := sqlDB.Stats().MaxLifetimeClosed; closed != 0 {
		t.Fatalf("%d connections closed for age within 2.5s of use; the 500ms timeout is being used as their lifetime", closed)
	}
}
