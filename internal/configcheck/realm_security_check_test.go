package configcheck

import (
	"context"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

func TestRealmSecurityCheck_GetCheckType(t *testing.T) {
	check := &RealmSecurityCheck{}
	if got := check.GetCheckType(); got != "realm_security" {
		t.Errorf("GetCheckType() = %v, want realm_security", got)
	}
}

func TestRealmSecurityCheck_GetDescription(t *testing.T) {
	check := &RealmSecurityCheck{}
	desc := check.GetDescription()
	if desc == "" {
		t.Error("GetDescription() returned empty string")
	}
}

func TestRealmSecurityCheck_Execute(t *testing.T) {
	log := logger.NewNoop()

	tests := []struct {
		name          string
		mockRealms    []*keycloakadmin.RealmRepresentation
		config        RealmSecurityCheckConfig
		expectedCount int
		wantErr       bool
	}{
		{
			name: "SSL not required - should alert",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:          "test-realm-id",
					Realm:       "test-realm",
					DisplayName: "Test Realm",
					Enabled:     true,
					SslRequired: "none",
				},
			},
			config: RealmSecurityCheckConfig{
				CheckSSLRequired: true,
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "Brute force protection disabled - should alert",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:                  "test-realm-id",
					Realm:               "test-realm",
					DisplayName:         "Test Realm",
					Enabled:             true,
					SslRequired:         "all",
					BruteForceProtected: false,
				},
			},
			config: RealmSecurityCheckConfig{
				CheckSSLRequired: true,
				CheckBruteForce:  true,
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "Weak password length - should alert",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:                  "test-realm-id",
					Realm:               "test-realm",
					DisplayName:         "Test Realm",
					Enabled:             true,
					SslRequired:         "all",
					BruteForceProtected: true,
					PasswordPolicy:      "length(6)",
				},
			},
			config: RealmSecurityCheckConfig{
				CheckPasswordPolicy: true,
				MinPasswordLength:   8,
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "Self-registration without email verification - should alert",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:                  "test-realm-id",
					Realm:               "test-realm",
					DisplayName:         "Test Realm",
					Enabled:             true,
					SslRequired:         "all",
					BruteForceProtected: true,
					PasswordPolicy:      "length(8) and upperCase(1) and lowerCase(1) and digits(1) and specialChars(1)",
					RegistrationAllowed: true,
					VerifyEmail:         false,
				},
			},
			config: RealmSecurityCheckConfig{
				CheckEmailVerification: true,
			},
			expectedCount: 1,
			wantErr:       false,
		},
		{
			name: "Secure realm - no alerts",
			mockRealms: []*keycloakadmin.RealmRepresentation{
				{
					ID:                        "test-realm-id",
					Realm:                     "test-realm",
					DisplayName:               "Test Realm",
					Enabled:                   true,
					SslRequired:               "all",
					BruteForceProtected:       true,
					PasswordPolicy:            "length(12) and upperCase(1) and lowerCase(1) and digits(1) and specialChars(1)",
					RegistrationAllowed:       true,
					VerifyEmail:               true,
					AdminEventsEnabled:        true,
					AdminEventsDetailsEnabled: true,
					EventsEnabled:             true,
					EventsListeners:           []string{"jboss-logging"},
					DuplicateEmailsAllowed:    false,
				},
			},
			config: RealmSecurityCheckConfig{
				CheckSSLRequired:          true,
				CheckBruteForce:           true,
				CheckPasswordPolicy:       true,
				CheckEmailVerification:    true,
				CheckAdminEvents:          true,
				CheckUserEvents:           true,
				CheckDuplicateEmails:      true,
				MinPasswordLength:         8,
				RequirePasswordComplexity: true,
			},
			expectedCount: 0,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &testKeycloakClient{
				realms: tt.mockRealms,
			}

			check := NewRealmSecurityCheck(
				mockClient,
				log,
				nil,
				"test-tenant",
				tt.config,
			)

			alerts, err := check.Execute(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(alerts) < tt.expectedCount {
				t.Errorf("Execute() got %d alerts, want at least %d", len(alerts), tt.expectedCount)
			}

			for _, alert := range alerts {
				if alert.TenantID != "test-tenant" {
					t.Errorf("Alert TenantID = %v, want test-tenant", alert.TenantID)
				}
				if alert.CheckType != "realm_security" {
					t.Errorf("Alert CheckType = %v, want realm_security", alert.CheckType)
				}
				if alert.Type != domain.AlertTypeRealm {
					t.Errorf("Alert Type = %v, want %v", alert.Type, domain.AlertTypeRealm)
				}
				if alert.Status != domain.AlertStatusActive {
					t.Errorf("Alert Status = %v, want %v", alert.Status, domain.AlertStatusActive)
				}
			}
		})
	}
}

func TestRealmSecurityCheck_CheckSSLRequired(t *testing.T) {
	log := logger.NewNoop()

	tests := []struct {
		name        string
		sslRequired string
		shouldAlert bool
		severity    domain.AlertSeverity
	}{
		{
			name:        "SSL required for all - no alert",
			sslRequired: "all",
			shouldAlert: false,
		},
		{
			name:        "SSL required for external - no alert",
			sslRequired: "external",
			shouldAlert: false,
		},
		{
			name:        "SSL not required - warning alert",
			sslRequired: "internal",
			shouldAlert: true,
			severity:    domain.AlertSeverityWarning,
		},
		{
			name:        "SSL disabled - critical alert",
			sslRequired: "none",
			shouldAlert: true,
			severity:    domain.AlertSeverityCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realm := &keycloakadmin.RealmRepresentation{
				ID:          "test-realm-id",
				Realm:       "test-realm",
				DisplayName: "Test Realm",
				SslRequired: tt.sslRequired,
			}

			config := RealmSecurityCheckConfig{
				CheckSSLRequired: true,
			}

			check := NewRealmSecurityCheck(
				nil,
				log,
				[]string{"test-realm"},
				"test-tenant",
				config,
			)

			alert := check.checkSSLRequired(realm)

			if tt.shouldAlert && alert == nil {
				t.Error("Expected alert but got none")
			}
			if !tt.shouldAlert && alert != nil {
				t.Errorf("Expected no alert but got one: %v", alert.Title)
			}
			if tt.shouldAlert && alert != nil {
				if alert.Severity != tt.severity {
					t.Errorf("Alert Severity = %v, want %v", alert.Severity, tt.severity)
				}
			}
		})
	}
}

func TestParsePasswordPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		expected map[string]string
	}{
		{
			name:     "Empty policy",
			policy:   "",
			expected: map[string]string{},
		},
		{
			name:   "Simple length policy",
			policy: "length(8)",
			expected: map[string]string{
				"length": "8",
			},
		},
		{
			name:   "Complex policy",
			policy: "length(12) and upperCase(1) and lowerCase(1) and digits(1) and specialChars(1)",
			expected: map[string]string{
				"length":       "12",
				"upperCase":    "1",
				"lowerCase":    "1",
				"digits":       "1",
				"specialChars": "1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePasswordPolicy(tt.policy)

			if len(result) != len(tt.expected) {
				t.Errorf("parsePasswordPolicy() returned %d items, want %d", len(result), len(tt.expected))
			}

			for key, expectedValue := range tt.expected {
				if actualValue, exists := result[key]; !exists {
					t.Errorf("parsePasswordPolicy() missing key %v", key)
				} else if actualValue != expectedValue {
					t.Errorf("parsePasswordPolicy()[%v] = %v, want %v", key, actualValue, expectedValue)
				}
			}
		})
	}
}

func TestParseIntValue(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValue int
		wantOK    bool
	}{
		{
			name:      "Valid integer",
			input:     "8",
			wantValue: 8,
			wantOK:    true,
		},
		{
			name:      "Invalid - not a number",
			input:     "abc",
			wantValue: 0,
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := parseIntValue(tt.input)

			if ok != tt.wantOK {
				t.Errorf("parseIntValue() ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && value != tt.wantValue {
				t.Errorf("parseIntValue() value = %v, want %v", value, tt.wantValue)
			}
		})
	}
}

func TestRealmSecurityCheck_GenerateAlertID(t *testing.T) {
	check := &RealmSecurityCheck{}

	id1 := check.generateAlertID("master", "ssl-not-required")
	id2 := check.generateAlertID("master", "ssl-not-required")
	if id1 != id2 {
		t.Error("Same inputs should produce same alert ID")
	}

	id3 := check.generateAlertID("other-realm", "ssl-not-required")
	if id1 == id3 {
		t.Error("Different realms should produce different alert IDs")
	}

	// Two tenants with the same realm/alert type must not collide —
	// otherwise the second tenant's Save silently overwrites the first
	// tenant's row (see the alert_id tenant-scoping fix).
	checkTenantA := &RealmSecurityCheck{tenantID: "tenant-a"}
	checkTenantB := &RealmSecurityCheck{tenantID: "tenant-b"}
	idTenantA := checkTenantA.generateAlertID("master", "ssl-not-required")
	idTenantB := checkTenantB.generateAlertID("master", "ssl-not-required")
	if idTenantA == idTenantB {
		t.Error("Different tenants with the same realm/alert type should produce different alert IDs")
	}
}
