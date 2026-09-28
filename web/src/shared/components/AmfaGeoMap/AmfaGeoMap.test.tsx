import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("react-leaflet", () => ({
  MapContainer: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="map-container">{children}</div>
  ),
  GeoJSON: (rest: Record<string, unknown>) => (
    <div data-testid="boundaries" data-attribution={String(rest.attribution)} />
  ),
  AttributionControl: () => <div data-testid="attribution" />,
  Marker: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="marker">{children}</div>
  ),
  Tooltip: ({ children }: { children?: React.ReactNode }) => (
    <span>{children}</span>
  ),
  Popup: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

vi.mock("react-leaflet-cluster", () => ({
  default: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="cluster">{children}</div>
  ),
}));

vi.mock("leaflet/dist/leaflet.css", () => ({}));
vi.mock("react-leaflet-cluster/lib/assets/MarkerCluster.css", () => ({}));
vi.mock(
  "react-leaflet-cluster/lib/assets/MarkerCluster.Default.css",
  () => ({}),
);

// CityLabels has its own tests; here it is only noise on top of the layer
// under test.
vi.mock("@/shared/components/CityLabels", () => ({
  CityLabels: () => <div data-testid="city-labels" />,
}));

vi.mock("@/shared/hooks", () => ({
  useWorldGeo: () => ({
    data: { type: "FeatureCollection", features: [] },
    isError: false,
  }),
}));

import { AmfaGeoMap } from "./AmfaGeoMap";

describe("AmfaGeoMap", () => {
  it("renders a marker per geo bucket", () => {
    render(
      <AmfaGeoMap
        buckets={[
          { country: "BR", lat: -23.5, long: -46.6, count: 14, risky_count: 0 },
          { country: "UK", lat: 51.5, long: -0.1, count: 6, risky_count: 1 },
        ]}
      />,
    );
    expect(screen.getAllByTestId("marker")).toHaveLength(2);
    expect(screen.getByTestId("map-container")).toBeInTheDocument();
  });

  it("draws bundled boundaries instead of fetching tiles", () => {
    // Third-party APIs are prohibited platform-wide; airgapped deployments
    // would otherwise get an empty panel with markers floating on it.
    render(<AmfaGeoMap buckets={[]} />);

    const boundaries = screen.getByTestId("boundaries");
    expect(boundaries).toBeInTheDocument();
    expect(boundaries.getAttribute("data-attribution")).toContain(
      "Natural Earth",
    );
  });
});
