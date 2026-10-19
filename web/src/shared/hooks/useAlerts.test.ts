import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import {
  useAlerts,
  useAlert,
  useAlertStats,
  useAmfaCheckerStatus,
  useReloadAmfaAlerts,
  useUpdateAlertStatus,
  useResolveAlert,
} from "./useAlerts";
import { alertsService } from "@/shared/services";

vi.mock("@/shared/services", () => ({
  alertsService: {
    getAlerts: vi.fn(),
    getAlert: vi.fn(),
    getAlertStats: vi.fn(),
    getAmfaCheckerStatus: vi.fn(),
    reloadAmfaAlerts: vi.fn(),
    updateAlertStatus: vi.fn(),
    resolveAlert: vi.fn(),
  },
}));

describe("useAlerts hooks", () => {
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

  describe("useAlerts", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(() => useAlerts({ tenantId: undefined }), { wrapper });

      expect(alertsService.getAlerts).not.toHaveBeenCalled();
    });

    it("should fetch alerts with default parameters", async () => {
      const mockData = { alerts: [], total: 0 };
      vi.mocked(alertsService.getAlerts).mockResolvedValue(mockData);

      const { result } = renderHook(() => useAlerts({ tenantId: "tenant-1" }), {
        wrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(alertsService.getAlerts).toHaveBeenCalledWith(
        "tenant-1",
        100, // default limit
        0, // default offset
        undefined,
        undefined,
        undefined,
        undefined,
        undefined,
      );
    });

    it("should fetch alerts with custom parameters", async () => {
      const mockData = { alerts: [], total: 0 };
      vi.mocked(alertsService.getAlerts).mockResolvedValue(mockData);

      const { result } = renderHook(
        () =>
          useAlerts({
            tenantId: "tenant-1",
            status: "active",
            severity: "critical",
            type: "security",
            realm: "master",
            resourceType: "realm",
            limit: 50,
            offset: 10,
          }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(alertsService.getAlerts).toHaveBeenCalledWith(
        "tenant-1",
        50,
        10,
        "active",
        "critical",
        "security",
        "master",
        "realm",
      );
    });
  });

  describe("useAlert", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(() => useAlert(undefined, "alert-1"), { wrapper });

      expect(alertsService.getAlert).not.toHaveBeenCalled();
    });

    it("should not fetch when alertId is undefined", () => {
      renderHook(() => useAlert("tenant-1", undefined), { wrapper });

      expect(alertsService.getAlert).not.toHaveBeenCalled();
    });

    it("should fetch alert details", async () => {
      const mockData = { alert_id: "alert-1", title: "Test Alert" };
      vi.mocked(alertsService.getAlert).mockResolvedValue(mockData);

      const { result } = renderHook(() => useAlert("tenant-1", "alert-1"), {
        wrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(alertsService.getAlert).toHaveBeenCalledWith(
        "tenant-1",
        "alert-1",
      );
      expect(result.current.data).toEqual(mockData);
    });
  });

  describe("useAlertStats", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(() => useAlertStats(undefined), { wrapper });

      expect(alertsService.getAlertStats).not.toHaveBeenCalled();
    });

    it("should fetch alert statistics", async () => {
      const mockData = { total_active: 5, by_severity: {} };
      vi.mocked(alertsService.getAlertStats).mockResolvedValue(mockData);

      const { result } = renderHook(() => useAlertStats("tenant-1"), {
        wrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(alertsService.getAlertStats).toHaveBeenCalledWith("tenant-1");
      expect(result.current.data).toEqual(mockData);
    });

    // The dashboard tile reads these counts, and nothing else refetches them:
    // without polling, a failed first request stays failed and a new alert
    // never shows until the page is reloaded.
    describe("polling", () => {
      beforeEach(() => {
        vi.useFakeTimers({ shouldAdvanceTime: true });
      });

      afterEach(() => {
        vi.useRealTimers();
      });

      it("recovers on its own after a failed request", async () => {
        const stats = { total_active: 2, by_severity: { critical: 1 } };
        vi.mocked(alertsService.getAlertStats)
          .mockRejectedValueOnce(new Error("HTTP 500"))
          .mockResolvedValue(stats);

        const { result } = renderHook(() => useAlertStats("tenant-1"), {
          wrapper,
        });
        await waitFor(() => {
          expect(result.current.isError).toBe(true);
        });

        await act(async () => {
          await vi.advanceTimersByTimeAsync(10_000);
        });

        await waitFor(() => {
          expect(result.current.isError).toBe(false);
        });
        expect(result.current.data).toEqual(stats);
      });

      it("picks up a changed count without a remount", async () => {
        vi.mocked(alertsService.getAlertStats)
          .mockResolvedValueOnce({ total_active: 0, by_severity: {} })
          .mockResolvedValue({ total_active: 1, by_severity: { critical: 1 } });

        const { result } = renderHook(() => useAlertStats("tenant-1"), {
          wrapper,
        });
        await waitFor(() => {
          expect(result.current.data?.total_active).toBe(0);
        });

        await act(async () => {
          await vi.advanceTimersByTimeAsync(10_000);
        });

        await waitFor(() => {
          expect(result.current.data?.total_active).toBe(1);
        });
      });
    });
  });

  describe("useAmfaCheckerStatus", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(() => useAmfaCheckerStatus(undefined), { wrapper });

      expect(alertsService.getAmfaCheckerStatus).not.toHaveBeenCalled();
    });

    it("should fetch and return the enabled flag", async () => {
      vi.mocked(alertsService.getAmfaCheckerStatus).mockResolvedValue({
        enabled: true,
      });

      const { result } = renderHook(() => useAmfaCheckerStatus("tenant-1"), {
        wrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(alertsService.getAmfaCheckerStatus).toHaveBeenCalledWith(
        "tenant-1",
      );
      expect(result.current.data).toEqual({ enabled: true });
    });
  });

  describe("useReloadAmfaAlerts", () => {
    it("should reload amfa alerts and invalidate queries", async () => {
      vi.mocked(alertsService.reloadAmfaAlerts).mockResolvedValue({
        status: "ok",
      });

      const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

      const { result } = renderHook(() => useReloadAmfaAlerts(), { wrapper });

      await act(async () => {
        await result.current.mutateAsync("tenant-1");
      });

      expect(alertsService.reloadAmfaAlerts).toHaveBeenCalledWith("tenant-1");

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["alerts", "tenant-1"],
      });
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["alert-stats", "tenant-1"],
      });
    });

    it("should surface the error when reload fails", async () => {
      const error = new Error("amfa_unavailable");
      vi.mocked(alertsService.reloadAmfaAlerts).mockRejectedValue(error);

      const { result } = renderHook(() => useReloadAmfaAlerts(), { wrapper });

      await act(async () => {
        await expect(result.current.mutateAsync("tenant-1")).rejects.toThrow(
          "amfa_unavailable",
        );
      });

      // The rejection settles before React Query's state update, which it
      // batches through notifyManager, so asserting isError straight after the
      // await raced the re-render and made this flake.
      await waitFor(() => expect(result.current.isError).toBe(true));
    });
  });

  describe("useUpdateAlertStatus", () => {
    it("should update alert status and invalidate queries", async () => {
      vi.mocked(alertsService.updateAlertStatus).mockResolvedValue({});

      const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

      const { result } = renderHook(() => useUpdateAlertStatus(), { wrapper });

      await act(async () => {
        await result.current.mutateAsync({
          tenantId: "tenant-1",
          alertId: "alert-1",
          status: "acknowledged",
          acknowledgedBy: "user@example.com",
        });
      });

      expect(alertsService.updateAlertStatus).toHaveBeenCalledWith(
        "tenant-1",
        "alert-1",
        "acknowledged",
        "user@example.com",
      );

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["alerts", "tenant-1"],
      });
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["alert", "tenant-1", "alert-1"],
      });
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["alert-stats", "tenant-1"],
      });
    });
  });

  describe("useResolveAlert", () => {
    it("should resolve alert and invalidate queries", async () => {
      vi.mocked(alertsService.resolveAlert).mockResolvedValue({});

      const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

      const { result } = renderHook(() => useResolveAlert(), { wrapper });

      await act(async () => {
        await result.current.mutateAsync({
          tenantId: "tenant-1",
          alertId: "alert-1",
        });
      });

      expect(alertsService.resolveAlert).toHaveBeenCalledWith(
        "tenant-1",
        "alert-1",
      );

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["alerts", "tenant-1"],
      });
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["alert", "tenant-1", "alert-1"],
      });
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["alert-stats", "tenant-1"],
      });
    });
  });
});
