/**
 * CARTO basemap tile URL construction.
 *
 * CARTO's raster basemaps require an API key. Without one the CDN still
 * answers 200 with a valid PNG, but the image has "API KEY REQUIRED"
 * watermarked across it, so the map looks broken while every network request
 * appears healthy. That failure mode is why this is worth a named function
 * with a test rather than an inline string.
 *
 * A key is free: CARTO's FAQ states anyone may request one and use the
 * basemaps "free of charge up to a fair use limit of 5 million tile requests
 * per calendar month", with no CARTO account needed and no requirement to
 * declare commercial use. Request one at https://carto.com/basemaps/apikey/
 * and supply it as VITE_CARTO_BASEMAP_KEY at build time.
 *
 * Note that CARTO describe the raster tiles as "being retired" (no timeline
 * given) in favour of vector styles, so treat this as the supported path for
 * now rather than indefinitely.
 */

/** Base template, minus the key. Leaflet substitutes {s} {z} {x} {y} {r}. */
const CARTO_DARK_TEMPLATE =
  "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png";

/**
 * Returns the dark basemap tile URL, appending the API key when one is set.
 *
 * With no key the unkeyed template is returned unchanged, which renders the
 * watermarked tiles rather than failing outright. That is deliberate: a
 * developer with no key still gets a usable, obviously-annotated map instead
 * of an empty panel, and the watermark names the page to fix it.
 */
export function cartoDarkTileUrl(apiKey?: string): string {
  const key = apiKey?.trim();
  if (!key) {
    return CARTO_DARK_TEMPLATE;
  }
  return `${CARTO_DARK_TEMPLATE}?key=${encodeURIComponent(key)}`;
}
