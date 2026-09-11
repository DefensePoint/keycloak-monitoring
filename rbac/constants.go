package rbac

// Permission constants for the system.
// Format: resource:action
const (
	// Tenant permissions
	PermissionTenantsRead   = "tenants:read"
	PermissionTenantsWrite  = "tenants:write"
	PermissionTenantsDelete = "tenants:delete"

	// Realm permissions
	PermissionRealmsRead   = "realms:read"
	PermissionRealmsWrite  = "realms:write"
	PermissionRealmsDelete = "realms:delete"

	// User permissions (Keycloak users, not platform users)
	PermissionUsersRead   = "users:read"
	PermissionUsersWrite  = "users:write"
	PermissionUsersDelete = "users:delete"

	// Client permissions
	PermissionClientsRead   = "clients:read"
	PermissionClientsWrite  = "clients:write"
	PermissionClientsDelete = "clients:delete"

	// Event permissions
	PermissionEventsRead = "events:read"

	// Alert permissions
	PermissionAlertsRead        = "alerts:read"
	PermissionAlertsAcknowledge = "alerts:acknowledge"
	PermissionAlertsResolve     = "alerts:resolve"
	PermissionAlertsWrite       = "alerts:write"
	PermissionAlertsDelete      = "alerts:delete"

	// Metric permissions
	PermissionMetricsRead = "metrics:read"

	// AMFA (Adaptive MFA) permissions
	PermissionAmfaRead = "amfa:read"

	// Keycloak general permissions
	PermissionKeycloakRead = "keycloak:read"

	// Role management permissions
	PermissionRolesRead   = "roles:read"
	PermissionRolesWrite  = "roles:write"
	PermissionRolesDelete = "roles:delete"
	PermissionRolesAssign = "roles:assign"

	// Permission management
	PermissionPermissionsRead   = "permissions:read"
	PermissionPermissionsWrite  = "permissions:write"
	PermissionPermissionsDelete = "permissions:delete"

	// Platform user management
	PermissionPlatformUsersRead   = "platform_users:read"
	PermissionPlatformUsersWrite  = "platform_users:write"
	PermissionPlatformUsersDelete = "platform_users:delete"

	// Policy management
	PermissionPoliciesRead   = "policies:read"
	PermissionPoliciesWrite  = "policies:write"
	PermissionPoliciesDelete = "policies:delete"

	// System configuration
	PermissionSystemConfigRead  = "system_config:read"
	PermissionSystemConfigWrite = "system_config:write"
)

// Role constants
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

// RoleDefinition represents a role with its permissions.
type RoleDefinition struct {
	Name        string
	DisplayName string
	Description string
	Permissions []string
}

// GetSystemRoles returns the definitions of system roles.
func GetSystemRoles() []RoleDefinition {
	return []RoleDefinition{
		{
			Name:        RoleAdmin,
			DisplayName: "Administrator",
			Description: "Full system access - can manage all resources, users, and configurations",
			Permissions: []string{
				PermissionTenantsRead, PermissionTenantsWrite, PermissionTenantsDelete,
				PermissionRealmsRead, PermissionRealmsWrite, PermissionRealmsDelete,
				PermissionUsersRead, PermissionUsersWrite, PermissionUsersDelete,
				PermissionClientsRead, PermissionClientsWrite, PermissionClientsDelete,
				PermissionEventsRead,
				PermissionAlertsRead, PermissionAlertsAcknowledge, PermissionAlertsResolve, PermissionAlertsWrite, PermissionAlertsDelete,
				PermissionMetricsRead, PermissionAmfaRead, PermissionKeycloakRead,
				PermissionRolesRead, PermissionRolesWrite, PermissionRolesDelete, PermissionRolesAssign,
				PermissionPermissionsRead, PermissionPermissionsWrite, PermissionPermissionsDelete,
				PermissionPlatformUsersRead, PermissionPlatformUsersWrite, PermissionPlatformUsersDelete,
				PermissionPoliciesRead, PermissionPoliciesWrite, PermissionPoliciesDelete,
				PermissionSystemConfigRead, PermissionSystemConfigWrite,
			},
		},
		{
			Name:        RoleOperator,
			DisplayName: "Operator (SOC Analyst)",
			Description: "Can view and act on alerts/events within assigned tenants and realms",
			Permissions: []string{
				PermissionTenantsRead, PermissionRealmsRead,
				PermissionUsersRead, PermissionClientsRead,
				PermissionEventsRead,
				PermissionAlertsRead, PermissionAlertsAcknowledge, PermissionAlertsResolve,
				PermissionMetricsRead, PermissionAmfaRead, PermissionKeycloakRead,
			},
		},
		{
			Name:        RoleViewer,
			DisplayName: "Viewer",
			Description: "Read-only access to assigned tenants and realms",
			Permissions: []string{
				PermissionTenantsRead, PermissionRealmsRead,
				PermissionUsersRead, PermissionClientsRead,
				PermissionEventsRead, PermissionAlertsRead,
				PermissionMetricsRead, PermissionAmfaRead, PermissionKeycloakRead,
			},
		},
	}
}

// PermissionDefinition represents a permission with its metadata.
type PermissionDefinition struct {
	Name        string
	DisplayName string
	Description string
	Resource    string
	Action      string
}

// GetSystemPermissions returns all system permission definitions.
func GetSystemPermissions() []PermissionDefinition {
	return []PermissionDefinition{
		// Tenant permissions
		{Name: PermissionTenantsRead, DisplayName: "Read Tenants", Description: "View tenant information", Resource: "tenants", Action: "read"},
		{Name: PermissionTenantsWrite, DisplayName: "Write Tenants", Description: "Create and modify tenants", Resource: "tenants", Action: "write"},
		{Name: PermissionTenantsDelete, DisplayName: "Delete Tenants", Description: "Delete tenants", Resource: "tenants", Action: "delete"},

		// Realm permissions
		{Name: PermissionRealmsRead, DisplayName: "Read Realms", Description: "View realm information", Resource: "realms", Action: "read"},
		{Name: PermissionRealmsWrite, DisplayName: "Write Realms", Description: "Modify realm configurations", Resource: "realms", Action: "write"},
		{Name: PermissionRealmsDelete, DisplayName: "Delete Realms", Description: "Delete realms", Resource: "realms", Action: "delete"},

		// User permissions
		{Name: PermissionUsersRead, DisplayName: "Read Users", Description: "View Keycloak user information", Resource: "users", Action: "read"},
		{Name: PermissionUsersWrite, DisplayName: "Write Users", Description: "Create and modify Keycloak users", Resource: "users", Action: "write"},
		{Name: PermissionUsersDelete, DisplayName: "Delete Users", Description: "Delete Keycloak users", Resource: "users", Action: "delete"},

		// Client permissions
		{Name: PermissionClientsRead, DisplayName: "Read Clients", Description: "View client information", Resource: "clients", Action: "read"},
		{Name: PermissionClientsWrite, DisplayName: "Write Clients", Description: "Create and modify clients", Resource: "clients", Action: "write"},
		{Name: PermissionClientsDelete, DisplayName: "Delete Clients", Description: "Delete clients", Resource: "clients", Action: "delete"},

		// Event permissions
		{Name: PermissionEventsRead, DisplayName: "Read Events", Description: "View event logs", Resource: "events", Action: "read"},

		// Alert permissions
		{Name: PermissionAlertsRead, DisplayName: "Read Alerts", Description: "View alerts", Resource: "alerts", Action: "read"},
		{Name: PermissionAlertsAcknowledge, DisplayName: "Acknowledge Alerts", Description: "Acknowledge alerts", Resource: "alerts", Action: "acknowledge"},
		{Name: PermissionAlertsResolve, DisplayName: "Resolve Alerts", Description: "Resolve alerts", Resource: "alerts", Action: "resolve"},
		{Name: PermissionAlertsWrite, DisplayName: "Write Alerts", Description: "Create and modify alerts", Resource: "alerts", Action: "write"},
		{Name: PermissionAlertsDelete, DisplayName: "Delete Alerts", Description: "Delete alerts", Resource: "alerts", Action: "delete"},

		// Metric permissions
		{Name: PermissionMetricsRead, DisplayName: "Read Metrics", Description: "View metrics and dashboards", Resource: "metrics", Action: "read"},

		// AMFA (Adaptive MFA) permissions
		{Name: PermissionAmfaRead, DisplayName: "Read AMFA Events", Description: "View AMFA (Adaptive MFA) events, risk metrics, and geolocation data", Resource: "amfa", Action: "read"},

		// Keycloak general permissions
		{Name: PermissionKeycloakRead, DisplayName: "Read Keycloak Data", Description: "View Keycloak health, metrics, and general information", Resource: "keycloak", Action: "read"},

		// Role management permissions
		{Name: PermissionRolesRead, DisplayName: "Read Roles", Description: "View role information", Resource: "roles", Action: "read"},
		{Name: PermissionRolesWrite, DisplayName: "Write Roles", Description: "Create and modify roles", Resource: "roles", Action: "write"},
		{Name: PermissionRolesDelete, DisplayName: "Delete Roles", Description: "Delete roles", Resource: "roles", Action: "delete"},
		{Name: PermissionRolesAssign, DisplayName: "Assign Roles", Description: "Assign roles to users", Resource: "roles", Action: "assign"},

		// Permission management
		{Name: PermissionPermissionsRead, DisplayName: "Read Permissions", Description: "View permission information", Resource: "permissions", Action: "read"},
		{Name: PermissionPermissionsWrite, DisplayName: "Write Permissions", Description: "Create and modify permissions", Resource: "permissions", Action: "write"},
		{Name: PermissionPermissionsDelete, DisplayName: "Delete Permissions", Description: "Delete permissions", Resource: "permissions", Action: "delete"},

		// Platform user management
		{Name: PermissionPlatformUsersRead, DisplayName: "Read Platform Users", Description: "View platform user information", Resource: "platform_users", Action: "read"},
		{Name: PermissionPlatformUsersWrite, DisplayName: "Write Platform Users", Description: "Create and modify platform users", Resource: "platform_users", Action: "write"},
		{Name: PermissionPlatformUsersDelete, DisplayName: "Delete Platform Users", Description: "Delete platform users", Resource: "platform_users", Action: "delete"},

		// Policy management
		{Name: PermissionPoliciesRead, DisplayName: "Read Policies", Description: "View tenant access policies", Resource: "policies", Action: "read"},
		{Name: PermissionPoliciesWrite, DisplayName: "Write Policies", Description: "Create and modify tenant access policies", Resource: "policies", Action: "write"},
		{Name: PermissionPoliciesDelete, DisplayName: "Delete Policies", Description: "Delete tenant access policies", Resource: "policies", Action: "delete"},

		// System configuration
		{Name: PermissionSystemConfigRead, DisplayName: "Read System Config", Description: "View system configuration", Resource: "system_config", Action: "read"},
		{Name: PermissionSystemConfigWrite, DisplayName: "Write System Config", Description: "Modify system configuration", Resource: "system_config", Action: "write"},
	}
}
