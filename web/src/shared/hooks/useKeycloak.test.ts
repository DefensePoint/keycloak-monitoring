import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import {
  useKeycloakDashboard,
  useRealmDashboard,
  useRealmEvents,
  useRealmUsers,
  useRealmClients,
} from "./useKeycloak";
import { keycloakService } from "@/shared/services";

vi.mock("@/shared/services", () => ({
  keycloakService: {
    getKeycloakDashboard: vi.fn(),
    getRealmEvents: vi.fn(),
    getRealmUsers: vi.fn(),
    getRealmClients: vi.fn(),
  },
}));

describe("useKeycloak hooks", () => {
  let queryClient: QueryClient;

  const wrapper = ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);

  beforeEach(() => {
    vi.clearAllMocks();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });
  });

  describe("useKeycloakDashboard", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(() => useKeycloakDashboard({ tenantId: undefined }), {
        wrapper,
      });

      expect(keycloakService.getKeycloakDashboard).not.toHaveBeenCalled();
    });

    it("should fetch keycloak dashboard when tenantId is provided", async () => {
      const mockData = { realms: [], health: {} };
      vi.mocked(keycloakService.getKeycloakDashboard).mockResolvedValue(
        mockData,
      );

      const { result } = renderHook(
        () => useKeycloakDashboard({ tenantId: "tenant-1" }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(keycloakService.getKeycloakDashboard).toHaveBeenCalledWith(
        "tenant-1",
      );
      expect(result.current.data).toEqual(mockData);
    });
  });

  describe("useRealmDashboard", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(
        () => useRealmDashboard({ tenantId: undefined, realm: "master" }),
        { wrapper },
      );

      expect(keycloakService.getKeycloakDashboard).not.toHaveBeenCalled();
    });

    it("should not fetch when realm is undefined", () => {
      renderHook(
        () => useRealmDashboard({ tenantId: "tenant-1", realm: undefined }),
        { wrapper },
      );

      expect(keycloakService.getKeycloakDashboard).not.toHaveBeenCalled();
    });

    it("should fetch realm dashboard when both tenantId and realm are provided", async () => {
      const mockData = { realm_info: {}, metrics: {} };
      vi.mocked(keycloakService.getKeycloakDashboard).mockResolvedValue(
        mockData,
      );

      const { result } = renderHook(
        () => useRealmDashboard({ tenantId: "tenant-1", realm: "master" }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(keycloakService.getKeycloakDashboard).toHaveBeenCalledWith(
        "tenant-1",
        "master",
      );
    });
  });

  describe("useRealmEvents", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(
        () => useRealmEvents({ tenantId: undefined, realmName: "master" }),
        { wrapper },
      );

      expect(keycloakService.getRealmEvents).not.toHaveBeenCalled();
    });

    it("should not fetch when realmName is undefined", () => {
      renderHook(
        () => useRealmEvents({ tenantId: "tenant-1", realmName: undefined }),
        { wrapper },
      );

      expect(keycloakService.getRealmEvents).not.toHaveBeenCalled();
    });

    it("should fetch realm events with default parameters", async () => {
      const mockData = { events: [], total: 0 };
      vi.mocked(keycloakService.getRealmEvents).mockResolvedValue(mockData);

      const { result } = renderHook(
        () => useRealmEvents({ tenantId: "tenant-1", realmName: "master" }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(keycloakService.getRealmEvents).toHaveBeenCalledWith(
        "tenant-1",
        "master",
        50, // default limit
        undefined,
        undefined,
      );
    });

    it("should fetch realm events with custom parameters", async () => {
      const mockData = { events: [], total: 0 };
      vi.mocked(keycloakService.getRealmEvents).mockResolvedValue(mockData);

      const { result } = renderHook(
        () =>
          useRealmEvents({
            tenantId: "tenant-1",
            realmName: "master",
            limit: 100,
            startTime: "2024-01-01",
            endTime: "2024-01-31",
          }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(keycloakService.getRealmEvents).toHaveBeenCalledWith(
        "tenant-1",
        "master",
        100,
        "2024-01-01",
        "2024-01-31",
      );
    });
  });

  describe("useRealmUsers", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(
        () => useRealmUsers({ tenantId: undefined, realmName: "master" }),
        { wrapper },
      );

      expect(keycloakService.getRealmUsers).not.toHaveBeenCalled();
    });

    it("should fetch realm users with default parameters", async () => {
      const mockData = { users: [], total: 0 };
      vi.mocked(keycloakService.getRealmUsers).mockResolvedValue(mockData);

      const { result } = renderHook(
        () => useRealmUsers({ tenantId: "tenant-1", realmName: "master" }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(keycloakService.getRealmUsers).toHaveBeenCalledWith(
        "tenant-1",
        "master",
        0,
        100,
      );
    });
  });

  describe("useRealmClients", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(
        () => useRealmClients({ tenantId: undefined, realmName: "master" }),
        { wrapper },
      );

      expect(keycloakService.getRealmClients).not.toHaveBeenCalled();
    });

    it("should fetch realm clients", async () => {
      const mockData = { clients: [] };
      vi.mocked(keycloakService.getRealmClients).mockResolvedValue(mockData);

      const { result } = renderHook(
        () => useRealmClients({ tenantId: "tenant-1", realmName: "master" }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(keycloakService.getRealmClients).toHaveBeenCalledWith(
        "tenant-1",
        "master",
      );
    });
  });
});
