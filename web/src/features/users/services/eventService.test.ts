import { describe, it, expect, vi, beforeEach } from "vitest";
import { eventService } from "./eventService";
import { apiClient } from "@/shared/lib/apiClient";

vi.mock("@/shared/lib/apiClient", () => ({
  apiClient: { get: vi.fn() },
}));

const requestedParams = () => {
  const url = vi.mocked(apiClient.get).mock.calls[0][0] as string;
  return new URL(url, "http://x").searchParams;
};

describe("users eventService.getEvents", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(apiClient.get).mockResolvedValue({ events: [], total: 0 });
  });

  it("scopes the query to the user on the server", async () => {
    await eventService.getEvents("tenant-1", 1000, 0, undefined, undefined, {
      userId: "user-123",
      severity: "error",
    });

    const params = requestedParams();
    expect(params.get("user_id")).toBe("user-123");
    expect(params.get("severity")).toBe("error");
    expect(params.get("limit")).toBe("1000");
  });

  it("scopes the query to the realm on the server", async () => {
    await eventService.getEvents("tenant-1", 1000, 0, undefined, undefined, {
      userId: "user-123",
      realm: "realm-a",
    });

    expect(requestedParams().get("realm")).toBe("realm-a");
  });

  it("sends no filter when none is given", async () => {
    await eventService.getEvents("tenant-1", 1000, 0);

    const params = requestedParams();
    expect(params.has("user_id")).toBe(false);
    expect(params.has("severity")).toBe(false);
    expect(params.has("realm")).toBe(false);
  });
});
