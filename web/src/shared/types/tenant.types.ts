// TenantAmfa holds a tenant's AMFA integration settings. Only the API endpoint
// is carried: reading AMFA over HTTP needs no database credentials.
export interface TenantAmfa {
  enabled: boolean;
  api_base_url: string;
  events_lookback_days?: number;
  api_timeout_seconds?: number;
}

// Tenant represents a Keycloak instance being monitored
export interface Tenant {
  id: number;
  tenant_id: string;
  name: string;
  description?: string;
  server_url: string;
  admin_realm: string;
  // Note: client_secret is never sent from the backend
  client_id?: string;
  configuration?: string;
  default_realm?: string;
  enabled: boolean;
  last_health_check: string;
  health_status: string;
  health_message?: string;
  last_error?: string;
  last_error_at?: string;
  is_default: boolean;
  // Managed via config.yaml + restart; treat as read-only in the UI.
  is_config_defined: boolean;
  tags?: string[];
  owner?: string;
  infinispan_enabled?: boolean;
  amfa?: TenantAmfa;
  created_at: string;
  updated_at: string;
}

// TenantCreate represents the payload for creating a new tenant
export interface TenantCreate {
  tenant_id: string;
  name: string;
  description?: string;
  server_url: string;
  admin_realm: string;
  client_id: string;
  client_secret: string;
  configuration?: string;
  default_realm?: string;
  enabled: boolean;
  is_default?: boolean;
  tags?: string[];
  owner?: string;
  amfa?: TenantAmfa;
}

// TenantUpdate represents the payload for updating a tenant
// All fields are optional to support partial updates
export interface TenantUpdate {
  name?: string;
  description?: string;
  server_url?: string;
  admin_realm?: string;
  client_id?: string;
  client_secret?: string;
  configuration?: string;
  default_realm?: string;
  enabled?: boolean;
  is_default?: boolean;
  tags?: string[];
  owner?: string;
  amfa?: TenantAmfa;
}

// TenantsResponse represents the API response for listing tenants
export interface TenantsResponse {
  tenants: Tenant[];
  count: number;
}

// TenantHealth represents the health status of a tenant
export interface TenantHealth {
  tenant_id: string;
  health_status: string;
  health_message?: string;
  last_health_check: string;
  last_error?: string;
  last_error_at?: string;
}
