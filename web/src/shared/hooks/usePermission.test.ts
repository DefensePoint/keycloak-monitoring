import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@/test-utils";
import {
  usePermission,
  useRole,
  useUserRoles,
  useUserPermissions,
  useIsAdmin,
  useTenantAccess,
} from "./usePermission";
import { useAuth } from "@/shared/context";

vi.mock("@/shared/context", () => ({
  useAuth: vi.fn(),
}));

describe("usePermission hooks", () => {
  const mockUser = {
    id: 1,
    name: "Test User",
    email: "test@example.com",
    roles: [
      { id: 1, role_id: 1, role: { name: "admin", display_name: "Admin" } },
      { id: 2, role_id: 2, role: { name: "viewer", display_name: "Viewer" } },
    ],
    permissions: ["users:read", "users:write", "tenants:read"],
    tenant_policies: [{ tenant_id: "tenant-1", allowed_realms: ["*"] }],
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("usePermission", () => {
    it("should return hasPermission true for single valid permission", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: (perm: string) => mockUser.permissions.includes(perm),
        hasAnyPermission: vi.fn(),
        hasRole: vi.fn(),
        isAdmin: vi.fn(),
        hasAccessToTenant: vi.fn(),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() => usePermission("users:read"));

      expect(result.current.hasPermission).toBe(true);
      expect(result.current.isLoading).toBe(false);
    });

    it("should return hasPermission false for invalid permission", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: (perm: string) => mockUser.permissions.includes(perm),
        hasAnyPermission: vi.fn(),
        hasRole: vi.fn(),
        isAdmin: vi.fn(),
        hasAccessToTenant: vi.fn(),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() => usePermission("admin:delete"));

      expect(result.current.hasPermission).toBe(false);
    });

    it("should check any permission for array of permissions", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: vi.fn(),
        hasAnyPermission: (perms: string[]) =>
          perms.some((p) => mockUser.permissions.includes(p)),
        hasRole: vi.fn(),
        isAdmin: vi.fn(),
        hasAccessToTenant: vi.fn(),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() =>
        usePermission(["admin:delete", "users:read"]),
      );

      expect(result.current.hasPermission).toBe(true);
    });

    it("should return isLoading true when user is null", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: null,
        hasPermission: () => false,
        hasAnyPermission: () => false,
        hasRole: vi.fn(),
        isAdmin: vi.fn(),
        hasAccessToTenant: vi.fn(),
        isAuthenticated: false,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: true,
      });

      const { result } = renderHook(() => usePermission("users:read"));

      expect(result.current.isLoading).toBe(true);
    });
  });

  describe("useRole", () => {
    it("should return hasRole true for valid role", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: vi.fn(),
        hasAnyPermission: vi.fn(),
        hasRole: (role: string) =>
          mockUser.roles.some((r) => r.role?.name === role),
        isAdmin: vi.fn(),
        hasAccessToTenant: vi.fn(),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() => useRole("admin"));

      expect(result.current.hasRole).toBe(true);
      expect(result.current.isLoading).toBe(false);
    });

    it("should return hasRole false for invalid role", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: vi.fn(),
        hasAnyPermission: vi.fn(),
        hasRole: (role: string) =>
          mockUser.roles.some((r) => r.role?.name === role),
        isAdmin: vi.fn(),
        hasAccessToTenant: vi.fn(),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() => useRole("superadmin"));

      expect(result.current.hasRole).toBe(false);
    });
  });

  describe("useUserRoles", () => {
    it("should return user roles", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: vi.fn(),
        hasAnyPermission: vi.fn(),
        hasRole: vi.fn(),
        isAdmin: vi.fn(),
        hasAccessToTenant: vi.fn(),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() => useUserRoles());

      expect(result.current.roles).toEqual(mockUser.roles);
      expect(result.current.isLoading).toBe(false);
    });

    it("should return empty array when user is null", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: null,
        hasPermission: vi.fn(),
        hasAnyPermission: vi.fn(),
        hasRole: vi.fn(),
        isAdmin: vi.fn(),
        hasAccessToTenant: vi.fn(),
        isAuthenticated: false,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: true,
      });

      const { result } = renderHook(() => useUserRoles());

      expect(result.current.roles).toEqual([]);
      expect(result.current.isLoading).toBe(true);
    });
  });

  describe("useUserPermissions", () => {
    it("should return user permissions", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: vi.fn(),
        hasAnyPermission: vi.fn(),
        hasRole: vi.fn(),
        isAdmin: vi.fn(),
        hasAccessToTenant: vi.fn(),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() => useUserPermissions());

      expect(result.current.permissions).toEqual(mockUser.permissions);
      expect(result.current.isLoading).toBe(false);
    });

    it("should return empty array when user is null", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: null,
        hasPermission: vi.fn(),
        hasAnyPermission: vi.fn(),
        hasRole: vi.fn(),
        isAdmin: vi.fn(),
        hasAccessToTenant: vi.fn(),
        isAuthenticated: false,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: true,
      });

      const { result } = renderHook(() => useUserPermissions());

      expect(result.current.permissions).toEqual([]);
      expect(result.current.isLoading).toBe(true);
    });
  });

  describe("useIsAdmin", () => {
    it("should return isAdmin true when user is admin", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: vi.fn(),
        hasAnyPermission: vi.fn(),
        hasRole: vi.fn(),
        isAdmin: () => true,
        hasAccessToTenant: vi.fn(),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() => useIsAdmin());

      expect(result.current.isAdmin).toBe(true);
      expect(result.current.isLoading).toBe(false);
    });

    it("should return isAdmin false when user is not admin", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: vi.fn(),
        hasAnyPermission: vi.fn(),
        hasRole: vi.fn(),
        isAdmin: () => false,
        hasAccessToTenant: vi.fn(),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() => useIsAdmin());

      expect(result.current.isAdmin).toBe(false);
    });
  });

  describe("useTenantAccess", () => {
    it("should return hasAccess true for valid tenant", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: vi.fn(),
        hasAnyPermission: vi.fn(),
        hasRole: vi.fn(),
        isAdmin: vi.fn(),
        hasAccessToTenant: (tenantId: string) =>
          mockUser.tenant_policies.some((p) => p.tenant_id === tenantId),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() => useTenantAccess("tenant-1"));

      expect(result.current.hasAccess).toBe(true);
      expect(result.current.isLoading).toBe(false);
    });

    it("should return hasAccess false for invalid tenant", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: vi.fn(),
        hasAnyPermission: vi.fn(),
        hasRole: vi.fn(),
        isAdmin: vi.fn(),
        hasAccessToTenant: (tenantId: string) =>
          mockUser.tenant_policies.some((p) => p.tenant_id === tenantId),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() => useTenantAccess("tenant-999"));

      expect(result.current.hasAccess).toBe(false);
    });

    it("should return hasAccess false when tenantId is empty", () => {
      vi.mocked(useAuth).mockReturnValue({
        user: mockUser,
        hasPermission: vi.fn(),
        hasAnyPermission: vi.fn(),
        hasRole: vi.fn(),
        isAdmin: vi.fn(),
        hasAccessToTenant: vi.fn(),
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        isLoading: false,
      });

      const { result } = renderHook(() => useTenantAccess(""));

      expect(result.current.hasAccess).toBe(false);
    });
  });
});
