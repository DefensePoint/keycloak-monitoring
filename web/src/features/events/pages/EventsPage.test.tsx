import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { EventsPage } from "./EventsPage";
import { useTenant } from "@/shared/context";

const mockSetSearchParams = vi.fn();
const mockUseKeycloakDashboard = vi.fn();
const mockUseEvents = vi.fn();
const mockUseAmfaRealms = vi.fn<() => { data: string[] }>(() => ({ data: [] }));
const mockUseAmfaStats = vi.fn<() => { data: unknown; error: unknown }>(() => ({
  data: undefined,
  error: undefined,
}));
const mockIsAllRealmsUnsupportedError = vi.fn<(error: unknown) => boolean>(
  () => false,
);
const mockIsRealmScopeRequiresRealmError = vi.fn<(error: unknown) => boolean>(
  () => false,
);
// Mutable so individual tests can preset `?event=<id>` and let EventsPage's
// own effect auto-open that event's detail dialog, without needing to drive
// clicks through the mocked MaterialReactTable.
let mockSearchParams = new URLSearchParams();

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/tenant-1/events" }),
  useParams: () => ({ tenantId: "tenant-1" }),
  useSearchParams: () => [mockSearchParams, mockSetSearchParams],
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
}));

vi.mock("@/shared/hooks", () => ({
  useKeycloakDashboard: () => mockUseKeycloakDashboard(),
  useRealmSelector: () => ({
    selectedRealm: "all",
    handleRealmChange: vi.fn(),
  }),
  useEvents: () => mockUseEvents(),
  useAmfaStats: () => mockUseAmfaStats(),
  useAmfaGeo: () => ({ data: undefined }),
  useAmfaRealms: () => mockUseAmfaRealms(),
  isAllRealmsUnsupportedError: (error: unknown) =>
    mockIsAllRealmsUnsupportedError(error),
  isRealmScopeRequiresRealmError: (error: unknown) =>
    mockIsRealmScopeRequiresRealmError(error),
  useSessionStorage: (key: string, defaultValue: unknown) => [
    defaultValue,
    vi.fn(),
  ],
  useTimeRangeStorage: () => ({
    stored: null,
    saveTimeRange: vi.fn(),
    getStoredTimeRange: vi.fn(),
  }),
}));

vi.mock("@/shared/components", () => ({
  PageHeader: ({
    title,
    subtitle,
    children,
  }: {
    title: string;
    subtitle?: string;
    children?: React.ReactNode;
  }) => (
    <div data-testid="page-header">
      <div>{title}</div>
      {subtitle && <div>{subtitle}</div>}
      {children}
    </div>
  ),
  RealmSelector: () => <div data-testid="realm-selector">Realm Selector</div>,
  TimeSelector: () => <div data-testid="time-selector">Time Selector</div>,
  PageSizeSelector: ({
    value,
    onChange,
  }: {
    value: number;
    onChange: (value: number) => void;
  }) => (
    <div data-testid="page-size-selector">
      <span>Show:</span>
      <select value={value} onChange={(e) => onChange(Number(e.target.value))}>
        <option value="25">25 events</option>
        <option value="50">50 events</option>
        <option value="100">100 events</option>
      </select>
    </div>
  ),
  FeatureErrorBoundary: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  RiskBadge: ({ level }: { level: number | null }) => (
    <span data-testid="risk-badge">{level ?? "-"}</span>
  ),
  AmfaKpiRow: () => <div data-testid="amfa-kpi-row" />,
  AmfaGeoMap: () => <div data-testid="amfa-geo-map" />,
  LocationMap: ({ lat, long }: { lat: number; long: number }) => (
    <div data-testid="location-map">
      {lat}, {long}
    </div>
  ),
  AlertBanner: ({ message }: { message: React.ReactNode }) => (
    <div data-testid="alert-banner">{message}</div>
  ),
  SectionCard: ({
    title,
    children,
  }: {
    title: string;
    children?: React.ReactNode;
  }) => (
    <div data-testid="section-card">
      <div>{title}</div>
      {children}
    </div>
  ),
}));

vi.mock("material-react-table", () => ({
  MaterialReactTable: ({
    data,
    columns,
    renderBottomToolbarCustomActions,
  }: {
    data: unknown[];
    columns?: {
      id?: string;
      Cell?: (ctx: { row: { original: unknown } }) => React.ReactNode;
    }[];
    renderBottomToolbarCustomActions?: () => React.ReactNode;
  }) => {
    // Render just the actions cell for each row. The real table renders every
    // column, but the row-level buttons are the only part of the column
    // definitions worth exercising here, and rendering them all would drag
    // every other Cell's dependencies into this mock.
    const actions = columns?.find((column) => column.id === "actions");
    return (
      <div data-testid="events-table">
        <div>Table with {data.length} events</div>
        {actions?.Cell &&
          data.map((original, index) => (
            <div data-testid="row-actions" key={index}>
              {actions.Cell!({ row: { original } })}
            </div>
          ))}
        {renderBottomToolbarCustomActions && (
          <div>{renderBottomToolbarCustomActions()}</div>
        )}
      </div>
    );
  },
}));

const mockEventsResponse = {
  events: [
    {
      event_id: "event-1",
      timestamp: "2024-01-01T00:00:00Z",
      type: "LOGIN",
      severity: "info",
      description: "User logged in",
      source: "keycloak:master",
      user_id: "user-123",
      username: "testuser",
      email: "test@example.com",
      client_id: "admin-cli",
    },
    {
      event_id: "event-2",
      timestamp: "2024-01-02T00:00:00Z",
      type: "LOGIN_ERROR",
      severity: "error",
      description: "Failed login attempt",
      source: "keycloak:master",
      user_id: "user-456",
      username: "admin",
      client_id: "admin-cli",
    },
  ],
  total: 2,
};

const mockKeycloakDashboard = {
  realms: [
    { realm_name: "master", enabled: true },
    { realm_name: "test", enabled: true },
  ],
};

describe("EventsPage", () => {
  const mockSelectedTenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
    default_realm: "master",
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockSearchParams = new URLSearchParams();
    vi.mocked(useTenant).mockReturnValue({
      selectedTenant: mockSelectedTenant,
      tenants: [mockSelectedTenant],
      selectTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoading: false,
      error: null,
    });

    mockUseKeycloakDashboard.mockReturnValue({
      data: mockKeycloakDashboard,
      isLoading: false,
      isError: false,
    });

    mockUseEvents.mockReturnValue({
      data: mockEventsResponse,
      isLoading: false,
      isError: false,
    });

    mockUseAmfaRealms.mockReturnValue({ data: [] });
    mockUseAmfaStats.mockReturnValue({ data: undefined, error: undefined });
    mockIsAllRealmsUnsupportedError.mockReturnValue(false);
    mockIsRealmScopeRequiresRealmError.mockReturnValue(false);
  });

  it("should render events page header", async () => {
    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText("Events")).toBeInTheDocument();
    });

    expect(
      screen.getByText("View and analyze security events"),
    ).toBeInTheDocument();
  });

  it("should load and display events", async () => {
    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("events-table")).toBeInTheDocument();
    });

    expect(screen.getByText("Table with 2 events")).toBeInTheDocument();
  });

  it("should display realm selector", async () => {
    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("realm-selector")).toBeInTheDocument();
    });
  });

  it("should display time selector", async () => {
    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("time-selector")).toBeInTheDocument();
    });
  });

  it("should allow changing events limit", async () => {
    const user = userEvent.setup();
    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("page-size-selector")).toBeInTheDocument();
    });

    const select = screen.getByRole("combobox");
    await user.selectOptions(select, "100");

    expect(select).toHaveValue("100");
  });

  it("should show event details dialog when event is clicked", async () => {
    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("events-table")).toBeInTheDocument();
    });

    // Note: This test would require mocking the MaterialReactTable with clickable rows
    // For now, we just verify the table is rendered
    expect(screen.getByText("Table with 2 events")).toBeInTheDocument();
  });

  it("should display pagination info", async () => {
    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText(/Page 1 of/)).toBeInTheDocument();
    });

    expect(screen.getByText(/2 total events/)).toBeInTheDocument();
  });

  it("hides the AMFA KPI strip when no realm uses AMFA", async () => {
    mockUseAmfaRealms.mockReturnValue({ data: [] });
    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText("Events")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("amfa-kpi-row")).not.toBeInTheDocument();
  });

  it("shows the AMFA KPI strip when a realm uses AMFA", async () => {
    // selectedRealm is "all" in this suite, so any AMFA-enabled realm turns the
    // section on.
    mockUseAmfaRealms.mockReturnValue({ data: ["AdaptiveAuth"] });
    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("amfa-kpi-row")).toBeInTheDocument();
    });
  });

  it("shows an explanatory banner instead of the KPI strip when All Realms aggregation is unsupported", async () => {
    // selectedRealm is "all" in this suite; simulate the backend's 501 for a
    // tenant whose AMFA transport can't aggregate across realms.
    mockUseAmfaRealms.mockReturnValue({ data: ["AdaptiveAuth"] });
    const unsupportedError = new Error("amfa_all_realms_unsupported");
    mockUseAmfaStats.mockReturnValue({
      data: undefined,
      error: unsupportedError,
    });
    mockIsAllRealmsUnsupportedError.mockImplementation(
      (error) => error === unsupportedError,
    );

    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("alert-banner")).toBeInTheDocument();
    });
    expect(
      screen.getByText(/doesn't support aggregating stats/),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("amfa-kpi-row")).not.toBeInTheDocument();
  });

  it("blames the caller's realm scope, not the integration, when their access is the reason", async () => {
    mockUseAmfaRealms.mockReturnValue({ data: ["AdaptiveAuth"] });
    const scopeError = new Error("amfa_realm_scope_requires_realm");
    mockUseAmfaStats.mockReturnValue({ data: undefined, error: scopeError });
    mockIsRealmScopeRequiresRealmError.mockImplementation(
      (error) => error === scopeError,
    );

    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("alert-banner")).toBeInTheDocument();
    });
    expect(
      screen.getByText(/Your access is limited to specific realms/),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/doesn't support aggregating stats/),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId("amfa-kpi-row")).not.toBeInTheDocument();
  });

  it("names the realm the KPI numbers actually cover when the request was narrowed", async () => {
    mockUseAmfaRealms.mockReturnValue({ data: ["AdaptiveAuth"] });
    mockUseAmfaStats.mockReturnValue({
      data: {
        total: 12,
        risky: 3,
        unique_users: 7,
        flagged_ips: 1,
        applied_realm_id: "AdaptiveAuth",
      },
      error: undefined,
    });

    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("amfa-kpi-row")).toBeInTheDocument();
    });
    expect(
      screen.getByText(/these AMFA metrics cover that realm/),
    ).toBeInTheDocument();
    expect(screen.getByText(/AdaptiveAuth/)).toBeInTheDocument();
  });

  it("says nothing about narrowing when the answer covers what was asked for", async () => {
    mockUseAmfaRealms.mockReturnValue({ data: ["AdaptiveAuth"] });
    mockUseAmfaStats.mockReturnValue({
      data: { total: 12, risky: 3, unique_users: 7, flagged_ips: 1 },
      error: undefined,
    });

    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("amfa-kpi-row")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("alert-banner")).not.toBeInTheDocument();
  });

  describe("AMFA Details section in the event dialog", () => {
    // Regression coverage for the amfa_event_id-only gate: an event carrying
    // just a risk_level (the OSS SPI case, where no mirror join stamps an
    // amfa_event_id) must still surface the section, not just events with an
    // amfa_event_id.
    it("shows AMFA Details for an event with a risk_level but no amfa_event_id", async () => {
      mockSearchParams = new URLSearchParams("event=event-risk-only");
      mockUseEvents.mockReturnValue({
        data: {
          events: [
            {
              event_id: "event-risk-only",
              timestamp: "2024-01-03T00:00:00Z",
              type: "LOGIN",
              severity: "info",
              description: "User logged in",
              source: "keycloak:master",
              user_id: "user-789",
              username: "riskyuser",
              client_id: "admin-cli",
              risk_level: 2,
            },
          ],
          total: 1,
        },
        isLoading: false,
        isError: false,
      });

      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByText("AMFA Details")).toBeInTheDocument();
      });
      // is_vpn is unknown here, not false — the chip must not claim "No VPN".
      expect(screen.queryByText("No VPN")).not.toBeInTheDocument();
      expect(screen.queryByText("VPN detected")).not.toBeInTheDocument();
    });

    it("hides AMFA Details for a plain event with neither risk_level nor amfa_event_id", async () => {
      mockSearchParams = new URLSearchParams("event=event-1");
      mockUseEvents.mockReturnValue({
        data: mockEventsResponse,
        isLoading: false,
        isError: false,
      });

      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByText("User logged in")).toBeInTheDocument();
      });
      expect(screen.queryByText("AMFA Details")).not.toBeInTheDocument();
    });

    it("shows the VPN chip for a fully-merged AMFA event", async () => {
      mockSearchParams = new URLSearchParams("event=event-merged");
      mockUseEvents.mockReturnValue({
        data: {
          events: [
            {
              event_id: "event-merged",
              timestamp: "2024-01-04T00:00:00Z",
              type: "LOGIN",
              severity: "info",
              description: "User logged in",
              source: "keycloak:master",
              user_id: "user-999",
              username: "mergeduser",
              client_id: "admin-cli",
              amfa_event_id: "amfa-1",
              risk_level: 1,
              is_vpn: false,
            },
          ],
          total: 1,
        },
        isLoading: false,
        isError: false,
      });

      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByText("AMFA Details")).toBeInTheDocument();
      });
      expect(screen.getByText("No VPN")).toBeInTheDocument();
    });
  });
  describe("event location map in the details dialog", () => {
    const locatedEvent = {
      event_id: "event-located",
      timestamp: "2024-01-05T00:00:00Z",
      type: "LOGIN",
      severity: "info",
      description: "User logged in",
      source: "keycloak:master",
      user_id: "user-1",
      username: "locateduser",
      client_id: "admin-cli",
      source_ip: "203.0.113.7",
      amfa_event_id: "amfa-2",
      city: "Lisbon",
      country: "PT",
      lat: 38.72,
      long: -9.14,
    };

    it("renders no location pin in the table", async () => {
      // The map lives in the details dialog now. A geolocated row must not
      // carry a second button beside the eye.
      mockUseEvents.mockReturnValue({
        data: { events: [locatedEvent], total: 1 },
        isLoading: false,
        isError: false,
      });

      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByTestId("events-table")).toBeInTheDocument();
      });
      expect(screen.getAllByTestId("row-actions")).toHaveLength(1);
      expect(
        screen.queryByLabelText("Show location on map"),
      ).not.toBeInTheDocument();
    });

    it("shows the map on the event's coordinates inside the details dialog", async () => {
      mockSearchParams = new URLSearchParams("event=event-located");
      mockUseEvents.mockReturnValue({
        data: { events: [locatedEvent], total: 1 },
        isLoading: false,
        isError: false,
      });

      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByText("AMFA Details")).toBeInTheDocument();
      });
      // The map mounts only after the dialog transition finishes, so wait for
      // it rather than asserting on the frame the dialog opens.
      await waitFor(() => {
        expect(screen.getByTestId("location-map")).toBeInTheDocument();
      });
      expect(screen.getByTestId("location-map")).toHaveTextContent(
        "38.72, -9.14",
      );
    });

    it("shows neither coordinates nor a map for an event stranded on Null Island", async () => {
      // A failed GeoIP lookup reports 0,0. It is a valid LatLng, so nothing
      // downstream would complain; the map simply must not appear, and the
      // Coordinates row must not either — the two share one gate.
      mockSearchParams = new URLSearchParams("event=event-located");
      mockUseEvents.mockReturnValue({
        data: {
          events: [{ ...locatedEvent, lat: 0, long: 0 }],
          total: 1,
        },
        isLoading: false,
        isError: false,
      });

      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByText("AMFA Details")).toBeInTheDocument();
      });
      expect(screen.queryByText("Coordinates")).not.toBeInTheDocument();
      expect(screen.queryByTestId("location-map")).not.toBeInTheDocument();
    });

    it("remounts the map when the details dialog is closed and reopened", async () => {
      // Regression guard for the mount gate: the "dialog has finished
      // animating" flag has to reset on close, or the second open renders an
      // empty panel where the map should be.
      mockUseEvents.mockReturnValue({
        data: { events: [locatedEvent], total: 1 },
        isLoading: false,
        isError: false,
      });

      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByTestId("events-table")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByLabelText("View event details"));
      await waitFor(() => {
        expect(screen.getByTestId("location-map")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByTestId("CloseIcon").closest("button")!);
      await waitFor(() => {
        expect(screen.queryByTestId("location-map")).not.toBeInTheDocument();
      });

      await userEvent.click(screen.getByLabelText("View event details"));
      await waitFor(() => {
        expect(screen.getByTestId("location-map")).toBeInTheDocument();
      });
    });
  });
});
