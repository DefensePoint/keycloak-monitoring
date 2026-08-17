package config

import (
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
			RHBKFallbackURL:  "https://access.redhat.com",
			RHBKDocsBaseURL:  "https://docs.redhat.com/en/documentation/red_hat_build_of_keycloak",
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
}

func TestAmfaCheckerConfig_ApplyDefaults_PreservesExplicit(t *testing.T) {
	c := AmfaCheckerConfig{
		PollInterval: 30 * time.Second,
		Checks: AmfaCheckerChecksConfig{
			RepeatedRisky: RepeatedRiskyConfig{Threshold: 10, Window: time.Hour, MinRiskLevel: 4},
			LoginError:    ThresholdCheckConfig{Threshold: 99, Window: time.Minute},
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
}
