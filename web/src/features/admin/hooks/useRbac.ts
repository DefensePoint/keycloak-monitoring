import {
  useQuery,
  useQueries,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { rbacService } from "../services";
import type { CreateRoleRequest, CreateUserRequest, User } from "../types";

// ============ Roles Hooks ============

export function useRoles() {
  return useQuery({
    queryKey: ["rbac", "roles"],
    queryFn: () => rbacService.getRoles(),
  });
}

export function useRole(roleId: number | undefined) {
  return useQuery({
    queryKey: ["rbac", "role", roleId],
    queryFn: () => rbacService.getRole(roleId!),
    enabled: !!roleId,
  });
}

export function useCreateRole() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateRoleRequest) => rbacService.createRole(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rbac", "roles"] });
    },
  });
}

export function useDeleteRole() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (roleId: number) => rbacService.deleteRole(roleId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rbac", "roles"] });
    },
  });
}

// ============ Permissions Hooks ============

export function usePermissions() {
  return useQuery({
    queryKey: ["rbac", "permissions"],
    queryFn: () => rbacService.getPermissions(),
  });
}

// ============ Users Hooks ============

export function useUsers() {
  return useQuery({
    queryKey: ["rbac", "users"],
    queryFn: () => rbacService.getUsers(),
  });
}

export function useCreateUser() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateUserRequest) => rbacService.createUser(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rbac", "users"] });
    },
  });
}

export function useUpdateUser() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      userId,
      updates,
    }: {
      userId: number;
      updates: Partial<User>;
    }) => rbacService.updateUser(userId, updates),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rbac", "users"] });
    },
  });
}

export function useDeleteUser() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (userId: number) => rbacService.deleteUser(userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rbac", "users"] });
    },
  });
}

// ============ Role Assignments Hooks ============

export function useUserRoles(userId: number | undefined) {
  return useQuery({
    queryKey: ["rbac", "user-roles", userId],
    queryFn: () => rbacService.getUserRoles(userId!),
    enabled: !!userId,
  });
}

export function useUsersRoles(users: { id: number }[]) {
  return useQueries({
    queries: users.map((user) => ({
      queryKey: ["rbac", "user-roles", user.id],
      queryFn: () => rbacService.getUserRoles(user.id),
    })),
  });
}

interface AssignRoleParams {
  user_id: number;
  role_id: number;
  tenant_id?: string;
}

export function useAssignRole() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (params: AssignRoleParams) => rbacService.assignRole(params),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["rbac", "user-roles", variables.user_id],
      });
      queryClient.invalidateQueries({ queryKey: ["rbac", "users"] });
    },
  });
}

export function useRevokeRole() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      userId,
      roleId,
      tenantId,
    }: {
      userId: number;
      roleId: number;
      tenantId?: string;
    }) => rbacService.revokeRoleAssignment(userId, roleId, tenantId),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["rbac", "user-roles", variables.userId],
      });
      queryClient.invalidateQueries({ queryKey: ["rbac", "users"] });
    },
  });
}
