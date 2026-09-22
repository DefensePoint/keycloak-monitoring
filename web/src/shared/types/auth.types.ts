export interface UserRole {
  id: number;
  user_id: number;
  role_id: number;
  role?: Role;
  tenant_id?: string | null;
  assigned_by?: string;
  assigned_at: string;
  expires_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface Role {
  id: number;
  name: string;
  display_name: string;
  description?: string;
  is_system: boolean;
  is_active: boolean;
  permissions?: Permission[];
  created_at: string;
  updated_at: string;
}

export interface Permission {
  id: number;
  name: string;
  display_name: string;
  description?: string;
  resource: string;
  action: string;
  is_system: boolean;
  created_at: string;
  updated_at: string;
}

export interface TenantPolicy {
  id: number;
  user_id: number;
  tenant_id: string;
  allowed_realms?: string[];
  granted_by?: string;
  granted_at: string;
  created_at: string;
  updated_at: string;
}

export type AuthMethod = "simple" | "oauth";

export interface UserInfo {
  subject: string;
  email: string;
  email_verified: boolean;
  name: string;
  given_name: string;
  family_name: string;
  preferred_username: string;
  locale: string;
  updated_at: string;
  auth_method: AuthMethod;
  // RBAC fields
  id?: number;
  roles?: UserRole[];
  permissions?: string[];
  tenant_policies?: TenantPolicy[];
}

export interface UserWithRBAC extends UserInfo {
  roles: UserRole[];
  permissions: string[];
  tenant_policies: TenantPolicy[];
}

export interface AuthConfig {
  simple_enabled: boolean;
  oauth2_enabled: boolean;
  auth_required: boolean;
}
