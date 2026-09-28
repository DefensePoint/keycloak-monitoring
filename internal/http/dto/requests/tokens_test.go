package requests

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestCreateAPIToken_Validation(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name     string
		input    CreateAPIToken
		wantErr  bool
		errField string
	}{
		{
			name: "valid request",
			input: CreateAPIToken{
				Name:      "ci-pipeline",
				UserID:    1,
				TenantIDs: []string{"prod-main", "customer-abc"},
				ExpiresAt: strPtr("2026-12-31T23:59:59Z"),
			},
			wantErr: false,
		},
		{
			name: "valid request without optional fields",
			input: CreateAPIToken{
				Name:      "ci-pipeline",
				UserID:    1,
				TenantIDs: []string{"prod-main"},
			},
			wantErr: false,
		},
		{
			name: "missing tenant_ids",
			input: CreateAPIToken{
				Name:   "ci-pipeline",
				UserID: 1,
			},
			wantErr:  true,
			errField: "TenantIDs",
		},
		{
			name: "empty tenant_ids",
			input: CreateAPIToken{
				Name:      "ci-pipeline",
				UserID:    1,
				TenantIDs: []string{},
			},
			wantErr:  true,
			errField: "TenantIDs",
		},
		{
			name: "missing name",
			input: CreateAPIToken{
				UserID:    1,
				TenantIDs: []string{"prod-main"},
			},
			wantErr:  true,
			errField: "Name",
		},
		{
			name: "name too long",
			input: CreateAPIToken{
				Name:      strings.Repeat("a", 256),
				UserID:    1,
				TenantIDs: []string{"prod-main"},
			},
			wantErr:  true,
			errField: "Name",
		},
		{
			name: "missing user_id",
			input: CreateAPIToken{
				Name:      "ci-pipeline",
				TenantIDs: []string{"prod-main"},
			},
			wantErr:  true,
			errField: "UserID",
		},
		{
			name: "user_id zero",
			input: CreateAPIToken{
				Name:      "ci-pipeline",
				UserID:    0,
				TenantIDs: []string{"prod-main"},
			},
			wantErr:  true,
			errField: "UserID",
		},
		{
			name: "tenant_ids containing an empty string",
			input: CreateAPIToken{
				Name:      "ci-pipeline",
				UserID:    1,
				TenantIDs: []string{"prod-main", ""},
			},
			wantErr:  true,
			errField: "TenantIDs[1]",
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
