import { describe, expect, it } from "vitest";

import { cartoDarkTileUrl } from "./basemap";

describe("cartoDarkTileUrl", () => {
  it("appends the key so CARTO serves un-watermarked tiles", () => {
    expect(cartoDarkTileUrl("abc123")).toBe(
      "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png?key=abc123",
    );
  });

  it("preserves Leaflet's template placeholders", () => {
    // Leaflet substitutes these at request time; mangling them yields 404s
    // for every tile, so pin them explicitly.
    const url = cartoDarkTileUrl("abc123");
    for (const token of ["{s}", "{z}", "{x}", "{y}", "{r}"]) {
      expect(url).toContain(token);
    }
  });

  it("returns the unkeyed template when no key is configured", () => {
    // Deliberate: a developer with no key gets a watermarked but usable map
    // rather than an empty panel, and the watermark names the page to fix it.
    const expected =
      "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png";
    expect(cartoDarkTileUrl(undefined)).toBe(expected);
    expect(cartoDarkTileUrl("")).toBe(expected);
    expect(cartoDarkTileUrl("   ")).toBe(expected);
  });

  it("url-encodes the key rather than splicing it in raw", () => {
    // Vite inlines whatever is in the env var, so a stray character must not
    // be able to alter the query string.
    expect(cartoDarkTileUrl("a b&z=1")).toContain("?key=a%20b%26z%3D1");
  });
});
