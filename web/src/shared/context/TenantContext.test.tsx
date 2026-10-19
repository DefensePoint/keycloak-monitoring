import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, renderHook, screen, waitFor } from "@/test-utils";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TenantProvider, useTenant } from "./TenantContext";
import { tenantService } from "@/shared/services";
import type { Tenant } from "@/shared/types";

const mockNavigate = vi.fn();
let mockPathname = "/tenant-1/dashboard";
let mockHasAccessToTenant: (tenantId: string) => boolean = () => true;

vi.mock("react-router-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router-dom")>();
  return {
    ...actual,
    useLocation: () => ({ pathname: mockPathname }),
    useParams: () => ({ tenantId: "tenant-1" }),
    useNavigate: () => mockNavigate,
  };
});

vi.mock("@/shared/services", () => ({
  tenantService: {
    getTenants: vi.fn(),
  },
}));

vi.mock("./AuthContext", () => ({
  useAuth: () => ({
    hasAccessToTenant: (tenantId: string) => mockHasAccessToTenant(tenantId),
    isLoading: false,
  }),
}));

vi.mock("@/shared/components", () => ({
  LoadingSkeleton: () => <div data-testid="loading-skeleton">Loading...</div>,
  AccessDeniedPage: () => <div data-testid="access-denied">Access Denied</div>,
}));

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <TenantProvider>{children}</TenantProvider>
    </QueryClientProvider>
  );
};

describe("TenantContext", () => {
  const mockTenants: Tenant[] = [
    {
      tenant_id: "tenant-1",
      name: "Tenant 1",
      server_url: "https://tenant1.example.com",
      enabled: true,
      is_default: true,
      health_status: "healthy",
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
    },
    {
      tenant_id: "tenant-2",
      name: "Tenant 2",
      server_url: "https://tenant2.example.com",
      enabled: true,
      is_default: false,
      health_status: "unhealthy",
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    mockPathname = "/tenant-1/dashboard";
    mockHasAccessToTenant = () => true;
  });

  describe("TenantProvider", () => {
    it("should load tenants on mount", async () => {
      vi.mocked(tenantService.getTenants).mockResolvedValue({
        tenants: mockTenants,
      });

      const { result } = renderHook(() => useTenant(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.tenants).toEqual(mockTenants);
      expect(tenantService.getTenants).toHaveBeenCalled();
    });

    it("should select tenant from URL params", async () => {
      vi.mocked(tenantService.getTenants).mockResolvedValue({
        tenants: mockTenants,
      });

      const { result } = renderHook(() => useTenant(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.selectedTenant?.tenant_id).toBe("tenant-1");
    });

    it("should handle loading state", async () => {
      vi.mocked(tenantService.getTenants).mockImplementation(
        () => new Promise(() => {}), // Never resolves
      );

      // When loading with a tenantId in URL, LoadingSkeleton is rendered
      // So we test the loading skeleton appears instead of the context
      render(
        <QueryClientProvider
          client={
            new QueryClient({ defaultOptions: { queries: { retry: false } } })
          }
        >
          <TenantProvider>
            <div data-testid="child">Content</div>
          </TenantProvider>
        </QueryClientProvider>,
      );

      // LoadingSkeleton should be rendered while loading with tenant in URL
      expect(screen.getByTestId("loading-skeleton")).toBeInTheDocument();
    });

    it("should handle error state", async () => {
      vi.mocked(tenantService.getTenants).mockRejectedValue(
        new Error("Failed to fetch tenants"),
      );

      const { result } = renderHook(() => useTenant(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.error).toBe("Failed to fetch tenants");
      });
    });

    it("should navigate when selecting tenant", async () => {
      vi.mocked(tenantService.getTenants).mockResolvedValue({
        tenants: mockTenants,
      });

      const { result } = renderHook(() => useTenant(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      result.current.selectTenant(mockTenants[1]);

      expect(mockNavigate).toHaveBeenCalledWith("/tenant-2");
      expect(localStorage.getItem("selectedTenantId")).toBe("tenant-2");
    });

    it("should navigate to tenants page when deselecting tenant", async () => {
      vi.mocked(tenantService.getTenants).mockResolvedValue({
        tenants: mockTenants,
      });

      const { result } = renderHook(() => useTenant(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      result.current.selectTenant(null);

      expect(mockNavigate).toHaveBeenCalledWith("/tenants");
      expect(localStorage.getItem("selectedTenantId")).toBeNull();
    });

    it("should save selected tenant to localStorage", async () => {
      vi.mocked(tenantService.getTenants).mockResolvedValue({
        tenants: mockTenants,
      });

      const { result } = renderHook(() => useTenant(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      // selectedTenant is set from URL params (tenant-1)
      expect(localStorage.getItem("selectedTenantId")).toBe("tenant-1");
    });
  });

  // The tenant list holds only the tenants the user can access. A link to
  // any other tenant used to fall back to the last selected one, so the page
  // showed that tenant's data under the linked tenant's name.
  describe("a tenant named in the URL", () => {
    const renderAt = async (
      pathname: string,
      tenants: Tenant[] = mockTenants,
    ) => {
      mockPathname = pathname;
      vi.mocked(tenantService.getTenants).mockResolvedValue({ tenants });
      const { result } = renderHook(() => useTenant(), {
        wrapper: createWrapper(),
      });
      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });
      return result;
    };

    it("is refused, not replaced, when the user cannot see it", async () => {
      localStorage.setItem("selectedTenantId", "tenant-2");

      const result = await renderAt("/other-customer/alerts");

      await waitFor(() => {
        expect(result.current.accessDenied).toBe(true);
      });
      expect(result.current.accessDeniedTenantId).toBe("other-customer");
      expect(result.current.selectedTenant).toBeNull();
      expect(localStorage.getItem("selectedTenantId")).toBe("tenant-2");
    });

    it("is refused when listed but the user's roles do not reach it", async () => {
      localStorage.setItem("selectedTenantId", "tenant-1");
      mockHasAccessToTenant = (tenantId) => tenantId !== "tenant-2";

      const result = await renderAt("/tenant-2/events");

      await waitFor(() => {
        expect(result.current.accessDenied).toBe(true);
      });
      expect(result.current.selectedTenant).toBeNull();
    });

    it("is refused for a user with no tenants at all", async () => {
      const result = await renderAt("/tenant-1/alerts", []);

      await waitFor(() => {
        expect(result.current.accessDenied).toBe(true);
      });
      expect(result.current.selectedTenant).toBeNull();
    });

    it("is not refused when the tenant list failed to load", async () => {
      mockPathname = "/tenant-1/alerts";
      vi.mocked(tenantService.getTenants).mockRejectedValue(
        new Error("Failed to fetch tenants"),
      );
      const { result } = renderHook(() => useTenant(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.error).toBe("Failed to fetch tenants");
      });
      expect(result.current.accessDenied).toBe(false);
      expect(result.current.selectedTenant).toBeNull();
    });

    it("is selected when its ID starts with a non-tenant path", async () => {
      const prefixed = ["admin-kc", "tenants-prod"].map((id) => ({
        ...mockTenants[0],
        tenant_id: id,
      }));

      for (const tenant of prefixed) {
        localStorage.setItem("selectedTenantId", "tenant-1");
        const result = await renderAt(`/${tenant.tenant_id}/alerts`, [
          ...mockTenants,
          ...prefixed,
        ]);
        await waitFor(() => {
          expect(result.current.selectedTenant?.tenant_id).toBe(
            tenant.tenant_id,
          );
        });
        expect(result.current.accessDenied).toBe(false);
      }
    });
  });

  describe("a page without a tenant in the URL", () => {
    it("keeps the last selected tenant", async () => {
      localStorage.setItem("selectedTenantId", "tenant-2");
      mockPathname = "/tenants";
      vi.mocked(tenantService.getTenants).mockResolvedValue({
        tenants: mockTenants,
      });
      const { result } = renderHook(() => useTenant(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.selectedTenant?.tenant_id).toBe("tenant-2");
      });
      expect(result.current.accessDenied).toBe(false);
    });
  });

  describe("useTenant hook", () => {
    it("should throw error when used outside TenantProvider", () => {
      const consoleError = console.error;
      console.error = vi.fn();

      expect(() => {
        renderHook(() => useTenant());
      }).toThrow("useTenant must be used within a TenantProvider");

      console.error = consoleError;
    });

    it("should provide refreshTenants function", async () => {
      vi.mocked(tenantService.getTenants).mockResolvedValue({
        tenants: mockTenants,
      });

      const { result } = renderHook(() => useTenant(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(typeof result.current.refreshTenants).toBe("function");
    });
  });
});
