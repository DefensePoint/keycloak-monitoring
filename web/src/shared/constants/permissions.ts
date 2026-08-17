/**
 * Permission Constants
 *
 * Centralized permission strings for authorization checks across the application.
 * These must match the permissions defined in the backend authorization system.
 */

// Keycloak permissions
export const KEYCLOAK_PERMISSIONS = {
  READ: "keycloak:read",
} as const;

// Realm permissions
export const REALM_PERMISSIONS = {
  READ: "realms:read",
  WRITE: "realms:write",
  DELETE: "realms:delete",
} as const;

// User permissions (Keycloak realm users)
export const USER_PERMISSIONS = {
  READ: "users:read",
  WRITE: "users:write",
  DELETE: "users:delete",
} as const;

// Event permissions
export const EVENT_PERMISSIONS = {
  READ: "events:read",
} as const;

// Alert permissions
export const ALERT_PERMISSIONS = {
  READ: "alerts:read",
  WRITE: "alerts:write",
  DELETE: "alerts:delete",
  ACKNOWLEDGE: "alerts:acknowledge",
  RESOLVE: "alerts:resolve",
} as const;

// Metrics permissions
export const METRICS_PERMISSIONS = {
  READ: "metrics:read",
} as const;

// Role permissions
export const ROLE_PERMISSIONS = {
  READ: "roles:read",
  WRITE: "roles:write",
  DELETE: "roles:delete",
  ASSIGN: "roles:assign",
} as const;

// Platform user permissions (admin users)
export const PLATFORM_USER_PERMISSIONS = {
  READ: "platform_users:read",
  WRITE: "platform_users:write",
  DELETE: "platform_users:delete",
} as const;

// AMFA permissions
export const AMFA_PERMISSIONS = {
  READ: "amfa:read",
} as const;

/**
 * All permissions grouped by resource
 */
export const PERMISSIONS = {
  KEYCLOAK: KEYCLOAK_PERMISSIONS,
  REALMS: REALM_PERMISSIONS,
  USERS: USER_PERMISSIONS,
  EVENTS: EVENT_PERMISSIONS,
  ALERTS: ALERT_PERMISSIONS,
  METRICS: METRICS_PERMISSIONS,
  ROLES: ROLE_PERMISSIONS,
  PLATFORM_USERS: PLATFORM_USER_PERMISSIONS,
  AMFA: AMFA_PERMISSIONS,
} as const;

// Type exports
export type KeycloakPermission =
  (typeof KEYCLOAK_PERMISSIONS)[keyof typeof KEYCLOAK_PERMISSIONS];
export type RealmPermission =
  (typeof REALM_PERMISSIONS)[keyof typeof REALM_PERMISSIONS];
export type UserPermission =
  (typeof USER_PERMISSIONS)[keyof typeof USER_PERMISSIONS];
export type EventPermission =
  (typeof EVENT_PERMISSIONS)[keyof typeof EVENT_PERMISSIONS];
export type AlertPermission =
  (typeof ALERT_PERMISSIONS)[keyof typeof ALERT_PERMISSIONS];
export type MetricsPermission =
  (typeof METRICS_PERMISSIONS)[keyof typeof METRICS_PERMISSIONS];
export type RolePermission =
  (typeof ROLE_PERMISSIONS)[keyof typeof ROLE_PERMISSIONS];
export type PlatformUserPermission =
  (typeof PLATFORM_USER_PERMISSIONS)[keyof typeof PLATFORM_USER_PERMISSIONS];
export type AmfaPermission =
  (typeof AMFA_PERMISSIONS)[keyof typeof AMFA_PERMISSIONS];

export type Permission =
  | KeycloakPermission
  | RealmPermission
  | UserPermission
  | EventPermission
  | AlertPermission
  | MetricsPermission
  | RolePermission
  | PlatformUserPermission
  | AmfaPermission;
