import { useCallback, useMemo, useState } from "react";
import { CircleMarker, Tooltip, useMap, useMapEvents } from "react-leaflet";
import { GlobalStyles } from "@mui/material";
import {
  BASEMAP_STYLE,
  CITY_LABEL_LIMIT,
  CITY_LABEL_MIN_ZOOM,
  CITY_LABEL_ZOOM_BONUS,
} from "@/shared/lib/basemap";
import { useWorldCities } from "@/shared/hooks";

/** Class on the Leaflet tooltip, so it can be styled outside the React tree. */
const LABEL_CLASS = "kmt-city-label";

/**
 * Labels the basemap with nearby cities.
 *
 * The boundary layer draws countries and coastlines and nothing else, so a
 * marker inland sits in an unlabelled polygon with nothing to orient against.
 * These labels are what turn "somewhere in France" into "just west of Paris".
 *
 * They do not make the location more precise. Coordinates still come from IP
 * geolocation, which resolves to a city at best, and the accuracy circle and
 * caveat text say so. Labels buy orientation, not accuracy.
 *
 * Must be rendered inside a MapContainer.
 */
export function CityLabels() {
  const { data: cities } = useWorldCities();
  const map = useMap();

  // Leaflet owns the viewport, so the current zoom and bounds have to be
  // mirrored into React state to drive what gets rendered.
  const [view, setView] = useState(() => ({
    zoom: map.getZoom(),
    bounds: map.getBounds(),
  }));
  const sync = useCallback(
    () => setView({ zoom: map.getZoom(), bounds: map.getBounds() }),
    [map],
  );
  // Memoised deliberately. react-leaflet's useEventHandlers lists the handler
  // object in its effect dependencies, so a fresh literal would make it
  // detach and reattach Leaflet's listeners on every render, which is every
  // pan. Besides the churn, an event firing inside that window is dropped,
  // which would leave the labels stale until the next pan.
  const handlers = useMemo(() => ({ zoomend: sync, moveend: sync }), [sync]);
  useMapEvents(handlers);

  if (!cities || view.zoom < CITY_LABEL_MIN_ZOOM) return null;

  // Take only what is on screen and significant enough for this zoom. The
  // list arrives sorted by minZoom, so stopping at the limit keeps the most
  // significant cities rather than an arbitrary slice.
  const visible: typeof cities = [];
  const threshold = view.zoom + CITY_LABEL_ZOOM_BONUS;
  for (const city of cities) {
    if (city.minZoom > threshold) continue;
    if (!view.bounds.contains([city.lat, city.lon])) continue;
    visible.push(city);
    if (visible.length >= CITY_LABEL_LIMIT) break;
  }

  return (
    <>
      <GlobalStyles
        styles={{
          // Leaflet renders tooltips into its own pane, outside the React
          // tree, so they cannot be styled with sx and default to a white box
          // with a border and arrow that looks pasted onto a dark map.
          [`.leaflet-tooltip.${LABEL_CLASS}`]: {
            background: "none",
            border: "none",
            boxShadow: "none",
            padding: "0 0 0 3px",
            color: BASEMAP_STYLE.cityLabel,
            fontSize: "10px",
            fontWeight: 500,
            whiteSpace: "nowrap",
            // The label annotates a dot; it must never swallow a click meant
            // for a marker underneath it.
            pointerEvents: "none",
          },
          [`.leaflet-tooltip.${LABEL_CLASS}::before`]: { display: "none" },
        }}
      />
      {visible.map((city) => (
        <CircleMarker
          key={`${city.name}_${city.lat}_${city.lon}`}
          center={[city.lat, city.lon]}
          radius={city.capital ? 2.5 : 1.8}
          pathOptions={{
            color: BASEMAP_STYLE.city,
            fillColor: BASEMAP_STYLE.city,
            fillOpacity: 1,
            weight: 0,
          }}
          // Scenery, like the boundaries: the event marker is the target.
          interactive={false}
        >
          <Tooltip
            permanent
            direction="right"
            offset={[2, 0]}
            className={LABEL_CLASS}
          >
            {city.name}
          </Tooltip>
        </CircleMarker>
      ))}
    </>
  );
}
