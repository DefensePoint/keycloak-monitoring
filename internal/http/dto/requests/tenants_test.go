package requests

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestCreateTenant_Validation(t *testing.T) {
	tests := []struct {
		name     string
		input    CreateTenant
		wantErr  bool
		errField string
	}{
		{
			name: "valid request with required fields only",
			input: CreateTenant{
				TenantID:     "my-keycloak",
				Name:         "My Keycloak Instance",
				ServerURL:    "https://keycloak.example.com",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			wantErr: false,
		},
		{
			name: "valid request with all fields",
			input: CreateTenant{
				TenantID:      "my-keycloak",
				Name:          "My Keycloak Instance",
				Description:   "Production Keycloak server",
				ServerURL:     "https://keycloak.example.com",
				AdminRealm:    "master",
				ClientID:      "monitoring-service",
				ClientSecret:  "client-secret",
				Configuration: "{}",
				DefaultRealm:  "master",
				Enabled:       true,
				IsDefault:     false,
				Tags:          []string{"production", "main"},
				Owner:         "admin@example.com",
			},
			wantErr: false,
		},
		{
			name: "missing tenant_id",
			input: CreateTenant{
				Name:      "My Keycloak Instance",
				ServerURL: "https://keycloak.example.com",
			},
			wantErr:  true,
			errField: "TenantID",
		},
		{
			name: "tenant_id too long",
			input: CreateTenant{
				TenantID:  strings.Repeat("a", 101),
				Name:      "My Keycloak Instance",
				ServerURL: "https://keycloak.example.com",
			},
			wantErr:  true,
			errField: "TenantID",
		},
		{
			name: "missing name",
			input: CreateTenant{
				TenantID:  "my-keycloak",
				ServerURL: "https://keycloak.example.com",
			},
			wantErr:  true,
			errField: "Name",
		},
		{
			name: "name too long",
			input: CreateTenant{
				TenantID:  "my-keycloak",
				Name:      strings.Repeat("a", 256),
				ServerURL: "https://keycloak.example.com",
			},
			wantErr:  true,
			errField: "Name",
		},
		{
			name: "missing server_url",
			input: CreateTenant{
				TenantID: "my-keycloak",
				Name:     "My Keycloak Instance",
			},
			wantErr:  true,
			errField: "ServerURL",
		},
		{
			name: "invalid server_url format",
			input: CreateTenant{
				TenantID:  "my-keycloak",
				Name:      "My Keycloak Instance",
				ServerURL: "not-a-url",
			},
			wantErr:  true,
			errField: "ServerURL",
		},
		{
			name: "missing client_id",
			input: CreateTenant{
				TenantID:     "my-keycloak",
				Name:         "My Keycloak Instance",
				ServerURL:    "https://keycloak.example.com",
				ClientSecret: "client-secret",
			},
			wantErr:  true,
			errField: "ClientID",
		},
		{
			name: "missing client_secret",
			input: CreateTenant{
				TenantID:  "my-keycloak",
				Name:      "My Keycloak Instance",
				ServerURL: "https://keycloak.example.com",
				ClientID:  "monitoring-service",
			},
			wantErr:  true,
			errField: "ClientSecret",
		},
		{
			name: "description too long",
			input: CreateTenant{
				TenantID:    "my-keycloak",
				Name:        "My Keycloak Instance",
				Description: strings.Repeat("a", 1001),
				ServerURL:   "https://keycloak.example.com",
			},
			wantErr:  true,
			errField: "Description",
		},
		{
			// Without this, a tenant can be saved as "AMFA enabled" with no
			// endpoint to read from, register with the AMFA sync as a silent
			// no-op, and never expose AMFA data with no error anywhere.
			name: "amfa enabled with no api_base_url",
			input: CreateTenant{
				TenantID:     "my-keycloak",
				Name:         "My Keycloak Instance",
				ServerURL:    "https://keycloak.example.com",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
				Amfa:         &TenantAmfa{Enabled: true},
			},
			wantErr:  true,
			errField: "APIBaseURL",
		},
		{
			name: "amfa disabled with no api_base_url is fine",
			input: CreateTenant{
				TenantID:     "my-keycloak",
				Name:         "My Keycloak Instance",
				ServerURL:    "https://keycloak.example.com",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
				Amfa:         &TenantAmfa{Enabled: false},
			},
			wantErr: false,
		},
		{
			name: "amfa enabled with a valid api_base_url",
			input: CreateTenant{
				TenantID:     "my-keycloak",
				Name:         "My Keycloak Instance",
				ServerURL:    "https://keycloak.example.com",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
				Amfa:         &TenantAmfa{Enabled: true, APIBaseURL: "https://amfa.internal:8000"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected validation error for field %s, got nil", tt.errField)
					return
				}
				validationErrs, ok := err.(validator.ValidationErrors)
				if !ok {
					t.Errorf("expected validator.ValidationErrors, got %T", err)
					return
				}
				found := false
				for _, ve := range validationErrs {
					if ve.Field() == tt.errField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error on field %s, got errors: %v", tt.errField, validationErrs)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestUpdateTenant_Validation(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name     string
		input    UpdateTenant
		wantErr  bool
		errField string
	}{
		{
			name:    "empty request is valid (all optional)",
			input:   UpdateTenant{},
			wantErr: false,
		},
		{
			name: "valid name update",
			input: UpdateTenant{
				Name: strPtr("Updated Name"),
			},
			wantErr: false,
		},
		{
			name: "name too long",
			input: UpdateTenant{
				Name: strPtr(strings.Repeat("a", 256)),
			},
			wantErr:  true,
			errField: "Name",
		},
		{
			name: "valid server_url update",
			input: UpdateTenant{
				ServerURL: strPtr("https://new-keycloak.example.com"),
			},
			wantErr: false,
		},
		{
			name: "invalid server_url format",
			input: UpdateTenant{
				ServerURL: strPtr("not-a-url"),
			},
			wantErr:  true,
			errField: "ServerURL",
		},
		{
			name: "valid boolean updates",
			input: UpdateTenant{
				Enabled:   boolPtr(true),
				IsDefault: boolPtr(false),
			},
			wantErr: false,
		},
		{
			name: "valid tags update",
			input: UpdateTenant{
				Tags: []string{"production", "updated"},
			},
			wantErr: false,
		},
		{
			name: "description too long",
			input: UpdateTenant{
				Description: strPtr(strings.Repeat("a", 1001)),
			},
			wantErr:  true,
			errField: "Description",
		},
		{
			name: "admin_realm too long",
			input: UpdateTenant{
				AdminRealm: strPtr(strings.Repeat("a", 101)),
			},
			wantErr:  true,
			errField: "AdminRealm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected validation error for field %s, got nil", tt.errField)
					return
				}
				validationErrs, ok := err.(validator.ValidationErrors)
				if !ok {
					t.Errorf("expected validator.ValidationErrors, got %T", err)
					return
				}
				found := false
				for _, ve := range validationErrs {
					if ve.Field() == tt.errField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error on field %s, got errors: %v", tt.errField, validationErrs)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestTestTenantConnection_Validation(t *testing.T) {
	tests := []struct {
		name     string
		input    TestTenantConnection
		wantErr  bool
		errField string
	}{
		{
			name: "valid request",
			input: TestTenantConnection{
				ServerURL:    "https://keycloak.example.com",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			wantErr: false,
		},
		{
			name: "valid request with optional admin_realm",
			input: TestTenantConnection{
				ServerURL:    "https://keycloak.example.com",
				AdminRealm:   "master",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			wantErr: false,
		},
		{
			name: "missing server_url",
			input: TestTenantConnection{
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			wantErr:  true,
			errField: "ServerURL",
		},
		{
			name: "invalid server_url format",
			input: TestTenantConnection{
				ServerURL:    "not-a-url",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			wantErr:  true,
			errField: "ServerURL",
		},
		{
			name: "missing client_id",
			input: TestTenantConnection{
				ServerURL:    "https://keycloak.example.com",
				ClientSecret: "client-secret",
			},
			wantErr:  true,
			errField: "ClientID",
		},
		{
			name: "missing client_secret",
			input: TestTenantConnection{
				ServerURL: "https://keycloak.example.com",
				ClientID:  "monitoring-service",
			},
			wantErr:  true,
			errField: "ClientSecret",
		},
		{
			name: "admin_realm too long",
			input: TestTenantConnection{
				ServerURL:    "https://keycloak.example.com",
				AdminRealm:   strings.Repeat("a", 101),
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			wantErr:  true,
			errField: "AdminRealm",
		},
		{
			// Pentest report PoC: admin_realm with traversal chars must be
			// rejected before it can interpolate into the Keycloak URL.
			name: "admin_realm path traversal payload",
			input: TestTenantConnection{
				ServerURL:    "http://169.254.169.254/",
				AdminRealm:   "../../latest/meta-data/iam/security-credentials/role?x=",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			wantErr:  true,
			errField: "AdminRealm",
		},
		{
			name: "admin_realm with slash",
			input: TestTenantConnection{
				ServerURL:    "https://keycloak.example.com",
				AdminRealm:   "foo/bar",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			wantErr:  true,
			errField: "AdminRealm",
		},
		{
			name: "admin_realm with dot",
			input: TestTenantConnection{
				ServerURL:    "https://keycloak.example.com",
				AdminRealm:   "acme.production",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			wantErr:  true,
			errField: "AdminRealm",
		},
		{
			name: "admin_realm valid hyphenated",
			input: TestTenantConnection{
				ServerURL:    "https://keycloak.example.com",
				AdminRealm:   "acme-corp_2",
				ClientID:     "monitoring-service",
				ClientSecret: "client-secret",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected validation error for field %s, got nil", tt.errField)
					return
				}
				validationErrs, ok := err.(validator.ValidationErrors)
				if !ok {
					t.Errorf("expected validator.ValidationErrors, got %T", err)
					return
				}
				found := false
				for _, ve := range validationErrs {
					if ve.Field() == tt.errField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error on field %s, got errors: %v", tt.errField, validationErrs)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}
