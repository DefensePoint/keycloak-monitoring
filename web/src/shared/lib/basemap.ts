/**
 * Offline basemap definition.
 *
 * KMT draws its maps from world boundary data shipped with the application,
 * not from a tile service. Third-party APIs are prohibited in this platform
 * because deployments can be airgapped, and a tile service fails there in the
 * worst possible way: the CDN is unreachable, so the map renders as an empty
 * panel while every other part of the page looks healthy.
 *
 * The trade-off is detail. Boundaries stop at country and coastline level, so
 * there are no streets and no city polygons. That matches the data being
 * plotted: AMFA locations come from IP geolocation, which resolves to a city
 * at best and often to the registered centroid of the ISP's address block.
 * Street-level tiles implied a precision the coordinates never had.
 *
 * See web/public/geo/SOURCE.md for the data's provenance and licence.
 */

/**
 * URL of the vendored TopoJSON, served from our own origin.
 *
 * A root-relative path, so it resolves against whatever host serves the app
 * and never leaves it. Lives in web/public/ rather than being imported, which
 * keeps 756 KB of JSON out of the JavaScript bundle and lets the browser and
 * any proxy cache it as a normal static asset.
 */
export const WORLD_TOPOJSON_URL = "/geo/countries-50m.json";

/**
 * URL of the vendored city list, served from our own origin.
 *
 * A slimmed extract of Natural Earth's 1:10m populated places: 7,342 cities
 * reduced to name, country, population, Natural Earth's own `min_zoom` label
 * guidance, and coordinates. 766 KB raw, about 161 KB gzipped, against 4.8 MB
 * for the source GeoJSON.
 *
 * 1:10m rather than 1:50m because the coarser tier is too thin once a map is
 * zoomed into one country: it holds 7 cities for the United Kingdom, 5 for
 * Germany and 1 for Albania, against 57, 58 and 26 here. Albania decided it,
 * being both a real source of traffic and, at one city, unlabellable.
 */
export const WORLD_CITIES_URL = "/geo/cities-10m.json";

/**
 * Zoom at which city labels start appearing.
 *
 * Below this the map is a world view where the event markers are the subject
 * and labels would only crowd them. From here up, the reader needs something
 * to orient against, because the boundary layer alone leaves a marker sitting
 * in an unlabelled polygon.
 */
export const CITY_LABEL_MIN_ZOOM = 3;

/**
 * How far ahead of Natural Earth's per-city guidance to label.
 *
 * Each city carries a `minZoom` saying when a cartographer would introduce it.
 * Zero means following that judgement exactly, which the 1:10m dataset makes
 * the right default: it already yields around 20 names over France and 17 over
 * southern England at the per-event map's zoom.
 *
 * This existed as a density lever while the city list was the thinner 1:50m
 * tier. It is kept because it is the one knob for this, but note that raising
 * it now overshoots: 1 pushed the England view to 58 labels, which collided
 * with each other and with the event marker, since Leaflet does no label
 * collision avoidance.
 */
export const CITY_LABEL_ZOOM_BONUS = 0;

/**
 * Upper bound on labels drawn at once.
 *
 * Natural Earth's per-city `min_zoom` already thins the list, but a dense
 * region at high zoom can still return hundreds. Each label is a DOM node, so
 * this caps both the clutter and the cost. The list is sorted by `min_zoom`,
 * so the cap keeps the most significant cities.
 */
export const CITY_LABEL_LIMIT = 80;

/**
 * Attribution rendered in the map corner.
 *
 * Natural Earth is public domain and requires no attribution, so unlike the
 * tile service this replaces, nothing here is a licence condition. The credit
 * is kept because Natural Earth asks for it as a courtesy, and because a map
 * that names its data source is easier to trust than one that does not.
 */
export const BASEMAP_ATTRIBUTION =
  'Boundaries: <a href="https://www.naturalearthdata.com">Natural Earth</a> (public domain)';

/**
 * Maximum zoom the maps allow.
 *
 * 1:50m boundaries turn visibly angular past this, and there is no further
 * detail underneath to reveal. Capping it is more honest than letting someone
 * zoom to a street view that is drawing nothing but a coastline.
 */
export const BASEMAP_MAX_ZOOM = 7;

/** Minimum zoom. Below this the world repeats and the view stops meaning much. */
export const BASEMAP_MIN_ZOOM = 2;

/**
 * Fill and stroke for the boundary layer, plus the ground behind it.
 *
 * Hard-coded rather than read from the MUI theme because Leaflet paints into
 * its own SVG/canvas panes outside React's styling, and because both maps must
 * look identical. Tuned for the dark dashboard: land slightly lifted from the
 * ground, borders present but quiet, so markers stay the loudest thing.
 */
export const BASEMAP_STYLE = {
  /** Behind everything: the "sea". */
  background: "#0f1216",
  /** Country fill. */
  land: "#1c2128",
  /** Country borders. */
  border: "#2f3742",
  /** City dot and its label. */
  city: "#5c6875",
  cityLabel: "#8b97a5",
} as const;
