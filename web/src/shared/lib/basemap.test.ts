import { describe, expect, it } from "vitest";

import {
  BASEMAP_ATTRIBUTION,
  BASEMAP_MAX_ZOOM,
  BASEMAP_MIN_ZOOM,
  BASEMAP_STYLE,
  WORLD_TOPOJSON_URL,
} from "./basemap";

describe("basemap", () => {
  it("serves the boundary data from our own origin", () => {
    // The whole point of this module: no third-party host may appear here.
    // Airgapped deployments have no route off the network, and a tile service
    // fails silently there, rendering an empty panel while the rest of the
    // page looks healthy.
    expect(WORLD_TOPOJSON_URL.startsWith("/")).toBe(true);
    expect(WORLD_TOPOJSON_URL).not.toMatch(/^https?:/);
    expect(WORLD_TOPOJSON_URL).not.toContain("//");
  });

  it("points at the vendored file that actually ships", () => {
    // web/public/geo/countries-50m.json. If the filename drifts, the map
    // silently loses its basemap and keeps its markers.
    expect(WORLD_TOPOJSON_URL).toBe("/geo/countries-50m.json");
  });

  it("credits Natural Earth as the data source", () => {
    // Public domain, so this is courtesy rather than a licence condition,
    // unlike the tile service this replaced.
    expect(BASEMAP_ATTRIBUTION).toContain("Natural Earth");
    expect(BASEMAP_ATTRIBUTION).toContain("naturalearthdata.com");
  });

  it("caps zoom where the boundary detail runs out", () => {
    // 1:50m boundaries turn angular past ~7, and there is nothing beneath
    // them to reveal, so letting someone zoom further only looks broken.
    expect(BASEMAP_MAX_ZOOM).toBeLessThanOrEqual(8);
    expect(BASEMAP_MIN_ZOOM).toBeLessThan(BASEMAP_MAX_ZOOM);
  });

  it("defines a ground colour, land and border", () => {
    // With no tile layer, an unset background lets the host page show through
    // and the map reads as failed rather than styled.
    for (const value of Object.values(BASEMAP_STYLE)) {
      expect(value).toMatch(/^#[0-9a-f]{6}$/i);
    }
    expect(BASEMAP_STYLE.land).not.toBe(BASEMAP_STYLE.background);
  });
});
