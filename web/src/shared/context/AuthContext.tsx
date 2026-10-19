import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  ReactNode,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  authService,
  ServerUnavailableError,
} from "@/shared/services/authService";
import { SESSION_EXPIRED_EVENT } from "@/shared/lib/apiClient";
import type { UserInfo, AuthConfig } from "@/shared/types/auth.types";

interface AuthContextType {
  user: UserInfo | null;
  isAuthenticated: boolean;
  authConfig: AuthConfig | null;
  isLoading: boolean;
  // The session ended while the user was signed in.
  sessionExpired: boolean;
  // Startup could not reach the server; it is retried until it can.
  serverUnavailable: boolean;
  // When the next startup attempt runs (ms since the epoch), while unavailable.
  nextRetryAt: number | null;
  // Retry startup at once instead of waiting for nextRetryAt.
  retryNow: () => void;
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
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

// Startup retries wait 2s, 4s, 8s and 16s, then every 30s until the server
// answers.
const STARTUP_RETRY_DELAYS_MS = [2000, 4000, 8000, 16000];
const MAX_STARTUP_RETRY_DELAY_MS = 30000;

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [user, setUser] = useState<UserInfo | null>(null);
  const [authConfig, setAuthConfig] = useState<AuthConfig | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [sessionExpired, setSessionExpired] = useState(false);
  const userRef = useRef<UserInfo | null>(null);
  userRef.current = user;

  const refreshUser = async () => {
    try {
      const userInfo = await authService.getUserInfo();
      setUser(userInfo);
    } catch {
      setUser(null);
    }
  };

  const [serverUnavailable, setServerUnavailable] = useState(false);
  const [nextRetryAt, setNextRetryAt] = useState<number | null>(null);
  const mountedRef = useRef(false);
  const startedRef = useRef(false);
  const inFlightRef = useRef(false);
  const failuresRef = useRef(0);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // startup reads the auth configuration and the session. Only a server that
  // answered counts: a failed config read, or a session read that did not get
  // a 401, means the server could not be reached, and startup is tried again
  // with backoff rather than leaving a sign-in card with no way to sign in.
  const startup = useCallback(async () => {
    if (inFlightRef.current || startedRef.current) return;
    inFlightRef.current = true;
    if (retryTimerRef.current) {
      clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
    }
    try {
      const config = await authService.getAuthConfig();
      let userInfo: UserInfo | null = null;
      try {
        userInfo = await authService.getUserInfo();
      } catch (error: unknown) {
        if (error instanceof ServerUnavailableError) throw error;
      }
      if (!mountedRef.current) return;
      startedRef.current = true;
      setAuthConfig(config);
      setUser(userInfo);
      setServerUnavailable(false);
      setNextRetryAt(null);
    } catch (error: unknown) {
      if (!mountedRef.current) return;
      if (failuresRef.current === 0) {
        console.error("Failed to initialize auth; retrying:", error);
      }
      const delay =
        STARTUP_RETRY_DELAYS_MS[failuresRef.current] ??
        MAX_STARTUP_RETRY_DELAY_MS;
      failuresRef.current += 1;
      setServerUnavailable(true);
      setNextRetryAt(Date.now() + delay);
      retryTimerRef.current = setTimeout(() => {
        void startup();
      }, delay);
    } finally {
      inFlightRef.current = false;
      if (mountedRef.current) setIsLoading(false);
    }
  }, []);

  const retryNow = useCallback(() => {
    void startup();
  }, [startup]);

  useEffect(() => {
    mountedRef.current = true;
    void startup();

    // A browser back online, or a tab shown again, is worth an attempt now
    // rather than at the end of a long backoff.
    const onVisible = () => {
      if (document.visibilityState === "visible") void startup();
    };
    const onOnline = () => void startup();
    window.addEventListener("online", onOnline);
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      mountedRef.current = false;
      window.removeEventListener("online", onOnline);
      document.removeEventListener("visibilitychange", onVisible);
      if (retryTimerRef.current) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }
    };
  }, [startup]);

  // The API answered 401: sign out here rather than navigate away, so the
  // login form shows at the current URL and signing in again returns to it.
  useEffect(() => {
    const onSessionExpired = () => {
      if (userRef.current) {
        setSessionExpired(true);
      }
      setUser(null);
    };
    window.addEventListener(SESSION_EXPIRED_EVENT, onSessionExpired);
    return () =>
      window.removeEventListener(SESSION_EXPIRED_EVENT, onSessionExpired);
  }, []);

  const loginSimple = async (username: string, password: string) => {
    await authService.loginSimple(username, password);
    // An expired session signs out without leaving the page, so the cache
    // still holds what the previous user could see. Whoever signs in now
    // must not be shown it while their own data loads.
    queryClient.clear();
    // Fetch full user info with RBAC data
    await refreshUser();
    setSessionExpired(false);
  };

  const loginOAuth2 = () => {
    authService.loginOAuth2();
  };

  const logout = () => {
    authService.logout();
  };

  // RBAC helper functions

  // A role assigned without a tenant applies to every tenant. /api/rbac/me
  // sends such a row with tenant_id omitted, not null, so both mean global.
  const isGlobal = (userRole: { tenant_id?: string | null }): boolean =>
    userRole.tenant_id === null || userRole.tenant_id === undefined;

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
          roleMatches && (isGlobal(userRole) || userRole.tenant_id === tenantId)
        );
      }

      return roleMatches;
    });
  };

  // Mirrors rbac.IsAdmin on the server: only a global admin grant makes a
  // platform admin. /api/rbac/me lists only roles that grant (active, not
  // deleted, not expired); the expiry check also covers a grant expiring while
  // the page is open.
  const isAdmin = (): boolean => {
    if (!user?.roles) return false;
    const now = Date.now();
    return user.roles.some(
      (userRole) =>
        userRole.role?.name === "admin" &&
        isGlobal(userRole) &&
        (!userRole.expires_at || new Date(userRole.expires_at).getTime() > now),
    );
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
          isGlobal(userRole), // Global role (access to all tenants)
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

  return (
    <AuthContext.Provider
      value={{
        user,
        isAuthenticated: user !== null,
        authConfig,
        isLoading,
        sessionExpired,
        serverUnavailable,
        nextRetryAt,
        retryNow,
        loginSimple,
        loginOAuth2,
        logout,
        refreshUser,
        hasPermission,
        hasAnyPermission,
        hasRole,
        isAdmin,
        hasAccessToTenant,
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
