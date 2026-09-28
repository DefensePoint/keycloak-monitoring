package responses

import "time"

// AuthConfig represents the authentication configuration response.
//
//	@Description	Authentication configuration showing enabled methods
type AuthConfig struct {
	SimpleEnabled bool `json:"simple_enabled" example:"true"`
	OAuth2Enabled bool `json:"oauth2_enabled" example:"true"`
	AuthRequired  bool `json:"auth_required" example:"true"`
}

// LoginSuccess represents a successful login response.
//
//	@Description	Successful login response with user information
type LoginSuccess struct {
	Success bool      `json:"success" example:"true"`
	User    *UserInfo `json:"user"`
}

// LoginFailure represents a failed login response.
//
//	@Description	Failed login response
type LoginFailure struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"Invalid username or password"`
}

// UserInfo represents authenticated user information.
//
//	@Description	User information from authentication
type UserInfo struct {
	ID                 uint      `json:"id" example:"1"`
	Subject            string    `json:"subject" example:"f47ac10b-58cc-4372-a567-0e02b2c3d479"`
	Email              string    `json:"email" example:"user@example.com"`
	EmailVerified      bool      `json:"email_verified" example:"true"`
	Name               string    `json:"name" example:"John Doe"`
	GivenName          string    `json:"given_name" example:"John"`
	FamilyName         string    `json:"family_name" example:"Doe"`
	PreferredUsername  string    `json:"preferred_username" example:"johndoe"`
	Locale             string    `json:"locale" example:"en"`
	UpdatedAt          time.Time `json:"updated_at"`
	MustChangePassword bool      `json:"must_change_password" example:"false"`
}

// TokenRefreshSuccess represents a successful token refresh response.
//
//	@Description	Successful token refresh response
type TokenRefreshSuccess struct {
	Status  string    `json:"status" example:"success"`
	Message string    `json:"message" example:"Token refreshed successfully"`
	Expiry  time.Time `json:"expiry"`
}

// PasswordChangeSuccess represents a successful password change response.
//
//	@Description	Successful password change response
type PasswordChangeSuccess struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Password changed successfully"`
}

// PasswordChangeFailure represents a failed password change response.
//
//	@Description	Failed password change response
type PasswordChangeFailure struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"Current password is incorrect"`
}

// PasswordRequirements represents password requirements response.
//
//	@Description	Password requirements information
type PasswordRequirements struct {
	Description string `json:"description" example:"Password must be at least 8 characters"`
	MinLength   int    `json:"min_length" example:"8"`
}
