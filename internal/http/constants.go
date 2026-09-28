package http

import "time"

// HTTP server constants
const (
	// DefaultReadTimeout is the default timeout for reading requests
	DefaultReadTimeout = 30 * time.Second

	// DefaultWriteTimeout is the default timeout for writing responses
	DefaultWriteTimeout = 30 * time.Second

	// DefaultIdleTimeout is the default timeout for idle connections
	DefaultIdleTimeout = 120 * time.Second

	// DefaultShutdownTimeout is the default timeout for graceful shutdown
	DefaultShutdownTimeout = 10 * time.Second

	// DefaultMaxHeaderBytes is the default maximum size of request headers
	DefaultMaxHeaderBytes = 1 << 20 // 1 MB
)

// Pagination constants
const (
	// DefaultPageLimit is the default number of items per page
	DefaultPageLimit = 100

	// MaxPageLimit is the maximum number of items per page
	MaxPageLimit = 1000

	// DefaultPageOffset is the default page offset
	DefaultPageOffset = 0
)

// Content types
const (
	ContentTypeJSON           = "application/json"
	ContentTypeText           = "text/plain"
	ContentTypeHTML           = "text/html"
	ContentTypeFormURLEncoded = "application/x-www-form-urlencoded"
)

// HTTP methods
const (
	MethodGet     = "GET"
	MethodPost    = "POST"
	MethodPut     = "PUT"
	MethodDelete  = "DELETE"
	MethodPatch   = "PATCH"
	MethodOptions = "OPTIONS"
	MethodHead    = "HEAD"
)

// HTTP headers
const (
	HeaderContentType   = "Content-Type"
	HeaderAuthorization = "Authorization"
	HeaderAccept        = "Accept"
)

// Auth constants
const (
	BearerPrefix = "Bearer "
)

// Common error messages
const (
	ErrMethodNotAllowed     = "Method not allowed"
	ErrInternalServerError  = "Internal server error"
	ErrUnauthorized         = "Unauthorized"
	ErrPermissionDenied     = "Permission denied"
	ErrInvalidRequest       = "Invalid request"
	ErrTenantIDNotInContext = "Tenant ID not found in context"
)
