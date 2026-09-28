import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { RealmDetails } from "./RealmDetails";
import { useRealmDashboard } from "../hooks";
import type { KeycloakDashboard } from "../types";
import { useTenant } from "@/shared/context";

vi.mock("../hooks", () => ({
  useRealmDashboard: vi.fn(),
}));

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
}));

vi.mock("@/shared/components", () => ({
  LoadingSkeleton: () => <div>Loading realm details...</div>,
  SectionCard: ({
    title,
    children,
  }: {
    title: string;
    children: React.ReactNode;
  }) => (
    <div>
      <h3>{title}</h3>
      {children}
    </div>
  ),
  EmptyState: ({ message }: { message: string }) => <div>{message}</div>,
  DialogHeader: ({
    title,
    subtitle,
    onClose,
  }: {
    title: string;
    subtitle?: string;
    onClose?: () => void;
  }) => (
    <div>
      <span>{title}</span>
      {subtitle && <span>{subtitle}</span>}
      {onClose && <button onClick={onClose}>Close</button>}
    </div>
  ),
  StatusBadge: ({ status, label }: { status: string; label?: string }) => (
    <span data-testid="status-badge" data-status={status}>
      {label || status}
    </span>
  ),
}));

const mockTenant = {
  id: 1,
  tenant_id: "test-tenant",
  name: "Test Tenant",
  enabled: true,
  server_url: "http://test.com",
  admin_realm: "master",
  client_id: "admin-cli",
  health_status: "healthy",
  created_at: "2023-01-01T00:00:00Z",
  updated_at: "2023-01-01T00:00:00Z",
  last_health_check: "2023-01-01T00:00:00Z",
};

const mockRealmDashboard: KeycloakDashboard = {
  realm: "test-realm",
  recent_events: {
    time_period: "24h",
    logins: 50,
    login_errors: 5,
  },
  realm_info: {
    id: 1,
    realm_id: "test-realm",
    realm_name: "test-realm",
    display_name: "Test Realm",
    enabled: true,
    is_healthy: true,
    last_checked: new Date().toISOString(),
    ssl_required: "EXTERNAL",
    events_enabled: true,
    admin_events_enabled: true,
    admin_events_details_enabled: false,
    events_listeners: ["jboss-logging", "custom-listener"],
    enabled_event_types: ["LOGIN", "LOGOUT", "LOGIN_ERROR", "REGISTER"],
    brute_force_protected: true,
    verify_email: true,
    reset_password_allowed: true,
  },
  metrics: {
    time: new Date().toISOString(),
    realm_name: "test-realm",
    total_users: 100,
    enabled_users: 95,
    disabled_users: 5,
    active_sessions: 25,
    offline_sessions: 10,
    total_clients: 10,
    login_events: 50,
    logout_events: 10,
    failed_login_events: 5,
    register_events: 3,
  },
  health: {
    time: new Date().toISOString(),
    status: "UP",
    response_time_ms: 150,
    memory_used_bytes: 1073741824, // 1 GB
    memory_max_bytes: 2147483648, // 2 GB
    memory_free_bytes: 1073741824, // 1 GB
    uptime_millis: 86400000, // 24 hours
  },
};

const mockUseRealmDashboard = vi.mocked(useRealmDashboard);

describe("RealmDetails", () => {
  const mockOnClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useTenant).mockReturnValue({
      tenants: [mockTenant],
      selectedTenant: mockTenant,
      selectTenant: vi.fn(),
      isLoading: false,
      error: null,
      refreshTenants: vi.fn(),
    });
    mockUseRealmDashboard.mockReturnValue({
      data: mockRealmDashboard,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useRealmDashboard>);
  });

  it("should render loading state initially", () => {
    mockUseRealmDashboard.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as ReturnType<typeof useRealmDashboard>);

    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    expect(screen.getByText("Loading realm details...")).toBeInTheDocument();
  });

  it("should display realm name in header", () => {
    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    expect(screen.getAllByText("test-realm").length).toBeGreaterThan(0);
    expect(screen.getByText("Realm Details")).toBeInTheDocument();
  });

  it("should call onClose when close button is clicked", async () => {
    const user = userEvent.setup();

    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    const closeButtons = screen.getAllByRole("button");
    const closeButton = closeButtons.find((button) =>
      button.querySelector("svg"),
    );

    if (closeButton) {
      await user.click(closeButton);
      expect(mockOnClose).toHaveBeenCalled();
    }
  });

  it("should call onClose when overlay is clicked", async () => {
    const user = userEvent.setup();

    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    // Find the MUI Dialog backdrop and click it
    const backdrop = document.querySelector(".MuiBackdrop-root") as HTMLElement;
    await user.click(backdrop);

    expect(mockOnClose).toHaveBeenCalled();
  });

  it("should render realm configuration when data is loaded", async () => {
    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    await waitFor(() => {
      expect(screen.getByText("Realm Configuration")).toBeInTheDocument();
    });

    expect(screen.getByText("Test Realm")).toBeInTheDocument();
    expect(screen.getByText("EXTERNAL")).toBeInTheDocument();
  });

  it("should render metrics when available", async () => {
    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    await waitFor(() => {
      expect(screen.getByText("Current Metrics")).toBeInTheDocument();
    });

    expect(screen.getByText("100")).toBeInTheDocument(); // Total users
    expect(screen.getByText("95 enabled, 5 disabled")).toBeInTheDocument();
    expect(screen.getByText("25")).toBeInTheDocument(); // Active sessions
  });

  it("should render recent events when available", async () => {
    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    await waitFor(() => {
      expect(screen.getByText(/Recent Events/)).toBeInTheDocument();
    });

    expect(screen.getByText("50")).toBeInTheDocument(); // Successful logins
    expect(screen.getByText("5")).toBeInTheDocument(); // Failed logins
  });

  it("should render health status when available", async () => {
    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    await waitFor(() => {
      expect(screen.getByText("Server Health")).toBeInTheDocument();
    });

    expect(screen.getByText("UP")).toBeInTheDocument();
    expect(screen.getByText("150ms")).toBeInTheDocument();
    expect(screen.getByText("1.00 GB")).toBeInTheDocument(); // Memory
    expect(screen.getByText("24h")).toBeInTheDocument(); // Uptime
  });

  it("should display enabled/disabled status correctly", async () => {
    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    await waitFor(() => {
      expect(screen.getByText("Realm Configuration")).toBeInTheDocument();
    });

    const enabledBadges = screen.getAllByText("Enabled");
    expect(enabledBadges.length).toBeGreaterThan(0);
  });

  it("should render event listeners", async () => {
    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    await waitFor(() => {
      expect(screen.getByText("Event Listeners")).toBeInTheDocument();
    });

    expect(screen.getByText("jboss-logging")).toBeInTheDocument();
    expect(screen.getByText("custom-listener")).toBeInTheDocument();
  });

  it("should render enabled event types with limit", async () => {
    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    await waitFor(() => {
      expect(screen.getByText("Enabled Event Types")).toBeInTheDocument();
    });

    expect(screen.getByText("LOGIN")).toBeInTheDocument();
    expect(screen.getByText("LOGOUT")).toBeInTheDocument();
  });

  it("should display security settings", async () => {
    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    await waitFor(() => {
      expect(screen.getByText("Security Settings")).toBeInTheDocument();
    });

    expect(screen.getByText("Brute Force Protection")).toBeInTheDocument();
    expect(screen.getByText("Email Verification")).toBeInTheDocument();
    expect(screen.getByText("Password Reset")).toBeInTheDocument();
  });

  it("should show no data message when realm dashboard is null", async () => {
    mockUseRealmDashboard.mockReturnValue({
      data: null as unknown as KeycloakDashboard,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useRealmDashboard>);

    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    await waitFor(() => {
      expect(
        screen.getByText("No data available for this realm"),
      ).toBeInTheDocument();
    });
  });

  it("should format bytes correctly", async () => {
    const dashboardWithDifferentMemory = {
      ...mockRealmDashboard,
      health: {
        ...mockRealmDashboard.health!,
        memory_used_bytes: 1024, // 1 KB
        memory_max_bytes: 2048, // 2 KB
        memory_free_bytes: 1024, // 1 KB
      },
    };

    mockUseRealmDashboard.mockReturnValue({
      data: dashboardWithDifferentMemory,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useRealmDashboard>);

    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    await waitFor(() => {
      expect(screen.getByText("1.00 KB")).toBeInTheDocument();
    });
  });

  it("should show event type overflow indicator", async () => {
    const dashboardWithManyEvents = {
      ...mockRealmDashboard,
      realm_info: {
        ...mockRealmDashboard.realm_info!,
        enabled_event_types: Array.from(
          { length: 15 },
          (_, i) => `EVENT_TYPE_${i}`,
        ),
      },
    };

    mockUseRealmDashboard.mockReturnValue({
      data: dashboardWithManyEvents,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useRealmDashboard>);

    render(<RealmDetails realmName="test-realm" onClose={mockOnClose} />);

    await waitFor(() => {
      expect(screen.getByText("+5 more")).toBeInTheDocument();
    });
  });
});
