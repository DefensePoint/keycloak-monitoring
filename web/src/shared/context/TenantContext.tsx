import { createContext, useContext, useMemo, ReactNode } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { tenantService } from "@/shared/services";
import { useAuth } from "./AuthContext";
import { LoadingSkeleton } from "@/shared/components";
import type { Tenant } from "@/shared/types";

interface TenantContextType {
  tenants: Tenant[];
  selectedTenant: Tenant | null;
  selectTenant: (tenant: Tenant | null) => void;
  isLoading: boolean;
  error: string | null;
  refreshTenants: () => Promise<void>;
  accessDenied: boolean;
  accessDeniedTenantId: string | null;
}

const TenantContext = createContext<TenantContextType | undefined>(undefined);

// Extract tenantId from pathname (e.g., "/prod-keycloak/events" -> "prod-keycloak")
function extractTenantIdFromPath(pathname: string): string | null {
  // Skip known non-tenant paths
  const nonTenantPaths = ["/tenants", "/admin", "/theme-test", "/login"];
  if (nonTenantPaths.some((p) => pathname.startsWith(p)) || pathname === "/") {
    return null;
  }

  // Extract first segment after leading slash
  const segments = pathname.split("/").filter(Boolean);
  return segments.length > 0 ? segments[0] : null;
}

export function TenantProvider({ children }: { children: ReactNode }) {
  const location = useLocation();
  const navigate = useNavigate();
  const { hasAccessToTenant, isLoading: isAuthLoading } = useAuth();
  const tenantId = extractTenantIdFromPath(location.pathname);

  // Load tenants with React Query
  const {
    data: tenantsResponse,
    isLoading: isTenantsLoading,
    error: queryError,
    refetch,
  } = useQuery({
    queryKey: ["tenants"],
    queryFn: () => tenantService.getTenants(),
    staleTime: 60000,
    refetchInterval: 60000,
    refetchIntervalInBackground: true,
  });

  const tenants = useMemo(
    () => tenantsResponse?.tenants || [],
    [tenantsResponse?.tenants],
  );
  const error = queryError instanceof Error ? queryError.message : null;
  const isLoading = isTenantsLoading || isAuthLoading;

  // Check if user has access to the tenant from URL
  const accessCheck = useMemo(() => {
    // No tenant in URL, no access check needed
    if (!tenantId) {
      return { hasAccess: true, tenantExists: false };
    }

    // Still loading, defer the check
    if (isLoading || tenants.length === 0) {
      return { hasAccess: true, tenantExists: false };
    }

    // Check if tenant exists
    const tenantExists = tenants.some((t) => t.tenant_id === tenantId);
    if (!tenantExists) {
      // Tenant doesn't exist, let the page handle it
      return { hasAccess: true, tenantExists: false };
    }

    // Tenant exists, check access
    const hasAccess = hasAccessToTenant(tenantId);
    return { hasAccess, tenantExists: true };
  }, [tenantId, tenants, hasAccessToTenant, isLoading]);

  // Selected tenant is simply the one from URL, or fallback to localStorage
  const selectedTenant = useMemo(() => {
    if (tenants.length === 0) return null;

    // Priority 1: Use tenant from URL if available and user has access
    if (tenantId && accessCheck.hasAccess) {
      const tenant = tenants.find((t) => t.tenant_id === tenantId);
      if (tenant) {
        localStorage.setItem("selectedTenantId", tenant.tenant_id);
        return tenant;
      }
    }

    // Priority 2: Fallback to last selected from localStorage
    const savedId = localStorage.getItem("selectedTenantId");
    if (savedId) {
      const savedTenant = tenants.find((t) => t.tenant_id === savedId);
      if (savedTenant) {
        return savedTenant;
      }
    }

    return null;
  }, [tenantId, tenants, accessCheck.hasAccess]);

  const refreshTenants = async () => {
    await refetch();
  };

  const selectTenant = (tenant: Tenant | null) => {
    if (tenant) {
      localStorage.setItem("selectedTenantId", tenant.tenant_id);
      navigate(`/${tenant.tenant_id}`);
    } else {
      localStorage.removeItem("selectedTenantId");
      navigate("/tenants");
    }
  };

  // Compute access denied state
  const accessDenied = !accessCheck.hasAccess && accessCheck.tenantExists;
  const accessDeniedTenantId = accessDenied ? tenantId : null;

  // Show loading while checking access
  if (isLoading && tenantId) {
    return <LoadingSkeleton variant="spinner" message="Loading..." />;
  }

  return (
    <TenantContext.Provider
      value={{
        tenants,
        selectedTenant,
        selectTenant,
        isLoading,
        error,
        refreshTenants,
        accessDenied,
        accessDeniedTenantId,
      }}
    >
      {children}
    </TenantContext.Provider>
  );
}

export function useTenant() {
  const context = useContext(TenantContext);
  if (context === undefined) {
    throw new Error("useTenant must be used within a TenantProvider");
  }
  return context;
}

export { TenantContext };
