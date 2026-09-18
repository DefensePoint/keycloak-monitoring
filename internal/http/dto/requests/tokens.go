package requests

// CreateAPIToken represents the request to create an API token.
//
//	@Description	Request body for API token creation
type CreateAPIToken struct {
	Name      string   `json:"name" validate:"required,min=1,max=255" example:"ci-pipeline"`
	UserID    uint     `json:"user_id" validate:"required,gt=0" example:"1"`
	TenantIDs []string `json:"tenant_ids" validate:"required,min=1,dive,min=1" example:"prod-main,customer-abc"`
	ExpiresAt *string  `json:"expires_at,omitempty" example:"2026-12-31T23:59:59Z"`
}
