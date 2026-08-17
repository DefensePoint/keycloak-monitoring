import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { RealmPage } from "./RealmPage";
import { useAuth, useTenant } from "@/shared/context";

const mockUseRealmDashboard = vi.fn();
const mockUseKeycloakDashboard = vi.fn();
const mockUseRealmEvents = vi.fn();
const mockUseRealmUsers = vi.fn();
const mockUseRealmClients = vi.fn();
const mockUseAlerts = vi.fn();
const mockUseNavigate = vi.fn();

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/tenant-1/realms/master" }),
  useParams: () => ({ realmName: "master", tenantId: "tenant-1" }),
  useNavigate: () => mockUseNavigate,
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useAuth: vi.fn(),
  useTenant: vi.fn(),
}));

vi.mock("../hooks", () => ({
  useRealmNavigation: () => ({ navigateToRealm: vi.fn() }),
}));

vi.mock("@/shared/hooks", () => ({
  useAlerts: () => mockUseAlerts(),
  useRealmSelector: () => ({
    selectedRealm: "master",
    handleRealmChange: vi.fn(),
  }),
  useRealmDashboard: () => mockUseRealmDashboard(),
  useKeycloakDashboard: () => mockUseKeycloakDashboard(),
  useRealmEvents: () => mockUseRealmEvents(),
  useRealmUsers: () => mockUseRealmUsers(),
  useRealmClients: () => mockUseRealmClients(),
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
  LoadingSkeleton: () => (
    <div data-testid="loading-state">Loading realm details...</div>
  ),
  FeatureErrorBoundary: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

vi.mock("@/shared/utils", () => ({
  formatBytes: (bytes: number) => `${bytes}B`,
}));

vi.mock("../components", () => ({
  RealmPageHeader: () => <div data-testid="realm-header">Header</div>,
  RealmSelector: ({
    selectedRealm,
    onRealmChange,
  }: {
    selectedRealm: string;
    onRealmChange: (realm: string) => void;
  }) => (
    <select
      data-testid="realm-selector"
      value={selectedRealm}
      onChange={(e) => onRealmChange(e.target.value)}
    >
      <option value="all">All Realms</option>
      <option value="master">Master</option>
    </select>
  ),
}));

const mockRealmDashboard = {
  realm_info: {
    realm_id: "master",
    display_name: "Master Realm",
    enabled: true,
    ssl_required: "external",
    events_enabled: true,
    admin_events_enabled: true,
    brute_force_protected: true,
    verify_email: false,
    reset_password_allowed: true,
  },
  metrics: {
    total_users: 100,
    enabled_users: 95,
    disabled_users: 5,
    active_sessions: 10,
    offline_sessions: 2,
    total_clients: 15,
  },
  recent_events: {
    logins: 50,
    login_errors: 5,
    time_period: "Last 24 hours",
  },
  health: {
    status: "UP",
    response_time_ms: 10,
    memory_used_bytes: 1000000,
    uptime_millis: 3600000,
  },
};

const mockEventsResponse = {
  events: [
    {
      event_id: "event-1",
      timestamp: "2024-01-01T00:00:00Z",
      type: "LOGIN",
      description: "User logged in",
      severity: "info",
    },
  ],
};

const mockUsersResponse = {
  users: [
    {
      id: "user-1",
      username: "testuser",
      email: "test@example.com",
      enabled: true,
    },
  ],
};

const mockClientsResponse = {
  clients: [
    {
      id: "client-1",
      clientId: "test-client",
      name: "Test Client",
      enabled: true,
      publicClient: false,
      bearerOnly: false,
    },
  ],
};

const mockAlertsResponse = {
  alerts: [
    {
      alert_id: "alert-1",
      title: "Test Alert",
      description: "Test description",
      severity: "critical",
      status: "active",
    },
  ],
};

describe("RealmPage", () => {
  const mockUser = {
    name: "Test User",
    email: "test@example.com",
  };

  const mockSelectedTenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
    default_realm: "master",
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useAuth).mockReturnValue({
      user: mockUser,
      authConfig: null,
      loginSimple: vi.fn(),
      loginOAuth2: vi.fn(),
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(true),
      hasAnyPermission: vi.fn().mockReturnValue(true),
      hasRole: vi.fn().mockReturnValue(true),
      isAdmin: vi.fn().mockReturnValue(true),
      hasAccessToTenant: vi.fn().mockReturnValue(true),
    });

    vi.mocked(useTenant).mockReturnValue({
      selectedTenant: mockSelectedTenant,
      tenants: [mockSelectedTenant],
      selectTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoading: false,
      error: null,
    });

    mockUseRealmDashboard.mockReturnValue({
      data: mockRealmDashboard,
      isLoading: false,
      isError: false,
    });

    mockUseKeycloakDashboard.mockReturnValue({
      data: { realms: [{ realm_name: "master" }] },
      isLoading: false,
      isError: false,
    });

    mockUseRealmEvents.mockReturnValue({
      data: mockEventsResponse,
      isLoading: false,
      isError: false,
    });

    mockUseRealmUsers.mockReturnValue({
      data: mockUsersResponse,
      isLoading: false,
      isError: false,
    });

    mockUseRealmClients.mockReturnValue({
      data: mockClientsResponse,
      isLoading: false,
      isError: false,
    });

    mockUseAlerts.mockReturnValue({
      data: mockAlertsResponse,
      isLoading: false,
      isError: false,
    });
  });

  it("should render loading state initially", () => {
    mockUseRealmDashboard.mockReturnValue({
      data: null,
      isLoading: true,
      isError: false,
    });

    render(<RealmPage />);
    expect(screen.getByTestId("loading-state")).toBeInTheDocument();
    expect(screen.getByText("Loading realm details...")).toBeInTheDocument();
  });

  it("should display realm configuration section", async () => {
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("Realm Configuration")).toBeInTheDocument();
    });

    // "master" appears in multiple places (selector, config)
    expect(screen.getAllByText("master").length).toBeGreaterThan(0);
    expect(screen.getByText("Master Realm")).toBeInTheDocument();
    expect(screen.getAllByText("Enabled").length).toBeGreaterThan(0);
  });

  it("should display event configuration", async () => {
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("Event Configuration")).toBeInTheDocument();
    });

    expect(screen.getAllByText("Enabled").length).toBeGreaterThan(0);
  });

  it("should display security settings", async () => {
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("Security Settings")).toBeInTheDocument();
    });

    expect(screen.getByText("Brute Force Protection")).toBeInTheDocument();
    expect(screen.getByText("Email Verification")).toBeInTheDocument();
    expect(screen.getByText("Password Reset")).toBeInTheDocument();
  });

  it("should display configuration alerts", async () => {
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("Configuration Alerts")).toBeInTheDocument();
    });

    expect(screen.getByText("1 Active")).toBeInTheDocument();
    expect(screen.getByText("Test Alert")).toBeInTheDocument();
  });

  it("should show no active alerts message when no alerts", async () => {
    mockUseAlerts.mockReturnValue({
      data: { alerts: [] },
      isLoading: false,
      isError: false,
    });

    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("No active alerts")).toBeInTheDocument();
    });
  });

  it("should display current metrics", async () => {
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("Current Metrics")).toBeInTheDocument();
    });

    expect(screen.getByText("100")).toBeInTheDocument(); // Total users
    expect(screen.getByText("10")).toBeInTheDocument(); // Active sessions
    expect(screen.getByText("15")).toBeInTheDocument(); // Total clients
  });

  it("should display users list with search", async () => {
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("Users")).toBeInTheDocument();
    });

    expect(screen.getByText("testuser")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Search users...")).toBeInTheDocument();
  });

  it("should display clients list with search", async () => {
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("Clients")).toBeInTheDocument();
    });

    expect(screen.getByText("test-client")).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText("Search clients..."),
    ).toBeInTheDocument();
  });

  it("should display recent events", async () => {
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText(/Recent Events/)).toBeInTheDocument();
    });

    expect(screen.getByText("50")).toBeInTheDocument(); // Successful logins
    expect(screen.getByText("5")).toBeInTheDocument(); // Failed logins
  });

  it("should display realm events table", async () => {
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("Realm Events")).toBeInTheDocument();
    });

    expect(screen.getByText("LOGIN")).toBeInTheDocument();
  });

  it("should display server health", async () => {
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("Server Health")).toBeInTheDocument();
    });

    expect(screen.getByText("UP")).toBeInTheDocument();
    expect(screen.getByText("10ms")).toBeInTheDocument();
  });

  it("should open event details dialog when event is clicked", async () => {
    const user = userEvent.setup();
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("LOGIN")).toBeInTheDocument();
    });

    const loginEvent = screen.getByText("LOGIN");
    await user.click(loginEvent);

    await waitFor(() => {
      expect(screen.getByText("Event Details")).toBeInTheDocument();
    });
  });

  it("should navigate to user details when user is clicked", async () => {
    const user = userEvent.setup();
    render(<RealmPage />);

    await waitFor(() => {
      expect(screen.getByText("testuser")).toBeInTheDocument();
    });

    const userElement = screen.getByText("testuser").closest("button");
    if (userElement) {
      await user.click(userElement);
      expect(mockUseNavigate).toHaveBeenCalledWith(
        "/tenant-1/realm/master/user/user-1",
      );
    }
  });

  it("should handle user search", async () => {
    const user = userEvent.setup();
    render(<RealmPage />);

    await waitFor(() => {
      expect(
        screen.getByPlaceholderText("Search users..."),
      ).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText("Search users...");
    await user.type(searchInput, "test");

    expect(searchInput).toHaveValue("test");
  });

  it("should handle client search", async () => {
    const user = userEvent.setup();
    render(<RealmPage />);

    await waitFor(() => {
      expect(
        screen.getByPlaceholderText("Search clients..."),
      ).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText("Search clients...");
    await user.type(searchInput, "test");

    expect(searchInput).toHaveValue("test");
  });

  it("should show no data message when realm data not available", () => {
    mockUseRealmDashboard.mockReturnValue({
      data: null,
      isLoading: false,
      isError: false,
    });

    render(<RealmPage />);

    expect(screen.getByText("No realm data available")).toBeInTheDocument();
  });
});
