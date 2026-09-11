import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import {
  QueryClient,
  QueryClientProvider,
  onlineManager,
} from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createElement } from "react";

import { useWorldGeo } from "./useWorldGeo";

// A minimal but real TopoJSON topology: one square "country". Enough to prove
// the conversion runs and yields GeoJSON features, without shipping a fixture
// the size of the real file.
const topology = {
  type: "Topology",
  arcs: [
    [
      [0, 0],
      [1, 0],
      [0, 1],
      [-1, 0],
      [0, -1],
    ],
  ],
  transform: { scale: [1, 1], translate: [0, 0] },
  objects: {
    countries: {
      type: "GeometryCollection",
      geometries: [
        { type: "Polygon", arcs: [[0]], properties: { name: "Testland" } },
      ],
    },
  },
};

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return createElement(QueryClientProvider, { client }, children);
}

describe("useWorldGeo", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("converts the vendored topology into GeoJSON features", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => topology,
    } as Response);

    const { result } = renderHook(() => useWorldGeo(), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.type).toBe("FeatureCollection");
    expect(result.current.data?.features).toHaveLength(1);
  });

  it("requests the data from our own origin", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => topology,
    } as Response);

    const { result } = renderHook(() => useWorldGeo(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // Guards the airgap requirement at the one place a host could creep in.
    const url = String(vi.mocked(fetch).mock.calls[0][0]);
    expect(url.startsWith("/")).toBe(true);
    expect(url).not.toMatch(/^https?:/);
  });

  it("fails loudly when the vendored file is missing", async () => {
    // A 404 here means a broken deployment, not a network problem, so the
    // error has to carry the URL that was tried.
    vi.mocked(fetch).mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({}),
    } as Response);

    const { result } = renderHook(() => useWorldGeo(), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toContain("404");
    expect(result.current.error?.message).toContain("countries-50m.json");
  });

  it("rejects a topology with no countries object", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ type: "Topology", arcs: [], objects: {} }),
    } as Response);

    const { result } = renderHook(() => useWorldGeo(), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toContain("malformed");
  });

  describe("while the browser reports itself offline", () => {
    // The airgap case, and the reason this hook sets networkMode: "always".
    // React Query's default ("online") pauses the fetch and its retries when
    // it believes there is no connection, so the query sits in "pending" with
    // fetchStatus "paused" forever: no data and no error, which renders a map
    // with no basemap and no explanation. The file is a static asset on our
    // own origin, so connectivity has nothing to do with reading it.
    beforeEach(() => onlineManager.setOnline(false));
    afterEach(() => onlineManager.setOnline(true));

    it("still loads the boundaries", async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => topology,
      } as Response);

      const { result } = renderHook(() => useWorldGeo(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true), {
        timeout: 5000,
      });
      expect(result.current.fetchStatus).not.toBe("paused");
      expect(result.current.data?.features).toHaveLength(1);
    });

    it("still reports a failure instead of hanging", async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 404,
        json: async () => ({}),
      } as Response);

      const { result } = renderHook(() => useWorldGeo(), { wrapper });

      await waitFor(() => expect(result.current.isError).toBe(true), {
        timeout: 5000,
      });
      expect(result.current.error?.message).toContain("404");
    });
  });

  describe("while the tab is hidden", () => {
    // The reason this hook sets retry: false. query-core gates each retry on
    // canContinue(), which requires focusManager.isFocused() - which resolves
    // to document.visibilityState !== "hidden". In a hidden tab it pauses
    // instead, and the query sits in "pending" with fetchStatus "paused" until
    // the tab is shown again: no data and no error, so a retrying map would
    // show neither a basemap nor an explanation.
    //
    // networkMode: "always" does not help here; it only bypasses the online
    // check, not the visibility check. Setting retry: 1 makes the first test
    // below time out, which is how this was pinned.
    // Override the document rather than focusManager.setFocused(). That
    // setter mutates a module-level singleton shared by every test file in
    // the worker, which made unrelated mutation tests flake: mutations run
    // through the same retryer, so a lingering "unfocused" left their retries
    // paused. focusManager.isFocused() consults visibilityState when nothing
    // has been set explicitly, so this reaches the same gate locally.
    beforeEach(() => {
      Object.defineProperty(document, "visibilityState", {
        configurable: true,
        get: () => "hidden",
      });
    });
    afterEach(() => {
      // @ts-expect-error - removing the override restores jsdom's own getter
      delete document.visibilityState;
    });

    it("reports a failure immediately rather than pausing", async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 404,
        json: async () => ({}),
      } as Response);

      const { result } = renderHook(() => useWorldGeo(), { wrapper });

      await waitFor(() => expect(result.current.isError).toBe(true), {
        timeout: 5000,
      });
      expect(result.current.fetchStatus).toBe("idle");
      expect(result.current.error?.message).toContain("404");
    });

    it("still loads the boundaries", async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => topology,
      } as Response);

      const { result } = renderHook(() => useWorldGeo(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true), {
        timeout: 5000,
      });
      expect(result.current.data?.features).toHaveLength(1);
    });
  });
});
