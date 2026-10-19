import { ReactNode } from "react";
import { useTenant } from "@/shared/context";
import { AccessDeniedPage } from "@/shared/components/AccessDenied";

interface TenantAccessGuardProps {
  children: ReactNode;
}

/**
 * Guards content based on tenant access.
 * Shows AccessDeniedPage if user doesn't have access to the current tenant.
 * This component should be used INSIDE the Layout to keep the sidebar visible.
 */
export function TenantAccessGuard({ children }: TenantAccessGuardProps) {
  const { accessDenied, accessDeniedTenantId } = useTenant();

  if (accessDenied) {
    return (
      <AccessDeniedPage
        title="Tenant Access Denied"
        message={`The tenant "${accessDeniedTenantId}" does not exist, or you don't have permission to access it. Please contact your administrator if you need access.`}
      />
    );
  }

  return <>{children}</>;
}
