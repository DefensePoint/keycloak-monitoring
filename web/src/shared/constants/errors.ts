/**
 * API Error Message Constants
 *
 * Centralized error messages for API operations to maintain consistency
 * across the application and facilitate localization.
 */

// Fetch operation error messages
export const API_ERRORS = {
  // Event operations
  FETCH_EVENTS: "Failed to fetch events",
  FETCH_STATS: "Failed to fetch stats",

  // Keycloak operations
  FETCH_KEYCLOAK_DASHBOARD: "Failed to fetch Keycloak dashboard",
  FETCH_KEYCLOAK_HEALTH: "Failed to fetch Keycloak health",
  FETCH_VERSION: "Failed to fetch version",

  // Realm operations
  FETCH_REALM_EVENTS: "Failed to fetch realm events",
  FETCH_REALM_USERS: "Failed to fetch realm users",
  FETCH_REALM_CLIENTS: "Failed to fetch realm clients",
  FETCH_REALMS: "Failed to fetch realms",
  FETCH_REALM: "Failed to fetch realm",

  // Alert operations
  FETCH_ALERT_STATS: "Failed to fetch alert stats",
  FETCH_ALERTS: "Failed to fetch alerts",
  FETCH_ALERT: "Failed to fetch alert",
  UPDATE_ALERT_STATUS: "Failed to update alert status",
  RESOLVE_ALERT: "Failed to resolve alert",
  ACKNOWLEDGE_ALERT: "Failed to acknowledge alert",
  IGNORE_ALERT: "Failed to ignore alert",

  // User operations
  FETCH_USER_DETAILS: "Failed to fetch user details",
  FETCH_USERS: "Failed to fetch users",
  CREATE_USER: "Failed to create user",
  UPDATE_USER: "Failed to update user",
  DELETE_USER: "Failed to delete user",

  // Tenant operations
  FETCH_TENANTS: "Failed to fetch tenants",
  FETCH_TENANT: "Failed to fetch tenant",
  FETCH_TENANT_HEALTH: "Failed to fetch tenant health",
  CREATE_TENANT: "Failed to create tenant",
  UPDATE_TENANT: "Failed to update tenant",
  DELETE_TENANT: "Failed to delete tenant",

  // Role operations
  FETCH_ROLES: "Failed to fetch roles",
  FETCH_ROLE: "Failed to fetch role",
  CREATE_ROLE: "Failed to create role",
  UPDATE_ROLE: "Failed to update role",
  DELETE_ROLE: "Failed to delete role",

  // Permission operations
  FETCH_PERMISSIONS: "Failed to fetch permissions",
  FETCH_PERMISSION: "Failed to fetch permission",
  UPDATE_PERMISSIONS: "Failed to update permissions",

  // Client operations
  FETCH_CLIENTS: "Failed to fetch clients",
  FETCH_CLIENT: "Failed to fetch client",
  CREATE_CLIENT: "Failed to create client",
  UPDATE_CLIENT: "Failed to update client",
  DELETE_CLIENT: "Failed to delete client",

  // General errors
  LOAD_DATA: "Failed to load data",
  SAVE_DATA: "Failed to save data",
  PARSE_RESPONSE: "Failed to parse response",
  NETWORK_ERROR: "Network error occurred",
} as const;

// Validation error messages
export const VALIDATION_ERRORS = {
  INVALID_EMAIL: "Invalid email address",
  INVALID_PASSWORD: "Invalid password",
  REQUIRED_FIELD: "This field is required",
  INVALID_FORMAT: "Invalid format",
  PASSWORDS_DONT_MATCH: "Passwords do not match",
  INVALID_TENANT_ID: "Invalid tenant ID",
  INVALID_USER_ID: "Invalid user ID",
  INVALID_ROLE_ID: "Invalid role ID",
} as const;

// Success messages
export const SUCCESS_MESSAGES = {
  ALERT_ACKNOWLEDGED: "Alert acknowledged successfully",
  ALERT_RESOLVED: "Alert resolved successfully",
  ALERT_IGNORED: "Alert ignored successfully",
  TENANT_CREATED: "Tenant created successfully",
  TENANT_UPDATED: "Tenant updated successfully",
  TENANT_DELETED: "Tenant deleted successfully",
  USER_CREATED: "User created successfully",
  USER_UPDATED: "User updated successfully",
  USER_DELETED: "User deleted successfully",
  ROLE_CREATED: "Role created successfully",
  ROLE_UPDATED: "Role updated successfully",
  ROLE_DELETED: "Role deleted successfully",
  CHANGES_SAVED: "Changes saved successfully",
} as const;

// Error Boundary messages
export const BOUNDARY_ERRORS = {
  // Global level
  GLOBAL_TITLE: "Application Error",
  GLOBAL_DESCRIPTION:
    "An unexpected error occurred. The application needs to be reloaded.",

  // Route level
  ROUTE_TITLE: "Page Error",
  ROUTE_DESCRIPTION:
    "This page couldn't load properly. You can try again or navigate to a different page.",
  ROUTE_GENERIC: "Failed to load page content",

  // Feature level
  FEATURE_TITLE: "Component Error",
  FEATURE_DESCRIPTION: "This section couldn't load properly.",
  FEATURE_GENERIC: "This component encountered an error",

  // Generic
  GENERIC_ERROR: "An unexpected error occurred",
} as const;

// Type exports for TypeScript
export type ApiError = (typeof API_ERRORS)[keyof typeof API_ERRORS];
export type ValidationError =
  (typeof VALIDATION_ERRORS)[keyof typeof VALIDATION_ERRORS];
export type SuccessMessage =
  (typeof SUCCESS_MESSAGES)[keyof typeof SUCCESS_MESSAGES];
export type BoundaryError =
  (typeof BOUNDARY_ERRORS)[keyof typeof BOUNDARY_ERRORS];
