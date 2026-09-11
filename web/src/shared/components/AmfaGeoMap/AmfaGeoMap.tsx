import "leaflet/dist/leaflet.css";
import "react-leaflet-cluster/lib/assets/MarkerCluster.css";
import "react-leaflet-cluster/lib/assets/MarkerCluster.Default.css";
import {
  AttributionControl,
  GeoJSON,
  MapContainer,
  Marker,
  Popup,
  Tooltip,
} from "react-leaflet";
import MarkerClusterGroup from "react-leaflet-cluster";
import { Box, Typography } from "@mui/material";
import type { AmfaGeoBucket } from "@/shared/types";
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

interface Props {
  buckets: AmfaGeoBucket[];
}

/**
 * Map height in pixels.
 *
 * Must be a number on MapContainer's own style prop: see the note below on why
 * a percentage cannot be used here. Sized so the whole world fits without the
 * map dominating a page it now shares with the events table below it.
 */
const GEO_MAP_HEIGHT = 540;

export function AmfaGeoMap({ buckets }: Props) {
  const { data: world, isError } = useWorldGeo();

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
        center={[20, 0]}
        zoom={2}
        minZoom={BASEMAP_MIN_ZOOM}
        maxZoom={BASEMAP_MAX_ZOOM}
        scrollWheelZoom
        attributionControl={false}
        style={{
          height: GEO_MAP_HEIGHT,
          width: "100%",
          // The ground the boundaries sit on. With no tile layer there is
          // nothing else painting the container, so without this the map
          // renders on whatever is behind it and reads as broken.
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
            // Leaflet collects attribution from its layers, so the credit
            // belongs to the layer that carries the data.
            attribution={BASEMAP_ATTRIBUTION}
            style={{
              fillColor: BASEMAP_STYLE.land,
              fillOpacity: 1,
              color: BASEMAP_STYLE.border,
              weight: 0.5,
            }}
            // Boundaries are scenery, not targets: clicks and hovers belong to
            // the markers on top of them.
            interactive={false}
          />
        )}
        <AttributionControl prefix={false} />
        <CityLabels />
        <MarkerClusterGroup>
          {buckets.map((b) => (
            <Marker
              key={`${b.lat}_${b.long}_${b.country ?? ""}`}
              position={[b.lat, b.long]}
            >
              <Tooltip permanent direction="top">
                {(b.country ?? "—") +
                  ": " +
                  b.count +
                  (b.risky_count > 0 ? ` (⚠ ${b.risky_count})` : "")}
              </Tooltip>
              <Popup>
                <strong>{b.country ?? "Unknown"}</strong>
                <br />
                Total events: {b.count}
                <br />
                Risky (3+): {b.risky_count}
              </Popup>
            </Marker>
          ))}
        </MarkerClusterGroup>
      </MapContainer>

      {isError && (
        <Typography
          variant="caption"
          color="warning.main"
          sx={{ mt: 1, display: "block" }}
        >
          World boundaries could not be loaded, so markers are shown without a
          basemap. The data ships with the application, so this points at the
          deployment rather than the network.
        </Typography>
      )}
    </Box>
  );
}
