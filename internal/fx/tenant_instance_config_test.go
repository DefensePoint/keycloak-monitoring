package fx

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

func globalInstanceConfig() *config.KeycloakInstanceConfig {
	return &config.KeycloakInstanceConfig{
		Realms:         []string{"alpha", "beta"},
		InfinispanPort: 11222,
		Polling: config.KeycloakPollingConfig{
			MetricsInterval:   2 * time.Minute,
			EventsInterval:    15 * time.Second,
			HealthInterval:    45 * time.Second,
			RealmInfoInterval: 20 * time.Minute,
		},
		Events: config.KeycloakEventsConfig{
			Types:            []string{"LOGIN", "LOGIN_ERROR"},
			MaxEventsPerPoll: 250,
			LookbackDuration: 2 * time.Hour,
		},
		Connection: config.KeycloakConnectionConfig{
			Timeout:       12 * time.Second,
			SkipTLSVerify: true,
			MaxRetries:    7,
			RetryBackoff:  3 * time.Second,
		},
	}
}

func tenantConfigFactory(buf *bytes.Buffer) *keycloakMonitorFactory {
	return &keycloakMonitorFactory{log: logger.New(zerolog.New(buf)), cfg: &config.AppConfig{}}
}

func resolveTenantConfig(t *testing.T, f *keycloakMonitorFactory, global *config.KeycloakInstanceConfig, configuration string) *config.KeycloakInstanceConfig {
	t.Helper()
	got, err := f.TenantInstanceConfig("t1", global, configuration)
	if err != nil {
		t.Fatalf("TenantInstanceConfig: %v", err)
	}
	kc, ok := got.(*config.KeycloakInstanceConfig)
	if !ok {
		t.Fatalf("got %T, want *config.KeycloakInstanceConfig: the factory falls back to built-in defaults for anything else", got)
	}
	return kc
}

// The reported bug: any per-tenant configuration replaced the tenant's whole
// Keycloak config with built-in defaults, global settings included.
func TestTenantInstanceConfig_KeepsGlobalSettingsTheOverrideDoesNotName(t *testing.T) {
	var buf bytes.Buffer
	got := resolveTenantConfig(t, tenantConfigFactory(&buf), globalInstanceConfig(), `{"polling":{"health_interval":"20s"}}`)

	want := globalInstanceConfig()
	want.Polling.HealthInterval = 20 * time.Second
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged config\n got: %+v\nwant: %+v", got, want)
	}
}

// Overrides use the config file's own keys and duration strings.
func TestTenantInstanceConfig_AppliesOverridesInConfigFileForm(t *testing.T) {
	var buf bytes.Buffer
	got := resolveTenantConfig(t, tenantConfigFactory(&buf), globalInstanceConfig(),
		`{"connection":{"skip_tls_verify":false,"timeout":"5s","max_retries":1},"events":{"max_events_per_poll":10}}`)

	if got.Connection.SkipTLSVerify || got.Connection.Timeout != 5*time.Second || got.Connection.MaxRetries != 1 {
		t.Errorf("connection = %+v, want skip_tls_verify false, timeout 5s, max_retries 1", got.Connection)
	}
	if got.Connection.RetryBackoff != 3*time.Second {
		t.Errorf("retry_backoff = %v, want the global 3s", got.Connection.RetryBackoff)
	}
	if got.Events.MaxEventsPerPoll != 10 || got.Events.LookbackDuration != 2*time.Hour {
		t.Errorf("events = %+v, want max_events_per_poll 10 and the global lookback", got.Events)
	}
}

// A list in the override is the tenant's list, not a patch over the global one.
func TestTenantInstanceConfig_ReplacesListsInsteadOfPatchingThem(t *testing.T) {
	var buf bytes.Buffer
	got := resolveTenantConfig(t, tenantConfigFactory(&buf), globalInstanceConfig(),
		`{"realms":["only"],"events":{"types":["ADMIN"]}}`)

	if !reflect.DeepEqual(got.Realms, []string{"only"}) {
		t.Errorf("realms = %v, want [only]", got.Realms)
	}
	if !reflect.DeepEqual(got.Events.Types, []string{"ADMIN"}) {
		t.Errorf("event types = %v, want [ADMIN]", got.Events.Types)
	}
}

// Every tenant starts from the same global config; one tenant's override must
// not change it for the others.
func TestTenantInstanceConfig_LeavesTheGlobalConfigUnchanged(t *testing.T) {
	var buf bytes.Buffer
	global := globalInstanceConfig()
	resolveTenantConfig(t, tenantConfigFactory(&buf), global,
		`{"realms":["only"],"events":{"types":["ADMIN"]},"connection":{"timeout":"1s"}}`)

	if !reflect.DeepEqual(global, globalInstanceConfig()) {
		t.Errorf("global config changed to %+v", global)
	}
}

func TestTenantInstanceConfig_RefusesInvalidJSON(t *testing.T) {
	var buf bytes.Buffer
	if _, err := tenantConfigFactory(&buf).TenantInstanceConfig("t1", globalInstanceConfig(), `{"polling":`); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

func TestTenantInstanceConfig_RefusesAWrongValueType(t *testing.T) {
	var buf bytes.Buffer
	if _, err := tenantConfigFactory(&buf).TenantInstanceConfig("t1", globalInstanceConfig(), `{"connection":{"timeout":"soon"}}`); err == nil {
		t.Fatal("expected an error for a timeout that is not a duration")
	}
}

// A misspelt key would otherwise be dropped as silently as the bug dropped
// everything.
func TestTenantInstanceConfig_ReportsUnknownKeys(t *testing.T) {
	var buf bytes.Buffer
	got := resolveTenantConfig(t, tenantConfigFactory(&buf), globalInstanceConfig(),
		`{"connection":{"skip_tls_verfy":false,"timeout":"5s"}}`)

	if got.Connection.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want the valid key applied", got.Connection.Timeout)
	}
	if out := buf.String(); !strings.Contains(out, "connection.skip_tls_verfy") || !strings.Contains(out, `"tenant_id":"t1"`) {
		t.Errorf("warning does not name the tenant and the unknown key: %s", out)
	}
}

// An empty override is no override.
func TestTenantInstanceConfig_EmptyObjectAndNullKeepTheGlobalConfig(t *testing.T) {
	for _, configuration := range []string{`{}`, `null`} {
		var buf bytes.Buffer
		got := resolveTenantConfig(t, tenantConfigFactory(&buf), globalInstanceConfig(), configuration)
		if !reflect.DeepEqual(got, globalInstanceConfig()) {
			t.Errorf("%s: got %+v, want the global config", configuration, got)
		}
	}
}

func TestTenantInstanceConfig_RefusesANonObject(t *testing.T) {
	var buf bytes.Buffer
	if _, err := tenantConfigFactory(&buf).TenantInstanceConfig("t1", globalInstanceConfig(), `["realms"]`); err == nil {
		t.Fatal("expected an error for a JSON array")
	}
}

// Keys match without regard to case, as they do in config.yaml.
func TestTenantInstanceConfig_MatchesKeysWithoutRegardToCase(t *testing.T) {
	var buf bytes.Buffer
	got := resolveTenantConfig(t, tenantConfigFactory(&buf), globalInstanceConfig(), `{"Connection":{"Timeout":"5s"}}`)
	if got.Connection.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", got.Connection.Timeout)
	}
	if strings.Contains(buf.String(), "unknown") {
		t.Errorf("mixed-case keys reported as unknown: %s", buf.String())
	}
}

// A tenant whose Keycloak has a longer transaction timeout overrides
// late_commit_window alone.
func TestTenantInstanceConfig_OverridesTheLateCommitWindow(t *testing.T) {
	var buf bytes.Buffer
	global := globalInstanceConfig()
	global.Events.LateCommitWindow = 5 * time.Minute

	got := resolveTenantConfig(t, tenantConfigFactory(&buf), global, `{"events": {"late_commit_window": "20m"}}`)

	if got.Events.LateCommitWindow != 20*time.Minute {
		t.Errorf("late_commit_window = %v, want 20m", got.Events.LateCommitWindow)
	}
	if got.Events.LookbackDuration != global.Events.LookbackDuration || global.Events.LateCommitWindow != 5*time.Minute {
		t.Error("the override changed another setting or the global config")
	}
}
