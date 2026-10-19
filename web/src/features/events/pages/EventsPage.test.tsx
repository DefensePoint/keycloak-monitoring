import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { EventsPage, getSeverityColor } from "./EventsPage";
import { useTenant } from "@/shared/context";

const mockSetSearchParams = vi.fn();
const mockUseKeycloakDashboard = vi.fn();
const mockUseEvents = vi.fn();
const mockUseEvent = vi.fn();
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

const mockUseAuth = vi.fn();

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
  useAuth: () => mockUseAuth(),
}));

vi.mock("@/shared/hooks", () => ({
  useKeycloakDashboard: () => mockUseKeycloakDashboard(),
  useRealmSelector: () => ({
    selectedRealm: "all",
    handleRealmChange: vi.fn(),
  }),
  useEvents: (args: unknown) => mockUseEvents(args),
  useEvent: (...args: unknown[]) => mockUseEvent(...args),
  isEventNotFoundError: (error: unknown) =>
    error instanceof Error && error.message === "Event not found",
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
    // The user column is rendered too: it decides what a row shows when the
    // event carries no username, which is most of them for token events, and
    // that is a display decision worth holding still. It needs nothing but
    // layout components, so it does not drag anything into this mock.
    const user = columns?.find((column) => column.accessorKey === "username");
    return (
      <div data-testid="events-table">
        <div>Table with {data.length} events</div>
        {user?.Cell &&
          data.map((original, index) => (
            <div data-testid="row-user" key={index}>
              {user.Cell!({ row: { original } })}
            </div>
          ))}
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
    {
      // Keycloak puts no username in the details of a token event, so where
      // the login has no AMFA counterpart the id is all the row carries.
      event_id: "event-3",
      timestamp: "2024-01-03T00:00:00Z",
      type: "REFRESH_TOKEN",
      severity: "info",
      description: "Token refreshed",
      source: "keycloak:master",
      user_id: "11111111-2222-3333-4444-555555555555",
      client_id: "admin-cli",
    },
  ],
  total: 3,
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
    mockUseAuth.mockReturnValue({ isAdmin: () => true, user: null });

    mockUseEvents.mockReturnValue({
      data: mockEventsResponse,
      isLoading: false,
      isError: false,
    });

    mockUseAmfaRealms.mockReturnValue({ data: [] });
    mockUseEvent.mockReturnValue({ data: undefined, isError: false });
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

    expect(screen.getByText("Table with 3 events")).toBeInTheDocument();
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

    const select = within(screen.getByTestId("page-size-selector")).getByRole(
      "combobox",
    );
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
    expect(screen.getByText("Table with 3 events")).toBeInTheDocument();
  });

  it("should display pagination info", async () => {
    render(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText(/Page 1 of/)).toBeInTheDocument();
    });

    expect(screen.getByText(/3 total events/)).toBeInTheDocument();
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

  describe("server-side event filters", () => {
    const lastEventsQuery = () =>
      mockUseEvents.mock.calls[mockUseEvents.mock.calls.length - 1][0];

    it("queries with the chosen severity from the first page", async () => {
      const user = userEvent.setup();
      render(<EventsPage />);

      await user.click(screen.getByRole("combobox", { name: /severity/i }));
      await user.click(screen.getByRole("option", { name: "Error" }));

      await waitFor(() => {
        expect(lastEventsQuery()).toMatchObject({
          severity: "error",
          offset: 0,
        });
      });
    });

    it("queries with a minimum risk for AMFA realms, and drops it once AMFA is off", async () => {
      const user = userEvent.setup();
      mockUseAmfaRealms.mockReturnValue({ data: ["AdaptiveAuth"] });
      const { rerender } = render(<EventsPage />);

      await user.click(screen.getByRole("combobox", { name: /min risk/i }));
      await user.click(screen.getByRole("option", { name: "3+" }));
      await waitFor(() => {
        expect(lastEventsQuery()).toMatchObject({ minRisk: 3 });
      });

      // The control disappears with the Risk column; a filter the user can
      // no longer see must not keep narrowing the table.
      mockUseAmfaRealms.mockReturnValue({ data: [] });
      rerender(<EventsPage />);

      await waitFor(() => {
        expect(
          screen.queryByRole("combobox", { name: /min risk/i }),
        ).not.toBeInTheDocument();
      });
      expect(lastEventsQuery().minRisk).toBeUndefined();
    });
  });

  describe("opening an event from a link", () => {
    const lastEventLookup = () => mockUseEvent.mock.calls.at(-1);

    it("opens an event on the loaded page without fetching it", async () => {
      mockSearchParams = new URLSearchParams("event=event-2");
      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByRole("dialog")).toHaveTextContent(
          "Failed login attempt",
        );
      });
      expect(lastEventLookup()).toEqual(["tenant-1", "event-2", false]);
    });

    it("fetches and opens an event the loaded page does not hold", async () => {
      mockSearchParams = new URLSearchParams("event=old-1");
      mockUseEvent.mockReturnValue({
        data: {
          event_id: "old-1",
          timestamp: "2023-06-01T00:00:00Z",
          type: "LOGIN",
          severity: "info",
          description: "An older login",
          source: "keycloak:master",
          client_id: "admin-cli",
        },
        isError: false,
      });
      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByRole("dialog")).toHaveTextContent("An older login");
      });
      expect(lastEventLookup()).toEqual(["tenant-1", "old-1", true]);
    });

    it("says so when the linked event cannot be shown", async () => {
      mockSearchParams = new URLSearchParams("event=gone-1");
      mockUseEvent.mockReturnValue({
        data: undefined,
        isError: true,
        error: new Error("Event not found"),
      });
      render(<EventsPage />);

      await waitFor(() => {
        expect(
          screen.getByText(/linked event was not found/i),
        ).toBeInTheDocument();
      });
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });

  describe("a linked event whose lookup failed", () => {
    it("says the lookup failed, not that the event is missing, on a non-404", async () => {
      mockSearchParams = new URLSearchParams("event=gone-1");
      mockUseEvent.mockReturnValue({
        data: undefined,
        isError: true,
        error: new Error("HTTP 500"),
      });
      render(<EventsPage />);

      expect(
        await screen.findByText(/linked event could not be loaded/i),
      ).toBeInTheDocument();
      expect(
        screen.queryByText(/linked event was not found/i),
      ).not.toBeInTheDocument();
    });

    // react-query keeps a failed query's error after it is disabled, so once
    // the list brings the event in, the stale error must not keep the banner.
    it("drops the banner once the event is on screen after all", async () => {
      mockSearchParams = new URLSearchParams("event=event-2");
      mockUseEvent.mockReturnValue({
        data: undefined,
        isError: true,
        error: new Error("Event not found"),
      });
      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByRole("dialog")).toHaveTextContent(
          "Failed login attempt",
        );
      });
      expect(
        screen.queryByText(/linked event was not found/i),
      ).not.toBeInTheDocument();
    });
  });

  describe("explaining an empty page when the tenant has no realms", () => {
    const noRealmsLoaded = () =>
      mockUseKeycloakDashboard.mockReturnValue({
        data: { realms: [] },
        isLoading: false,
        isError: false,
        isSuccess: true,
      });
    const limitedTo = (tenantId: string) =>
      mockUseAuth.mockReturnValue({
        isAdmin: () => false,
        user: {
          tenant_policies: [{ tenant_id: tenantId, allowed_realms: ["x"] }],
        },
      });

    it("says nothing has synced yet to an unrestricted caller", async () => {
      noRealmsLoaded();
      render(<EventsPage />);

      expect(
        await screen.findByText(
          /no realms have been synced for this tenant yet/i,
        ),
      ).toBeInTheDocument();
    });

    // The server sends realms: null, not [], when nothing has synced.
    it("treats the null realm list the server sends as no realms", async () => {
      mockUseKeycloakDashboard.mockReturnValue({
        data: { realms: null },
        isLoading: false,
        isError: false,
        isSuccess: true,
      });
      render(<EventsPage />);

      expect(
        await screen.findByText(
          /no realms have been synced for this tenant yet/i,
        ),
      ).toBeInTheDocument();
    });

    it("points a realm-limited caller at their own access instead", async () => {
      noRealmsLoaded();
      limitedTo("tenant-1");
      render(<EventsPage />);

      expect(
        await screen.findByText(
          /none of this tenant's realms are available to you/i,
        ),
      ).toBeInTheDocument();
      expect(screen.queryByText(/have been synced/i)).not.toBeInTheDocument();
    });

    // A policy row with no realm list is unrestricted, the same as no row.
    // Found in review: every row used to read as a limit, so such a caller
    // was told to ask an administrator when nothing had synced.
    it("says nothing has synced to a caller whose policy here is unrestricted", async () => {
      noRealmsLoaded();
      mockUseAuth.mockReturnValue({
        isAdmin: () => false,
        user: {
          tenant_policies: [
            { tenant_id: "tenant-1", realms_unrestricted: true },
          ],
        },
      });
      render(<EventsPage />);

      expect(
        await screen.findByText(
          /no realms have been synced for this tenant yet/i,
        ),
      ).toBeInTheDocument();
      expect(
        screen.queryByText(/none of this tenant's realms are available/i),
      ).not.toBeInTheDocument();
    });

    it("lets an unrestricted row win over a restricted one, as the server does", async () => {
      noRealmsLoaded();
      mockUseAuth.mockReturnValue({
        isAdmin: () => false,
        user: {
          tenant_policies: [
            { tenant_id: "tenant-1", allowed_realms: ["x"] },
            { tenant_id: "tenant-1", realms_unrestricted: true },
          ],
        },
      });
      render(<EventsPage />);

      expect(
        await screen.findByText(
          /no realms have been synced for this tenant yet/i,
        ),
      ).toBeInTheDocument();
    });

    it("keeps a deny-all row as a limit", async () => {
      noRealmsLoaded();
      mockUseAuth.mockReturnValue({
        isAdmin: () => false,
        user: {
          tenant_policies: [
            { tenant_id: "tenant-1", realms_unrestricted: false },
          ],
        },
      });
      render(<EventsPage />);

      expect(
        await screen.findByText(
          /none of this tenant's realms are available to you/i,
        ),
      ).toBeInTheDocument();
    });

    it("ignores a policy that limits the caller on another tenant", async () => {
      noRealmsLoaded();
      limitedTo("tenant-2");
      render(<EventsPage />);

      expect(
        await screen.findByText(
          /no realms have been synced for this tenant yet/i,
        ),
      ).toBeInTheDocument();
    });

    it("stays quiet when the tenant has realms", async () => {
      mockUseKeycloakDashboard.mockReturnValue({
        data: mockKeycloakDashboard,
        isLoading: false,
        isError: false,
        isSuccess: true,
      });
      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByTestId("events-table")).toBeInTheDocument();
      });
      expect(
        screen.queryByText(/realms have been synced/i),
      ).not.toBeInTheDocument();
      expect(screen.queryByText(/available to you/i)).not.toBeInTheDocument();
    });

    it("stays quiet until the realm list has loaded", async () => {
      mockUseKeycloakDashboard.mockReturnValue({
        data: undefined,
        isLoading: true,
        isError: false,
        isSuccess: false,
      });
      render(<EventsPage />);

      await waitFor(() => {
        expect(screen.getByTestId("events-table")).toBeInTheDocument();
      });
      expect(
        screen.queryByText(/realms have been synced/i),
      ).not.toBeInTheDocument();
    });
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
  describe("the user column", () => {
    // A row with no username showed its user id twice: once as the label and
    // again in the monospace line beneath it, which is what made the table look
    // like it was full of raw UUIDs.
    it("shows the user id once when the row carries no name", async () => {
      render(<EventsPage />);
      await waitFor(() => {
        expect(screen.getByTestId("events-table")).toBeInTheDocument();
      });

      const id = "11111111-2222-3333-4444-555555555555";
      await waitFor(() => {
        expect(screen.getAllByText(id)).toHaveLength(1);
      });
    });

    it("shows the name, and the id beneath it, when the row has one", async () => {
      render(<EventsPage />);
      await waitFor(() => {
        expect(screen.getByTestId("events-table")).toBeInTheDocument();
      });

      await waitFor(() => {
        expect(screen.getByText("testuser")).toBeInTheDocument();
      });
      // The id still appears for a named row: it is additional information
      // there rather than a substitute for the name.
      expect(screen.getAllByText("user-123")).toHaveLength(1);
    });
  });
});

describe("getSeverityColor", () => {
  it("renders critical in the error red, not the info blue", () => {
    expect(getSeverityColor("critical")).toBe("error");
  });

  it.each([
    ["error", "error"],
    ["warning", "warning"],
    ["info", "info"],
    ["unknown", "info"],
  ])("maps %s to %s", (severity, color) => {
    expect(getSeverityColor(severity)).toBe(color);
  });
});
