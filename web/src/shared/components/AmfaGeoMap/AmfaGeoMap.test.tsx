import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("react-leaflet", () => ({
  MapContainer: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="map-container">{children}</div>
  ),
  TileLayer: () => <div data-testid="tile-layer" />,
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
vi.mock("react-leaflet-cluster/lib/assets/MarkerCluster.Default.css", () => ({}));

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
});
