import { useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { LoadingSkeleton } from "@/shared/components";
import { useTenant, useAuth } from "@/shared/context";

/**
 * Smart redirect component that sends users to the first ACCESSIBLE tenant dashboard,
 * by the same check that guards each tenant's pages: admins and users with a
 * global role reach every tenant, others the tenants their roles or policies
 * name.
 */
export function HomeRedirect() {
  const { tenants, isLoading, error } = useTenant();
  const { hasAccessToTenant } = useAuth();
  const navigate = useNavigate();
  const hasNavigated = useRef(false);

  useEffect(() => {
    if (isLoading || hasNavigated.current) return;

    if (error) {
      hasNavigated.current = true;
      navigate("/tenants", { replace: true });
      return;
    }

    const accessibleTenant = tenants.find(
      (t) => t.tenant_id && hasAccessToTenant(t.tenant_id),
    );

    if (accessibleTenant) {
      hasNavigated.current = true;
      navigate(`/${accessibleTenant.tenant_id}`, { replace: true });
    } else {
      // No accessible tenants - show tenants page (will display appropriate message)
      hasNavigated.current = true;
      navigate("/tenants", { replace: true });
    }
  }, [tenants, isLoading, error, navigate, hasAccessToTenant]);

  return <LoadingSkeleton variant="spinner" message="Redirecting..." />;
}
