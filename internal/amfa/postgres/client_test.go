package postgres

import (
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
)

func TestBuildDSN_OpensTheSessionReadOnly(t *testing.T) {
	dsn := buildDSN(config.AmfaDatabaseConfig{
		Host: "primary.internal", Port: 5432, Database: "adaptive_mfa", User: "ro", Password: "pw",
	})
	if !strings.Contains(dsn, "options='-c default_transaction_read_only=on'") {
		t.Errorf("expected DSN to open the session read-only, got %q", dsn)
	}
}

func TestBuildDSN_PrefersReplicaWhenSet(t *testing.T) {
	cfg := config.AmfaDatabaseConfig{
		Host:            "primary.internal",
		Port:            5432,
		Database:        "adaptive_mfa",
		User:            "ro",
		Password:        "pw",
		SSLMode:         "require",
		ReadReplicaHost: "replica.internal",
	}
	dsn := buildDSN(cfg)
	if !strings.Contains(dsn, "host=replica.internal") {
		t.Errorf("expected DSN to contain replica host, got %q", dsn)
	}
	if strings.Contains(dsn, "host=primary.internal") {
		t.Errorf("expected DSN NOT to contain primary host when replica set, got %q", dsn)
	}
	if !strings.Contains(dsn, "dbname=adaptive_mfa") {
		t.Errorf("expected DSN to contain dbname, got %q", dsn)
	}
}

func TestBuildDSN_UsesPrimaryWhenNoReplica(t *testing.T) {
	cfg := config.AmfaDatabaseConfig{
		Host:     "primary.internal",
		Port:     5432,
		Database: "adaptive_mfa",
		User:     "ro",
		Password: "pw",
		SSLMode:  "disable",
	}
	dsn := buildDSN(cfg)
	if !strings.Contains(dsn, "host=primary.internal") {
		t.Errorf("expected DSN to contain primary host, got %q", dsn)
	}
	if !strings.Contains(dsn, "sslmode=disable") {
		t.Errorf("expected DSN to contain sslmode=disable, got %q", dsn)
	}
}

func TestBuildDSN_DefaultsSSLModeToRequire(t *testing.T) {
	cfg := config.AmfaDatabaseConfig{
		Host:     "primary.internal",
		Port:     5432,
		Database: "adaptive_mfa",
		User:     "ro",
		Password: "pw",
		// SSLMode intentionally left empty
	}
	dsn := buildDSN(cfg)
	if !strings.Contains(dsn, "sslmode=require") {
		t.Errorf("expected DSN to default sslmode to require, got %q", dsn)
	}
}

func TestHostUsed_PrefersReplica(t *testing.T) {
	cfg := config.AmfaDatabaseConfig{Host: "p", ReadReplicaHost: "r"}
	if got := hostUsed(cfg); got != "r" {
		t.Errorf("expected hostUsed=r, got %q", got)
	}
}

func TestHostUsed_FallsBackToPrimary(t *testing.T) {
	cfg := config.AmfaDatabaseConfig{Host: "p"}
	if got := hostUsed(cfg); got != "p" {
		t.Errorf("expected hostUsed=p, got %q", got)
	}
}

func TestNewClient_RejectsMissingHost(t *testing.T) {
	cfg := config.AmfaDatabaseConfig{Host: "", Database: "d", User: "u"}
	_, err := NewClient(cfg, nil)
	if err == nil {
		t.Errorf("expected error when Host is empty, got nil")
	}
}

func TestNewClient_RejectsMissingDatabase(t *testing.T) {
	cfg := config.AmfaDatabaseConfig{Host: "h", Database: "", User: "u"}
	_, err := NewClient(cfg, nil)
	if err == nil {
		t.Errorf("expected error when Database is empty, got nil")
	}
}

func TestNewClient_RejectsMissingUser(t *testing.T) {
	cfg := config.AmfaDatabaseConfig{Host: "h", Database: "d", User: ""}
	_, err := NewClient(cfg, nil)
	if err == nil {
		t.Errorf("expected error when User is empty, got nil")
	}
}
