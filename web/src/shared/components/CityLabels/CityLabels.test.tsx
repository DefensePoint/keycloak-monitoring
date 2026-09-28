import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

const cities = vi.hoisted(() => ({
  current: [
    // minZoom ascending, as the vendored file is sorted
    {
      name: "Tokyo",
      country: "Japan",
      pop: 35676000,
      minZoom: 1.7,
      capital: 0,
      lat: 35.69,
      lon: 139.75,
    },
    {
      name: "Paris",
      country: "France",
      pop: 10894000,
      minZoom: 2.4,
      capital: 1,
      lat: 48.87,
      lon: 2.33,
    },
    {
      name: "Nancy",
      country: "France",
      pop: 268976,
      minZoom: 5.1,
      capital: 0,
      lat: 48.68,
      lon: 6.2,
    },
    {
      name: "Amiens",
      country: "France",
      pop: 143086,
      minZoom: 6.7,
      capital: 0,
      lat: 49.9,
      lon: 2.3,
    },
    {
      name: "Bombo",
      country: "Uganda",
      pop: 75000,
      minZoom: 9,
      capital: 0,
      lat: 0.58,
      lon: 32.53,
    },
  ],
}));

const mapView = vi.hoisted(() => ({ current: { zoom: 6, inBounds: true } }));

vi.mock("@/shared/hooks", () => ({
  useWorldCities: () => ({ data: cities.current, isError: false }),
}));

vi.mock("react-leaflet", () => ({
  useMap: () => ({
    getZoom: () => mapView.current.zoom,
    getBounds: () => ({ contains: () => mapView.current.inBounds }),
  }),
  useMapEvents: () => null,
  CircleMarker: ({
    children,
    radius,
  }: {
    children?: React.ReactNode;
    radius?: number;
  }) => (
    <div data-testid="city-dot" data-radius={String(radius)}>
      {children}
    </div>
  ),
  Tooltip: ({ children }: { children?: React.ReactNode }) => (
    <span className="kmt-city-label">{children}</span>
  ),
}));

import {
  CITY_LABEL_MIN_ZOOM,
  CITY_LABEL_ZOOM_BONUS,
} from "@/shared/lib/basemap";
import { CityLabels } from "./CityLabels";

/** Zoom at which a city with this minZoom is expected to be labelled. */
const zoomThatShows = (minZoom: number) => minZoom - CITY_LABEL_ZOOM_BONUS;

describe("CityLabels", () => {
  beforeEach(() => {
    mapView.current = { zoom: 6, inBounds: true };
  });

  it("labels a city once the zoom reaches its own minZoom", () => {
    // Natural Earth's per-city minZoom is the thinning rule, offset by
    // CITY_LABEL_ZOOM_BONUS. Expectations are derived from the constants so
    // that retuning density does not require rewriting the test.
    mapView.current = { zoom: zoomThatShows(6.7), inBounds: true };
    render(<CityLabels />);

    expect(screen.getByText("Paris")).toBeInTheDocument(); // 2.4
    expect(screen.getByText("Nancy")).toBeInTheDocument(); // 5.1
    expect(screen.getByText("Amiens")).toBeInTheDocument(); // 6.7, exactly due
    expect(screen.queryByText("Bombo")).not.toBeInTheDocument(); // 9
  });

  it("withholds a city until its minZoom is reached", () => {
    // One notch below Amiens' own threshold it must stay unlabelled, while
    // the more significant names remain.
    mapView.current = { zoom: zoomThatShows(6.7) - 1, inBounds: true };
    render(<CityLabels />);

    expect(screen.getByText("Paris")).toBeInTheDocument();
    expect(screen.queryByText("Amiens")).not.toBeInTheDocument();
  });

  it("thins the list down to the majors on a continental view", () => {
    mapView.current = { zoom: CITY_LABEL_MIN_ZOOM, inBounds: true };
    render(<CityLabels />);

    expect(screen.getByText("Paris")).toBeInTheDocument();
    expect(screen.queryByText("Nancy")).not.toBeInTheDocument();
    expect(screen.queryByText("Amiens")).not.toBeInTheDocument();
  });

  it("renders nothing on a world view", () => {
    // The aggregate map opens at zoom 2, below CITY_LABEL_MIN_ZOOM: there the
    // event markers are the subject and labels would only crowd them.
    mapView.current = { zoom: CITY_LABEL_MIN_ZOOM - 1, inBounds: true };
    render(<CityLabels />);

    expect(screen.queryByTestId("city-dot")).not.toBeInTheDocument();
  });

  it("skips cities outside the viewport", () => {
    mapView.current = { zoom: 6, inBounds: false };
    render(<CityLabels />);

    expect(screen.queryByTestId("city-dot")).not.toBeInTheDocument();
  });

  it("draws capitals slightly larger", () => {
    render(<CityLabels />);

    const radii = screen
      .getAllByTestId("city-dot")
      .map((el) => Number(el.dataset.radius));
    expect(Math.max(...radii)).toBeGreaterThan(Math.min(...radii));
  });

  it("renders nothing when the city list is unavailable", () => {
    // Labels are an aid, so a missing list degrades quietly: the map still
    // answers "where did this happen" without them.
    cities.current = [];
    try {
      render(<CityLabels />);
      expect(screen.queryByTestId("city-dot")).not.toBeInTheDocument();
    } finally {
      cities.current = [
        {
          name: "Paris",
          country: "France",
          pop: 1,
          minZoom: 2.4,
          capital: 1,
          lat: 48.87,
          lon: 2.33,
        },
      ];
    }
  });
});
