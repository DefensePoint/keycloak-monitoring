import "leaflet/dist/leaflet.css";
import { useEffect } from "react";
import {
  AttributionControl,
  Circle,
  GeoJSON,
  MapContainer,
  Marker,
  Popup,
  useMap,
} from "react-leaflet";
import { Box, Chip, Typography } from "@mui/material";
import {
  BASEMAP_ATTRIBUTION,
  BASEMAP_MAX_ZOOM,
  BASEMAP_MIN_ZOOM,
  BASEMAP_STYLE,
} from "@/shared/lib/basemap";
import { configureLeafletDefaultIcons } from "@/shared/lib/leafletIcons";
import { useWorldGeo } from "@/shared/hooks";
import { CityLabels } from "@/shared/components/CityLabels";

configureLeafletDefaultIcons();

/**
 * Radius of the accuracy circle, in metres.
 *
 * IP geolocation resolves to a city or, worse, to the registered centroid of
 * the ISP's address block, so a bare pin implies precision the data does not
 * have. 20km is the order of magnitude of city-level accuracy: the circle
 * exists to stop anyone reading the marker as a street address.
 */
const ACCURACY_RADIUS_METRES = 20_000;

/**
 * Regional zoom, showing the surrounding country rather than a city block.
 *
 * The basemap draws country boundaries and coastlines only, so a city-level
 * zoom would put the marker in the middle of an empty polygon with nothing to
 * orient against. Framing the region is also the more honest view: the
 * coordinates come from IP geolocation, which is city-accurate at best.
 *
 * One notch below the basemap's maximum, which is where two requirements
 * meet. The accuracy circle has to be visible to do its job: at zoom 5 a 20km
 * radius is under three pixels, so the marker reads as an exact point, the
 * exact impression the circle exists to prevent. At the maximum the circle is
 * clear but the frame holds little more than flat land fill, so the country
 * is unrecognisable. Here the circle still reads and the view spans enough
 * coastline and border to place the point.
 */
const DEFAULT_ZOOM = BASEMAP_MAX_ZOOM - 1;

export interface LocationMapProps {
  /** Latitude, -90 to 90. */
  lat: number;
  /** Longitude, -180 to 180. */
  long: number;
  /** Place name shown in the popup and caption, e.g. "Lisbon, PT". */
  label?: string;
  /** Optional secondary identifier shown in monospace, e.g. an IP address. */
  detail?: string;
  /** Renders a warning chip. For traffic known to come via VPN or proxy. */
  isProxied?: boolean;
  /** Timestamp shown in the marker popup, as a preformatted string. */
  timestamp?: string;
  /** Map height in pixels. Must be a number: see the sizing note below. */
  height?: number;
}

/**
 * Forces Leaflet to re-measure its container once after mount.
 *
 * Leaflet measures its container on init and never again on its own. Inside a
 * MUI Dialog the map can mount while the dialog's scale transform is still
 * running, and a transform does change getBoundingClientRect, so Leaflet
 * caches a too-small size and renders a partial map with the tiles offset.
 * Callers should also delay mounting until the dialog transition finishes;
 * this is the second line of defence, and it is cheap.
 */
function InvalidateSizeOnMount() {
  const map = useMap();
  useEffect(() => {
    // rAF rather than a timeout: runs after the next paint, once layout for
    // this frame has settled.
    const frame = requestAnimationFrame(() => map.invalidateSize());
    return () => cancelAnimationFrame(frame);
  }, [map]);
  return null;
}

/**
 * A single-point map: one marker at the given coordinates, with an accuracy
 * circle and the identifying details underneath.
 *
 * Deliberately separate from AmfaGeoMap, which aggregates many points into
 * clustered buckets. The two answer different questions ("where is traffic
 * coming from" versus "where did this one thing happen"), and folding both
 * into one component would mean a mode flag threaded through clustering,
 * zoom, and tooltip logic.
 */
export function LocationMap({
  lat,
  long,
  label,
  detail,
  isProxied = false,
  timestamp,
  height = 360,
}: LocationMapProps) {
  const wq = useWorldGeo();
  const { data: world, isError: worldFailed } = wq;
  const position: [number, number] = [lat, long];
  const place = label?.trim() || "Unknown location";

  return (
    <Box sx={{ width: "100%" }}>
      {/*
        IMPORTANT: MapContainer needs an explicit pixel height on its own style
        prop. react-leaflet 4 only forwards `height` and `width` from `style`
        (other keys like minHeight are dropped), and a percentage height races
        MUI's layout pass — Leaflet measures 0×0 at mount, never re-measures,
        and the whole map silently renders invisible. Always set height in px.
      */}
      <MapContainer
        center={position}
        zoom={DEFAULT_ZOOM}
        minZoom={BASEMAP_MIN_ZOOM}
        maxZoom={BASEMAP_MAX_ZOOM}
        // Off deliberately, not by oversight. This map is embedded in the
        // event details dialog's scrolling body, where Leaflet would swallow
        // the wheel and zoom instead of letting the dialog scroll. Drag and
        // the +/- controls still zoom.
        scrollWheelZoom={false}
        attributionControl={false}
        style={{
          height,
          width: "100%",
          // Nothing else paints the container once the tile layer is gone.
          background: BASEMAP_STYLE.background,
        }}
      >
        {world && (
          <GeoJSON
            data={world}
            // Draw into the tile pane, where a basemap belongs. Vector layers
            // otherwise share the overlay pane with the markers and accuracy
            // circle, and because this layer mounts only once the boundary
            // data arrives, it lands on top of them and hides them.
            pane="tilePane"
            attribution={BASEMAP_ATTRIBUTION}
            style={{
              fillColor: BASEMAP_STYLE.land,
              fillOpacity: 1,
              color: BASEMAP_STYLE.border,
              weight: 0.5,
            }}
            interactive={false}
          />
        )}
        <AttributionControl prefix={false} />
        <Circle
          center={position}
          radius={ACCURACY_RADIUS_METRES}
          pathOptions={{ color: "#4dabf7", weight: 1.5, fillOpacity: 0.12 }}
        />
        <Marker position={position}>
          <Popup>
            <strong>{place}</strong>
            {timestamp && (
              <>
                <br />
                {timestamp}
              </>
            )}
          </Popup>
        </Marker>
        <CityLabels />
        <InvalidateSizeOnMount />
      </MapContainer>

      <Box
        sx={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          gap: 1,
          mt: 1.5,
        }}
      >
        <Typography variant="body2" sx={{ fontWeight: 600 }}>
          {place}
        </Typography>
        <Typography
          variant="body2"
          sx={{ fontFamily: "monospace", color: "text.secondary" }}
        >
          {lat}, {long}
        </Typography>
        {detail && (
          <Typography
            variant="body2"
            sx={{ fontFamily: "monospace", color: "text.secondary" }}
          >
            {detail}
          </Typography>
        )}
        {isProxied && (
          <Chip label="VPN detected" color="warning" size="small" />
        )}
      </Box>

      {worldFailed && (
        <Typography
          variant="caption"
          color="warning.main"
          sx={{ mt: 0.5, display: "block", textTransform: "none" }}
        >
          World boundaries could not be loaded, so the marker is shown without a
          basemap. The data ships with the application, so this points at the
          deployment rather than the network.
        </Typography>
      )}

      <Typography
        variant="caption"
        color="text.secondary"
        // The theme uppercases the caption variant, which turns this sentence
        // into shouting. It is a caveat, not a label.
        sx={{ mt: 0.5, display: "block", textTransform: "none" }}
      >
        Approximate location derived from the source IP. Accurate to the city at
        best, and a VPN or proxy puts it somewhere else entirely.
      </Typography>
    </Box>
  );
}
