import "leaflet/dist/leaflet.css";
import "react-leaflet-cluster/lib/assets/MarkerCluster.css";
import "react-leaflet-cluster/lib/assets/MarkerCluster.Default.css";
import L from "leaflet";
import iconRetinaUrl from "leaflet/dist/images/marker-icon-2x.png";
import iconUrl from "leaflet/dist/images/marker-icon.png";
import shadowUrl from "leaflet/dist/images/marker-shadow.png";
import { MapContainer, Marker, Popup, TileLayer, Tooltip } from "react-leaflet";
import MarkerClusterGroup from "react-leaflet-cluster";
import { Box } from "@mui/material";
import type { AmfaGeoBucket } from "@/shared/types";

// react-leaflet 4.x doesn't bundle marker icons correctly under Vite; merge the
// canonical default icon URLs so markers render without manual asset handling.
L.Icon.Default.mergeOptions({
  iconRetinaUrl,
  iconUrl,
  shadowUrl,
});

interface Props {
  buckets: AmfaGeoBucket[];
}

export function AmfaGeoMap({ buckets }: Props) {
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
        scrollWheelZoom
        style={{ height: 600, width: "100%" }}
      >
        <TileLayer
          attribution="&copy; OpenStreetMap, &copy; CARTO"
          url="https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png"
        />
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
    </Box>
  );
}
