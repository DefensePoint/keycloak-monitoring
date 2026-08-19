/**
 * Shared Keycloak type definitions
 * These types are used across multiple features (realms, users, admin, tenants, etc.)
 * Previously these were duplicated in individual feature modules with comments like
 * "redefined locally to avoid cross-feature imports" which created inconsistencies.
 */

// ============================================================================
// User Types
// ============================================================================

export interface KeycloakUser {
  id: string;
  createdTimestamp: number;
  username: string;
  enabled: boolean;
  emailVerified: boolean;
  firstName?: string;
  lastName?: string;
  email?: string;
  attributes?: Record<string, string[]>;
  requiredActions?: string[];
}

// ============================================================================
// Role Types
// ============================================================================

export interface KeycloakRole {
  id: string;
  name: string;
  description?: string;
  composite?: boolean;
  clientRole?: boolean;
  containerId?: string;
}

export interface RoleMappings {
  realmMappings?: KeycloakRole[];
  clientMappings?: Record<
    string,
    {
      client?: string;
      mappings: KeycloakRole[];
    }
  >;
}

// ============================================================================
// Group Types
// ============================================================================

export interface KeycloakGroup {
  id: string;
  name: string;
  path?: string;
  subGroups?: KeycloakGroup[];
}

// ============================================================================
// Client Types
// ============================================================================

export interface KeycloakClient {
  id: string;
  clientId: string;
  name?: string;
  description?: string;
  enabled: boolean;
  publicClient: boolean;
  bearerOnly: boolean;
  serviceAccountsEnabled: boolean;
  redirectUris?: string[];
  protocol?: string;
  standardFlowEnabled?: boolean;
  directAccessGrantsEnabled?: boolean;
  rootUrl?: string;
  baseUrl?: string;
  webOrigins?: string[];
}

// ============================================================================
// Event Types
// ============================================================================

export interface KeycloakEvent {
  id: number;
  event_id: string;
  timestamp: string;
  type: string;
  category: string;
  severity: string;
  source: string;
  source_ip?: string;
  source_system: string;
  description: string;
  user_id?: string;
  username?: string;
  email?: string;
  client_id?: string;
  location?: string;
  raw_data?: string;
  status: string;
  created_at: string;
  updated_at: string;
  // AMFA fields, present on login events that have an AMFA counterpart
  // (merged by the backend on amfa_event_id); absent on plain Keycloak events.
  amfa_event_id?: string;
  risk_level?: RiskLevel;
  final_status?: string;
  is_vpn?: boolean;
  country?: string;
  city?: string;
  lat?: number;
  long?: number;
  operating_system?: string;
  browser?: string;
  device?: string;
  system_language?: string;
  screen_resolution?: string;
}

/** Risk level from the AMFA decision engine (ACR semantics), 1 (low) .. 4 (rejected). */
export type RiskLevel = 1 | 2 | 3 | 4;

export interface EventsResponse {
  events: KeycloakEvent[];
  count: number;
  total: number;
}

// ============================================================================
// Alert Types
// ============================================================================

export type AlertSeverity = "info" | "warning" | "error" | "critical";
export type AlertStatus = "active" | "acknowledged" | "resolved" | "ignored";

export interface ConfigurationAlert {
  id: number;
  tenant_id: string;
  alert_id: string;
  source: string;
  type: string;
  severity: AlertSeverity;
  status: AlertStatus;
  title: string;
  description: string;
  recommendation: string;
  resource_type: string;
  resource_id: string;
  resource_name: string;
  realm_name: string;
  check_type: string;
  rule_id?: string;
  event_id?: string;
  metadata?: string;
  first_detected: string;
  last_seen: string;
  resolved_at?: string;
  acknowledged_at?: string;
  acknowledged_by?: string;
  created_at: string;
  updated_at: string;
}

// ============================================================================
// User Details Response
// ============================================================================

export interface UserDetailsResponse {
  user: KeycloakUser;
  groups: KeycloakGroup[];
  roleMappings: RoleMappings;
}

// ============================================================================
// API Response Types
// ============================================================================

export interface UsersResponse {
  realm: string;
  users: KeycloakUser[];
  count: number;
  first: number;
  max: number;
}

export interface ClientsResponse {
  realm: string;
  clients: KeycloakClient[];
  count: number;
}

// ============================================================================
// Health & Metrics Types
// ============================================================================

export interface KeycloakHealth {
  time: string;
  status: string;
  response_time_ms: number;
  server_version?: string;
  uptime_millis: number;
  memory_used_bytes: number;
  memory_max_bytes: number;
  memory_free_bytes: number;
  error_message?: string;
}

export interface KeycloakMetrics {
  time: string;
  realm_name: string;
  total_users: number;
  enabled_users: number;
  disabled_users: number;
  active_sessions: number;
  offline_sessions: number;
  total_clients: number;
  login_events: number;
  logout_events: number;
  failed_login_events: number;
  register_events: number;
}

export interface KeycloakRealmInfo {
  id: number;
  realm_id: string;
  realm_name: string;
  display_name?: string;
  enabled: boolean;
  is_healthy: boolean;
  health_message?: string;
  last_checked: string;
  ssl_required?: string;
  brute_force_protected: boolean;
  verify_email: boolean;
  reset_password_allowed: boolean;
  events_enabled: boolean;
  events_listeners?: string[];
  enabled_event_types?: string[];
  admin_events_enabled: boolean;
  admin_events_details_enabled: boolean;
}

export interface KeycloakVersionInfo {
  current_version: string;
  latest_keycloak_version: string;
  keycloak_release_url: string;
  is_keycloak_outdated: boolean;
  has_security_updates: boolean;
}

// ============================================================================
// Dashboard Types
// ============================================================================

export interface KeycloakDashboard {
  realm: string;
  health?: KeycloakHealth;
  metrics?: KeycloakMetrics;
  realm_info?: KeycloakRealmInfo;
  recent_events?: {
    logins: number;
    login_errors: number;
    time_period: string;
  };
  version_info?: KeycloakVersionInfo;
}

export interface RealmListItem {
  realm_name: string;
  enabled: boolean;
  is_healthy: boolean;
  events_enabled: boolean;
  events_listeners?: string[];
  metrics?: KeycloakMetrics;
}

export interface KeycloakDashboardAll {
  health?: KeycloakHealth;
  realms: RealmListItem[];
  total_realms: number;
  version_info?: KeycloakVersionInfo;
}
