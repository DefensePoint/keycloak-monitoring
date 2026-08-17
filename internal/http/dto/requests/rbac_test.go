package requests

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestCreateRole_Validation(t *testing.T) {
	tests := []struct {
		name     string
		input    CreateRole
		wantErr  bool
		errField string
	}{
		{
			name: "valid request",
			input: CreateRole{
				Name:        "analyst",
				DisplayName: "Security Analyst",
				Description: "Can view and acknowledge alerts",
				Permissions: []string{"alerts:read", "alerts:acknowledge"},
			},
			wantErr: false,
		},
		{
			name: "valid request without optional fields",
			input: CreateRole{
				Name:        "analyst",
				DisplayName: "Security Analyst",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			input: CreateRole{
				DisplayName: "Security Analyst",
			},
			wantErr:  true,
			errField: "Name",
		},
		{
			name: "name too long",
			input: CreateRole{
				Name:        strings.Repeat("a", 101),
				DisplayName: "Security Analyst",
			},
			wantErr:  true,
			errField: "Name",
		},
		{
			name: "missing display_name",
			input: CreateRole{
				Name: "analyst",
			},
			wantErr:  true,
			errField: "DisplayName",
		},
		{
			name: "display_name too long",
			input: CreateRole{
				Name:        "analyst",
				DisplayName: strings.Repeat("a", 256),
			},
			wantErr:  true,
			errField: "DisplayName",
		},
		{
			name: "description too long",
			input: CreateRole{
				Name:        "analyst",
				DisplayName: "Security Analyst",
				Description: strings.Repeat("a", 1001),
			},
			wantErr:  true,
			errField: "Description",
		},
		{
			name: "empty permissions list is valid",
			input: CreateRole{
				Name:        "analyst",
				DisplayName: "Security Analyst",
				Permissions: []string{},
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

func TestUpdateRole_Validation(t *testing.T) {
	tests := []struct {
		name     string
		input    UpdateRole
		wantErr  bool
		errField string
	}{
		{
			name:    "empty request is valid (all optional)",
			input:   UpdateRole{},
			wantErr: false,
		},
		{
			name: "valid display_name update",
			input: UpdateRole{
				DisplayName: "Updated Display Name",
			},
			wantErr: false,
		},
		{
			name: "display_name too long",
			input: UpdateRole{
				DisplayName: strings.Repeat("a", 256),
			},
			wantErr:  true,
			errField: "DisplayName",
		},
		{
			name: "valid description update",
			input: UpdateRole{
				Description: "Updated description",
			},
			wantErr: false,
		},
		{
			name: "description too long",
			input: UpdateRole{
				Description: strings.Repeat("a", 1001),
			},
			wantErr:  true,
			errField: "Description",
		},
		{
			name: "valid permissions update",
			input: UpdateRole{
				Permissions: []string{"alerts:read", "alerts:write"},
			},
			wantErr: false,
		},
		{
			name: "valid full update",
			input: UpdateRole{
				DisplayName: "Updated Name",
				Description: "Updated description",
				Permissions: []string{"alerts:read"},
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

func TestAssignRole_Validation(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name     string
		input    AssignRole
		wantErr  bool
		errField string
	}{
		{
			name: "valid request",
			input: AssignRole{
				RoleID: 1,
			},
			wantErr: false,
		},
		{
			name: "valid request with optional fields",
			input: AssignRole{
				RoleID:    1,
				TenantID:  strPtr("my-keycloak"),
				ExpiresAt: strPtr("2024-12-31T23:59:59Z"),
			},
			wantErr: false,
		},
		{
			name: "missing role_id",
			input: AssignRole{
				TenantID: strPtr("my-keycloak"),
			},
			wantErr:  true,
			errField: "RoleID",
		},
		{
			name: "role_id zero",
			input: AssignRole{
				RoleID: 0,
			},
			wantErr:  true,
			errField: "RoleID",
		},
		{
			name: "role_id negative would be 0 due to uint",
			input: AssignRole{
				RoleID: 0,
			},
			wantErr:  true,
			errField: "RoleID",
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

func TestCreateTenantPolicy_Validation(t *testing.T) {
	tests := []struct {
		name     string
		input    CreateTenantPolicy
		wantErr  bool
		errField string
	}{
		{
			name: "valid request",
			input: CreateTenantPolicy{
				TenantID:      "my-keycloak",
				AllowedRealms: []string{"master", "production"},
			},
			wantErr: false,
		},
		{
			name: "valid request without allowed_realms",
			input: CreateTenantPolicy{
				TenantID: "my-keycloak",
			},
			wantErr: false,
		},
		{
			name: "missing tenant_id",
			input: CreateTenantPolicy{
				AllowedRealms: []string{"master"},
			},
			wantErr:  true,
			errField: "TenantID",
		},
		{
			name:     "empty tenant_id",
			input:    CreateTenantPolicy{},
			wantErr:  true,
			errField: "TenantID",
		},
		{
			name: "empty allowed_realms list is valid",
			input: CreateTenantPolicy{
				TenantID:      "my-keycloak",
				AllowedRealms: []string{},
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
