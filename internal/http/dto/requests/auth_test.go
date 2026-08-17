package requests

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestSimpleLogin_Validation(t *testing.T) {
	tests := []struct {
		name     string
		input    SimpleLogin
		wantErr  bool
		errField string
	}{
		{
			name: "valid request",
			input: SimpleLogin{
				Username: "johndoe",
				Password: "mypassword",
			},
			wantErr: false,
		},
		{
			name: "missing username",
			input: SimpleLogin{
				Password: "mypassword",
			},
			wantErr:  true,
			errField: "Username",
		},
		{
			name: "empty username",
			input: SimpleLogin{
				Username: "",
				Password: "mypassword",
			},
			wantErr:  true,
			errField: "Username",
		},
		{
			name: "missing password",
			input: SimpleLogin{
				Username: "johndoe",
			},
			wantErr:  true,
			errField: "Password",
		},
		{
			name: "empty password",
			input: SimpleLogin{
				Username: "johndoe",
				Password: "",
			},
			wantErr:  true,
			errField: "Password",
		},
		{
			name:     "both fields missing",
			input:    SimpleLogin{},
			wantErr:  true,
			errField: "Username",
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

func TestChangePassword_Validation(t *testing.T) {
	tests := []struct {
		name     string
		input    ChangePassword
		wantErr  bool
		errField string
	}{
		{
			name: "valid request",
			input: ChangePassword{
				Username:    "johndoe",
				OldPassword: "oldpassword",
				NewPassword: "NewSecurePassword123!",
			},
			wantErr: false,
		},
		{
			name: "missing username",
			input: ChangePassword{
				OldPassword: "oldpassword",
				NewPassword: "NewSecurePassword123!",
			},
			wantErr:  true,
			errField: "Username",
		},
		{
			name: "missing old password",
			input: ChangePassword{
				Username:    "johndoe",
				NewPassword: "NewSecurePassword123!",
			},
			wantErr:  true,
			errField: "OldPassword",
		},
		{
			name: "missing new password",
			input: ChangePassword{
				Username:    "johndoe",
				OldPassword: "oldpassword",
			},
			wantErr:  true,
			errField: "NewPassword",
		},
		{
			name: "new password too short",
			input: ChangePassword{
				Username:    "johndoe",
				OldPassword: "oldpassword",
				NewPassword: "short",
			},
			wantErr:  true,
			errField: "NewPassword",
		},
		{
			name: "new password exactly 8 chars",
			input: ChangePassword{
				Username:    "johndoe",
				OldPassword: "oldpassword",
				NewPassword: "12345678",
			},
			wantErr: false,
		},
		{
			name: "new password 7 chars fails",
			input: ChangePassword{
				Username:    "johndoe",
				OldPassword: "oldpassword",
				NewPassword: "1234567",
			},
			wantErr:  true,
			errField: "NewPassword",
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
