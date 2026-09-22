package config

import (
	"strings"
	"testing"
	"time"
)

func TestKeycloakConfig_Structure(t *testing.T) {
	cfg := KeycloakConfig{
		Global: KeycloakInstanceConfig{
			Realms: []string{"master", "myrealm"},
			Polling: KeycloakPollingConfig{
				MetricsInterval:   5 * time.Minute,
				EventsInterval:    30 * time.Second,
				HealthInterval:    1 * time.Minute,
				RealmInfoInterval: 15 * time.Minute,
			},
			Events: KeycloakEventsConfig{
				Types:            []string{"LOGIN", "LOGOUT"},
				MaxEventsPerPoll: 1000,
				LookbackDuration: 1 * time.Minute,
			},
			Connection: KeycloakConnectionConfig{
				Timeout:       30 * time.Second,
				SkipTLSVerify: false,
				MaxRetries:    3,
				RetryBackoff:  5 * time.Second,
			},
		},
		Tenants: map[string]TenantConfig{
			"prod-keycloak": {
				Name:         "Production Keycloak",
				Description:  "Main production instance",
				ServerURL:    "https://keycloak.example.com",
				AdminRealm:   "master",
				ClientID:     "monitoring-service",
				ClientSecret: "test-secret",
				Enabled:      true,
				Tags:         []string{"production"},
				Owner:        "platform-team",
			},
		},
	}

	// Test global settings
	if len(cfg.Global.Realms) != 2 {
		t.Errorf("Expected 2 realms, got %d", len(cfg.Global.Realms))
	}
	if cfg.Global.Polling.MetricsInterval != 5*time.Minute {
		t.Errorf("Expected metrics_interval 5m, got %v", cfg.Global.Polling.MetricsInterval)
	}

	// Test tenants map
	if len(cfg.Tenants) != 1 {
		t.Errorf("Expected 1 tenant, got %d", len(cfg.Tenants))
	}

	tenant, exists := cfg.Tenants["prod-keycloak"]
	if !exists {
		t.Fatal("Expected prod-keycloak tenant to exist")
	}

	if tenant.Name != "Production Keycloak" {
		t.Errorf("Expected name 'Production Keycloak', got %s", tenant.Name)
	}
	if tenant.ServerURL != "https://keycloak.example.com" {
		t.Errorf("Expected server_url 'https://keycloak.example.com', got %s", tenant.ServerURL)
	}
}

func TestTenantConfig_Defaults(t *testing.T) {
	tenant := TenantConfig{
		Name:      "Test Tenant",
		ServerURL: "https://keycloak.example.com",
		// ClientID, AdminRealm not set - should be set by sync logic
		Enabled: true,
	}

	// Verify fields that need defaults
	if tenant.ClientID != "" {
		t.Errorf("ClientID should be empty initially, got %s", tenant.ClientID)
	}
	if tenant.AdminRealm != "" {
		t.Errorf("AdminRealm should be empty initially, got %s", tenant.AdminRealm)
	}

	// Verify required fields
	if tenant.ServerURL == "" {
		t.Error("ServerURL should be set")
	}
}

func TestKeycloakInstanceConfig_AllSettings(t *testing.T) {
	globalCfg := KeycloakInstanceConfig{
		Realms: []string{"master", "app"},
		Polling: KeycloakPollingConfig{
			MetricsInterval:   10 * time.Minute,
			EventsInterval:    1 * time.Minute,
			HealthInterval:    2 * time.Minute,
			RealmInfoInterval: 30 * time.Minute,
		},
		Events: KeycloakEventsConfig{
			Types:            []string{"LOGIN", "LOGOUT", "REGISTER"},
			MaxEventsPerPoll: 500,
			LookbackDuration: 2 * time.Minute,
		},
		Connection: KeycloakConnectionConfig{
			Timeout:       60 * time.Second,
			SkipTLSVerify: true,
			MaxRetries:    5,
			RetryBackoff:  10 * time.Second,
		},
		ConfigChecker: ConfigCheckerConfig{
			PollInterval: 10 * time.Minute,
			Checks: ConfigCheckerChecks{
				IdentityProviderMetadata: IdentityProviderMetadataCheckConfig{
					Enabled:           true,
					CheckSAMLOnly:     true,
					ExcludedRealms:    []string{"test"},
					ExcludedProviders: []string{"legacy"},
				},
			},
		},
		VersionChecker: VersionCheckerConfig{
			CacheTTL:         24 * time.Hour,
			HTTPTimeout:      10 * time.Second,
			GitHubAPIBaseURL: "https://api.github.com",
			KeycloakOwner:    "keycloak",
			KeycloakRepo:     "keycloak",
		},
	}

	// Verify all global settings are accessible
	if len(globalCfg.Realms) != 2 {
		t.Errorf("Expected 2 realms, got %d", len(globalCfg.Realms))
	}
	if globalCfg.Polling.MetricsInterval != 10*time.Minute {
		t.Error("Polling settings not correctly set")
	}
	if len(globalCfg.Events.Types) != 3 {
		t.Errorf("Expected 3 event types, got %d", len(globalCfg.Events.Types))
	}
	if globalCfg.Connection.MaxRetries != 5 {
		t.Error("Connection settings not correctly set")
	}
	if !globalCfg.ConfigChecker.Checks.IdentityProviderMetadata.Enabled {
		t.Error("ConfigChecker settings not correctly set")
	}
	if globalCfg.VersionChecker.CacheTTL != 24*time.Hour {
		t.Error("VersionChecker settings not correctly set")
	}
}

func TestTenantConfig_AllFields(t *testing.T) {
	tenant := TenantConfig{
		Name:         "Full Tenant",
		Description:  "Tenant with all fields",
		ServerURL:    "https://keycloak.example.com",
		AdminRealm:   "master",
		ClientID:     "my-client",
		ClientSecret: "client-secret",
		Enabled:      true,
		Tags:         []string{"production", "critical"},
		Owner:        "platform-team",
	}

	// Verify all fields are accessible
	if tenant.Name != "Full Tenant" {
		t.Error("Name not correctly set")
	}
	if tenant.Description != "Tenant with all fields" {
		t.Error("Description not correctly set")
	}
	if tenant.ServerURL != "https://keycloak.example.com" {
		t.Error("ServerURL not correctly set")
	}
	if tenant.AdminRealm != "master" {
		t.Error("AdminRealm not correctly set")
	}
	if tenant.ClientID != "my-client" {
		t.Error("ClientID not correctly set")
	}
	if tenant.ClientSecret != "client-secret" {
		t.Error("ClientSecret not correctly set")
	}
	if !tenant.Enabled {
		t.Error("Enabled not correctly set")
	}
	if len(tenant.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(tenant.Tags))
	}
	if tenant.Owner != "platform-team" {
		t.Error("Owner not correctly set")
	}
}

func TestKeycloakConfig_MultipleTenants(t *testing.T) {
	cfg := KeycloakConfig{
		Global: KeycloakInstanceConfig{
			Realms: []string{"master"},
		},
		Tenants: map[string]TenantConfig{
			"tenant-1": {
				Name:      "Tenant 1",
				ServerURL: "https://kc1.example.com",
				Enabled:   true,
			},
			"tenant-2": {
				Name:      "Tenant 2",
				ServerURL: "https://kc2.example.com",
				Enabled:   false,
			},
			"tenant-3": {
				Name:      "Tenant 3",
				ServerURL: "https://kc3.example.com",
				Enabled:   true,
			},
		},
	}

	// Verify multiple tenants
	if len(cfg.Tenants) != 3 {
		t.Errorf("Expected 3 tenants, got %d", len(cfg.Tenants))
	}

	// Verify each tenant is accessible by ID
	tenant1, exists1 := cfg.Tenants["tenant-1"]
	tenant2, exists2 := cfg.Tenants["tenant-2"]
	tenant3, exists3 := cfg.Tenants["tenant-3"]

	if !exists1 || !exists2 || !exists3 {
		t.Error("Not all tenants are accessible")
	}

	if !tenant1.Enabled || tenant2.Enabled || !tenant3.Enabled {
		t.Error("Tenant enabled states are incorrect")
	}
}

func TestKeycloakConfig_EmptyTenants(t *testing.T) {
	cfg := KeycloakConfig{
		Global: KeycloakInstanceConfig{
			Realms: []string{"master"},
		},
		Tenants: map[string]TenantConfig{},
	}

	if cfg.Tenants == nil {
		t.Error("Tenants map should not be nil")
	}
	if len(cfg.Tenants) != 0 {
		t.Errorf("Expected 0 tenants, got %d", len(cfg.Tenants))
	}
}

func TestTenantConfig_AmfaIsNilByDefault(t *testing.T) {
	cfg := TenantConfig{}
	if cfg.Amfa != nil {
		t.Errorf("Expected Amfa to be nil by default, got %+v", cfg.Amfa)
	}
}

func TestAmfaTenantConfig_Validate_DisabledIsNoOp(t *testing.T) {
	amfa := &AmfaTenantConfig{Enabled: false}
	if err := amfa.Validate(); err != nil {
		t.Errorf("Expected no error for disabled config, got %v", err)
	}

	// No defaults should be populated when disabled
	if amfa.Database.Port != 0 {
		t.Errorf("Expected Port to remain 0 when disabled, got %d", amfa.Database.Port)
	}
	if amfa.Database.SSLMode != "" {
		t.Errorf("Expected SSLMode to remain empty when disabled, got %q", amfa.Database.SSLMode)
	}
	if amfa.Database.MaxConns != 0 {
		t.Errorf("Expected MaxConns to remain 0 when disabled, got %d", amfa.Database.MaxConns)
	}
	if amfa.EventsLookbackDays != 0 {
		t.Errorf("Expected EventsLookbackDays to remain 0 when disabled, got %d", amfa.EventsLookbackDays)
	}
}

func TestAmfaTenantConfig_Validate_AppliesDefaults(t *testing.T) {
	amfa := &AmfaTenantConfig{
		Enabled: true,
		Database: AmfaDatabaseConfig{
			Host:     "amfa.example.com",
			Database: "amfa_db",
			User:     "amfa_user",
		},
	}

	if err := amfa.Validate(); err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if amfa.Database.Port != 5432 {
		t.Errorf("Expected default Port 5432, got %d", amfa.Database.Port)
	}
	if amfa.Database.SSLMode != "require" {
		t.Errorf("Expected default SSLMode \"require\", got %q", amfa.Database.SSLMode)
	}
	if amfa.Database.MaxConns != 5 {
		t.Errorf("Expected default MaxConns 5, got %d", amfa.Database.MaxConns)
	}
	if amfa.Database.MinConns != 1 {
		t.Errorf("Expected default MinConns 1, got %d", amfa.Database.MinConns)
	}
	if amfa.Database.Timeout != 10*time.Second {
		t.Errorf("Expected default Timeout 10s, got %v", amfa.Database.Timeout)
	}
	if amfa.EventsLookbackDays != 30 {
		t.Errorf("Expected default EventsLookbackDays 30, got %d", amfa.EventsLookbackDays)
	}
}

func TestAmfaTenantConfig_Validate_RequiresHost(t *testing.T) {
	amfa := &AmfaTenantConfig{
		Enabled: true,
		Database: AmfaDatabaseConfig{
			Database: "amfa_db",
			User:     "amfa_user",
		},
	}

	if err := amfa.Validate(); err == nil {
		t.Error("Expected error for missing Host, got nil")
	}
}

func TestAmfaTenantConfig_Validate_RequiresDatabase(t *testing.T) {
	amfa := &AmfaTenantConfig{
		Enabled: true,
		Database: AmfaDatabaseConfig{
			Host: "amfa.example.com",
			User: "amfa_user",
		},
	}

	if err := amfa.Validate(); err == nil {
		t.Error("Expected error for missing Database, got nil")
	}
}

func TestAmfaTenantConfig_Validate_RequiresUser(t *testing.T) {
	amfa := &AmfaTenantConfig{
		Enabled: true,
		Database: AmfaDatabaseConfig{
			Host:     "amfa.example.com",
			Database: "amfa_db",
		},
	}

	if err := amfa.Validate(); err == nil {
		t.Error("Expected error for missing User, got nil")
	}
}

func TestAmfaTenantConfig_Validate_CapsLookbackAt90Days(t *testing.T) {
	amfa := &AmfaTenantConfig{
		Enabled:            true,
		EventsLookbackDays: 365,
		Database: AmfaDatabaseConfig{
			Host:     "amfa.example.com",
			Database: "amfa_db",
			User:     "amfa_user",
		},
	}

	if err := amfa.Validate(); err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if amfa.EventsLookbackDays != 90 {
		t.Errorf("Expected EventsLookbackDays to be clamped to 90, got %d", amfa.EventsLookbackDays)
	}
}

func TestAmfaCheckerConfig_ApplyDefaults(t *testing.T) {
	var c AmfaCheckerConfig
	c.ApplyDefaults()

	if c.PollInterval != time.Minute {
		t.Errorf("PollInterval default = %v, want 1m", c.PollInterval)
	}
	if c.Checks.RepeatedRisky.Threshold != 3 {
		t.Errorf("RepeatedRisky.Threshold default = %d, want 3", c.Checks.RepeatedRisky.Threshold)
	}
	if c.Checks.RepeatedRisky.Window != 24*time.Hour {
		t.Errorf("RepeatedRisky.Window default = %v, want 24h", c.Checks.RepeatedRisky.Window)
	}
	if c.Checks.RepeatedRisky.MinRiskLevel != 2 {
		t.Errorf("RepeatedRisky.MinRiskLevel default = %d, want 2", c.Checks.RepeatedRisky.MinRiskLevel)
	}
	if c.Checks.LoginError.Threshold != 5 {
		t.Errorf("LoginError.Threshold default = %d, want 5", c.Checks.LoginError.Threshold)
	}
	if c.Checks.LoginError.Window != 5*time.Minute {
		t.Errorf("LoginError.Window default = %v, want 5m", c.Checks.LoginError.Window)
	}
	if c.Checks.ClientLoginError.Threshold != 5 {
		t.Errorf("ClientLoginError.Threshold default = %d, want 5", c.Checks.ClientLoginError.Threshold)
	}
	if c.Checks.ClientLoginError.Window != 5*time.Minute {
		t.Errorf("ClientLoginError.Window default = %v, want 5m", c.Checks.ClientLoginError.Window)
	}
	if c.Checks.LoginErrorByClient.Threshold != 5 {
		t.Errorf("LoginErrorByClient.Threshold default = %d, want 5", c.Checks.LoginErrorByClient.Threshold)
	}
	if c.Checks.LoginErrorByClient.Window != 5*time.Minute {
		t.Errorf("LoginErrorByClient.Window default = %v, want 5m", c.Checks.LoginErrorByClient.Window)
	}
	if c.Checks.RealmRejectBurst.Threshold != 10 {
		t.Errorf("RealmRejectBurst.Threshold default = %d, want 10", c.Checks.RealmRejectBurst.Threshold)
	}
	if c.Checks.RealmRejectBurst.Window != 10*time.Minute {
		t.Errorf("RealmRejectBurst.Window default = %v, want 10m", c.Checks.RealmRejectBurst.Window)
	}
	if c.Checks.LoginErrorByAccount.Threshold != 5 {
		t.Errorf("LoginErrorByAccount.Threshold default = %d, want 5", c.Checks.LoginErrorByAccount.Threshold)
	}
	if c.Checks.LoginErrorByAccount.Window != 5*time.Minute {
		t.Errorf("LoginErrorByAccount.Window default = %v, want 5m", c.Checks.LoginErrorByAccount.Window)
	}
}

func TestAmfaCheckerConfig_ApplyDefaults_PreservesExplicit(t *testing.T) {
	c := AmfaCheckerConfig{
		PollInterval: 30 * time.Second,
		Checks: AmfaCheckerChecksConfig{
			RepeatedRisky:       RepeatedRiskyConfig{Threshold: 10, Window: time.Hour, MinRiskLevel: 4},
			LoginError:          ThresholdCheckConfig{Threshold: 99, Window: time.Minute},
			LoginErrorByClient:  ThresholdCheckConfig{Threshold: 42, Window: time.Minute},
			RealmRejectBurst:    ThresholdCheckConfig{Threshold: 42, Window: time.Minute},
			LoginErrorByAccount: ThresholdCheckConfig{Threshold: 42, Window: time.Minute},
		},
	}
	c.ApplyDefaults()

	if c.PollInterval != 30*time.Second {
		t.Errorf("PollInterval overwritten: got %v", c.PollInterval)
	}
	if c.Checks.RepeatedRisky.Threshold != 10 {
		t.Errorf("RepeatedRisky.Threshold overwritten: got %d", c.Checks.RepeatedRisky.Threshold)
	}
	if c.Checks.LoginError.Threshold != 99 {
		t.Errorf("LoginError.Threshold overwritten: got %d", c.Checks.LoginError.Threshold)
	}
	if c.Checks.LoginErrorByClient.Threshold != 42 {
		t.Errorf("LoginErrorByClient.Threshold overwritten: got %d", c.Checks.LoginErrorByClient.Threshold)
	}
	if c.Checks.RealmRejectBurst.Threshold != 42 {
		t.Errorf("RealmRejectBurst.Threshold overwritten: got %d", c.Checks.RealmRejectBurst.Threshold)
	}
	if c.Checks.LoginErrorByAccount.Threshold != 42 {
		t.Errorf("LoginErrorByAccount.Threshold overwritten: got %d", c.Checks.LoginErrorByAccount.Threshold)
	}
}

// --- AMFA API transport selection -----------------------------------------

func TestAmfaAPIConfigValidateAppliesDefaults(t *testing.T) {
	cfg := AmfaAPIConfig{BaseURL: "https://amfa.internal/"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// The trailing slash must go: paths are joined onto this, and "base//path"
	// is a different URL to the server.
	if cfg.BaseURL != "https://amfa.internal" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", cfg.BaseURL)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want a 30s default", cfg.Timeout)
	}
}

func TestAmfaAPIConfigRejectsUnusableURLs(t *testing.T) {
	// Checked at startup rather than at the first poll: each of these parses
	// cleanly enough to pass unnoticed, then fails on every request.
	for _, tc := range []struct {
		name    string
		baseURL string
	}{
		{"no scheme", "amfa.internal"},
		{"no host", "https://"},
		{"wrong scheme", "ftp://amfa.internal"},
		{"database dsn pasted by mistake", "postgres://user@host/db"},
	} {
		cfg := AmfaAPIConfig{BaseURL: tc.baseURL}
		if err := cfg.Validate(); err == nil {
			t.Errorf("%s (%q): expected rejection", tc.name, tc.baseURL)
		}
	}
}

func TestAmfaAPIConfiguredIgnoresWhitespace(t *testing.T) {
	// A commented-out or blanked base_url in YAML can arrive as spaces; that is
	// "not configured", not a URL.
	if (&AmfaAPIConfig{BaseURL: "   "}).Configured() {
		t.Error("whitespace-only base_url should not count as configured")
	}
	if (&AmfaAPIConfig{}).Configured() {
		t.Error("empty base_url should not count as configured")
	}
	if !(&AmfaAPIConfig{BaseURL: "https://amfa"}).Configured() {
		t.Error("a real base_url should count as configured")
	}
}

func TestAmfaTenantValidateAcceptsAPIWithoutDatabase(t *testing.T) {
	// The point of the API path: no database credentials in config at all.
	cfg := AmfaTenantConfig{
		Enabled: true,
		API:     AmfaAPIConfig{BaseURL: "https://amfa.internal"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("API-only config should validate: %v", err)
	}
	if cfg.EventsLookbackDays != 30 {
		t.Errorf("EventsLookbackDays = %d, want a 30 default", cfg.EventsLookbackDays)
	}
}

func TestAmfaTenantValidateStillAcceptsDatabaseOnly(t *testing.T) {
	// Deployments whose AMFA predates the API must keep working untouched.
	cfg := AmfaTenantConfig{
		Enabled: true,
		Database: AmfaDatabaseConfig{
			Host: "amfa-db", Database: "adaptive_mfa", User: "kmt_ro",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("database-only config should validate: %v", err)
	}
	if cfg.Database.Port != 5432 || cfg.Database.SSLMode != "require" {
		t.Errorf("database defaults not applied: %+v", cfg.Database)
	}
}

func TestAmfaTenantValidateRequiresOneTransport(t *testing.T) {
	cfg := AmfaTenantConfig{Enabled: true}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected an error when neither transport is configured")
	}
	// The message has to name both options, or an operator adding the API block
	// is told only that a database host is missing.
	if !strings.Contains(err.Error(), "api.base_url") || !strings.Contains(err.Error(), "database.host") {
		t.Errorf("error should name both transports, got: %v", err)
	}
}

func TestAmfaTenantValidateSkipsDatabaseChecksWhenAPIWins(t *testing.T) {
	// A half-filled database block left behind after migrating must not fail
	// validation, since it is no longer read.
	cfg := AmfaTenantConfig{
		Enabled:  true,
		API:      AmfaAPIConfig{BaseURL: "https://amfa.internal"},
		Database: AmfaDatabaseConfig{Host: "stale-host"}, // no database, no user
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a leftover database block should not block the API path: %v", err)
	}
}

func TestAmfaTenantValidateIsNoOpWhenDisabled(t *testing.T) {
	cfg := AmfaTenantConfig{Enabled: false}
	if err := cfg.Validate(); err != nil {
		t.Errorf("disabled AMFA should require nothing: %v", err)
	}
}

func TestAmfaLookbackIsCappedOnBothTransports(t *testing.T) {
	// Both paths must cap identically, or the same tenant would read a
	// different span of history depending on which transport it uses.
	api := AmfaTenantConfig{
		Enabled: true, EventsLookbackDays: 9999,
		API: AmfaAPIConfig{BaseURL: "https://amfa"},
	}
	db := AmfaTenantConfig{
		Enabled: true, EventsLookbackDays: 9999,
		Database: AmfaDatabaseConfig{Host: "h", Database: "d", User: "u"},
	}
	for name, cfg := range map[string]*AmfaTenantConfig{"api": &api, "database": &db} {
		if err := cfg.Validate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if cfg.EventsLookbackDays != 90 {
			t.Errorf("%s: lookback = %d, want the 90-day cap", name, cfg.EventsLookbackDays)
		}
	}
}
