import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { DashboardPage } from "./DashboardPage";
import { theme } from "@/theme";
import { useTenant } from "@/shared/context";

const mockUseNavigate = vi.fn();
const mockUseKeycloakDashboard = vi.fn();
const mockUseEvents = vi.fn();
const mockUseEventStats = vi.fn();
const mockUseAlertStats = vi.fn();
const mockUseKeycloakEventStats = vi.fn();

const { mockShowToast, mockGenerateReport } = vi.hoisted(() => ({
  mockShowToast: vi.fn(),
  mockGenerateReport: vi.fn(),
}));

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/tenant-1/dashboard" }),
  useParams: () => ({ tenantId: "tenant-1" }),
  useNavigate: () => mockUseNavigate,
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
  useToast: () => ({ showToast: mockShowToast }),
}));

vi.mock("@/shared/hooks", () => ({
  useRealmSelector: () => ({
    selectedRealm: "all",
    handleRealmChange: vi.fn(),
  }),
  useKeycloakDashboard: () => mockUseKeycloakDashboard(),
  useEvents: () => mockUseEvents(),
  useEventStats: () => mockUseEventStats(),
  useKeycloakEventStats: () => mockUseKeycloakEventStats(),
  useAlertStats: () => mockUseAlertStats(),
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
  EmptyState: ({ message }: { message: string }) => (
    <div data-testid="empty-state">{message}</div>
  ),
  DialogHeader: ({ title, subtitle }: { title: string; subtitle?: string }) => (
    <div>
      <div>{title}</div>
      {subtitle && <div>{subtitle}</div>}
    </div>
  ),
  RealmSelector: () => <div data-testid="realm-selector">Realm Selector</div>,
  TimeSelector: ({
    onTimeRangeChange,
  }: {
    onTimeRangeChange: (start: Date | undefined, end: Date | undefined) => void;
  }) => (
    <div
      data-testid="time-selector"
      onClick={() => onTimeRangeChange(undefined, undefined)}
    >
      Time Selector
    </div>
  ),
  FeatureErrorBoundary: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

vi.mock("@/shared/utils", () => ({
  formatBytes: (bytes: number) => `${bytes}B`,
  formatUptime: (millis: number) => `${millis}ms`,
  isTenantConnectionBroken: (tenant?: { health_status?: string } | null) =>
    ["unhealthy", "degraded", "down", "error"].includes(
      (tenant?.health_status ?? "").toLowerCase(),
    ),
}));

vi.mock("../hooks", () => ({
  useGenerateReport: () => ({
    mutate: mockGenerateReport,
    isPending: false,
  }),
}));

const mockDashboard = {
  realms: [
    {
      realm_name: "master",
      enabled: true,
      is_healthy: true,
      metrics: {
        total_users: 100,
        active_sessions: 10,
        login_events: 50,
        failed_login_events: 5,
      },
      events_enabled: true,
      events_listeners: ["event-listener"],
    },
  ],
  health: {
    status: "UP",
    response_time_ms: 10,
    memory_used_bytes: 1000000,
    memory_max_bytes: 2000000,
    memory_free_bytes: 1000000,
    uptime_millis: 3600000,
    server_version: "1.0.0",
  },
};

const mockEvents = {
  events: [
    {
      event_id: "event-1",
      type: "LOGIN",
      severity: "info",
      description: "User logged in",
      source: "keycloak",
      timestamp: "2024-01-01T00:00:00Z",
    },
  ],
  total: 1,
};

const mockStats = {
  total_events: 1000,
};

const mockAlertStats = {
  total_active: 5,
  by_severity: { critical: 1, error: 2, warning: 2 },
};

function realmsDot() {
  return screen.getByText(/View realms →/).parentElement!.querySelector("svg");
}

describe("DashboardPage", () => {
  const mockSelectedTenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
    default_realm: "master",
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useTenant).mockReturnValue({
      selectedTenant: mockSelectedTenant,
      tenants: [mockSelectedTenant],
      selectTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoading: false,
      error: null,
    });

    mockUseKeycloakDashboard.mockReturnValue({
      data: mockDashboard,
      dataUpdatedAt: Date.now(),
      isLoading: false,
      isError: false,
    });

    mockUseEvents.mockReturnValue({
      data: mockEvents,
      isLoading: false,
      isError: false,
    });

    mockUseEventStats.mockReturnValue({
      data: mockStats,
      dataUpdatedAt: Date.now(),
      isLoading: false,
      isError: false,
    });

    mockUseAlertStats.mockReturnValue({
      data: mockAlertStats,
      isLoading: false,
      isError: false,
    });

    mockUseKeycloakEventStats.mockReturnValue({
      data: { login_count: 50, login_error_count: 5 },
      isLoading: false,
      isError: false,
    });
  });

  it("should render dashboard with key metrics", async () => {
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Dashboard")).toBeInTheDocument();
    });

    expect(
      screen.getByText("Real-time monitoring overview"),
    ).toBeInTheDocument();
  });

  it("should display total events metric", async () => {
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Total Events")).toBeInTheDocument();
    });

    // Query within the Total Events card to avoid ambiguity
    const totalEventsCard = screen.getByText("Total Events").closest("div");
    expect(totalEventsCard).toHaveTextContent(/1[,.]?000/);
  });

  it("should display realms count", async () => {
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Realms")).toBeInTheDocument();
    });

    // Query within the Realms card to avoid ambiguity
    const realmsCard = screen.getByText("Realms").closest("div");
    expect(realmsCard).toHaveTextContent("1");
  });

  it("should display active alerts", async () => {
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Active Alerts")).toBeInTheDocument();
    });

    // Query within the Active Alerts card to avoid ambiguity
    const alertsCard = screen.getByText("Active Alerts").closest("div");
    expect(alertsCard).toHaveTextContent("5");
  });

  it("should display system health status", async () => {
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getAllByText("System Health")[0]).toBeInTheDocument();
    });

    expect(screen.getByText("UP")).toBeInTheDocument();
  });

  it("should display memory usage", async () => {
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText(/Memory/)).toBeInTheDocument();
    });
  });

  it("should navigate to events page when events card is clicked", async () => {
    const user = userEvent.setup();
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Total Events")).toBeInTheDocument();
    });

    const eventsCard = screen.getByText("Total Events").closest("div");
    if (eventsCard) {
      await user.click(eventsCard);
      expect(mockUseNavigate).toHaveBeenCalledWith("/tenant-1/events");
    }
  });

  it("should navigate to realms page when realms card is clicked", async () => {
    const user = userEvent.setup();
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Realms")).toBeInTheDocument();
    });

    const realmsCard = screen.getByText("Realms").closest("div");
    if (realmsCard) {
      await user.click(realmsCard);
      expect(mockUseNavigate).toHaveBeenCalledWith("/tenant-1/realms");
    }
  });

  it("should navigate to alerts page when alerts card is clicked", async () => {
    const user = userEvent.setup();
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Active Alerts")).toBeInTheDocument();
    });

    const alertsCard = screen.getByText("Active Alerts").closest("div");
    if (alertsCard) {
      await user.click(alertsCard);
      expect(mockUseNavigate).toHaveBeenCalledWith("/tenant-1/alerts");
    }
  });

  it("should display recent events when available", async () => {
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Recent Activity")).toBeInTheDocument();
    });

    expect(screen.getByText("LOGIN")).toBeInTheDocument();
  });

  it("marks a critical recent event in the error red, not the info blue", async () => {
    const event = (type: string, severity: string) => ({
      ...mockEvents.events[0],
      event_id: type,
      type,
      severity,
    });
    mockUseEvents.mockReturnValue({
      data: {
        events: [
          event("CRITICAL_EVENT", "critical"),
          event("ERROR_EVENT", "error"),
          event("INFO_EVENT", "info"),
        ],
        total: 3,
      },
      isLoading: false,
      isError: false,
    });
    render(<DashboardPage />);

    // The severity dot is the sibling just before the block holding the type.
    const dotColor = async (type: string) =>
      getComputedStyle(
        (await screen.findByText(type)).parentElement!.previousElementSibling!,
      ).backgroundColor;

    expect(await dotColor("CRITICAL_EVENT")).toBe(
      await dotColor("ERROR_EVENT"),
    );
    expect(await dotColor("CRITICAL_EVENT")).not.toBe(
      await dotColor("INFO_EVENT"),
    );
  });

  it("should show empty state when no recent events", async () => {
    mockUseEvents.mockReturnValue({
      data: { events: [], total: 0 },
      isLoading: false,
      isError: false,
    });

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByTestId("empty-state")).toBeInTheDocument();
    });

    expect(screen.getByText("No recent events")).toBeInTheDocument();
  });

  it("should open report generation dialog", async () => {
    const user = userEvent.setup();
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Generate Report")).toBeInTheDocument();
    });

    const reportButton = screen.getByText("Generate Report");
    await user.click(reportButton);

    await waitFor(() => {
      expect(
        screen.getByText("Select the date range for the report"),
      ).toBeInTheDocument();
    });
  });

  it.each([
    [
      "HTTP 403",
      "You don't have permission to generate this report: it needs alerts:read or metrics:read",
    ],
    ["HTTP 500", "Failed to generate report"],
  ])("tells the user why a report failed (%s)", async (status, message) => {
    mockShowToast.mockClear();
    mockGenerateReport.mockImplementation(
      (_vars: unknown, opts?: { onError?: (e: Error) => void }) =>
        opts?.onError?.(new Error(status)),
    );
    const user = userEvent.setup();
    render(<DashboardPage />);

    await user.click(await screen.findByText("Generate Report"));
    await screen.findByText("Select the date range for the report");
    const buttons = screen.getAllByRole("button", { name: "Generate Report" });
    await user.click(buttons[buttons.length - 1]);

    await waitFor(() =>
      expect(mockShowToast).toHaveBeenCalledWith({ message, type: "error" }),
    );
  });

  it("should display realm selector and time selector", async () => {
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByTestId("realm-selector")).toBeInTheDocument();
    });

    expect(screen.getByTestId("time-selector")).toBeInTheDocument();
  });

  it("shows a connection-error indicator on the Realms card when the tenant is unhealthy", () => {
    vi.mocked(useTenant).mockReturnValue({
      selectedTenant: {
        tenant_id: "tenant-1",
        name: "Test Tenant",
        default_realm: "master",
        health_status: "unhealthy",
        last_error: "connection refused",
      },
      tenants: [],
      selectTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoading: false,
      error: null,
    });
    mockUseKeycloakDashboard.mockReturnValue({
      data: { realms: [], health: null },
      dataUpdatedAt: Date.now(),
      isLoading: false,
      isError: false,
    });

    render(<DashboardPage />);

    expect(screen.getByText(/connection error/i)).toBeInTheDocument();
  });

  // A realm KMT can no longer read shows its last known health, which says
  // nothing about now, so it does not count as healthy.
  it("does not count a realm it cannot read as healthy", () => {
    const realm = mockDashboard.realms[0];
    mockUseKeycloakDashboard.mockReturnValue({
      data: {
        ...mockDashboard,
        realms: [
          realm,
          {
            ...realm,
            realm_name: "rx",
            unreadable_since: "2026-10-14T10:05:00Z",
          },
        ],
      },
      dataUpdatedAt: Date.now(),
      isLoading: false,
      isError: false,
    });

    render(<DashboardPage />);

    expect(screen.getByText(/1 healthy • View realms/)).toBeInTheDocument();
  });

  // A failed stats request used to render as "0" total events, which reads as
  // a quiet window rather than a broken query.
  it("shows the total events as unavailable when the stats request fails", async () => {
    mockUseEventStats.mockReturnValue({
      data: undefined,
      dataUpdatedAt: 0,
      isLoading: false,
      isError: true,
    });

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Total Events")).toBeInTheDocument();
    });

    const eventsCard = screen.getByText("Total Events").closest("div");
    expect(eventsCard).toHaveTextContent("—");
    expect(eventsCard).not.toHaveTextContent(/\b0\b/);
    expect(screen.getByText("Event counts unavailable")).toBeInTheDocument();
    expect(screen.queryByText("View all events →")).not.toBeInTheDocument();
  });

  // A failed alert-stats request used to render as "0" active alerts behind a
  // green dot, which reads as "nothing to look at".
  it("shows the active alerts as unavailable when the alert stats request fails", async () => {
    mockUseAlertStats.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    });

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Active Alerts")).toBeInTheDocument();
    });

    const alertsCard = screen.getByText("Active Alerts").closest("div");
    expect(alertsCard).toHaveTextContent("—");
    expect(alertsCard).not.toHaveTextContent(/\b0\b/);
    expect(screen.getByText("Alert counts unavailable")).toBeInTheDocument();
    expect(screen.queryByText("View all alerts →")).not.toBeInTheDocument();
  });

  // TanStack Query clears error and isError on every refetch of a query that
  // never loaded, so between a failed first load and its retry a tile read
  // isError false and fell back to "0" behind a green dot. Only errorUpdatedAt
  // keeps saying the last finished request failed.
  const retryingAfterFailedFirstLoad = {
    data: undefined,
    error: null,
    isError: false,
    isLoading: true,
    isFetching: true,
    status: "pending",
    dataUpdatedAt: 0,
    errorUpdatedAt: 1_000,
  };

  it.each([
    ["Total Events", mockUseEventStats, "Event counts unavailable"],
    ["Active Alerts", mockUseAlertStats, "Alert counts unavailable"],
    ["Total Users", mockUseKeycloakDashboard, "User counts unavailable"],
    ["Failed Logins", mockUseKeycloakEventStats, "Login counts unavailable"],
  ])(
    "keeps %s unavailable while its failed first load is retried",
    async (title, hook, unavailable) => {
      hook.mockReturnValue(retryingAfterFailedFirstLoad);

      render(<DashboardPage />);

      await waitFor(() => {
        expect(screen.getByText(title)).toBeInTheDocument();
      });
      expect(screen.getByText(title).closest("div")).toHaveTextContent("—");
      expect(screen.getByText(unavailable)).toBeInTheDocument();
    },
  );

  // A tenant with no realms is a normal answer, not a warning; only a failed
  // request turns the dot amber.
  it("keeps the Realms dot green for a tenant whose realm list is empty", async () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: { ...mockDashboard, realms: [] },
      dataUpdatedAt: Date.now(),
      isLoading: false,
      isError: false,
    });

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("0 healthy • View realms →")).toBeInTheDocument();
    });
    const dot = realmsDot();
    expect(dot).toHaveStyle({ color: theme.palette.success.main });
    expect(dot).not.toHaveStyle({ color: theme.palette.warning.main });
  });

  // Without realms, "0 healthy out of 0" counted as all healthy, so a failed
  // dashboard request showed an empty realm list behind a green dot.
  it("shows the realm and user counts as unavailable when the keycloak dashboard request fails", async () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: undefined,
      dataUpdatedAt: 0,
      isLoading: false,
      isError: true,
    });

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Realms")).toBeInTheDocument();
    });

    const realmsCard = screen.getByText("Realms").closest("div");
    expect(realmsCard).not.toHaveTextContent(/\b0\b/);
    expect(
      screen.getByText("Realm data unavailable • View realms →"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/healthy • View realms →/),
    ).not.toBeInTheDocument();
    expect(realmsDot()).toHaveStyle({ color: theme.palette.warning.main });

    const usersCard = screen.getByText("Total Users").closest("div");
    expect(usersCard).toHaveTextContent("—");
    expect(usersCard).not.toHaveTextContent(/\b0\b/);
    expect(screen.getByText("User counts unavailable")).toBeInTheDocument();
    expect(screen.queryByText(/active sessions/)).not.toBeInTheDocument();
  });

  // The dashboard refreshes every ten seconds and keeps the last answer. A
  // failed refresh used to leave a green "UP" behind with no sign of its age.
  it("marks the keycloak status as out of date when a refresh fails", async () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: mockDashboard,
      dataUpdatedAt: new Date("2026-10-12T08:30:00Z").getTime(),
      isLoading: false,
      isError: true,
    });

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("UP")).toBeInTheDocument();
    });

    expect(
      screen.getByText(/Last updated .+, refresh failed/),
    ).toBeInTheDocument();
    expect(screen.queryByText(/ms response/)).not.toBeInTheDocument();
  });

  it("still shows a real zero as zero", async () => {
    mockUseEventStats.mockReturnValue({
      data: { total_events: 0 },
      dataUpdatedAt: Date.now(),
      isLoading: false,
      isError: false,
    });
    mockUseAlertStats.mockReturnValue({
      data: { total_active: 0, by_severity: { critical: 0 } },
      isLoading: false,
      isError: false,
    });

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Total Events")).toBeInTheDocument();
    });

    expect(screen.getByText("Total Events").closest("div")).toHaveTextContent(
      "0",
    );
    expect(screen.getByText("Active Alerts").closest("div")).toHaveTextContent(
      "0",
    );
    expect(screen.queryByText(/unavailable/)).not.toBeInTheDocument();
  });
});

describe("DashboardPage login KPIs", () => {
  // The realm metrics the dashboard used to sum are collected over one metrics
  // polling interval, so on any system that is not mid-login they read zero
  // while real logins sit in the events table. The KPI strip has to read the
  // event statistics for the selected window instead.
  it("reports logins from the event statistics, not from interval-scoped realm metrics", async () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: {
        ...mockDashboard,
        realms: [
          {
            ...mockDashboard.realms[0],
            metrics: {
              total_users: 100,
              active_sessions: 10,
              login_events: 0,
              failed_login_events: 0,
            },
          },
        ],
      },
      dataUpdatedAt: Date.now(),
      isLoading: false,
      isError: false,
    });
    mockUseKeycloakEventStats.mockReturnValue({
      data: { login_count: 13, login_error_count: 2 },
      isLoading: false,
      isError: false,
    });

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Failed Logins")).toBeInTheDocument();
    });

    const failedCard = screen.getByText("Failed Logins").closest("div");
    expect(failedCard).toHaveTextContent("2");
    expect(failedCard).not.toHaveTextContent(/\b0\b/);
  });

  // A failed login-stats request used to render as "0" failed logins, which
  // reads as a quiet day rather than a broken query.
  it("shows the login counts as unavailable when the stats request fails", async () => {
    mockUseKeycloakEventStats.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    });

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText("Failed Logins")).toBeInTheDocument();
    });

    const failedCard = screen.getByText("Failed Logins").closest("div");
    expect(failedCard).toHaveTextContent("—");
    expect(failedCard).not.toHaveTextContent(/\b0\b/);
    expect(screen.getByText("Login counts unavailable")).toBeInTheDocument();
    expect(screen.queryByText(/\d+ successful/)).not.toBeInTheDocument();
  });
});

describe("DashboardPage live indicator", () => {
  const tenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
    default_realm: "master",
  };

  function allFresh(at: number) {
    vi.mocked(useTenant).mockReturnValue({
      selectedTenant: tenant,
      tenants: [tenant],
      selectTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoading: false,
      error: null,
    });
    const ok = {
      isLoading: false,
      isError: false,
      error: null,
      dataUpdatedAt: at,
      errorUpdatedAt: 0,
    };
    mockUseKeycloakDashboard.mockReturnValue({ ...ok, data: mockDashboard });
    mockUseEvents.mockReturnValue({ ...ok, data: mockEvents });
    mockUseEventStats.mockReturnValue({ ...ok, data: mockStats });
    mockUseAlertStats.mockReturnValue({ ...ok, data: mockAlertStats });
    mockUseKeycloakEventStats.mockReturnValue({
      ...ok,
      data: { login_count: 50, login_error_count: 5 },
    });
  }

  it("is live when every query refreshed", () => {
    allFresh(Date.now());
    render(<DashboardPage />);
    expect(screen.getByRole("status")).toHaveAttribute("data-state", "live");
    expect(screen.getByText("Live")).toBeInTheDocument();
  });

  // The header used to take the newest time of two queries and a hardcoded
  // green dot, so a failing events query hid behind a refreshing Keycloak one.
  it("warns when one query fails, and dates the page by its old data", async () => {
    const now = Date.now();
    const old = now - 45_000;
    allFresh(now);
    mockUseEventStats.mockReturnValue({
      data: mockStats,
      dataUpdatedAt: old,
      errorUpdatedAt: now,
      isLoading: false,
      isError: true,
      error: Object.assign(new Error("HTTP 500"), { status: 500 }),
    });

    render(<DashboardPage />);

    expect(screen.getByRole("status")).toHaveAttribute("data-state", "failing");
    expect(screen.getByText("Not refreshing")).toBeInTheDocument();
    expect(
      screen.getByText(`Updated ${new Date(old).toLocaleTimeString()}`),
    ).toBeInTheDocument();
    await userEvent.hover(screen.getByRole("status"));
    expect(
      await screen.findByText(/Could not refresh: event counts\./),
    ).toBeInTheDocument();
  });

  it("does not warn about alert counts the user may not read", () => {
    allFresh(Date.now());
    mockUseAlertStats.mockReturnValue({
      data: undefined,
      dataUpdatedAt: 0,
      errorUpdatedAt: Date.now(),
      isLoading: false,
      isError: true,
      error: Object.assign(new Error("Insufficient permissions"), {
        status: 403,
      }),
    });
    render(<DashboardPage />);
    expect(screen.getByRole("status")).toHaveAttribute("data-state", "live");
  });
});
