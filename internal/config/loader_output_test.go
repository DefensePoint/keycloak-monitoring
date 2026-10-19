package config

import (
	"bytes"
	"strings"
	"testing"
)

// Startup narration goes to stdout, and from there to wherever logs are
// shipped. It printed the first 30 lines of the config file as written, so
// any secret typed inline near the top was published on every start.
func TestLoadConfig_StartupOutputHoldsNoConfigValues(t *testing.T) {
	secrets := []string{
		"bG9hZGVyLXRlc3QtZW5jcnlwdGlvbi1rZXktMzJieXQ=",
		"loader-DB-SECRET-pw",
		"loader-SESSION-SECRET-0123456789abcdef0123456789",
		"loaderWEBHOOKsecret",
		"loader-KC-CLIENT-SECRET",
	}
	path := writeConfig(t, `security:
  encryption_key: "`+secrets[0]+`"
database:
  host: "127.0.0.1"
  password: "`+secrets[1]+`"
auth:
  session:
    secret: "`+secrets[2]+`"
notifications:
  slack:
    enabled: true
    webhook_url: "https://hooks.slack.invalid/services/T1/B1/`+secrets[3]+`"
keycloak:
  tenants:
    t1:
      name: "T1"
      server_url: "https://kc.example.com"
      client_id: "monitoring-service"
      client_secret: "`+secrets[4]+`"
`)

	var out bytes.Buffer
	if _, err := loadConfig(path, &out); err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	for _, secret := range secrets {
		if strings.Contains(out.String(), secret) {
			t.Errorf("startup output holds %q", secret)
		}
	}
	if !strings.Contains(out.String(), "Config file LOADED: "+path) {
		t.Errorf("startup output does not name the loaded config file:\n%s", out.String())
	}
}

// A shell-style default is a natural place to write a secret, and the warning
// for it is printed at startup and logged. It names the variable and the line,
// which is enough to find the mistake, and nothing after the name.
func TestLoadConfig_UnsupportedPlaceholderWarningLeavesOutTheDefault(t *testing.T) {
	path := writeConfig(t, "database:\n  password: \"${PMP_TEST_DB_PASS:-loader-DEFAULT-SECRET}\"\n")

	var out bytes.Buffer
	cfg, err := loadConfig(path, &out)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	if len(cfg.PlaceholderWarnings) != 1 {
		t.Fatalf("warnings = %q, want one", cfg.PlaceholderWarnings)
	}
	warning := cfg.PlaceholderWarnings[0]
	if !strings.Contains(warning, "PMP_TEST_DB_PASS") || !strings.Contains(warning, "(line 2)") {
		t.Errorf("warning %q does not name the variable and line 2", warning)
	}
	for _, where := range []string{warning, out.String()} {
		if strings.Contains(where, "loader-DEFAULT-SECRET") {
			t.Errorf("the default value is reported: %s", where)
		}
	}
}
