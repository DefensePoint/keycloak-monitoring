import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import {
  useAmfaStats,
  isAllRealmsUnsupportedError,
  isRealmScopeRequiresRealmError,
  AMFA_ALL_REALMS_UNSUPPORTED,
  AMFA_REALM_SCOPE_REQUIRES_REALM,
} from "./useAmfaStats";
import { apiClient } from "@/shared/lib/apiClient";

vi.mock("@/shared/lib/apiClient", () => ({
  apiClient: {
    get: vi.fn(),
  },
}));

describe("isAllRealmsUnsupportedError", () => {
  it("matches the specific backend error code", () => {
    expect(
      isAllRealmsUnsupportedError(new Error(AMFA_ALL_REALMS_UNSUPPORTED)),
    ).toBe(true);
  });

  it("does not match other errors", () => {
    expect(isAllRealmsUnsupportedError(new Error("amfa_not_configured"))).toBe(
      false,
    );
  });

  it("does not match non-Error values", () => {
    expect(isAllRealmsUnsupportedError("amfa_all_realms_unsupported")).toBe(
      false,
    );
    expect(isAllRealmsUnsupportedError(undefined)).toBe(false);
  });

  it("does not match the permission-driven refusal", () => {
    expect(
      isAllRealmsUnsupportedError(new Error(AMFA_REALM_SCOPE_REQUIRES_REALM)),
    ).toBe(false);
  });
});

describe("isRealmScopeRequiresRealmError", () => {
  it("matches the permission-driven backend error code", () => {
    expect(
      isRealmScopeRequiresRealmError(
        new Error(AMFA_REALM_SCOPE_REQUIRES_REALM),
      ),
    ).toBe(true);
  });

  it("does not match the transport's all-realms refusal", () => {
    expect(
      isRealmScopeRequiresRealmError(new Error(AMFA_ALL_REALMS_UNSUPPORTED)),
    ).toBe(false);
  });

  it("does not match other errors or non-Error values", () => {
    expect(
      isRealmScopeRequiresRealmError(new Error("amfa_not_configured")),
    ).toBe(false);
    expect(
      isRealmScopeRequiresRealmError(AMFA_REALM_SCOPE_REQUIRES_REALM),
    ).toBe(false);
    expect(isRealmScopeRequiresRealmError(undefined)).toBe(false);
  });
});

describe("useAmfaStats", () => {
  let queryClient: QueryClient;

  const wrapper = ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);

  beforeEach(() => {
    vi.clearAllMocks();
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
  });

  it("does not fetch when tenantId is undefined", () => {
    renderHook(() => useAmfaStats({ tenantId: undefined }), { wrapper });

    expect(apiClient.get).not.toHaveBeenCalled();
  });

  it("fetches stats for a specific realm", async () => {
    const mockData = { total: 5, risky: 2, unique_users: 4, flagged_ips: 0 };
    vi.mocked(apiClient.get).mockResolvedValue(mockData);

    const { result } = renderHook(
      () => useAmfaStats({ tenantId: "tenant-1", realm: "master" }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(apiClient.get).toHaveBeenCalledWith("/tenants/tenant-1/amfa/stats", {
      realm_id: "master",
    });
    expect(result.current.data).toEqual(mockData);
  });

  it("stops retrying once the backend reports All Realms aggregation is unsupported", async () => {
    vi.mocked(apiClient.get).mockRejectedValue(
      new Error(AMFA_ALL_REALMS_UNSUPPORTED),
    );

    const { result } = renderHook(
      () => useAmfaStats({ tenantId: "tenant-1" }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isError).toBe(true));

    // Only the initial attempt - the unsupported error must not be retried.
    expect(apiClient.get).toHaveBeenCalledTimes(1);
  });

  it("stops retrying once the backend refuses on the caller's realm scope", async () => {
    vi.mocked(apiClient.get).mockRejectedValue(
      new Error(AMFA_REALM_SCOPE_REQUIRES_REALM),
    );

    const { result } = renderHook(
      () => useAmfaStats({ tenantId: "tenant-1" }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(apiClient.get).toHaveBeenCalledTimes(1);
  });

  it("surfaces the realm an all-realms request was narrowed to", async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      total: 5,
      risky: 2,
      unique_users: 4,
      flagged_ips: 0,
      applied_realm_id: "master",
    });

    const { result } = renderHook(
      () => useAmfaStats({ tenantId: "tenant-1" }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(apiClient.get).toHaveBeenCalledWith(
      "/tenants/tenant-1/amfa/stats",
      {},
    );
    expect(result.current.data?.applied_realm_id).toBe("master");
  });
});
