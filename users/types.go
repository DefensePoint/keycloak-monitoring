// Package users provides user repository interfaces and implementations.
package users

// CreateUserInput represents the input for creating a new user.
type CreateUserInput struct {
	Username string
	Email    string
	Password string
	Name     string
}

// UpdateUserInput represents the input for updating an existing user.
type UpdateUserInput struct {
	Email     *string
	Name      *string
	Username  *string
	Password  *string
	IsActive  *bool
	IsBlocked *bool
}
