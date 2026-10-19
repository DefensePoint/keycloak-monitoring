import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import {
  useEvents,
  useEventStats,
  isEventNotFoundError,
  EVENT_NOT_FOUND,
} from "./useEvents";
import { eventsService } from "@/shared/services";

vi.mock("@/shared/services", () => ({
  eventsService: {
    getEvents: vi.fn(),
    getStats: vi.fn(),
  },
}));

describe("useEvents hooks", () => {
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

  describe("useEvents", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(() => useEvents({ tenantId: undefined }), { wrapper });

      expect(eventsService.getEvents).not.toHaveBeenCalled();
    });

    it("should fetch events with default parameters", async () => {
      const mockData = { events: [], total: 0 };
      vi.mocked(eventsService.getEvents).mockResolvedValue(mockData);

      const { result } = renderHook(() => useEvents({ tenantId: "tenant-1" }), {
        wrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(eventsService.getEvents).toHaveBeenCalledWith(
        "tenant-1",
        100, // default limit
        0, // default offset
        undefined, // startTime
        undefined, // endTime
        undefined, // source
        undefined, // realm
        { type: undefined, severity: undefined, minRisk: undefined },
      );
    });

    it("should fetch events with custom parameters", async () => {
      const mockData = { events: [], total: 0 };
      vi.mocked(eventsService.getEvents).mockResolvedValue(mockData);

      const { result } = renderHook(
        () =>
          useEvents({
            tenantId: "tenant-1",
            limit: 50,
            offset: 10,
            startTime: "2024-01-01",
            endTime: "2024-01-31",
            realm: "master",
          }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      // A realm reaches the service as a realm, not encoded into `source`.
      expect(eventsService.getEvents).toHaveBeenCalledWith(
        "tenant-1",
        50,
        10,
        "2024-01-01",
        "2024-01-31",
        undefined, // source
        "master", // realm
        { type: undefined, severity: undefined, minRisk: undefined },
      );
    });

    it("forwards type, severity and minRisk to the service", async () => {
      vi.mocked(eventsService.getEvents).mockResolvedValue({
        events: [],
        total: 0,
      });

      const { result } = renderHook(
        () =>
          useEvents({
            tenantId: "tenant-1",
            type: "LOGIN_ERROR",
            severity: "error",
            minRisk: 3,
          }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(eventsService.getEvents).toHaveBeenCalledWith(
        "tenant-1",
        100,
        0,
        undefined,
        undefined,
        undefined,
        undefined,
        { type: "LOGIN_ERROR", severity: "error", minRisk: 3 },
      );
    });

    it("refetches when a filter changes", async () => {
      vi.mocked(eventsService.getEvents).mockResolvedValue({
        events: [],
        total: 0,
      });

      const { result, rerender } = renderHook(
        ({ severity }: { severity?: string }) =>
          useEvents({ tenantId: "tenant-1", severity }),
        { wrapper, initialProps: { severity: undefined } },
      );
      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      rerender({ severity: "critical" });

      await waitFor(() => {
        expect(eventsService.getEvents).toHaveBeenCalledTimes(2);
      });
      expect(vi.mocked(eventsService.getEvents).mock.calls[1][7]).toEqual({
        type: undefined,
        severity: "critical",
        minRisk: undefined,
      });
    });

    it("should return event data", async () => {
      const mockData = {
        events: [
          { event_id: "1", type: "LOGIN", severity: "info" },
          { event_id: "2", type: "LOGOUT", severity: "info" },
        ],
        total: 2,
      };
      vi.mocked(eventsService.getEvents).mockResolvedValue(mockData);

      const { result } = renderHook(() => useEvents({ tenantId: "tenant-1" }), {
        wrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toEqual(mockData);
    });
  });

  describe("useEventStats", () => {
    it("should not fetch when tenantId is undefined", () => {
      renderHook(() => useEventStats({ tenantId: undefined }), { wrapper });

      expect(eventsService.getStats).not.toHaveBeenCalled();
    });

    it("should fetch event statistics with default parameters", async () => {
      const mockData = { total_events: 100 };
      vi.mocked(eventsService.getStats).mockResolvedValue(mockData);

      const { result } = renderHook(
        () => useEventStats({ tenantId: "tenant-1" }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(eventsService.getStats).toHaveBeenCalledWith(
        "tenant-1",
        undefined,
        undefined,
        undefined,
      );
    });

    it("should fetch event statistics with custom parameters", async () => {
      const mockData = { total_events: 50 };
      vi.mocked(eventsService.getStats).mockResolvedValue(mockData);

      const { result } = renderHook(
        () =>
          useEventStats({
            tenantId: "tenant-1",
            startTime: "2024-01-01",
            endTime: "2024-01-31",
            realm: "master",
          }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(eventsService.getStats).toHaveBeenCalledWith(
        "tenant-1",
        "2024-01-01",
        "2024-01-31",
        "master",
      );
    });

    it("should return statistics data", async () => {
      const mockData = {
        total_events: 1000,
        by_type: { LOGIN: 500, LOGOUT: 300, LOGIN_ERROR: 200 },
      };
      vi.mocked(eventsService.getStats).mockResolvedValue(mockData);

      const { result } = renderHook(
        () => useEventStats({ tenantId: "tenant-1" }),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toEqual(mockData);
    });
  });
});

describe("isEventNotFoundError", () => {
  it("matches only the single-event endpoint's 404 message", () => {
    expect(isEventNotFoundError(new Error(EVENT_NOT_FOUND))).toBe(true);
    expect(isEventNotFoundError(new Error("HTTP 500"))).toBe(false);
    expect(isEventNotFoundError(new Error("Session expired"))).toBe(false);
    expect(isEventNotFoundError(EVENT_NOT_FOUND)).toBe(false);
    expect(isEventNotFoundError(undefined)).toBe(false);
  });
});
