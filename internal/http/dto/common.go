// Package dto contains Data Transfer Objects for HTTP API layer.
// DTOs are responsible for request validation and response serialization.
package dto

// ErrorResponse represents a standardized error response.
//
//	@Description	Standard error response with optional validation details
type ErrorResponse struct {
	Error   string            `json:"error" example:"Invalid request"`
	Details map[string]string `json:"details,omitempty" swaggertype:"object"`
}

// MessageResponse represents a simple success message.
//
//	@Description	Simple success message response
type MessageResponse struct {
	Message string `json:"message" example:"Operation completed successfully"`
}

// PaginatedMeta represents pagination metadata.
//
//	@Description	Pagination metadata for list responses
type PaginatedMeta struct {
	Total  int64 `json:"total" example:"100"`
	Limit  int   `json:"limit" example:"10"`
	Offset int   `json:"offset" example:"0"`
}

// SuccessResponse represents a generic success response.
//
//	@Description	Generic success response wrapper
type SuccessResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    interface{} `json:"data,omitempty"`
}
