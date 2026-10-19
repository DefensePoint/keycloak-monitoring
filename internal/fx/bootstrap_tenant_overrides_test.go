package fx

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

const tenantOverridesYAML = `
keycloak:
  global:
    realms: ["alpha", "beta"]
    connection:
      skip_tls_verify: true
    polling:
      metrics_interval: 2m
      events_interval: 15s
  tenants:
    prod:
      server_url: https://keycloak.example.com
      enabled: true
      config:
        default_realm: acme
        realms: ["acme", "ops"]
        connection:
          skip_tls_verify: false
        polling:
          events_interval: 10s
        events:
          types: ["LOGIN_ERROR"]
`

func loadTenantOverridesConfig(t *testing.T, body string) *config.AppConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadConfigQuiet(path)
	if err != nil {
		t.Fatalf("LoadConfigQuiet: %v", err)
	}
	return cfg
}

// A tenant's config: block in config.yaml was dropped at sync, apart from
// default_realm, so the tenant ran on the global settings with no warning.
func TestSyncTenant_YAMLOverridesReachTheTenantMonitor(t *testing.T) {
	cfg := loadTenantOverridesConfig(t, tenantOverridesYAML)
	svc := &stubTenantService{}
	if err := syncTenant(context.Background(), params(svc), "prod", cfg.Keycloak.Tenants["prod"]); err != nil {
		t.Fatalf("syncTenant: %v", err)
	}

	if svc.lastCreate.DefaultRealm != "acme" {
		t.Errorf("create DefaultRealm = %q, want acme", svc.lastCreate.DefaultRealm)
	}
	if svc.lastUpdate.Configuration == nil || *svc.lastUpdate.Configuration != svc.lastCreate.Configuration {
		t.Fatalf("update Configuration = %v, want the create request's %q", svc.lastUpdate.Configuration, svc.lastCreate.Configuration)
	}

	var stored map[string]any
	if err := json.Unmarshal([]byte(svc.lastCreate.Configuration), &stored); err != nil {
		t.Fatalf("stored Configuration %q is not JSON: %v", svc.lastCreate.Configuration, err)
	}
	if _, ok := stored["default_realm"]; ok {
		t.Errorf("stored Configuration repeats default_realm, which has its own column: %s", svc.lastCreate.Configuration)
	}

	var buf bytes.Buffer
	global := cfg.Keycloak.Global
	got := resolveTenantConfig(t, tenantConfigFactory(&buf), &global, svc.lastCreate.Configuration)

	if want := []string{"acme", "ops"}; !reflect.DeepEqual(got.Realms, want) {
		t.Errorf("Realms = %v, want %v", got.Realms, want)
	}
	if got.Connection.SkipTLSVerify {
		t.Error("SkipTLSVerify = true, want the tenant's explicit false")
	}
	if got.Polling.EventsInterval != 10*time.Second {
		t.Errorf("EventsInterval = %v, want 10s", got.Polling.EventsInterval)
	}
	if got.Polling.MetricsInterval != 2*time.Minute {
		t.Errorf("MetricsInterval = %v, want the global 2m", got.Polling.MetricsInterval)
	}
	if want := []string{"LOGIN_ERROR"}; !reflect.DeepEqual(got.Events.Types, want) {
		t.Errorf("Events.Types = %v, want %v", got.Events.Types, want)
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected warnings: %s", buf.String())
	}
}

// Some settings are only ever read from keycloak.global. Setting them for one
// tenant does nothing, which must be said rather than ignored.
func TestSyncTenant_WarnsAboutOverridesOnlyTheGlobalBlockHonours(t *testing.T) {
	cfg := loadTenantOverridesConfig(t, `
keycloak:
  tenants:
    prod:
      server_url: https://keycloak.example.com
      config:
        realms: ["acme"]
        infinispan_enabled: true
        config_checker:
          poll_interval: 15m
        connection:
          timeout: 60s
          allow_private_ranges: false
`)
	var buf bytes.Buffer
	p := params(&stubTenantService{})
	p.Logger = logger.New(zerolog.New(&buf))
	if err := syncTenant(context.Background(), p, "prod", cfg.Keycloak.Tenants["prod"]); err != nil {
		t.Fatalf("syncTenant: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"level":"warn"`) || !strings.Contains(out, `"tenant_id":"prod"`) {
		t.Fatalf("want a warning naming the tenant, got: %s", out)
	}
	if !strings.Contains(out, `"keys":["config_checker","connection.allow_private_ranges","infinispan_enabled"]`) {
		t.Errorf("want the ignored keys listed, got: %s", out)
	}
	for _, honoured := range []string{`"realms"`, `"connection.timeout"`} {
		if strings.Contains(out, honoured) {
			t.Errorf("warning lists %s, which does apply per tenant: %s", honoured, out)
		}
	}
}

func TestSyncTenant_NoWarningWhenEveryOverrideApplies(t *testing.T) {
	cfg := loadTenantOverridesConfig(t, tenantOverridesYAML)
	var buf bytes.Buffer
	p := params(&stubTenantService{})
	p.Logger = logger.New(zerolog.New(&buf))
	if err := syncTenant(context.Background(), p, "prod", cfg.Keycloak.Tenants["prod"]); err != nil {
		t.Fatalf("syncTenant: %v", err)
	}
	if strings.Contains(buf.String(), `"level":"warn"`) {
		t.Errorf("unexpected warning: %s", buf.String())
	}
}

// Removing a tenant's config: block must clear the override stored by an
// earlier sync, or the tenant keeps settings config.yaml no longer has.
func TestSyncTenant_NoYAMLOverridesClearsTheStoredOnes(t *testing.T) {
	svc := &stubTenantService{}
	if err := syncTenant(context.Background(), params(svc), "t1", testCfgTenant()); err != nil {
		t.Fatalf("syncTenant: %v", err)
	}
	if svc.lastCreate.Configuration != "" {
		t.Errorf("create Configuration = %q, want empty", svc.lastCreate.Configuration)
	}
	if svc.lastUpdate.Configuration == nil || *svc.lastUpdate.Configuration != "" {
		t.Errorf("update Configuration = %v, want a pointer to empty so the stored override is cleared", svc.lastUpdate.Configuration)
	}
}

// The block used to decode into typed fields, so a wrong type stopped
// startup. Keeping it raw must not lose that.
func TestLoadConfig_RejectsTenantOverrideOfTheWrongType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := `
keycloak:
  tenants:
    prod:
      server_url: https://keycloak.example.com
      config:
        polling:
          events_interval: soon
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := config.LoadConfigQuiet(path); err == nil {
		t.Fatal("LoadConfigQuiet accepted events_interval: soon")
	}
}
