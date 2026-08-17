package requests

// CreateUser represents the request to create a new user.
//
//	@Description	Request body for user creation
type CreateUser struct {
	Username string `json:"username" validate:"required,min=3,max=50" example:"johndoe"`
	Email    string `json:"email" validate:"required,email" example:"john@example.com"`
	Password string `json:"password" validate:"required,min=8" example:"SecurePass123!"`
	Name     string `json:"name" validate:"omitempty,max=100" example:"John Doe"`
}

// UpdateUser represents the request to update a user.
//
//	@Description	Request body for user update (all fields optional)
type UpdateUser struct {
	Email     *string `json:"email,omitempty" validate:"omitempty,email" example:"newemail@example.com"`
	Name      *string `json:"name,omitempty" validate:"omitempty,max=100" example:"New Name"`
	Username  *string `json:"username,omitempty" validate:"omitempty,min=3,max=50" example:"newusername"`
	Password  *string `json:"password,omitempty" validate:"omitempty,min=8" example:"NewSecurePass123!"`
	IsActive  *bool   `json:"is_active,omitempty" example:"true"`
	IsBlocked *bool   `json:"is_blocked,omitempty" example:"false"`
}
