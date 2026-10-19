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

  it("should fallback to all if the saved realm is unavailable", () => {
    sessionStorage.setItem("selectedRealm_tenant-1", "non-existent-realm");

    const { result } = renderHook(() =>
      useRealmSelector({
        tenantId: "tenant-1",
        defaultRealm: "master",
        availableRealms: mockRealms,
      }),
    );

    expect(result.current.selectedRealm).toBe("all");
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

  it.each([null, "test-realm", "all"])(
    "resolves saved choice %s after realms arrive without saving a loading fallback",
    (saved) => {
      if (saved !== null)
        sessionStorage.setItem("selectedRealm_tenant-1", saved);
      const { result, rerender } = renderHook(
        ({ realms }) =>
          useRealmSelector({
            tenantId: "tenant-1",
            defaultRealm: "master",
            availableRealms: realms,
          }),
        { initialProps: { realms: [] as typeof mockRealms } },
      );
      expect(result.current.selectedRealm).toBe("all");
      expect(sessionStorage.getItem("selectedRealm_tenant-1")).toBe(saved);
      rerender({ realms: mockRealms });
      expect(result.current.selectedRealm).toBe(saved ?? "master");
    },
  );

  it("restores each tenant's choice across tenant switches and loading", () => {
    const { result, rerender } = renderHook(
      ({ tenantId, defaultRealm, realms }) =>
        useRealmSelector({ tenantId, defaultRealm, availableRealms: realms }),
      {
        initialProps: {
          tenantId: "tenant-1",
          defaultRealm: "master",
          realms: mockRealms,
        },
      },
    );
    act(() => result.current.handleRealmChange("test-realm"));
    rerender({ tenantId: "tenant-2", defaultRealm: "dev-realm", realms: [] });
    expect(result.current.selectedRealm).toBe("all");
    rerender({
      tenantId: "tenant-2",
      defaultRealm: "dev-realm",
      realms: mockRealms,
    });
    expect(result.current.selectedRealm).toBe("dev-realm");
    act(() => result.current.handleRealmChange("all"));
    rerender({
      tenantId: "tenant-1",
      defaultRealm: "master",
      realms: mockRealms,
    });
    expect(result.current.selectedRealm).toBe("test-realm");
    rerender({
      tenantId: "tenant-2",
      defaultRealm: "dev-realm",
      realms: mockRealms,
    });
    expect(result.current.selectedRealm).toBe("all");
  });

  it("restores a user choice after a reload whose realms arrive late", () => {
    const first = renderHook(() =>
      useRealmSelector({
        tenantId: "tenant-1",
        defaultRealm: "master",
        availableRealms: mockRealms,
      }),
    );
    act(() => first.result.current.handleRealmChange("dev-realm"));
    first.unmount();
    const next = renderHook(
      ({ realms }) =>
        useRealmSelector({
          tenantId: "tenant-1",
          defaultRealm: "master",
          availableRealms: realms,
        }),
      { initialProps: { realms: [] as typeof mockRealms } },
    );
    expect(next.result.current.selectedRealm).toBe("all");
    next.rerender({ realms: mockRealms });
    expect(next.result.current.selectedRealm).toBe("dev-realm");
  });

  it("falls back when a selected realm is removed from the visible list", () => {
    const { result, rerender } = renderHook(
      ({ realms }) =>
        useRealmSelector({
          tenantId: "tenant-1",
          defaultRealm: "master",
          availableRealms: realms,
        }),
      { initialProps: { realms: mockRealms } },
    );
    act(() => result.current.handleRealmChange("test-realm"));
    rerender({ realms: [{ realm_name: "master" }] });
    expect(result.current.selectedRealm).toBe("all");
  });

  it("uses the default after realms load and keeps choices in memory when storage is blocked", () => {
    const read = vi
      .spyOn(Storage.prototype, "getItem")
      .mockImplementation(() => {
        throw new Error("blocked");
      });
    const write = vi
      .spyOn(Storage.prototype, "setItem")
      .mockImplementation(() => {
        throw new Error("blocked");
      });
    try {
      const { result, rerender } = renderHook(
        ({ realms }) =>
          useRealmSelector({
            tenantId: "tenant-1",
            defaultRealm: "master",
            availableRealms: realms,
          }),
        { initialProps: { realms: [] as typeof mockRealms } },
      );
      expect(result.current.selectedRealm).toBe("all");
      rerender({ realms: mockRealms });
      expect(result.current.selectedRealm).toBe("master");
      act(() => result.current.handleRealmChange("test-realm"));
      rerender({ realms: [...mockRealms] });
      expect(result.current.selectedRealm).toBe("test-realm");
    } finally {
      read.mockRestore();
      write.mockRestore();
    }
  });
});
