package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// A tenant's client_secret written as ${VAR}, as config.yaml.example shows,
// used to reach Keycloak as the literal placeholder text.
func TestLoadConfig_ExpandsEnvPlaceholdersInValues(t *testing.T) {
	t.Setenv("PMP_TEST_KC_SECRET", "s3cr$t-with-$dollar")
	t.Setenv("PMP_TEST_DB_PORT", "6543")
	t.Setenv("PMP_TEST_DB_HOST", "db.internal")
	path := writeConfig(t, `
database:
  host: "${PMP_TEST_DB_HOST}"
  port: ${PMP_TEST_DB_PORT}
  password: "pa$$word-${PMP_TEST_KC_SECRET}-$${NOT_EXPANDED}"
keycloak:
  tenants:
    staging-keycloak:
      name: "Staging"
      server_url: "https://kc.example.com"
      client_id: "monitoring-service"
      client_secret: "${PMP_TEST_KC_SECRET}"
      # password: "${PMP_TEST_ONLY_IN_A_COMMENT}"
`)

	cfg, err := LoadConfigQuiet(path)
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	if cfg.Database.Host != "db.internal" {
		t.Errorf("database.host = %q, want db.internal", cfg.Database.Host)
	}
	if cfg.Database.Port != 6543 {
		t.Errorf("database.port = %d, want 6543 from an unquoted placeholder", cfg.Database.Port)
	}
	if want := "pa$$word-s3cr$t-with-$dollar-${NOT_EXPANDED}"; cfg.Database.Password != want {
		t.Errorf("database.password = %q, want %q (bare $ kept, $${} escaped)", cfg.Database.Password, want)
	}
	tenant, ok := cfg.Keycloak.Tenants["staging-keycloak"]
	if !ok {
		t.Fatalf("tenant staging-keycloak missing; tenants = %v", cfg.Keycloak.Tenants)
	}
	if tenant.ClientSecret != "s3cr$t-with-$dollar" {
		t.Errorf("client_secret = %q, want the environment value", tenant.ClientSecret)
	}
}

// An unset variable keeps the literal text, as before expansion existed, and
// is reported with its line rather than failing every service that shares
// the file.
func TestLoadConfig_UnsetPlaceholderKeepsLiteralAndWarns(t *testing.T) {
	t.Setenv("PMP_TEST_SET", "set-value")
	path := writeConfig(t, "database:\n  password: \"${PMP_TEST_SURELY_UNSET_VAR}\"\n  user: \"${PMP_TEST_SET}\"\n")
	cfg, err := LoadConfigQuiet(path)
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	if cfg.Database.Password != "${PMP_TEST_SURELY_UNSET_VAR}" {
		t.Errorf("password = %q, want the literal placeholder", cfg.Database.Password)
	}
	if cfg.Database.User != "set-value" {
		t.Errorf("user = %q, want the set variable still expanded", cfg.Database.User)
	}
	if len(cfg.PlaceholderWarnings) != 1 || !strings.Contains(cfg.PlaceholderWarnings[0], "PMP_TEST_SURELY_UNSET_VAR (line 2)") {
		t.Errorf("warnings = %q, want one naming the variable and line", cfg.PlaceholderWarnings)
	}
}

// A file without placeholders, or whose placeholders all expand, warns about
// nothing.
func TestLoadConfig_NoWarningsWhenEverythingExpands(t *testing.T) {
	t.Setenv("PMP_TEST_SET", "x")
	for _, body := range []string{"database:\n  host: plain\n", "database:\n  host: \"${PMP_TEST_SET}\"\n"} {
		cfg, err := LoadConfigQuiet(writeConfig(t, body))
		if err != nil {
			t.Fatalf("LoadConfigQuiet: %v", err)
		}
		if len(cfg.PlaceholderWarnings) != 0 {
			t.Errorf("%q: warnings = %q, want none", body, cfg.PlaceholderWarnings)
		}
	}
}

func TestLoadConfig_EmptyPlaceholderIsAllowed(t *testing.T) {
	t.Setenv("PMP_TEST_EMPTY", "")
	path := writeConfig(t, "database:\n  password: \"${PMP_TEST_EMPTY}\"\n")
	cfg, err := LoadConfigQuiet(path)
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	if cfg.Database.Password != "" {
		t.Errorf("password = %q, want empty", cfg.Database.Password)
	}
}

// Environment overrides must still win over a value that came from a
// placeholder, as they do over any other file value.
func TestLoadConfig_EnvOverrideStillWinsOverExpandedValue(t *testing.T) {
	t.Setenv("PMP_TEST_DB_HOST", "from-placeholder")
	t.Setenv("MONITORING_DATABASE_HOST", "from-override")
	path := writeConfig(t, "database:\n  host: \"${PMP_TEST_DB_HOST}\"\n")
	cfg, err := LoadConfigQuiet(path)
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	if cfg.Database.Host != "from-override" {
		t.Errorf("database.host = %q, want the MONITORING_ override", cfg.Database.Host)
	}
}

// The shipped example must load once its referenced variables are set.
func TestLoadConfig_ShippedExampleLoads(t *testing.T) {
	t.Setenv("KC_STAGING_CLIENT_SECRET", "example-secret")
	t.Setenv("PROD_KEYCLOAK_CLIENT_SECRET", "example-secret")
	example, err := os.ReadFile(filepath.Join("..", "..", "config.yaml.example"))
	if err != nil {
		t.Fatalf("read config.yaml.example: %v", err)
	}
	cfg, err := LoadConfigQuiet(writeConfig(t, string(example)))
	if err != nil {
		t.Fatalf("config.yaml.example: %v", err)
	}
	for id, tenant := range cfg.Keycloak.Tenants {
		if strings.Contains(tenant.ClientSecret, "${") {
			t.Errorf("tenant %s client_secret still holds a placeholder: %q", id, tenant.ClientSecret)
		}
	}
}

// An unquoted placeholder must not have its value re-typed: 0123 read as
// octal became 83, true became 1, and an empty or null value fell back to
// the default password.
func TestLoadConfig_UnquotedPlaceholderKeepsTheValueAsWritten(t *testing.T) {
	for _, secret := range []string{"0123", "true", "1e3", "0x1F", "null", "~", ""} {
		t.Setenv("PMP_TEST_SECRET", secret)
		cfg, err := LoadConfigQuiet(writeConfig(t, "database:\n  password: ${PMP_TEST_SECRET}\n"))
		if err != nil {
			t.Fatalf("%q: LoadConfigQuiet: %v", secret, err)
		}
		if cfg.Database.Password != secret {
			t.Errorf("password from %q = %q, want it unchanged", secret, cfg.Database.Password)
		}
	}
}

// Numbers, booleans and durations still decode from an expanded string.
func TestLoadConfig_TypedFieldsDecodeFromPlaceholders(t *testing.T) {
	t.Setenv("PMP_TEST_PORT", "6543")
	t.Setenv("PMP_TEST_SKIP", "true")
	t.Setenv("PMP_TEST_TIMEOUT", "45s")
	cfg, err := LoadConfigQuiet(writeConfig(t, `
database:
  port: ${PMP_TEST_PORT}
keycloak:
  global:
    connection:
      skip_tls_verify: ${PMP_TEST_SKIP}
      timeout: "${PMP_TEST_TIMEOUT}"
`))
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	if cfg.Database.Port != 6543 {
		t.Errorf("port = %d, want 6543", cfg.Database.Port)
	}
	if !cfg.Keycloak.Global.Connection.SkipTLSVerify {
		t.Error("skip_tls_verify = false, want true")
	}
	if cfg.Keycloak.Global.Connection.Timeout != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", cfg.Keycloak.Global.Connection.Timeout)
	}
}

// Shell forms are not supported: they keep their literal text and are
// reported, so the mistake is visible instead of silently sent on. The report
// shows the variable name and cuts what follows it, which may be a secret.
func TestLoadConfig_UnsupportedPlaceholderSyntaxWarns(t *testing.T) {
	t.Setenv("PMP_TEST_SET", "x")
	for value, reported := range map[string]string{
		"${PMP_TEST_SET:-fallback}": "${PMP_TEST_SET…}",
		"${}":                       "${}",
		"${1BAD}":                   "${1BAD}",
		"${A B}":                    "${A…}",
	} {
		cfg, err := LoadConfigQuiet(writeConfig(t, "database:\n  password: \""+value+"\"\n"))
		if err != nil {
			t.Fatalf("%s: LoadConfigQuiet: %v", value, err)
		}
		if cfg.Database.Password != value {
			t.Errorf("%s: password = %q, want the literal text", value, cfg.Database.Password)
		}
		if len(cfg.PlaceholderWarnings) != 1 || !strings.Contains(cfg.PlaceholderWarnings[0], "unsupported placeholders") || !strings.Contains(cfg.PlaceholderWarnings[0], reported+" (line 2)") {
			t.Errorf("%s: warnings = %q, want one reporting %s and line 2", value, cfg.PlaceholderWarnings, reported)
		}
	}
}

// Lists and anchored values are expanded too, and an alias sees the
// expanded value of its anchor.
func TestLoadConfig_ExpandsInListsAndAnchors(t *testing.T) {
	t.Setenv("PMP_TEST_ORIGIN", "10.0.0.9")
	t.Setenv("PMP_TEST_SHARED", "shared-secret")
	cfg, err := LoadConfigQuiet(writeConfig(t, `
http:
  trusted_proxies: ["${PMP_TEST_ORIGIN}", "10.0.0.1"]
keycloak:
  tenants:
    one:
      name: one
      server_url: "https://one.example.com"
      client_id: monitoring-service
      client_secret: &secret "${PMP_TEST_SHARED}"
    two:
      name: two
      server_url: "https://two.example.com"
      client_id: monitoring-service
      client_secret: *secret
`))
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	for _, id := range []string{"one", "two"} {
		if got := cfg.Keycloak.Tenants[id].ClientSecret; got != "shared-secret" {
			t.Errorf("tenant %s client_secret = %q, want shared-secret", id, got)
		}
	}
	if proxies := cfg.HTTP.TrustedProxies; len(proxies) != 2 || proxies[0] != "10.0.0.9" || proxies[1] != "10.0.0.1" {
		t.Errorf("trusted_proxies = %v, want [10.0.0.9 10.0.0.1]", proxies)
	}
}

// Expanding re-encodes the whole document, so every other value must come
// out exactly as it would have without a placeholder. Each case loads the
// same document twice, once with the value written literally and once through
// a placeholder, and compares what viper reads.
func TestExpandEnvPlaceholders_ReencodingChangesNothingElse(t *testing.T) {
	const secret = `kc-$ecret "quoted" 'single' #hash: colon`
	t.Setenv("PMP_TEST_ROUNDTRIP", secret)
	example, err := os.ReadFile(filepath.Join("..", "..", "config.yaml.example"))
	if err != nil {
		t.Fatalf("read config.yaml.example: %v", err)
	}
	hard := `
defaults: &defaults
  timeout: 30s
  retries: 3
  enabled: on
service:
  <<: *defaults
  retries: 5
  secret: "${PMP_TEST_ROUNDTRIP}"
scalars:
  octal_like: 0123
  quoted_number: "0123"
  yes_word: yes
  on_word: on
  null_value: null
  tilde: ~
  empty:
  timestamp: 2026-09-29T10:00:00Z
  date: 2026-09-29
  float: 1.50
  hex: 0x1F
  big: 123456789012345678901234567890
  unicode: "héllo ✓ 日本"
  special: "a: b # not a comment"
  single: 'it''s'
  dollar: "cost $5 and $${ESCAPED}"
  literal_block: |
    line one
      indented ${NOT_A_PLACEHOLDER_IN_BLOCK_IS_STILL_A_VALUE}
    line three
  folded_block: >
    folded
    text
list:
  - one
  - {name: two, value: 2}
  - [3, "three"]
`
	for name, doc := range map[string]string{"config.yaml.example": string(example), "hard": hard} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("NOT_A_PLACEHOLDER_IN_BLOCK_IS_STILL_A_VALUE", "block-value")
			t.Setenv("KC_STAGING_CLIENT_SECRET", secret)
			t.Setenv("PROD_KEYCLOAK_CLIENT_SECRET", secret)
			literal := strings.NewReplacer(
				`"${PMP_TEST_ROUNDTRIP}"`, yamlQuote(secret),
				`"${KC_STAGING_CLIENT_SECRET}"`, yamlQuote(secret),
				`"${PROD_KEYCLOAK_CLIENT_SECRET}"`, yamlQuote(secret),
				`${NOT_A_PLACEHOLDER_IN_BLOCK_IS_STILL_A_VALUE}`, "block-value",
				`$${ESCAPED}`, "${ESCAPED}",
			).Replace(doc)
			expanded, warnings, err := expandEnvPlaceholders([]byte(doc))
			if err != nil || len(warnings) != 0 {
				t.Fatalf("expand: %v %v", err, warnings)
			}
			if string(expanded) == doc {
				t.Fatal("document was not re-encoded, so this case proves nothing")
			}
			if got, want := viperSettings(t, expanded), viperSettings(t, []byte(literal)); !reflect.DeepEqual(got, want) {
				t.Errorf("settings differ after re-encoding\n got: %v\nwant: %v", got, want)
			}
		})
	}
}

func yamlQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func viperSettings(t *testing.T, doc []byte) map[string]any {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader(doc)); err != nil {
		t.Fatalf("viper read: %v\n%s", err, doc)
	}
	return v.AllSettings()
}
