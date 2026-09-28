package configcheck

import (
	"context"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

func TestClientSecurityCheck_GetCheckType(t *testing.T) {
	check := &ClientSecurityCheck{}
	if got := check.GetCheckType(); got != "client_security" {
		t.Errorf("GetCheckType() = %v, want client_security", got)
	}
}

func TestClientSecurityCheck_GetDescription(t *testing.T) {
	check := &ClientSecurityCheck{}
	desc := check.GetDescription()
	if desc == "" {
		t.Error("GetDescription() returned empty string")
	}
}

func TestClientSecurityCheck_Execute(t *testing.T) {
	log := logger.NewNoop()

	tests := []struct {
		name          string
		mockRealms    []*keycloakadmin.RealmRepresentation
		mockClients   map[string][]*keycloakadmin.ClientRepresentation
		config        ClientSecurityCheckConfig
		expectedCount int // Minimum expected alert count
		wantErr       bool
	}{
		{
			name: "Wildcard redirect URI - should alert",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:      "test-realm-id",
					Realm:   "test-realm",
					Enabled: true,
				},
			},
			mockClients: map[string][]*keycloakadmin.ClientRepresentation{
				"test-realm": {
					{
						ID:           "client-1",
						ClientID:     "my-app",
						RedirectUris: []string{"https://example.com/*"}, // Wildcard
					},
				},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs: true,
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "Localhost in production - should alert",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:      "test-realm-id",
					Realm:   "test-realm",
					Enabled: true,
				},
			},
			mockClients: map[string][]*keycloakadmin.ClientRepresentation{
				"test-realm": {
					{
						ID:           "client-1",
						ClientID:     "my-app",
						RedirectUris: []string{"http://localhost:3000/callback"},
					},
				},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs:       true,
				ProductionEnvironment:   true,
				AllowLocalhostRedirects: false,
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "HTTP redirect in production - should alert",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:      "test-realm-id",
					Realm:   "test-realm",
					Enabled: true,
				},
			},
			mockClients: map[string][]*keycloakadmin.ClientRepresentation{
				"test-realm": {
					{
						ID:           "client-1",
						ClientID:     "my-app",
						RedirectUris: []string{"http://example.com/callback"},
					},
				},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs:     true,
				ProductionEnvironment: true,
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "Public client with service accounts - should alert critical",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:      "test-realm-id",
					Realm:   "test-realm",
					Enabled: true,
				},
			},
			mockClients: map[string][]*keycloakadmin.ClientRepresentation{
				"test-realm": {
					{
						ID:                     "client-1",
						ClientID:               "my-app",
						PublicClient:           true,
						ServiceAccountsEnabled: true, // Critical issue
					},
				},
			},
			config: ClientSecurityCheckConfig{
				CheckPublicClients: true,
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "Direct access grants enabled - should alert info",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:      "test-realm-id",
					Realm:   "test-realm",
					Enabled: true,
				},
			},
			mockClients: map[string][]*keycloakadmin.ClientRepresentation{
				"test-realm": {
					{
						ID:                        "client-1",
						ClientID:                  "my-app",
						DirectAccessGrantsEnabled: true, // ROPC flow
					},
				},
			},
			config: ClientSecurityCheckConfig{
				CheckDirectAccessGrants: true,
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "Confidential client without secret - should alert critical",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:      "test-realm-id",
					Realm:   "test-realm",
					Enabled: true,
				},
			},
			mockClients: map[string][]*keycloakadmin.ClientRepresentation{
				"test-realm": {
					{
						ID:           "client-1",
						ClientID:     "my-app",
						PublicClient: false,
						BearerOnly:   false,
						Secret:       "", // Missing secret
					},
				},
			},
			config: ClientSecurityCheckConfig{
				CheckClientSecret: true,
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "Implicit flow enabled - should alert warning",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:      "test-realm-id",
					Realm:   "test-realm",
					Enabled: true,
				},
			},
			mockClients: map[string][]*keycloakadmin.ClientRepresentation{
				"test-realm": {
					{
						ID:                  "client-1",
						ClientID:            "my-app",
						ImplicitFlowEnabled: true, // Deprecated
					},
				},
			},
			config: ClientSecurityCheckConfig{
				CheckImplicitFlow: true,
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "Multiple issues - should create multiple alerts",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:      "test-realm-id",
					Realm:   "test-realm",
					Enabled: true,
				},
			},
			mockClients: map[string][]*keycloakadmin.ClientRepresentation{
				"test-realm": {
					{
						ID:       "client-1",
						ClientID: "insecure-app",
						RedirectUris: []string{
							"https://example.com/*",       // Wildcard
							"http://localhost:3000",       // Localhost in prod
							"http://example.com/callback", // HTTP in prod
						},
						PublicClient:              true,
						ServiceAccountsEnabled:    true, // Critical
						DirectAccessGrantsEnabled: true,
						ImplicitFlowEnabled:       true,
					},
				},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs:       true,
				CheckPublicClients:      true,
				CheckDirectAccessGrants: true,
				CheckImplicitFlow:       true,
				ProductionEnvironment:   true,
				AllowLocalhostRedirects: false,
			},
			expectedCount: 6, // Multiple alerts
			wantErr:       false,
		},
		{
			name: "Secure client - no alerts",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:      "test-realm-id",
					Realm:   "test-realm",
					Enabled: true,
				},
			},
			mockClients: map[string][]*keycloakadmin.ClientRepresentation{
				"test-realm": {
					{
						ID:       "client-1",
						ClientID: "secure-app",
						RedirectUris: []string{
							"https://example.com/callback",
							"https://app.example.com/auth/callback",
						},
						PublicClient:              false,
						ServiceAccountsEnabled:    false,
						DirectAccessGrantsEnabled: false,
						ImplicitFlowEnabled:       false,
						Secret:                    "secret-value",
					},
				},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs:       true,
				CheckPublicClients:      true,
				CheckDirectAccessGrants: true,
				CheckClientSecret:       true,
				CheckImplicitFlow:       true,
				ProductionEnvironment:   true,
			},
			expectedCount: 0, // No alerts
			wantErr:       false,
		},
		{
			name: "Built-in clients - should skip",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:      "test-realm-id",
					Realm:   "test-realm",
					Enabled: true,
				},
			},
			mockClients: map[string][]*keycloakadmin.ClientRepresentation{
				"test-realm": {
					{
						ID:           "account-id",
						ClientID:     "account",
						RedirectUris: []string{"https://example.com/*"}, // Would alert but should skip
					},
					{
						ID:                     "admin-cli-id",
						ClientID:               "admin-cli",
						PublicClient:           true,
						ServiceAccountsEnabled: true, // Would alert but should skip
					},
				},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs:  true,
				CheckPublicClients: true,
			},
			expectedCount: 0, // Built-in clients skipped
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			mockClient := &testKeycloakClient{
				realms:  tt.mockRealms,
				clients: tt.mockClients,
			}

			// Create check
			check := NewClientSecurityCheck(
				mockClient,
				log,
				nil, // Will fetch all realms from mock
				"test-tenant",
				tt.config,
			)

			// Execute check
			alerts, err := check.Execute(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(alerts) < tt.expectedCount {
				t.Errorf("Execute() got %d alerts, want at least %d", len(alerts), tt.expectedCount)
				for i, alert := range alerts {
					t.Logf("Alert %d: %s - %s", i, alert.Title, alert.Description)
				}
			}

			// Verify alert properties
			for _, alert := range alerts {
				if alert.TenantID != "test-tenant" {
					t.Errorf("Alert TenantID = %v, want test-tenant", alert.TenantID)
				}
				if alert.CheckType != "client_security" {
					t.Errorf("Alert CheckType = %v, want client_security", alert.CheckType)
				}
				if alert.Type != domain.AlertTypeClient {
					t.Errorf("Alert Type = %v, want %v", alert.Type, domain.AlertTypeClient)
				}
				if alert.Status != domain.AlertStatusActive {
					t.Errorf("Alert Status = %v, want %v", alert.Status, domain.AlertStatusActive)
				}
				if alert.ResourceType != "client" {
					t.Errorf("Alert ResourceType = %v, want client", alert.ResourceType)
				}
				if alert.RealmName == "" {
					t.Error("Alert RealmName is empty")
				}
				if alert.Title == "" {
					t.Error("Alert Title is empty")
				}
				if alert.Description == "" {
					t.Error("Alert Description is empty")
				}
				if alert.Recommendation == "" {
					t.Error("Alert Recommendation is empty")
				}
			}
		})
	}
}

func TestClientSecurityCheck_IsBuiltInClient(t *testing.T) {
	log := logger.NewNoop()
	check := NewClientSecurityCheck(
		nil, // Not needed for this test
		log,
		[]string{"test-realm"},
		"test-tenant",
		ClientSecurityCheckConfig{},
	)

	builtInClients := []string{
		"account",
		"account-console",
		"admin-cli",
		"broker",
		"realm-management",
		"security-admin-console",
	}

	for _, clientID := range builtInClients {
		if !check.isBuiltInClient(clientID) {
			t.Errorf("isBuiltInClient(%s) = false, want true", clientID)
		}
	}

	customClients := []string{
		"my-app",
		"web-client",
		"mobile-app",
		"api-gateway",
	}

	for _, clientID := range customClients {
		if check.isBuiltInClient(clientID) {
			t.Errorf("isBuiltInClient(%s) = true, want false", clientID)
		}
	}
}

func TestClientSecurityCheck_CheckRedirectURIs(t *testing.T) {
	log := logger.NewNoop()

	tests := []struct {
		name          string
		client        *keycloakadmin.ClientRepresentation
		config        ClientSecurityCheckConfig
		expectedCount int
		checkTitles   []string
	}{
		{
			name: "Wildcard URI",
			client: &keycloakadmin.ClientRepresentation{
				ID:           "client-1",
				ClientID:     "my-app",
				RedirectUris: []string{"https://example.com/*"},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs: true,
			},
			expectedCount: 1,
			checkTitles:   []string{"Wildcard Redirect URI"},
		},
		{
			name: "Localhost in production",
			client: &keycloakadmin.ClientRepresentation{
				ID:           "client-1",
				ClientID:     "my-app",
				RedirectUris: []string{"http://localhost:3000"},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs:       true,
				ProductionEnvironment:   true,
				AllowLocalhostRedirects: false,
			},
			expectedCount: 1,
			checkTitles:   []string{"Localhost Redirect URI in Production"},
		},
		{
			name: "HTTP in production",
			client: &keycloakadmin.ClientRepresentation{
				ID:           "client-1",
				ClientID:     "my-app",
				RedirectUris: []string{"http://example.com/callback"},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs:     true,
				ProductionEnvironment: true,
			},
			expectedCount: 1,
			checkTitles:   []string{"HTTP Redirect URI in Production"},
		},
		{
			name: "Valid HTTPS URIs",
			client: &keycloakadmin.ClientRepresentation{
				ID:       "client-1",
				ClientID: "my-app",
				RedirectUris: []string{
					"https://example.com/callback",
					"https://app.example.com/auth",
				},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs:     true,
				ProductionEnvironment: true,
			},
			expectedCount: 0,
			checkTitles:   []string{},
		},
		{
			name: "Localhost allowed in production",
			client: &keycloakadmin.ClientRepresentation{
				ID:           "client-1",
				ClientID:     "my-app",
				RedirectUris: []string{"http://localhost:3000"},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs:       true,
				ProductionEnvironment:   true,
				AllowLocalhostRedirects: true, // Allowed
			},
			expectedCount: 0,
			checkTitles:   []string{},
		},
		{
			name: "Localhost wildcard allowed when localhost redirects enabled",
			client: &keycloakadmin.ClientRepresentation{
				ID:           "client-1",
				ClientID:     "my-app",
				RedirectUris: []string{"http://localhost*"},
			},
			config: ClientSecurityCheckConfig{
				CheckRedirectURIs:       true,
				ProductionEnvironment:   true,
				AllowLocalhostRedirects: true, // Allow localhost in production
			},
			expectedCount: 0, // Localhost wildcard is allowed when localhost is allowed
			checkTitles:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := NewClientSecurityCheck(
				nil, // Not needed for this test
				log,
				[]string{"test-realm"},
				"test-tenant",
				tt.config,
			)

			alerts := check.checkRedirectURIs("test-realm", tt.client)

			if len(alerts) != tt.expectedCount {
				t.Errorf("checkRedirectURIs() got %d alerts, want %d", len(alerts), tt.expectedCount)
			}

			for i, expectedTitle := range tt.checkTitles {
				if i >= len(alerts) {
					t.Errorf("Missing expected alert: %s", expectedTitle)
					continue
				}
				if alerts[i].Title != expectedTitle {
					t.Errorf("Alert[%d] Title = %v, want %v", i, alerts[i].Title, expectedTitle)
				}
			}
		})
	}
}

func TestClientSecurityCheck_GenerateAlertID(t *testing.T) {
	check := &ClientSecurityCheck{}

	id1 := check.generateAlertID("master", "my-client", "public-with-service-account")
	id2 := check.generateAlertID("master", "my-client", "public-with-service-account")
	if id1 != id2 {
		t.Error("Same inputs should produce same alert ID")
	}

	id3 := check.generateAlertID("other-realm", "my-client", "public-with-service-account")
	if id1 == id3 {
		t.Error("Different realms should produce different alert IDs")
	}

	// Two tenants with the same realm/client/alert type must not collide —
	// otherwise the second tenant's Save silently overwrites the first
	// tenant's row (see the alert_id tenant-scoping fix).
	checkTenantA := &ClientSecurityCheck{tenantID: "tenant-a"}
	checkTenantB := &ClientSecurityCheck{tenantID: "tenant-b"}
	idTenantA := checkTenantA.generateAlertID("master", "my-client", "public-with-service-account")
	idTenantB := checkTenantB.generateAlertID("master", "my-client", "public-with-service-account")
	if idTenantA == idTenantB {
		t.Error("Different tenants with the same realm/client/alert type should produce different alert IDs")
	}
}
