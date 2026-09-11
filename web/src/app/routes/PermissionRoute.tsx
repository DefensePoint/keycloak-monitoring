import { ReactNode } from "react";
import { usePermission, useIsAdmin } from "@/shared/hooks";
import { AccessDeniedPage, LoadingSkeleton } from "@/shared/components";

interface PermissionRouteProps {
  permission: string | string[];
  children: ReactNode;
}

/**
 * PermissionRoute - Route guard that checks for specific permissions
 *
 * Displays a 403 Access Denied page if the user doesn't have the required permission.
 * Admin users bypass all permission checks.
 */
export function PermissionRoute({
  permission,
  children,
}: PermissionRouteProps) {
  const { hasPermission, isLoading: permissionLoading } =
    usePermission(permission);
  const { isAdmin, isLoading: adminLoading } = useIsAdmin();

  const isLoading = permissionLoading || adminLoading;

  if (isLoading) {
    return (
      <LoadingSkeleton variant="spinner" message="Checking permissions..." />
    );
  }

  if (isAdmin) {
    return <>{children}</>;
  }

  if (!hasPermission) {
    return (
      <AccessDeniedPage
        title="Permission Required"
        message="You don't have permission to access this page. Contact your administrator."
      />
    );
  }

  return <>{children}</>;
}
