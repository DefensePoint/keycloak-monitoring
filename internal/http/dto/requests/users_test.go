package requests

import (
	"regexp"
	"testing"

	"github.com/go-playground/validator/v10"
)

// validate mirrors the production validator from internal/http/request.go,
// including the custom keycloak_realm rule. Keeping the registration here
// (rather than importing from internal/http) avoids an import cycle.
var validate = func() *validator.Validate {
	v := validator.New()
	pattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	_ = v.RegisterValidation("keycloak_realm", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		if s == "" {
			return true
		}
		return pattern.MatchString(s)
	})
	return v
}()

func TestCreateUser_Validation(t *testing.T) {
	tests := []struct {
		name     string
		input    CreateUser
		wantErr  bool
		errField string
	}{
		{
			name: "valid request",
			input: CreateUser{
				Username: "johndoe",
				Email:    "john@example.com",
				Password: "SecurePass123!",
				Name:     "John Doe",
			},
			wantErr: false,
		},
		{
			name: "valid request without optional name",
			input: CreateUser{
				Username: "johndoe",
				Email:    "john@example.com",
				Password: "SecurePass123!",
			},
			wantErr: false,
		},
		{
			name: "missing username",
			input: CreateUser{
				Email:    "john@example.com",
				Password: "SecurePass123!",
			},
			wantErr:  true,
			errField: "Username",
		},
		{
			name: "username too short",
			input: CreateUser{
				Username: "ab",
				Email:    "john@example.com",
				Password: "SecurePass123!",
			},
			wantErr:  true,
			errField: "Username",
		},
		{
			name: "username too long",
			input: CreateUser{
				Username: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz",
				Email:    "john@example.com",
				Password: "SecurePass123!",
			},
			wantErr:  true,
			errField: "Username",
		},
		{
			name: "missing email",
			input: CreateUser{
				Username: "johndoe",
				Password: "SecurePass123!",
			},
			wantErr:  true,
			errField: "Email",
		},
		{
			name: "invalid email format",
			input: CreateUser{
				Username: "johndoe",
				Email:    "not-an-email",
				Password: "SecurePass123!",
			},
			wantErr:  true,
			errField: "Email",
		},
		{
			name: "missing password",
			input: CreateUser{
				Username: "johndoe",
				Email:    "john@example.com",
			},
			wantErr:  true,
			errField: "Password",
		},
		{
			name: "password too short",
			input: CreateUser{
				Username: "johndoe",
				Email:    "john@example.com",
				Password: "short",
			},
			wantErr:  true,
			errField: "Password",
		},
		{
			name: "name too long",
			input: CreateUser{
				Username: "johndoe",
				Email:    "john@example.com",
				Password: "SecurePass123!",
				Name:     "This name is way too long and exceeds the maximum allowed length of one hundred characters which should fail validation",
			},
			wantErr:  true,
			errField: "Name",
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

func TestUpdateUser_Validation(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name     string
		input    UpdateUser
		wantErr  bool
		errField string
	}{
		{
			name:    "empty request is valid (all optional)",
			input:   UpdateUser{},
			wantErr: false,
		},
		{
			name: "valid email update",
			input: UpdateUser{
				Email: strPtr("newemail@example.com"),
			},
			wantErr: false,
		},
		{
			name: "invalid email format",
			input: UpdateUser{
				Email: strPtr("not-an-email"),
			},
			wantErr:  true,
			errField: "Email",
		},
		{
			name: "valid username update",
			input: UpdateUser{
				Username: strPtr("newusername"),
			},
			wantErr: false,
		},
		{
			name: "username too short",
			input: UpdateUser{
				Username: strPtr("ab"),
			},
			wantErr:  true,
			errField: "Username",
		},
		{
			name: "username too long",
			input: UpdateUser{
				Username: strPtr("abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"),
			},
			wantErr:  true,
			errField: "Username",
		},
		{
			name: "valid password update",
			input: UpdateUser{
				Password: strPtr("NewSecurePass123!"),
			},
			wantErr: false,
		},
		{
			name: "password too short",
			input: UpdateUser{
				Password: strPtr("short"),
			},
			wantErr:  true,
			errField: "Password",
		},
		{
			name: "valid boolean updates",
			input: UpdateUser{
				IsActive:  boolPtr(true),
				IsBlocked: boolPtr(false),
			},
			wantErr: false,
		},
		{
			name: "valid full update",
			input: UpdateUser{
				Email:     strPtr("new@example.com"),
				Name:      strPtr("New Name"),
				Username:  strPtr("newuser"),
				Password:  strPtr("NewPassword123!"),
				IsActive:  boolPtr(true),
				IsBlocked: boolPtr(false),
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
