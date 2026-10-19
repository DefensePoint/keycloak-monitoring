package fx

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// AMFA sslmode visibility
//
// AmfaTenantConfig.Validate fills an empty Database.SSLMode with "require",
// in place and without saying so. A deployment whose AMFA Postgres has TLS
// off then fails to connect with an error about TLS rather than about a
// default it never set. These tests pin the startup line that makes the
// resolved mode, and where it came from, visible before the connection is
// attempted.
//
// Each case builds its own config value. Validate mutates through the
// *AmfaTenantConfig pointer, so a config shared between cases would already
// carry "require" on the second pass and report source=config.

// captureLogs returns a logger writing JSON into buf.
func captureLogs(buf *bytes.Buffer) *logger.Logger {
	return logger.New(zerolog.New(buf))
}

// amfaDBTenant is an AMFA tenant on the database transport (no api.base_url).
// Host/Database/User are the fields Validate requires.
func amfaDBTenant(sslMode string) map[string]config.TenantConfig {
	return map[string]config.TenantConfig{
		"t1": {
			Name: "DB transport",
			Amfa: &config.AmfaTenantConfig{
				Enabled: true,
				Database: config.AmfaDatabaseConfig{
					Host:     "amfa.invalid",
					Database: "amfa",
					User:     "ro",
					SSLMode:  sslMode,
				},
			},
		},
	}
}

func TestBuildAmfaRegistry_LogsDefaultedSSLMode(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	// The tenant cannot connect (amfa.invalid does not resolve), which is the
	// point: the line must be emitted before the connection is attempted, or
	// it would be missing from exactly the failure it explains.
	buildAmfaRegistry(&config.AppConfig{}, amfaDBTenant(""), captureLogs(&buf))

	line := findLine(t, &buf, "AMFA database TLS mode resolved")
	if !strings.Contains(line, `"sslmode":"require"`) {
		t.Errorf("want sslmode=require, got %s", line)
	}
	if !strings.Contains(line, `"source":"default"`) {
		t.Errorf("want source=default, got %s", line)
	}
}

func TestBuildAmfaRegistry_LogsConfiguredSSLMode(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	buildAmfaRegistry(&config.AppConfig{}, amfaDBTenant("disable"), captureLogs(&buf))

	line := findLine(t, &buf, "AMFA database TLS mode resolved")
	if !strings.Contains(line, `"sslmode":"disable"`) {
		t.Errorf("want sslmode=disable, got %s", line)
	}
	if !strings.Contains(line, `"source":"config"`) {
		t.Errorf("want source=config, got %s", line)
	}
}

func TestBuildAmfaRegistry_NoSSLModeLineForAPITransport(t *testing.T) {
	t.Parallel()

	// An API-transport tenant opens no Postgres connection, so reporting a
	// TLS mode for it would describe a connection that is never made.
	tenants := amfaDBTenant("")
	tenants["t1"].Amfa.API = config.AmfaAPIConfig{BaseURL: "https://amfa.example.com"}

	var buf bytes.Buffer
	buildAmfaRegistry(&config.AppConfig{}, tenants, captureLogs(&buf))

	if strings.Contains(buf.String(), "AMFA database TLS mode resolved") {
		t.Errorf("API transport must not log a database TLS mode; got %s", buf.String())
	}
}

func findLine(t *testing.T, buf *bytes.Buffer, msg string) string {
	t.Helper()
	for _, l := range strings.Split(buf.String(), "\n") {
		if strings.Contains(l, msg) {
			return l
		}
	}
	t.Fatalf("no log line containing %q in:\n%s", msg, buf.String())
	return ""
}

// The legacy "sslmode" spelling is still accepted, and a tenant using it has
// configured a mode just as deliberately as one using "ssl_mode". Validate
// folds legacy into SSLMode, so reading only SSLMode before Validate would
// report source=default here.
func TestBuildAmfaRegistry_LegacyKeyCountsAsConfigured(t *testing.T) {
	t.Parallel()

	tenants := amfaDBTenant("")
	tenants["t1"].Amfa.Database.LegacySSLMode = "disable"

	var buf bytes.Buffer
	buildAmfaRegistry(&config.AppConfig{}, tenants, captureLogs(&buf))

	line := findLine(t, &buf, "AMFA database TLS mode resolved")
	if !strings.Contains(line, `"sslmode":"disable"`) {
		t.Errorf("want sslmode=disable, got %s", line)
	}
	if !strings.Contains(line, `"source":"config"`) {
		t.Errorf("want source=config for the legacy key, got %s", line)
	}
}
