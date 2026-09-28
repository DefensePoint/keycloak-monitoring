export interface Permission {
  id: number;
  name: string;
  display_name: string;
  description: string;
  resource: string;
  action: string;
}

export interface Role {
  id: number;
  name: string;
  display_name: string;
  description: string;
  is_system: boolean;
  permissions?: Permission[];
  created_at?: string;
  updated_at?: string;
}

export type AuthMethod = "simple" | "oauth";

export interface User {
  id: number;
  subject: string;
  email: string;
  email_verified: boolean;
  name: string;
  given_name?: string;
  family_name?: string;
  preferred_username: string;
  locale?: string;
  auth_method: AuthMethod;
  is_active: boolean;
  is_blocked: boolean;
  must_change_password: boolean;
  last_accessed?: string;
  created_at: string;
  updated_at: string;
}

export interface RoleAssignment {
  id: number;
  user_id: number;
  role_id: number;
  tenant_id?: string;
  assigned_by: string;
  assigned_at: string;
  expires_at?: string;
  role?: Role;
}

export interface CreateUserRequest {
  username: string;
  password: string;
  email: string;
  name: string;
}

export interface CreateRoleRequest {
  name: string;
  display_name: string;
  description: string;
  permission_ids: number[];
}

export interface UpdateRoleRequest {
  display_name?: string;
  description?: string;
  permission_ids?: number[];
}

export interface AssignRoleRequest {
  user_id: number;
  role_id: number;
  tenant_id?: string;
  assigned_by?: string; // Optional - backend sets from auth context if not provided
  expires_at?: string;
}

export interface UserWithRoles extends User {
  roleAssignments?: RoleAssignment[];
}
