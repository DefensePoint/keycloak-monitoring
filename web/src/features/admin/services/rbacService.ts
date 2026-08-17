import axios from "axios";
import type {
  Permission,
  Role,
  User,
  RoleAssignment,
  CreateUserRequest,
  CreateRoleRequest,
  UpdateRoleRequest,
  AssignRoleRequest,
} from "../types";

const API_BASE_URL = import.meta.env.VITE_API_URL || "";

class RbacService {
  // Users API
  async getUsers(): Promise<User[]> {
    const response = await axios.get(`${API_BASE_URL}/api/users`, {
      withCredentials: true,
    });
    return response.data.users || [];
  }

  async getUser(userId: number): Promise<User> {
    const response = await axios.get(`${API_BASE_URL}/api/users/${userId}`, {
      withCredentials: true,
    });
    return response.data;
  }

  async createUser(user: CreateUserRequest): Promise<User> {
    const response = await axios.post(`${API_BASE_URL}/api/users`, user, {
      withCredentials: true,
    });
    return response.data;
  }

  async updateUser(userId: number, updates: Partial<User>): Promise<User> {
    const response = await axios.put(
      `${API_BASE_URL}/api/users/${userId}`,
      updates,
      {
        withCredentials: true,
      },
    );
    return response.data;
  }

  async deleteUser(userId: number): Promise<void> {
    await axios.delete(`${API_BASE_URL}/api/users/${userId}`, {
      withCredentials: true,
    });
  }

  // Roles API
  async getRoles(): Promise<Role[]> {
    const response = await axios.get(`${API_BASE_URL}/api/rbac/roles`, {
      withCredentials: true,
    });
    return response.data.roles || [];
  }

  async getRole(roleId: number): Promise<Role> {
    const response = await axios.get(
      `${API_BASE_URL}/api/rbac/roles/${roleId}`,
      {
        withCredentials: true,
      },
    );
    return response.data;
  }

  async createRole(role: CreateRoleRequest): Promise<Role> {
    const response = await axios.post(`${API_BASE_URL}/api/rbac/roles`, role, {
      withCredentials: true,
    });
    return response.data;
  }

  async updateRole(roleId: number, updates: UpdateRoleRequest): Promise<Role> {
    const response = await axios.put(
      `${API_BASE_URL}/api/rbac/roles/${roleId}`,
      updates,
      {
        withCredentials: true,
      },
    );
    return response.data;
  }

  async deleteRole(roleId: number): Promise<void> {
    await axios.delete(`${API_BASE_URL}/api/rbac/roles/${roleId}`, {
      withCredentials: true,
    });
  }

  // Permissions API
  async getPermissions(): Promise<Permission[]> {
    const response = await axios.get(`${API_BASE_URL}/api/rbac/permissions`, {
      withCredentials: true,
    });
    return response.data.permissions || [];
  }

  // Role Assignments API
  async getUserRoles(userId: number): Promise<RoleAssignment[]> {
    const response = await axios.get(
      `${API_BASE_URL}/api/rbac/users/${userId}/roles`,
      {
        withCredentials: true,
      },
    );
    return response.data.roles || [];
  }

  async assignRole(assignment: AssignRoleRequest): Promise<RoleAssignment> {
    const response = await axios.post(
      `${API_BASE_URL}/api/rbac/users/${assignment.user_id}/roles`,
      assignment,
      {
        withCredentials: true,
      },
    );
    return response.data;
  }

  async revokeRoleAssignment(
    userId: number,
    roleId: number,
    tenantId?: string,
  ): Promise<void> {
    const url = `${API_BASE_URL}/api/rbac/users/${userId}/roles/${roleId}`;
    const params = tenantId ? { tenant_id: tenantId } : {};
    await axios.delete(url, {
      params,
      withCredentials: true,
    });
  }

  async checkPermission(
    userId: number,
    permission: string,
    tenantId?: string,
  ): Promise<boolean> {
    const params = tenantId ? { tenant_id: tenantId } : {};
    const response = await axios.get(
      `${API_BASE_URL}/api/rbac/users/${userId}/permissions/${permission}`,
      {
        params,
        withCredentials: true,
      },
    );
    return response.data.has_permission || false;
  }

  async isUserAdmin(userId: number): Promise<boolean> {
    const response = await axios.get(
      `${API_BASE_URL}/api/rbac/users/${userId}/is-admin`,
      {
        withCredentials: true,
      },
    );
    return response.data.is_admin || false;
  }
}

export const rbacService = new RbacService();
