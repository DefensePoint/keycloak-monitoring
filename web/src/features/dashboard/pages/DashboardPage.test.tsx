import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { DashboardPage } from "./DashboardPage";
import { useTenant } from "@/shared/context";

const mockUseNavigate = vi.fn();
const mockUseKeycloakDashboard = vi.fn();
const mockUseEvents = vi.fn();
const mockUseEventStats = vi.fn();
const mockUseAlertStats = vi.fn();
const mockUseKeycloakEventStats = vi.fn();

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
  useToast: () => ({ showToast: vi.fn() }),
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
    mutate: vi.fn(),
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
});
