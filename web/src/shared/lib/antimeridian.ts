import type {
  Feature,
  FeatureCollection,
  Geometry,
  Position,
} from "geojson";

/** One trip around the globe, in degrees of longitude. */
const FRAME = 360;

/**
 * Half a frame. A step larger than this between consecutive points cannot be
 * real geography at 50m resolution; it is the ring wrapping past +/-180.
 */
const HALF_FRAME = 180;

/**
 * Makes a ring's longitudes continuous by undoing +/-360 wraps.
 *
 * Natural Earth stores every longitude inside [-180, 180], so a country
 * straddling the antimeridian has a ring that steps from +179 straight to
 * -179. This walks the ring and keeps adding or removing whole frames so each
 * point stays within half a frame of its predecessor, which turns that step
 * into +179 to +181.
 */
function unwrap(ring: Position[]): Position[] {
  const out: Position[] = [ring[0]];
  let offset = 0;

  for (let i = 1; i < ring.length; i++) {
    const [lon, lat] = ring[i];
    const previousLon = out[i - 1][0];
    let candidate = lon + offset;

    while (candidate - previousLon > HALF_FRAME) {
      offset -= FRAME;
      candidate = lon + offset;
    }
    while (previousLon - candidate > HALF_FRAME) {
      offset += FRAME;
      candidate = lon + offset;
    }

    out.push([candidate, lat]);
  }

  return out;
}

/**
 * Rewrites one ring into the parts Leaflet can draw, which is one part
 * normally and two for a ring that crosses the antimeridian.
 *
 * Returns the ring unchanged in the two cases where rewriting would be wrong:
 *
 *  - It never crosses. Most rings.
 *  - It is a polar cap. Antarctica encircles the pole, so it genuinely spans
 *    every longitude and unwrapping cannot narrow that. Splitting a cap at
 *    +/-180 does not help either, because the span is real rather than an
 *    artifact; the dataset already closes the cap along -90, which Leaflet
 *    clamps to its Mercator limit. Left alone, it renders as it does today.
 */
function stitchRing(ring: Position[]): Position[][] {
  if (ring.length < 2) return [ring];

  const unwrapped = unwrap(ring);
  let min = Infinity;
  let max = -Infinity;
  for (const [lon] of unwrapped) {
    if (lon < min) min = lon;
    if (lon > max) max = lon;
  }

  if (max - min >= FRAME) return [ring];
  if (min >= -HALF_FRAME && max <= HALF_FRAME) return [ring];

  // The unwrapped ring now sits partly outside [-180, 180], so half of it
  // would fall off one edge of the map. Emitting the same shape one frame
  // over puts that half back on the opposite edge, which is what a spherical
  // projection would have done. Nothing is clipped: whichever copy lies
  // outside the view simply is not painted.
  const shift = max > HALF_FRAME ? -FRAME : FRAME;
  return [unwrapped, translate(unwrapped, shift)];
}

function translate(ring: Position[], shift: number): Position[] {
  return ring.map(([lon, lat]) => [lon + shift, lat]);
}

/**
 * Expands one polygon (a shell followed by its holes) into stitched polygons.
 *
 * Holes are unwrapped into whichever frame their shell ended up in, so a hole
 * cannot drift a full frame away from the shell that contains it.
 */
function stitchPolygon(rings: Position[][]): Position[][][] {
  const [shell, ...holes] = rings;
  const parts = stitchRing(shell);

  if (parts.length === 1) return [[parts[0], ...holes]];

  const shift = parts[1][0][0] - parts[0][0][0];
  const unwrappedHoles = holes.map(unwrap);
  return [
    [parts[0], ...unwrappedHoles],
    [parts[1], ...unwrappedHoles.map((hole) => translate(hole, shift))],
  ];
}

function stitchGeometry(geometry: Geometry): Geometry {
  if (geometry.type === "Polygon") {
    const parts = stitchPolygon(geometry.coordinates);
    return parts.length === 1
      ? { type: "Polygon", coordinates: parts[0] }
      : { type: "MultiPolygon", coordinates: parts };
  }

  if (geometry.type === "MultiPolygon") {
    return {
      type: "MultiPolygon",
      coordinates: geometry.coordinates.flatMap(stitchPolygon),
    };
  }

  // Points, lines and collections are not part of the basemap; pass them on.
  return geometry;
}

/**
 * Makes bundled world boundaries safe to draw with Leaflet.
 *
 * Leaflet projects each coordinate independently into planar Web Mercator and
 * has no notion of the antimeridian, so a ring stepping from +179 to -179 is
 * drawn as a straight line across every longitude. On a world view that shows
 * up as horizontal bands smeared across the map: in the 50m Natural Earth
 * countries this affects Russia, whose Chukotka reaches past +180, and Fiji,
 * which sits on the line.
 *
 * The vendored data comes from world-atlas, which targets d3's spherical
 * projections. d3-geo clips at the antimeridian on its own, so the file is not
 * wrong, it is simply built for a different renderer. Fixing it here rather
 * than in the file keeps the vendored asset and its checksum untouched, and
 * costs one pass over the rings on the single cached load.
 */
export function stitchAntimeridian(
  collection: FeatureCollection<Geometry>,
): FeatureCollection<Geometry> {
  return {
    ...collection,
    features: collection.features.map((feature): Feature<Geometry> => {
      if (!feature.geometry) return feature;
      return { ...feature, geometry: stitchGeometry(feature.geometry) };
    }),
  };
}
