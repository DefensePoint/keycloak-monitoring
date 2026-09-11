import { useQuery } from "@tanstack/react-query";
import { WORLD_CITIES_URL } from "@/shared/lib/basemap";

/** One populated place from the vendored Natural Earth extract. */
export interface WorldCity {
  name: string;
  country: string | null;
  pop: number | null;
  /**
   * Natural Earth's own guidance for the zoom at which this label belongs.
   * Tokyo is 1.7, a French regional capital around 6. Using it means the map
   * thins labels the way a cartographer would rather than by an arbitrary
   * population cutoff.
   */
  minZoom: number;
  /** 1 for a national capital. */
  capital: 0 | 1;
  lat: number;
  lon: number;
}

/**
 * Loads the vendored city list the maps label their basemap with.
 *
 * Same contract as useWorldGeo, and for the same reasons: served from our own
 * origin so it works airgapped, cached forever because the file only changes
 * when someone re-vendors it, networkMode "always" so the first attempt is not
 * gated on perceived connectivity, and no retry so a failure surfaces as an
 * error instead of sitting in "pending" while query-core waits for the
 * document to regain focus. See useWorldGeo for the mechanism.
 *
 * Failure is deliberately quiet here, unlike the boundaries: labels are an
 * aid, and a map without them still answers "where did this happen".
 */
export const useWorldCities = () =>
  useQuery<WorldCity[]>({
    queryKey: ["world-cities", WORLD_CITIES_URL],
    queryFn: async () => {
      const res = await fetch(WORLD_CITIES_URL);
      if (!res.ok) {
        throw new Error(
          `city labels unavailable: HTTP ${res.status} from ${WORLD_CITIES_URL}`,
        );
      }
      return (await res.json()) as WorldCity[];
    },
    staleTime: Infinity,
    gcTime: Infinity,
    refetchOnWindowFocus: false,
    retry: false,
    networkMode: "always",
  });
