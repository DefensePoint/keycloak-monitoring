import { useAuth } from "@/shared/context";

/**
 * Hook for checking user permissions
 * @param permission - Single permission or array of permissions to check
 */
export function usePermission(permission: string | string[]) {
  const { hasPermission, hasAnyPermission, isLoading } = useAuth();

  if (Array.isArray(permission)) {
    return {
      hasPermission: hasAnyPermission(permission),
      isLoading,
    };
  }

  return {
    hasPermission: hasPermission(permission),
    isLoading,
  };
}

/**
 * Hook for checking if user has a specific role
 * @param roleName - Role name to check
 * @param tenantId - Optional tenant ID for tenant-scoped roles
 */
export function useRole(roleName: string, tenantId?: string) {
  const { hasRole, isLoading } = useAuth();

  return {
    hasRole: hasRole(roleName, tenantId),
    isLoading,
  };
}

/**
 * Hook for getting all user roles
 */
export function useUserRoles() {
  const { user, isLoading } = useAuth();

  return {
    roles: user?.roles || [],
    isLoading,
  };
}

/**
 * Hook for getting all user permissions
 */
export function useUserPermissions() {
  const { user, isLoading } = useAuth();

  return {
    permissions: user?.permissions || [],
    isLoading,
  };
}

/**
 * Hook for checking if user is an admin
 */
export function useIsAdmin() {
  const { isAdmin, isLoading } = useAuth();

  return {
    isAdmin: isAdmin(),
    isLoading,
  };
}

/**
 * Hook for checking tenant access
 * @param tenantId - Tenant ID to check access for
 */
export function useTenantAccess(tenantId: string) {
  const { hasAccessToTenant, isLoading } = useAuth();

  return {
    hasAccess: tenantId ? hasAccessToTenant(tenantId) : false,
    isLoading,
  };
}
