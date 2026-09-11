import type { KeycloakEvent } from "@/shared/types";

/** The geo fields an event needs before it can be put on a map. */
type Locatable = Pick<KeycloakEvent, "lat" | "long">;

/**
 * Whether an event has coordinates worth mapping.
 *
 * Three cases have to be excluded, and each one looks like a real location
 * until you check:
 *
 *  - Absent. `lat`/`long` only arrive on events with an AMFA counterpart;
 *    plain Keycloak events have none, so most rows fail here.
 *  - Null Island. GeoIP lookups that resolve nothing frequently return
 *    exactly 0,0, which is a real point in the Gulf of Guinea. Mapping it
 *    claims a location we do not have.
 *  - Out of range or non-finite. Bad upstream data would otherwise reach
 *    Leaflet, which throws on an invalid LatLng and takes the dialog with it.
 *
 * Shared by the details dialog's Coordinates row and the map beneath it, so
 * the two can never disagree: printed coordinates with no map, or a map with
 * no coordinates above it, are both worse than either rule applied
 * consistently.
 */
export function hasEventLocation(event: Locatable): boolean {
  const { lat, long } = event;
  if (lat == null || long == null) return false;
  if (!Number.isFinite(lat) || !Number.isFinite(long)) return false;
  if (lat === 0 && long === 0) return false;
  return Math.abs(lat) <= 90 && Math.abs(long) <= 180;
}
