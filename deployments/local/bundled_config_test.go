package local

import (
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"testing"
	"time"
)

func TestBundledDemoConfigUsesEnvironmentSecrets(t *testing.T) {
	t.Setenv("BUNDLED_DB_PASSWORD", "db-$value")
	t.Setenv("BUNDLED_KC1_CLIENT_SECRET", "client-one-$value")
	t.Setenv("BUNDLED_KC2_CLIENT_SECRET", "client-two-$value")
	t.Setenv("BUNDLED_SESSION_SECRET", "session-$value")
	t.Setenv("MONITORING_AUTH_SIMPLE_DEFAULT_USER", "demo-operator")
	t.Setenv("MONITORING_AUTH_SIMPLE_DEFAULT_PASS", "Demo!Qx7nK2-2026")
	t.Setenv("MONITORING_AUTH_SIMPLE_DEFAULT_EMAIL", "demo@example.com")
	setAmfaEnv(t, "false")
	cfg, err := config.LoadConfigQuiet("config.bundled.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PlaceholderWarnings) != 0 {
		t.Fatalf("unresolved placeholders: %v", cfg.PlaceholderWarnings)
	}
	if cfg.Database.Host != "postgres" || cfg.Database.Password != "db-$value" {
		t.Fatal("demo database configuration did not resolve")
	}
	if cfg.Auth.Session.Secret != "session-$value" || cfg.Auth.Session.Secure {
		t.Fatal("demo session configuration did not resolve for local HTTP")
	}
	if cfg.Auth.Simple.DefaultUser != "demo-operator" || cfg.Auth.Simple.DefaultPass != "Demo!Qx7nK2-2026" {
		t.Fatal("demo admin bootstrap configuration did not resolve")
	}
	// The AMFA tenant is the third, there and disabled without AMFA=1.
	if len(cfg.Keycloak.Tenants) != 3 {
		t.Fatalf("expected three demo tenants, got %d", len(cfg.Keycloak.Tenants))
	}
	for id, want := range map[string]struct{ url, secret string }{
		"demo1": {"http://keycloak-1:8080", "client-one-$value"},
		"demo2": {"http://keycloak-2:8080", "client-two-$value"},
	} {
		tenant := cfg.Keycloak.Tenants[id]
		if !tenant.Enabled || tenant.ServerURL != want.url || tenant.ClientSecret != want.secret || tenant.ClientID != "monitoring-service" {
			t.Fatalf("tenant %s did not resolve its monitoring configuration", id)
		}
	}
	if !cfg.Keycloak.Global.Connection.AllowPrivateRanges {
		t.Fatal("demo must allow its internal Compose service addresses")
	}
	if cfg.Web.Server.Port != 7880 || cfg.Web.Server.APIHostURL != "http://api-server:7888" {
		t.Fatal("demo web proxy configuration is incorrect")
	}
	// The demo shows alerts: both checkers on, every check at its default, and
	// polled often enough that findings appear within a minute of startup.
	checks := cfg.Keycloak.Global
	if !checks.ConfigChecker.Enabled || checks.ConfigChecker.PollInterval != time.Minute {
		t.Fatal("demo configuration checks are not on with a one-minute poll")
	}
	if !checks.ConfigChecker.Checks.RealmSecurity.Enabled {
		t.Fatal("demo realm security check is not on")
	}
	if !checks.EventChecker.Enabled || checks.EventChecker.PollInterval != time.Minute {
		t.Fatal("demo event checks are not on with a one-minute poll")
	}
	if !checks.EventChecker.Checks.LoginError.Enabled {
		t.Fatal("demo login error check is not on")
	}
}

// setAmfaEnv sets what the Compose AMFA overlay gives the API server.
func setAmfaEnv(t *testing.T, on string) {
	t.Helper()
	t.Setenv("BUNDLED_DB_PASSWORD", "db-$value")
	t.Setenv("BUNDLED_KC1_CLIENT_SECRET", "client-one-$value")
	t.Setenv("BUNDLED_KC2_CLIENT_SECRET", "client-two-$value")
	t.Setenv("BUNDLED_SESSION_SECRET", "session-$value")
	t.Setenv("BUNDLED_AMFA", on)
	t.Setenv("BUNDLED_AMFA_KC_PORT", "18082")
	t.Setenv("BUNDLED_KC3_CLIENT_SECRET", "client-three-$value")
}

func TestBundledDemo_AmfaTenantFollowsTheSwitch(t *testing.T) {
	for _, on := range []bool{true, false} {
		setAmfaEnv(t, map[bool]string{true: "true", false: "false"}[on])
		cfg, err := config.LoadConfigQuiet("config.bundled.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.PlaceholderWarnings) != 0 {
			t.Fatalf("unresolved placeholders: %v", cfg.PlaceholderWarnings)
		}
		tenant := cfg.Keycloak.Tenants["demo3"]
		if tenant.Enabled != on || tenant.Amfa == nil || tenant.Amfa.Enabled != on {
			t.Fatalf("AMFA=%v: tenant enabled %v, amfa %+v", on, tenant.Enabled, tenant.Amfa)
		}
		if tenant.ServerURL != "http://keycloak-amfa:18082" || tenant.ClientSecret != "client-three-$value" {
			t.Fatalf("AMFA tenant connection did not resolve: %s", tenant.ServerURL)
		}
		// Over the engine's monitoring API, which listens on port 80 in the
		// AMFA Keycloak's network namespace; no database credentials.
		if tenant.Amfa.API.BaseURL != "http://keycloak-amfa" || tenant.Amfa.Database.Host != "" {
			t.Fatalf("AMFA tenant does not read the monitoring API: %+v", tenant.Amfa)
		}
	}
	global := func() config.KeycloakInstanceConfig {
		cfg, err := config.LoadConfigQuiet("config.bundled.yaml")
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Keycloak.Global
	}()
	// The seeded history is 30 days long; the mirror must look that far back on
	// its first run, or the Events table shows only new AMFA logins.
	if !global.AmfaMirror.Enabled || global.AmfaMirror.BackfillDays != 30 {
		t.Fatalf("AMFA mirror: %+v", global.AmfaMirror)
	}
	if !global.AmfaChecker.Enabled || !global.EventChecker.Checks.AmfaFallback.Enabled {
		t.Fatal("AMFA alert checks are not on")
	}
}
