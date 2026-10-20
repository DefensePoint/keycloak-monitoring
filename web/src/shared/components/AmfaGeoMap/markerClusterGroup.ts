import MarkerClusterGroupModule from "react-leaflet-cluster";

type MarkerClusterGroupComponent = typeof MarkerClusterGroupModule;

// react-leaflet-cluster ships CommonJS only, with the component as
// exports.default. The production build imports a CommonJS module's default
// the way Node does, as the whole module object, so the component is one
// level down; React refuses the module object ("Minified React error #130")
// and the AMFA geo map shows as unavailable. Vitest unwraps it already, which
// is why only markerClusterGroup.bundle.test.ts, which bundles this file,
// can see the difference.
export const MarkerClusterGroup: MarkerClusterGroupComponent =
  (
    MarkerClusterGroupModule as unknown as {
      default?: MarkerClusterGroupComponent;
    }
  ).default ?? MarkerClusterGroupModule;
