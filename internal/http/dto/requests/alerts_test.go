package requests

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestUpdateAlertStatus_Validation(t *testing.T) {
	tests := []struct {
		name     string
		input    UpdateAlertStatus
		wantErr  bool
		errField string
	}{
		{
			name: "valid status - active",
			input: UpdateAlertStatus{
				Status: "active",
			},
			wantErr: false,
		},
		{
			name: "valid status - resolved",
			input: UpdateAlertStatus{
				Status: "resolved",
			},
			wantErr: false,
		},
		{
			name: "valid status - acknowledged",
			input: UpdateAlertStatus{
				Status: "acknowledged",
			},
			wantErr: false,
		},
		{
			name: "valid status - ignored",
			input: UpdateAlertStatus{
				Status: "ignored",
			},
			wantErr: false,
		},
		{
			name: "valid status with acknowledged_by",
			input: UpdateAlertStatus{
				Status:         "acknowledged",
				AcknowledgedBy: "operator@example.com",
			},
			wantErr: false,
		},
		{
			name: "missing status",
			input: UpdateAlertStatus{
				AcknowledgedBy: "operator@example.com",
			},
			wantErr:  true,
			errField: "Status",
		},
		{
			name:     "empty status",
			input:    UpdateAlertStatus{},
			wantErr:  true,
			errField: "Status",
		},
		{
			name: "invalid status value",
			input: UpdateAlertStatus{
				Status: "invalid-status",
			},
			wantErr:  true,
			errField: "Status",
		},
		{
			name: "status with wrong case",
			input: UpdateAlertStatus{
				Status: "ACTIVE",
			},
			wantErr:  true,
			errField: "Status",
		},
		{
			name: "acknowledged_by too long",
			input: UpdateAlertStatus{
				Status:         "acknowledged",
				AcknowledgedBy: strings.Repeat("a", 256),
			},
			wantErr:  true,
			errField: "AcknowledgedBy",
		},
		{
			name: "acknowledged_by at max length",
			input: UpdateAlertStatus{
				Status:         "acknowledged",
				AcknowledgedBy: strings.Repeat("a", 255),
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
