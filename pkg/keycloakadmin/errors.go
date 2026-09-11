package keycloakadmin

import "errors"

// Configuration errors
var (
	ErrServerURLRequired    = errors.New("keycloak server URL is required")
	ErrAdminRealmRequired   = errors.New("admin realm is required")
	ErrClientIDRequired     = errors.New("client id is required")
	ErrClientSecretRequired = errors.New("client secret is required")
)

// Client errors
var (
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrNotFound             = errors.New("resource not found")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrForbidden            = errors.New("forbidden")
	ErrServerError          = errors.New("server error")
)
