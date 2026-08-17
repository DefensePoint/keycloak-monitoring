import { ReactNode } from "react";
import { usePermission, useRole, useIsAdmin } from "@/shared/hooks";

interface PermissionGateProps {
  children: ReactNode;
  permission?: string | string[];
  role?: string;
  requireAdmin?: boolean;
  fallback?: ReactNode;
  any?: boolean;
}

/**
 * Component that conditionally renders children based on user permissions/roles
 */
export function PermissionGate({
  children,
  permission,
  role,
  requireAdmin,
  fallback = null,
}: PermissionGateProps) {
  const permissionCheck = usePermission(permission || "");
  const roleCheck = useRole(role || "");
  const adminCheck = useIsAdmin();

  // ADMIN BYPASS: Admins can do anything
  if (!adminCheck.isLoading && adminCheck.isAdmin) {
    return <>{children}</>;
  }

  // If requireAdmin is specified, check admin status
  if (requireAdmin) {
    if (adminCheck.isLoading) {
      return null;
    }
    return adminCheck.isAdmin ? <>{children}</> : <>{fallback}</>;
  }

  // If role is specified, check role
  if (role) {
    if (adminCheck.isLoading || roleCheck.isLoading) {
      return null;
    }
    if (adminCheck.isAdmin) {
      return <>{children}</>;
    }
    return roleCheck.hasRole ? <>{children}</> : <>{fallback}</>;
  }

  // If permission is specified, check permission
  if (permission) {
    if (adminCheck.isLoading || permissionCheck.isLoading) {
      return null;
    }
    if (adminCheck.isAdmin) {
      return <>{children}</>;
    }
    return permissionCheck.hasPermission ? <>{children}</> : <>{fallback}</>;
  }

  // No restrictions specified, render children
  return <>{children}</>;
}

/**
 * Hook version of PermissionGate for conditional rendering in components
 */
export function useHasPermission(permission: string | string[]) {
  const { hasPermission, isLoading } = usePermission(permission);
  return { hasPermission, isLoading };
}

export function useHasRole(role: string) {
  const { hasRole, isLoading } = useRole(role);
  return { hasRole, isLoading };
}
