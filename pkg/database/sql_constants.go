package database

// SQL WHERE clause constants for common query patterns
const (
	// Primary key conditions
	SQLWhereID       = "id = ?"
	SQLWhereEventID  = "event_id = ?"
	SQLWhereAlertID  = "alert_id = ?"
	SQLWhereUserID   = "user_id = ?"
	SQLWhereRoleID   = "role_id = ?"
	SQLWhereTenantID = "tenant_id = ?"

	// Named field conditions
	SQLWhereRealmName = "realm_name = ?"
	SQLWhereSubject   = "subject = ?"
	SQLWhereStatus    = "status = ?"

	// Combined conditions
	SQLWhereTenantIDPrefix       = "tenant_id = ? AND "
	SQLWhereTenantIDAndStatus    = "tenant_id = ? AND status = ?"
	SQLWhereTenantIDAndAlertID   = "tenant_id = ? AND alert_id = ?"
	SQLWhereTenantIDAndID        = "tenant_id = ? AND id = ?"
	SQLWhereTenantIDAndRealmName = "tenant_id = ? AND realm_name = ?"
)

// SQL ORDER BY clause constants
const (
	SQLOrderByTimeDesc      = "time DESC"
	SQLOrderByCreatedAtDesc = "created_at DESC"
	SQLOrderByUpdatedAtDesc = "updated_at DESC"
)

// Common column names
const (
	ColTenantID    = "tenant_id"
	ColUserID      = "user_id"
	ColRoleID      = "role_id"
	ColID          = "id"
	ColCreatedAt   = "created_at"
	ColUpdatedAt   = "updated_at"
	ColDeletedAt   = "deleted_at"
	ColIsActive    = "is_active"
	ColName        = "name"
	ColDescription = "description"
	ColEmail       = "email"
	ColUsername    = "username"
	ColStatus      = "status"
	ColRealmName   = "realm_name"
)

// Pagination parameter names
const (
	ParamLimit  = "limit"
	ParamOffset = "offset"
)
