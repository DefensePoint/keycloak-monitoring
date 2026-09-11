import L from "leaflet";
import iconRetinaUrl from "leaflet/dist/images/marker-icon-2x.png";
import iconUrl from "leaflet/dist/images/marker-icon.png";
import shadowUrl from "leaflet/dist/images/marker-shadow.png";

/**
 * Point Leaflet at its own marker images.
 *
 * react-leaflet 4.x doesn't bundle marker icons correctly under Vite: Leaflet
 * derives icon URLs by inspecting its own stylesheet path, which Vite rewrites,
 * so markers render as broken images. Merging the canonical URLs (which Vite
 * resolves as asset imports) fixes it without manual asset handling.
 *
 * Shared because every map needs it and the symptom (invisible or broken
 * markers) gives no hint that a one-line setup call is missing. Idempotent, so
 * each map component can call it at module scope without coordinating.
 */
export function configureLeafletDefaultIcons(): void {
  L.Icon.Default.mergeOptions({
    iconRetinaUrl,
    iconUrl,
    shadowUrl,
  });
}
