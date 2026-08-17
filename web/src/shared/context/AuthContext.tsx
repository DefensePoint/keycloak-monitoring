import {
  createContext,
  useContext,
  useEffect,
  useState,
  ReactNode,
} from "react";
import { authService } from "@/shared/services/authService";
import type { UserInfo, AuthConfig } from "@/shared/types/auth.types";

interface AuthContextType {
  user: UserInfo | null;
  isAuthenticated: boolean;
  authConfig: AuthConfig | null;
  isLoading: boolean;
  loginSimple: (username: string, password: string) => Promise<void>;
  loginOAuth2: () => void;
  logout: () => void;
  refreshUser: () => Promise<void>;
  // RBAC functions
  hasPermission: (permission: string) => boolean;
  hasAnyPermission: (permissions: string[]) => boolean;
  hasRole: (roleName: string, tenantId?: string) => boolean;
  isAdmin: () => boolean;
  hasAccessToTenant: (tenantId: string) => boolean;
  getAccessibleTenantIds: () => string[];
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<UserInfo | null>(null);
  const [authConfig, setAuthConfig] = useState<AuthConfig | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const refreshUser = async () => {
    try {
      const userInfo = await authService.getUserInfo();
      setUser(userInfo);
    } catch {
      setUser(null);
    }
  };

  useEffect(() => {
    // Get auth configuration and check authentication status
    const initAuth = async () => {
      try {
        const config = await authService.getAuthConfig();
        setAuthConfig(config);

        // Check if user is authenticated
        await refreshUser();
      } catch (error: unknown) {
        console.error("Failed to initialize auth:", error);
      } finally {
        setIsLoading(false);
      }
    };

    initAuth();
  }, []);

  const loginSimple = async (username: string, password: string) => {
    await authService.loginSimple(username, password);
    // Fetch full user info with RBAC data
    await refreshUser();
  };

  const loginOAuth2 = () => {
    authService.loginOAuth2();
  };

  const logout = () => {
    authService.logout();
  };

  // RBAC helper functions
  const hasPermission = (permission: string): boolean => {
    if (!user || !user.permissions) return false;

    // Check if user has the permission (permissions are already filtered by backend for tenant scope)
    return user.permissions.includes(permission);
  };

  const hasAnyPermission = (permissions: string[]): boolean => {
    if (!user || !user.permissions) return false;

    return permissions.some((perm) => user.permissions!.includes(perm));
  };

  const hasRole = (roleName: string, tenantId?: string): boolean => {
    if (!user || !user.roles) return false;

    return user.roles.some((userRole) => {
      // Check if role matches
      const roleMatches = userRole.role?.name === roleName;

      // If tenantId is specified, check tenant scope
      if (tenantId) {
        return (
          roleMatches &&
          (userRole.tenant_id === null || // Global role
            userRole.tenant_id === tenantId) // Tenant-specific role
        );
      }

      return roleMatches;
    });
  };

  const isAdmin = (): boolean => {
    return hasRole("admin");
  };

  const hasAccessToTenant = (tenantId: string): boolean => {
    if (!user) return false;

    // Admins have access to all tenants
    if (isAdmin()) return true;

    // Check roles with tenant_id (operator/viewer assigned to specific tenant)
    if (user.roles && user.roles.length > 0) {
      const hasRoleForTenant = user.roles.some(
        (userRole) =>
          userRole.tenant_id === tenantId || // Role specific to this tenant
          userRole.tenant_id === null, // Global role (access to all tenants)
      );
      if (hasRoleForTenant) return true;
    }

    // Check tenant policies (additional access control)
    if (user.tenant_policies && user.tenant_policies.length > 0) {
      return user.tenant_policies.some(
        (policy) => policy.tenant_id === tenantId,
      );
    }

    return false;
  };

  const getAccessibleTenantIds = (): string[] => {
    if (!user) return [];

    // Admins have access to all tenants - return empty to indicate "all"
    // The caller should check isAdmin() first
    if (isAdmin()) return [];

    const tenantIds = new Set<string>();

    // Collect from roles with tenant_id
    if (user.roles) {
      user.roles.forEach((userRole) => {
        if (userRole.tenant_id) {
          tenantIds.add(userRole.tenant_id);
        }
      });
    }

    // Collect from tenant_policies
    if (user.tenant_policies) {
      user.tenant_policies.forEach((policy) => {
        tenantIds.add(policy.tenant_id);
      });
    }

    return Array.from(tenantIds);
  };

  return (
    <AuthContext.Provider
      value={{
        user,
        isAuthenticated: user !== null,
        authConfig,
        isLoading,
        loginSimple,
        loginOAuth2,
        logout,
        refreshUser,
        hasPermission,
        hasAnyPermission,
        hasRole,
        isAdmin,
        hasAccessToTenant,
        getAccessibleTenantIds,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}

export { AuthContext };
