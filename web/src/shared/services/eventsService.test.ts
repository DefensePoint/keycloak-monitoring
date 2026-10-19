import { describe, it, expect, vi, beforeEach } from "vitest";
import { eventsService } from "./eventsService";
import { apiClient } from "@/shared/lib/apiClient";

vi.mock("@/shared/lib/apiClient", () => ({
  apiClient: { get: vi.fn() },
}));

describe("eventsService.getEvents", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(apiClient.get).mockResolvedValue({ events: [] });
  });

  it("sends type, severity and min_risk when filters are set", async () => {
    await eventsService.getEvents(
      "tenant-1",
      50,
      0,
      undefined,
      undefined,
      undefined,
      "master",
      { type: "LOGIN_ERROR", severity: "error", minRisk: 3 },
    );

    expect(apiClient.get).toHaveBeenCalledWith("/tenants/tenant-1/events", {
      limit: 50,
      offset: 0,
      realm: "master",
      type: "LOGIN_ERROR",
      severity: "error",
      min_risk: 3,
    });
  });

  it("sends min_risk 0, which is a real threshold rather than unset", async () => {
    await eventsService.getEvents(
      "tenant-1",
      50,
      0,
      undefined,
      undefined,
      undefined,
      undefined,
      { minRisk: 0 },
    );

    expect(apiClient.get).toHaveBeenCalledWith("/tenants/tenant-1/events", {
      limit: 50,
      offset: 0,
      min_risk: 0,
    });
  });

  it("omits every filter param when no filters are given", async () => {
    await eventsService.getEvents("tenant-1", 50, 0);

    expect(apiClient.get).toHaveBeenCalledWith("/tenants/tenant-1/events", {
      limit: 50,
      offset: 0,
    });
  });
});
