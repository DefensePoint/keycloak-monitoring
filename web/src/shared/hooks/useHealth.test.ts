import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { useVersion, useInfinispanMetrics } from "./useHealth";
import { healthService } from "@/shared/services";

vi.mock("@/shared/services", () => ({
  healthService: {
    getVersion: vi.fn(),
    getInfinispanMetrics: vi.fn(),
  },
}));

describe("useHealth hooks", () => {
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

  describe("useVersion", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(() => useVersion(undefined), { wrapper });

      expect(healthService.getVersion).not.toHaveBeenCalled();
    });

    it("should fetch version information", async () => {
      const mockData = {
        version: "1.0.0",
        git_commit: "abc123",
        build_date: "2024-01-01",
        go_version: "go1.21.0",
        platform: "linux/amd64",
      };
      vi.mocked(healthService.getVersion).mockResolvedValue(mockData);

      const { result } = renderHook(() => useVersion("tenant-1"), { wrapper });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(healthService.getVersion).toHaveBeenCalledWith("tenant-1");
      expect(result.current.data).toEqual(mockData);
    });

    it("should have staleTime set to Infinity", async () => {
      const mockData = { version: "1.0.0" };
      vi.mocked(healthService.getVersion).mockResolvedValue(mockData);

      const { result } = renderHook(() => useVersion("tenant-1"), { wrapper });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      // The query should not refetch since staleTime is Infinity
      expect(result.current.isStale).toBe(false);
    });
  });

  describe("useInfinispanMetrics", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(() => useInfinispanMetrics(undefined), { wrapper });

      expect(healthService.getInfinispanMetrics).not.toHaveBeenCalled();
    });

    it("should fetch Infinispan metrics", async () => {
      const mockData = {
        caches: [
          { name: "sessions", entries: 100, hits: 1000, misses: 50 },
          { name: "authSessions", entries: 50, hits: 500, misses: 25 },
        ],
        cluster: {
          members: 3,
          status: "healthy",
        },
      };
      vi.mocked(healthService.getInfinispanMetrics).mockResolvedValue(mockData);

      const { result } = renderHook(() => useInfinispanMetrics("tenant-1"), {
        wrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(healthService.getInfinispanMetrics).toHaveBeenCalledWith(
        "tenant-1",
      );
      expect(result.current.data).toEqual(mockData);
    });

    it("should handle errors gracefully", async () => {
      const error = new Error("Failed to fetch metrics");
      vi.mocked(healthService.getInfinispanMetrics).mockRejectedValue(error);

      const { result } = renderHook(() => useInfinispanMetrics("tenant-1"), {
        wrapper,
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error).toEqual(error);
    });
  });
});
