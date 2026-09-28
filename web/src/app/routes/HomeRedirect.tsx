import { useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { LoadingSkeleton } from "@/shared/components";
import { useTenant, useAuth } from "@/shared/context";

/**
 * Smart redirect component that sends users to the first ACCESSIBLE tenant dashboard.
 * - Admins: redirected to first tenant in the list
 * - Operators/Viewers: redirected to first tenant they have access to
 */
export function HomeRedirect() {
  const { tenants, isLoading, error } = useTenant();
  const { isAdmin, getAccessibleTenantIds } = useAuth();
  const navigate = useNavigate();
  const hasNavigated = useRef(false);

  useEffect(() => {
    if (isLoading || hasNavigated.current) return;

    if (error) {
      hasNavigated.current = true;
      navigate("/tenants", { replace: true });
      return;
    }

    // Admins can access all tenants, go to first one
    if (isAdmin()) {
      const firstTenant = tenants[0];
      if (firstTenant?.tenant_id) {
        hasNavigated.current = true;
        navigate(`/${firstTenant.tenant_id}`, { replace: true });
        return;
      }
    }

    // Non-admins: find first tenant they have access to
    const accessibleIds = getAccessibleTenantIds();
    const accessibleTenant = tenants.find((t) =>
      accessibleIds.includes(t.tenant_id),
    );

    if (accessibleTenant) {
      hasNavigated.current = true;
      navigate(`/${accessibleTenant.tenant_id}`, { replace: true });
    } else {
      // No accessible tenants - show tenants page (will display appropriate message)
      hasNavigated.current = true;
      navigate("/tenants", { replace: true });
    }
  }, [tenants, isLoading, error, navigate, isAdmin, getAccessibleTenantIds]);

  return <LoadingSkeleton variant="spinner" message="Redirecting..." />;
}
