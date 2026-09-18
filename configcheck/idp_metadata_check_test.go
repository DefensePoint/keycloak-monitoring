package configcheck

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

func TestNewIdentityProviderMetadataCheck(t *testing.T) {
	client := &testKeycloakClient{}
	log := logger.NewNoop()
	realms := []string{"master", "test"}
	tenantID := "test-tenant"
	certWarningDays := 30
	certCheckTimeout := 30 * time.Second

	check := NewIdentityProviderMetadataCheck(client, log, realms, tenantID, certWarningDays, certCheckTimeout)

	if check == nil {
		t.Fatal("NewIdentityProviderMetadataCheck returned nil")
		return
	}
	if check.keycloakClient != client {
		t.Error("keycloakClient not set correctly")
	}
	if check.logger == nil {
		t.Error("logger not set")
	}
	if len(check.realms) != len(realms) {
		t.Errorf("expected %d realms, got %d", len(realms), len(check.realms))
	}
	if check.tenantID != tenantID {
		t.Errorf("expected tenant ID %s, got %s", tenantID, check.tenantID)
	}
	if check.certWarningDays != certWarningDays {
		t.Errorf("expected cert warning days %d, got %d", certWarningDays, check.certWarningDays)
	}
	if check.metadataFetcher == nil {
		t.Error("metadata fetcher not initialized")
	}
	if !check.checkCertificates {
		t.Error("certificate checking should be enabled by default")
	}
}

func TestIdentityProviderMetadataCheck_GetCheckType(t *testing.T) {
	check := &IdentityProviderMetadataCheck{}
	checkType := check.GetCheckType()

	if checkType != "identity_provider_metadata" {
		t.Errorf("expected check type 'identity_provider_metadata', got %s", checkType)
	}
}

func TestIdentityProviderMetadataCheck_GetDescription(t *testing.T) {
	check := &IdentityProviderMetadataCheck{}
	description := check.GetDescription()

	if description == "" {
		t.Error("GetDescription returned empty string")
	}
	if len(description) < 10 {
		t.Error("Description too short")
	}
}

func TestIdentityProviderMetadataCheck_Execute(t *testing.T) {
	tests := []struct {
		name              string
		realms            []*keycloakadmin.RealmRepresentation
		identityProviders map[string][]*keycloakadmin.IdentityProviderRepresentation
		configuredRealms  []string
		expectedAlerts    int
		expectError       bool
	}{
		{
			name: "SAML provider with metadata URL - no alert",
			realms: []*keycloakadmin.RealmRepresentation{
				{Realm: "master", Enabled: true},
			},
			identityProviders: map[string][]*keycloakadmin.IdentityProviderRepresentation{
				"master": {
					{
						Alias:      "saml-provider",
						ProviderID: "saml",
						Config: map[string]string{
							"metadataDescriptorUrl": "https://idp.example.com/metadata",
						},
					},
				},
			},
			configuredRealms: []string{},
			expectedAlerts:   0,
			expectError:      false,
		},
		{
			name: "SAML provider without metadata URL - creates alert",
			realms: []*keycloakadmin.RealmRepresentation{
				{Realm: "master", Enabled: true},
			},
			identityProviders: map[string][]*keycloakadmin.IdentityProviderRepresentation{
				"master": {
					{
						Alias:       "saml-provider",
						DisplayName: "SAML Provider",
						InternalID:  "id-123",
						ProviderID:  "saml",
						Enabled:     true,
						Config:      map[string]string{},
					},
				},
			},
			configuredRealms: []string{},
			expectedAlerts:   1,
			expectError:      false,
		},
		{
			name: "SAML provider with empty metadata URL - creates alert",
			realms: []*keycloakadmin.RealmRepresentation{
				{Realm: "master", Enabled: true},
			},
			identityProviders: map[string][]*keycloakadmin.IdentityProviderRepresentation{
				"master": {
					{
						Alias:       "saml-provider",
						DisplayName: "SAML Provider",
						InternalID:  "id-123",
						ProviderID:  "saml",
						Config: map[string]string{
							"metadataDescriptorUrl": "",
						},
					},
				},
			},
			configuredRealms: []string{},
			expectedAlerts:   1,
			expectError:      false,
		},
		{
			name: "OIDC provider - no alert (not SAML)",
			realms: []*keycloakadmin.RealmRepresentation{
				{Realm: "master", Enabled: true},
			},
			identityProviders: map[string][]*keycloakadmin.IdentityProviderRepresentation{
				"master": {
					{
						Alias:      "oidc-provider",
						ProviderID: "oidc",
						Config:     map[string]string{},
					},
				},
			},
			configuredRealms: []string{},
			expectedAlerts:   0,
			expectError:      false,
		},
		{
			name: "Multiple realms with mixed providers",
			realms: []*keycloakadmin.RealmRepresentation{
				{Realm: "master", Enabled: true},
				{Realm: "test", Enabled: true},
			},
			identityProviders: map[string][]*keycloakadmin.IdentityProviderRepresentation{
				"master": {
					{
						Alias:       "saml-1",
						DisplayName: "SAML 1",
						InternalID:  "id-1",
						ProviderID:  "saml",
						Config:      map[string]string{},
					},
					{
						Alias:      "oidc-1",
						ProviderID: "oidc",
						Config:     map[string]string{},
					},
				},
				"test": {
					{
						Alias:       "saml-2",
						DisplayName: "SAML 2",
						InternalID:  "id-2",
						ProviderID:  "saml",
						Config: map[string]string{
							"metadataDescriptorUrl": "",
						},
					},
				},
			},
			configuredRealms: []string{},
			expectedAlerts:   2,
			expectError:      false,
		},
		{
			name: "Disabled realm - not checked",
			realms: []*keycloakadmin.RealmRepresentation{
				{Realm: "disabled", Enabled: false},
			},
			identityProviders: map[string][]*keycloakadmin.IdentityProviderRepresentation{
				"disabled": {
					{
						Alias:       "saml-disabled",
						DisplayName: "Disabled SAML",
						InternalID:  "id-disabled",
						ProviderID:  "saml",
						Config:      map[string]string{},
					},
				},
			},
			configuredRealms: []string{},
			expectedAlerts:   0,
			expectError:      false,
		},
		{
			name: "Specific realms configured",
			realms: []*keycloakadmin.RealmRepresentation{
				{Realm: "master", Enabled: true},
				{Realm: "test", Enabled: true},
			},
			identityProviders: map[string][]*keycloakadmin.IdentityProviderRepresentation{
				"master": {
					{
						Alias:       "saml-master",
						DisplayName: "SAML Master",
						InternalID:  "id-master",
						ProviderID:  "saml",
						Config:      map[string]string{},
					},
				},
				"test": {
					{
						Alias:       "saml-test",
						DisplayName: "SAML Test",
						InternalID:  "id-test",
						ProviderID:  "saml",
						Config:      map[string]string{},
					},
				},
			},
			configuredRealms: []string{"master"},
			expectedAlerts:   1, // Only master realm is checked
			expectError:      false,
		},
		{
			name:              "No identity providers - no alerts",
			realms:            []*keycloakadmin.RealmRepresentation{{Realm: "master", Enabled: true}},
			identityProviders: map[string][]*keycloakadmin.IdentityProviderRepresentation{},
			configuredRealms:  []string{},
			expectedAlerts:    0,
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &testKeycloakClient{
				realms:            tt.realms,
				identityProviders: tt.identityProviders,
			}
			log := logger.NewNoop()
			check := NewIdentityProviderMetadataCheck(client, log, tt.configuredRealms, "test-tenant", 30, 30*time.Second)
			// Disable certificate checking for existing tests to maintain backward compatibility
			check.checkCertificates = false

			ctx := context.Background()
			alerts, err := check.Execute(ctx)

			if (err != nil) != tt.expectError {
				t.Errorf("Execute() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if len(alerts) != tt.expectedAlerts {
				t.Errorf("expected %d alerts, got %d", tt.expectedAlerts, len(alerts))
			}

			// Validate alert structure
			for _, alert := range alerts {
				if alert.TenantID != "test-tenant" {
					t.Errorf("expected tenant ID 'test-tenant', got %s", alert.TenantID)
				}
				if alert.AlertID == "" {
					t.Error("alert has empty AlertID")
				}
				if alert.Type != domain.AlertTypeIdentityProvider {
					t.Errorf("expected alert type %s, got %s", domain.AlertTypeIdentityProvider, alert.Type)
				}
				if alert.Severity != domain.AlertSeverityWarning {
					t.Errorf("expected severity %s, got %s", domain.AlertSeverityWarning, alert.Severity)
				}
				if alert.Status != domain.AlertStatusActive {
					t.Errorf("expected status %s, got %s", domain.AlertStatusActive, alert.Status)
				}
				if alert.Title == "" {
					t.Error("alert has empty Title")
				}
				if alert.Description == "" {
					t.Error("alert has empty Description")
				}
				if alert.Recommendation == "" {
					t.Error("alert has empty Recommendation")
				}
				if alert.ResourceType != "identity_provider" {
					t.Errorf("expected resource type 'identity_provider', got %s", alert.ResourceType)
				}
				if alert.CheckType != "identity_provider_metadata" {
					t.Errorf("expected check type 'identity_provider_metadata', got %s", alert.CheckType)
				}
			}
		})
	}
}

func TestIdentityProviderMetadataCheck_Execute_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		setupClient func() *testKeycloakClient
		expectError bool
	}{
		{
			name: "GetAllRealms error",
			setupClient: func() *testKeycloakClient {
				return &testKeycloakClient{
					err: errors.New("failed to get realms"),
				}
			},
			expectError: true,
		},
		{
			name: "GetIdentityProviders error - continues with other realms",
			setupClient: func() *testKeycloakClient {
				client := &testKeycloakClient{
					realms: []*keycloakadmin.RealmRepresentation{
						{Realm: "master", Enabled: true},
					},
				}
				return client
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			log := logger.NewNoop()
			check := NewIdentityProviderMetadataCheck(client, log, []string{}, "test-tenant", 30, 30*time.Second)

			ctx := context.Background()
			_, err := check.Execute(ctx)

			if (err != nil) != tt.expectError {
				t.Errorf("Execute() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestIdentityProviderMetadataCheck_GenerateAlertID(t *testing.T) {
	check := &IdentityProviderMetadataCheck{}

	// Test that same inputs produce same ID
	id1 := check.generateAlertID("master", "saml-provider")
	id2 := check.generateAlertID("master", "saml-provider")

	if id1 != id2 {
		t.Error("Same inputs should produce same alert ID")
	}

	// Test that different inputs produce different IDs
	id3 := check.generateAlertID("test", "saml-provider")
	if id1 == id3 {
		t.Error("Different realms should produce different alert IDs")
	}

	id4 := check.generateAlertID("master", "different-provider")
	if id1 == id4 {
		t.Error("Different providers should produce different alert IDs")
	}

	// Test that alert ID has expected format
	if len(id1) == 0 {
		t.Error("Alert ID should not be empty")
	}
	if id1[:13] != "idp-metadata-" {
		t.Errorf("Alert ID should start with 'idp-metadata-', got %s", id1)
	}

	// Two tenants with the same realm/provider must not collide — otherwise
	// the second tenant's Save silently overwrites the first tenant's row
	// (see the alert_id tenant-scoping fix).
	checkTenantA := &IdentityProviderMetadataCheck{tenantID: "tenant-a"}
	checkTenantB := &IdentityProviderMetadataCheck{tenantID: "tenant-b"}
	idTenantA := checkTenantA.generateAlertID("master", "saml-provider")
	idTenantB := checkTenantB.generateAlertID("master", "saml-provider")
	if idTenantA == idTenantB {
		t.Error("Different tenants with the same realm/provider should produce different alert IDs")
	}
}

func TestIdentityProviderMetadataCheck_GenerateCertAlertID(t *testing.T) {
	check := &IdentityProviderMetadataCheck{}

	id1 := check.generateCertAlertID("master", "saml-provider", "1234567890")
	id2 := check.generateCertAlertID("master", "saml-provider", "1234567890")
	if id1 != id2 {
		t.Error("Same inputs should produce same alert ID")
	}

	id3 := check.generateCertAlertID("master", "saml-provider", "0987654321")
	if id1 == id3 {
		t.Error("Different certificate serial numbers should produce different alert IDs")
	}

	// Two tenants with the same realm/provider/certificate must not collide
	// — otherwise the second tenant's Save silently overwrites the first
	// tenant's row (see the alert_id tenant-scoping fix).
	checkTenantA := &IdentityProviderMetadataCheck{tenantID: "tenant-a"}
	checkTenantB := &IdentityProviderMetadataCheck{tenantID: "tenant-b"}
	idTenantA := checkTenantA.generateCertAlertID("master", "saml-provider", "1234567890")
	idTenantB := checkTenantB.generateCertAlertID("master", "saml-provider", "1234567890")
	if idTenantA == idTenantB {
		t.Error("Different tenants with the same realm/provider/certificate should produce different alert IDs")
	}
}
