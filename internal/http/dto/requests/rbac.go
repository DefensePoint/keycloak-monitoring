package requests

import "encoding/json"

// CreateRole represents the request to create a new role.
//
//	@Description	Request body for role creation
type CreateRole struct {
	Name        string   `json:"name" validate:"required,min=1,max=100" example:"analyst"`
	DisplayName string   `json:"display_name" validate:"required,min=1,max=255" example:"Security Analyst"`
	Description string   `json:"description" validate:"omitempty,max=1000" example:"Can view and acknowledge alerts"`
	Permissions []string `json:"permissions" validate:"omitempty" example:"alerts:read,alerts:acknowledge"`
	// IsActive creates the role inactive when false; omitted means active.
	IsActive *bool `json:"is_active,omitempty" example:"true"`
	// PermissionIDs is the shape an older UI sent. It was never read, so the
	// role was saved with no permissions; it is now refused instead.
	PermissionIDs json.RawMessage `json:"permission_ids,omitempty" swaggerignore:"true"`
}

// UpdateRole represents the request to update a role. Fields left out are
// unchanged. Permissions, when present, replaces the role's permissions: any
// not listed are removed, and [] removes all.
//
//	@Description	Request body for role update
type UpdateRole struct {
	DisplayName *string   `json:"display_name,omitempty" validate:"omitempty,min=1,max=255" example:"Updated Display Name"`
	Description *string   `json:"description,omitempty" validate:"omitempty,max=1000" example:"Updated description"`
	Permissions *[]string `json:"permissions,omitempty" validate:"omitempty" example:"alerts:read,alerts:write"`
	// IsActive deactivates or reactivates the role. An inactive role grants
	// nothing to its users until it is reactivated.
	IsActive *bool `json:"is_active,omitempty" example:"false"`
	// PermissionIDs is refused; see CreateRole.
	PermissionIDs json.RawMessage `json:"permission_ids,omitempty" swaggerignore:"true"`
}

// AssignRole represents the request to assign a role to a user.
//
//	@Description	Request body for role assignment
type AssignRole struct {
	RoleID    uint    `json:"role_id" validate:"required,gt=0" example:"1"`
	TenantID  *string `json:"tenant_id,omitempty" example:"my-keycloak"`
	ExpiresAt *string `json:"expires_at,omitempty" example:"2024-12-31T23:59:59Z"`
}

// CreateTenantPolicy represents the request to create a tenant policy.
//
//	@Description	Request body for tenant policy creation
type CreateTenantPolicy struct {
	TenantID string `json:"tenant_id" validate:"required,min=1" example:"my-keycloak"`
	// dive,min=1 rejects a blank entry: a realm naming no realm reads as "no
	// restriction" downstream and would widen the caller to the whole tenant.
	// omitempty keeps an absent list meaning unrestricted.
	AllowedRealms []string `json:"allowed_realms" validate:"omitempty,dive,min=1" example:"master,production"`
}
