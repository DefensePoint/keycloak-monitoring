import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useRealmSelector } from "./useRealmSelector";

describe("useRealmSelector", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sessionStorage.clear();
  });

  const mockRealms = [
    { realm_name: "master" },
    { realm_name: "test-realm" },
    { realm_name: "dev-realm" },
  ];

  it("should return 'all' when no realms are available", () => {
    const { result } = renderHook(() =>
      useRealmSelector({
        tenantId: "tenant-1",
        defaultRealm: "master",
        availableRealms: [],
      }),
    );

    expect(result.current.selectedRealm).toBe("all");
  });

  it("should return 'all' when tenantId is undefined", () => {
    const { result } = renderHook(() =>
      useRealmSelector({
        tenantId: undefined,
        defaultRealm: "master",
        availableRealms: mockRealms,
      }),
    );

    expect(result.current.selectedRealm).toBe("all");
  });

  it("should return default realm when no session storage value exists", () => {
    const { result } = renderHook(() =>
      useRealmSelector({
        tenantId: "tenant-1",
        defaultRealm: "master",
        availableRealms: mockRealms,
      }),
    );

    expect(result.current.selectedRealm).toBe("master");
  });

  it("should return session storage value if it exists and is valid", () => {
    sessionStorage.setItem("selectedRealm_tenant-1", "test-realm");

    const { result } = renderHook(() =>
      useRealmSelector({
        tenantId: "tenant-1",
        defaultRealm: "master",
        availableRealms: mockRealms,
      }),
    );

    expect(result.current.selectedRealm).toBe("test-realm");
  });

  it("should return 'all' if session storage value is 'all'", () => {
    sessionStorage.setItem("selectedRealm_tenant-1", "all");

    const { result } = renderHook(() =>
      useRealmSelector({
        tenantId: "tenant-1",
        defaultRealm: "master",
        availableRealms: mockRealms,
      }),
    );

    expect(result.current.selectedRealm).toBe("all");
  });

  it("should fallback to default realm if session storage value is invalid", () => {
    sessionStorage.setItem("selectedRealm_tenant-1", "non-existent-realm");

    const { result } = renderHook(() =>
      useRealmSelector({
        tenantId: "tenant-1",
        defaultRealm: "master",
        availableRealms: mockRealms,
      }),
    );

    expect(result.current.selectedRealm).toBe("master");
  });

  it("should fallback to 'all' if default realm is not in available realms", () => {
    const { result } = renderHook(() =>
      useRealmSelector({
        tenantId: "tenant-1",
        defaultRealm: "non-existent",
        availableRealms: mockRealms,
      }),
    );

    expect(result.current.selectedRealm).toBe("all");
  });

  it("should update selected realm and save to session storage", () => {
    const { result } = renderHook(() =>
      useRealmSelector({
        tenantId: "tenant-1",
        defaultRealm: "master",
        availableRealms: mockRealms,
      }),
    );

    act(() => {
      result.current.handleRealmChange("test-realm");
    });

    expect(result.current.selectedRealm).toBe("test-realm");
    expect(sessionStorage.getItem("selectedRealm_tenant-1")).toBe("test-realm");
  });

  it("should handle realm change to 'all'", () => {
    const { result } = renderHook(() =>
      useRealmSelector({
        tenantId: "tenant-1",
        defaultRealm: "master",
        availableRealms: mockRealms,
      }),
    );

    act(() => {
      result.current.handleRealmChange("all");
    });

    expect(result.current.selectedRealm).toBe("all");
    expect(sessionStorage.getItem("selectedRealm_tenant-1")).toBe("all");
  });

  it("should not save to session storage if tenantId is undefined", () => {
    const { result } = renderHook(() =>
      useRealmSelector({
        tenantId: undefined,
        defaultRealm: "master",
        availableRealms: mockRealms,
      }),
    );

    act(() => {
      result.current.handleRealmChange("test-realm");
    });

    expect(result.current.selectedRealm).toBe("test-realm");
    expect(sessionStorage.getItem("selectedRealm_undefined")).toBeNull();
  });
});
