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

// Top-level paths that are not tenants. Compared as whole segments, so a tenant
// whose ID merely starts with one (say "admin-kc") is still a tenant.
const nonTenantSegments = new Set(["tenants", "admin", "theme-test", "login"]);

// Extract tenantId from pathname (e.g., "/prod-keycloak/events" -> "prod-keycloak")
function extractTenantIdFromPath(pathname: string): string | null {
  const first = pathname.split("/").filter(Boolean)[0];
  if (!first || nonTenantSegments.has(first)) {
    return null;
  }
  return first;
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

  // A tenant named in the URL is the only tenant that page may show. One the
  // user cannot see is refused, never replaced by another: the tenant list
  // holds only the tenants the user can access, so a missing one may exist
  // and belong to someone else.
  const urlTenant = useMemo(() => {
    if (!tenantId || isLoading || !tenantsResponse) {
      return null;
    }
    const tenant = tenants.find((t) => t.tenant_id === tenantId);
    return tenant && hasAccessToTenant(tenantId) ? tenant : null;
  }, [tenantId, tenants, tenantsResponse, hasAccessToTenant, isLoading]);

  // Selected tenant is the one from the URL, or, on pages without one, the
  // last selected from localStorage
  const selectedTenant = useMemo(() => {
    if (tenantId) {
      if (urlTenant) {
        localStorage.setItem("selectedTenantId", urlTenant.tenant_id);
      }
      return urlTenant;
    }

    const savedId = localStorage.getItem("selectedTenantId");
    return tenants.find((t) => t.tenant_id === savedId) ?? null;
  }, [tenantId, urlTenant, tenants]);

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

  // Denied once the tenant list has loaded without the URL's tenant in it. A
  // failed load is not a denial: the error is reported instead.
  const accessDenied =
    !!tenantId && !isLoading && !!tenantsResponse && !urlTenant;
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
