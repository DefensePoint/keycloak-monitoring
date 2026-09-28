import { describe, expect, it } from "vitest";
import type {
  FeatureCollection,
  Geometry,
  MultiPolygon,
  Polygon,
} from "geojson";
import { stitchAntimeridian } from "./antimeridian";

/** Wraps one polygon ring into the FeatureCollection shape the basemap uses. */
function fcOf(ring: number[][]): FeatureCollection<Geometry> {
  return {
    type: "FeatureCollection",
    features: [
      {
        type: "Feature",
        properties: { name: "test" },
        geometry: { type: "Polygon", coordinates: [ring] } as Polygon,
      },
    ],
  };
}

/** Largest longitude step between consecutive points, the smear's signature. */
function biggestLonJump(ring: number[][]): number {
  let max = 0;
  for (let i = 1; i < ring.length; i++) {
    max = Math.max(max, Math.abs(ring[i][0] - ring[i - 1][0]));
  }
  return max;
}

function partsOf(fc: FeatureCollection<Geometry>): number[][][] {
  const g = fc.features[0].geometry;
  if (g.type === "Polygon") return [(g as Polygon).coordinates[0]];
  if (g.type === "MultiPolygon")
    return (g as MultiPolygon).coordinates.map((p) => p[0]);
  throw new Error(`unexpected geometry ${g.type}`);
}

describe("stitchAntimeridian", () => {
  it("leaves a ring that never crosses the antimeridian untouched", () => {
    const ring = [
      [10, 50],
      [20, 50],
      [20, 60],
      [10, 60],
      [10, 50],
    ];

    const out = stitchAntimeridian(fcOf(ring));

    expect(partsOf(out)).toEqual([ring]);
  });

  it("removes the 360-degree jump that draws a line across the map", () => {
    // Chukotka's shape: the ring steps from +170 to -170, which Leaflet draws
    // as a straight line back across every longitude.
    const ring = [
      [170, 60],
      [-170, 60],
      [-170, 70],
      [170, 70],
      [170, 60],
    ];
    expect(biggestLonJump(ring)).toBeGreaterThan(180);

    const out = stitchAntimeridian(fcOf(ring));

    for (const part of partsOf(out)) {
      expect(biggestLonJump(part)).toBeLessThanOrEqual(180);
    }
  });

  it("keeps the geometry visible on both edges, as two parts 360 apart", () => {
    const ring = [
      [170, 60],
      [-170, 60],
      [-170, 70],
      [170, 70],
      [170, 60],
    ];

    const parts = partsOf(stitchAntimeridian(fcOf(ring)));

    expect(parts).toHaveLength(2);
    // Same shape, one frame apart: what falls off one edge appears on the other.
    const shifted = parts[1].map(([lon, lat]) => [lon + 360, lat]);
    expect(shifted).toEqual(parts[0]);
  });

  it("keeps a hole with its shell in both frames", () => {
    // No country in the 50m dataset currently has a crossing shell *and*
    // holes: Antarctica is the only crossing polygon with a hole, and its
    // shell is a polar cap, so it returns early. This covers the branch
    // anyway, because a hole left behind in one frame would punch a gap in
    // the wrong copy, and the dataset could gain an island tomorrow.
    const shell = [
      [170, 60],
      [-170, 60],
      [-170, 70],
      [170, 70],
      [170, 60],
    ];
    const hole = [
      [175, 63],
      [-175, 63],
      [-175, 66],
      [175, 66],
      [175, 63],
    ];

    const g = stitchAntimeridian({
      type: "FeatureCollection",
      features: [
        {
          type: "Feature",
          properties: {},
          geometry: { type: "Polygon", coordinates: [shell, hole] } as Polygon,
        },
      ],
    }).features[0].geometry as MultiPolygon;

    expect(g.type).toBe("MultiPolygon");
    expect(g.coordinates).toHaveLength(2);
    for (const [outer, inner] of g.coordinates) {
      expect(inner).toBeDefined();
      // The hole must sit inside its own shell's longitude range, not a
      // frame away from it.
      const lons = (r: number[][]) => r.map(([lon]) => lon);
      expect(Math.min(...lons(inner))).toBeGreaterThanOrEqual(
        Math.min(...lons(outer)),
      );
      expect(Math.max(...lons(inner))).toBeLessThanOrEqual(
        Math.max(...lons(outer)),
      );
    }
  });

  it("leaves a polar cap alone rather than trying to unwrap it", () => {
    // Antarctica encircles the pole, so it genuinely spans every longitude.
    // Unwrapping cannot narrow that, and cutting it would break the cap.
    const ring = [
      [-180, -85],
      [-90, -85],
      [0, -85],
      [90, -85],
      [180, -85],
      [180, -90],
      [-180, -90],
      [-180, -85],
    ];

    const out = stitchAntimeridian(fcOf(ring));

    expect(partsOf(out)).toEqual([ring]);
  });
});

describe("stitchAntimeridian on the vendored basemap", () => {
  /**
   * Guards the actual artifact rather than a synthetic stand-in. Reads the
   * shipped 50m countries file, converts it the way useWorldGeo does, and
   * checks that nothing is left that Leaflet would smear across the map.
   */
  it("leaves no ring that Leaflet would draw across the whole map", async () => {
    const { readFileSync } = await import("node:fs");
    const { feature } = await import("topojson-client");
    const raw = JSON.parse(
      readFileSync("public/geo/countries-50m.json", "utf8"),
    );
    const fc = feature(
      raw,
      raw.objects.countries,
    ) as FeatureCollection<Geometry>;

    const ringsOf = (c: FeatureCollection<Geometry>) =>
      c.features.flatMap((f) => {
        const g = f.geometry;
        if (g?.type === "Polygon") return (g as Polygon).coordinates;
        if (g?.type === "MultiPolygon")
          return (g as MultiPolygon).coordinates.flat();
        return [];
      });

    // Polar caps are judged geographically rather than by reusing the
    // implementation's own span rule, so this test can still fail if that rule
    // is wrong. A ring reaching beyond +/-85 is past Web Mercator's usable
    // range and is a cap Leaflet clamps: Antarctica, and nothing else here.
    // Russia tops out at 83.6 and Fiji sits near the equator.
    const isPolarCap = (ring: number[][]) =>
      ring.some(([, lat]) => Math.abs(lat) > 85);

    const before = ringsOf(fc).filter(
      (r) => !isPolarCap(r) && biggestLonJump(r) > 180,
    );
    expect(before.length).toBeGreaterThan(0); // the bug is really there

    const after = ringsOf(stitchAntimeridian(fc)).filter(
      (r) => !isPolarCap(r) && biggestLonJump(r) > 180,
    );
    expect(after).toEqual([]);
  });
});
