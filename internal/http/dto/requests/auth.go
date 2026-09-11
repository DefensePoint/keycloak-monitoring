package requests

// SimpleLogin represents the request for simple username/password login.
//
//	@Description	Request body for simple authentication
type SimpleLogin struct {
	Username string `json:"username" validate:"required,min=1" example:"johndoe"`
	Password string `json:"password" validate:"required,min=1" example:"mypassword"`
}

// ChangePassword represents the request to change password.
//
//	@Description	Request body for password change
type ChangePassword struct {
	Username    string `json:"username" validate:"required,min=1" example:"johndoe"`
	OldPassword string `json:"old_password" validate:"required,min=1" example:"oldpassword"`
	NewPassword string `json:"new_password" validate:"required,min=8" example:"newSecurePassword123!"`
}
