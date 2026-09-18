import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

// Leaflet needs a real layout engine, which jsdom does not provide, so the
// react-leaflet primitives are stubbed. What is worth asserting here is the
// props we hand Leaflet (centre, radius, attribution) and the details we
// render around the map, not Leaflet's own rendering.
const mapProps = vi.hoisted(() => ({ current: {} as Record<string, unknown> }));
const circleProps = vi.hoisted(() => ({
  current: {} as Record<string, unknown>,
}));
const geoProps = vi.hoisted(() => ({
  current: {} as Record<string, unknown>,
}));

vi.mock("react-leaflet", () => ({
  MapContainer: ({
    children,
    ...rest
  }: {
    children?: React.ReactNode;
    [key: string]: unknown;
  }) => {
    mapProps.current = rest;
    return <div data-testid="map-container">{children}</div>;
  },
  GeoJSON: (rest: Record<string, unknown>) => {
    geoProps.current = rest;
    return <div data-testid="boundaries" />;
  },
  AttributionControl: () => <div data-testid="attribution" />,
  Circle: (rest: Record<string, unknown>) => {
    circleProps.current = rest;
    return <div data-testid="accuracy-circle" />;
  },
  Marker: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="marker">{children}</div>
  ),
  Popup: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="popup">{children}</div>
  ),
  useMap: () => ({ invalidateSize: vi.fn() }),
}));

vi.mock("leaflet/dist/leaflet.css", () => ({}));
// CityLabels has its own tests; here it is only noise on top of the layer
// under test.
vi.mock("@/shared/components/CityLabels", () => ({
  CityLabels: () => <div data-testid="city-labels" />,
}));

// The boundary data is fetched, so the hook is stubbed with a minimal
// FeatureCollection. The component must not depend on its contents.
const worldGeo = vi.hoisted(() => ({
  current: {
    data: { type: "FeatureCollection", features: [] } as unknown,
    isError: false,
  },
}));

vi.mock("@/shared/hooks", () => ({
  useWorldGeo: () => worldGeo.current,
}));

import { LocationMap } from "./LocationMap";

describe("LocationMap", () => {
  it("centres the map on the given coordinates", () => {
    render(<LocationMap lat={38.72} long={-9.14} label="Lisbon, PT" />);

    expect(screen.getByTestId("map-container")).toBeInTheDocument();
    expect(mapProps.current.center).toEqual([38.72, -9.14]);
    expect(screen.getAllByTestId("marker")).toHaveLength(1);
  });

  it("renders an accuracy circle rather than implying an exact address", () => {
    // The circle and the caption are the honesty guardrails on city-level
    // GeoIP data. A refactor that drops either overstates the precision.
    render(<LocationMap lat={38.72} long={-9.14} label="Lisbon, PT" />);

    expect(screen.getByTestId("accuracy-circle")).toBeInTheDocument();
    expect(circleProps.current.center).toEqual([38.72, -9.14]);
    expect(circleProps.current.radius).toBeGreaterThan(0);
    expect(screen.getByText(/approximate location/i)).toBeInTheDocument();
  });

  it("draws boundaries from bundled data, with no third-party request", () => {
    // Third-party APIs are prohibited: deployments can be airgapped, where a
    // tile service renders an empty panel while the page looks healthy.
    render(<LocationMap lat={38.72} long={-9.14} />);

    expect(screen.getByTestId("boundaries")).toBeInTheDocument();
    expect(String(geoProps.current.attribution)).toContain("Natural Earth");
    expect(JSON.stringify(mapProps.current)).not.toMatch(/https?:/);
  });

  it("caps zoom so the view cannot outrun the boundary detail", () => {
    render(<LocationMap lat={38.72} long={-9.14} />);

    expect(Number(mapProps.current.maxZoom)).toBeLessThanOrEqual(8);
    expect(Number(mapProps.current.zoom)).toBeLessThanOrEqual(
      Number(mapProps.current.maxZoom),
    );
  });

  it("never zooms on the wheel", () => {
    // The only caller embeds this in a scrolling dialog, where Leaflet would
    // swallow the wheel and zoom instead of letting the dialog scroll.
    render(<LocationMap lat={38.72} long={-9.14} />);

    expect(mapProps.current.scrollWheelZoom).toBe(false);
  });

  it("shows the place, coordinates, detail and proxy warning", () => {
    render(
      <LocationMap
        lat={38.72}
        long={-9.14}
        label="Lisbon, PT"
        detail="203.0.113.7"
        isProxied
      />,
    );

    expect(screen.getAllByText("Lisbon, PT").length).toBeGreaterThan(0);
    expect(screen.getByText("38.72, -9.14")).toBeInTheDocument();
    expect(screen.getByText("203.0.113.7")).toBeInTheDocument();
    expect(screen.getByText("VPN detected")).toBeInTheDocument();
  });

  it("falls back to a placeholder when no label is given", () => {
    render(<LocationMap lat={38.72} long={-9.14} />);

    expect(screen.getAllByText("Unknown location").length).toBeGreaterThan(0);
    expect(screen.queryByText("VPN detected")).not.toBeInTheDocument();
  });

  it("says so when the bundled boundaries fail to load", () => {
    // The data ships with the app, so a failure here means a broken
    // deployment. Showing a marker on blank ground with no explanation reads
    // as a broken feature instead.
    worldGeo.current = { data: undefined, isError: true };
    try {
      render(<LocationMap lat={38.72} long={-9.14} label="Lisbon, PT" />);
      expect(
        screen.getByText(/boundaries could not be loaded/i),
      ).toBeInTheDocument();
      expect(screen.getAllByTestId("marker")).toHaveLength(1);
    } finally {
      worldGeo.current = {
        data: { type: "FeatureCollection", features: [] },
        isError: false,
      };
    }
  });
});
