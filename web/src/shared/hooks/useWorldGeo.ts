import { useQuery } from "@tanstack/react-query";
import { feature } from "topojson-client";
import type { Topology, GeometryCollection } from "topojson-specification";
import type { FeatureCollection, Geometry } from "geojson";
import { WORLD_TOPOJSON_URL } from "@/shared/lib/basemap";
import { stitchAntimeridian } from "@/shared/lib/antimeridian";

/**
 * Loads the vendored world boundaries for the maps to draw.
 *
 * Fetched from our own origin rather than imported so the 756 KB of JSON stays
 * out of the JavaScript bundle, and stored as TopoJSON rather than GeoJSON
 * because the topology form is roughly a third of the size on the wire.
 * Converting it costs one pass at load.
 *
 * Cached forever: the file is vendored and only changes when someone bumps it
 * deliberately, so refetching it would be pure waste. Both maps share this
 * query, so opening a per-event map after the aggregate map has loaded costs
 * no network at all.
 */
export const useWorldGeo = () =>
  useQuery<FeatureCollection<Geometry>>({
    queryKey: ["world-geo", WORLD_TOPOJSON_URL],
    queryFn: async () => {
      const res = await fetch(WORLD_TOPOJSON_URL);
      if (!res.ok) {
        throw new Error(
          `world boundaries unavailable: HTTP ${res.status} from ${WORLD_TOPOJSON_URL}`,
        );
      }
      const topology = (await res.json()) as Topology;
      const countries = topology.objects.countries as GeometryCollection;
      if (!countries) {
        throw new Error("world boundaries malformed: no 'countries' object");
      }
      // Leaflet has no antimeridian handling, and this file is authored for
      // d3's spherical projections, which do. Without this the rings that
      // cross +/-180 smear horizontal bands across a world view.
      return stitchAntimeridian(
        feature(topology, countries) as FeatureCollection<Geometry>,
      );
    },
    staleTime: Infinity,
    gcTime: Infinity,
    refetchOnWindowFocus: false,
    // No retry, for two reasons.
    //
    // The file either shipped with the build or it did not, so a second
    // attempt cannot succeed where the first failed.
    //
    // More importantly, a retry can strand the query. query-core gates each
    // retry on `canContinue()`, which requires focusManager.isFocused() -
    // despite the name, that resolves to document.visibilityState !== "hidden".
    // In a hidden tab it calls pause() instead, and the query sits in status
    // "pending" with fetchStatus "paused" until the tab is shown again.
    // Pending means no data and no error, so a retrying map would show neither
    // a basemap nor an explanation. With no retry the first failure becomes an
    // error immediately, whatever the tab is doing.
    //
    // A user looking at the map is by definition looking at a visible tab, so
    // this is rarely reachable in normal use; it showed up under browser
    // automation driving a hidden tab. Kept because a query that can sit
    // pending forever is worth designing out rather than relying on the tab
    // being visible.
    retry: false,
    // Still needed alongside retry: false, because the two guard different
    // moments. The default networkMode of "online" gates even the *first*
    // attempt on onlineManager.isOnline(), pausing before the fetch is made.
    // That is wrong here twice over: the file is a static asset on our own
    // origin, so connectivity has nothing to do with reading it, and this
    // platform is built to run airgapped, where a browser reporting itself
    // offline is the normal case rather than the broken one.
    //
    // Note "always" only bypasses the online check. The visibility check in
    // canContinue() applies regardless, which is why retry: false above is
    // the part that guarantees an error rather than an endless pending state.
    networkMode: "always",
  });
