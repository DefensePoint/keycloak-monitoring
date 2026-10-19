import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { HealthPage } from "./HealthPage";
import { useTenant } from "@/shared/context";

const mockUseKeycloakDashboard = vi.fn();

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/tenant-1/health" }),
  useParams: () => ({ tenantId: "tenant-1" }),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/hooks", () => ({
  useKeycloakDashboard: () => mockUseKeycloakDashboard(),
}));

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
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
    <div data-testid="loading-state">Loading health status...</div>
  ),
  EmptyState: ({ message }: { message: string }) => (
    <div data-testid="empty-state">{message}</div>
  ),
  ErrorState: ({
    title,
    error,
    onRetry,
  }: {
    title?: string;
    error: Error | string | null;
    onRetry?: () => void;
  }) => (
    <div data-testid="error-state">
      <div>{title}</div>
      <div>{error instanceof Error ? error.message : error}</div>
      <button onClick={onRetry}>Try Again</button>
    </div>
  ),
  AlertBanner: ({ title, message }: { title?: string; message: string }) => (
    <div data-testid="alert-banner">
      <div>{title}</div>
      <div>{message}</div>
    </div>
  ),
  SectionCard: ({
    title,
    children,
  }: {
    title: string;
    children?: React.ReactNode;
  }) => (
    <div data-testid={`section-${title.toLowerCase().replace(/\s+/g, "-")}`}>
      <div>{title}</div>
      {children}
    </div>
  ),
  FeatureErrorBoundary: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

vi.mock("@/shared/utils", () => ({
  formatBytes: (bytes: number) => `${bytes}B`,
}));

vi.mock("../components/HighAvailability", () => ({
  HighAvailability: () => (
    <div data-testid="high-availability">HA Component</div>
  ),
}));

vi.mock("../components/InfinispanMetrics", () => ({
  InfinispanMetrics: () => (
    <div data-testid="infinispan-metrics">Infinispan</div>
  ),
}));

const mockKeycloakDashboard = {
  health: {
    status: "UP",
    response_time_ms: 15,
    memory_used_bytes: 1073741824,
    memory_max_bytes: 2147483648,
    memory_free_bytes: 1073741824,
    uptime_millis: 86400000,
    server_version: "23.0.1",
  },
  version_info: {
    latest_keycloak_version: "24.0.0",
    is_keycloak_outdated: true,
    has_security_updates: true,
    keycloak_release_url: "https://github.com/keycloak/keycloak/releases",
  },
};

describe("HealthPage", () => {
  const mockSelectedTenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
    infinispan_enabled: true,
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
      data: mockKeycloakDashboard,
      isLoading: false,
      isError: false,
    });
  });

  it("should render loading state initially", () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: null,
      isLoading: true,
      isError: false,
    });

    render(<HealthPage />);
    expect(screen.getByTestId("loading-state")).toBeInTheDocument();
    expect(screen.getByText("Loading health status...")).toBeInTheDocument();
  });

  it("should display system health header", async () => {
    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText("System Health")).toBeInTheDocument();
    });

    expect(
      screen.getByText("Monitor system health and performance metrics"),
    ).toBeInTheDocument();
  });

  it("should display keycloak server status", async () => {
    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText("Keycloak Server Status")).toBeInTheDocument();
    });

    expect(screen.getByText("UP")).toBeInTheDocument();
    expect(screen.getByText("Response Time")).toBeInTheDocument();
    expect(screen.getByText("15ms")).toBeInTheDocument();
  });

  it("should display server version", async () => {
    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText("23.0.1")).toBeInTheDocument();
    });
  });

  it("should show update warning when outdated", async () => {
    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText(/Security Update Required/)).toBeInTheDocument();
    });

    expect(screen.getByText(/24\.0\.0/)).toBeInTheDocument();
  });

  it("should display memory usage", async () => {
    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText("Memory Usage")).toBeInTheDocument();
    });

    // There are multiple elements with "1073741824B" (Memory Used and Memory Free)
    const memoryElements = screen.getAllByText("1073741824B");
    expect(memoryElements.length).toBeGreaterThan(0);
    expect(screen.getByText("2147483648B")).toBeInTheDocument();
  });

  it("should display memory usage percentage", async () => {
    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText("50.0%")).toBeInTheDocument();
    });
  });

  it("should display uptime", async () => {
    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText("Uptime")).toBeInTheDocument();
    });

    // 86400000ms = 1 day
    expect(screen.getByText("1")).toBeInTheDocument(); // Days
  });

  it("should display high availability component when infinispan enabled", async () => {
    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByTestId("high-availability")).toBeInTheDocument();
    });
  });

  it("should display infinispan metrics when enabled", async () => {
    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByTestId("infinispan-metrics")).toBeInTheDocument();
    });
  });

  it("should not display infinispan components when disabled", async () => {
    vi.mocked(useTenant).mockReturnValue({
      selectedTenant: { ...mockSelectedTenant, infinispan_enabled: false },
      tenants: [mockSelectedTenant],
      selectTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoading: false,
      error: null,
    });

    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText("System Health")).toBeInTheDocument();
    });

    expect(screen.queryByTestId("infinispan-metrics")).not.toBeInTheDocument();
  });

  it("should show empty state when no health data", async () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: null,
      isLoading: false,
      isError: false,
    });

    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByTestId("empty-state")).toBeInTheDocument();
    });

    expect(screen.getByText("No health data available")).toBeInTheDocument();
  });

  it("should display error message when present", async () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: {
        health: {
          ...mockKeycloakDashboard.health,
          error_message: "Connection timeout",
        },
      },
      isLoading: false,
      isError: false,
    });

    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText("Error")).toBeInTheDocument();
    });

    expect(screen.getByText("Connection timeout")).toBeInTheDocument();
  });

  it("should show an update warning for an outdated Keycloak", async () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: {
        health: {
          ...mockKeycloakDashboard.health,
          server_version: "23.0.1",
        },
        version_info: {
          ...mockKeycloakDashboard.version_info,
          is_keycloak_outdated: true,
          has_security_updates: false,
        },
      },
      isLoading: false,
      isError: false,
    });

    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText(/Update Available/)).toBeInTheDocument();
    });
  });

  // A failed first load used to render the same "No health data available" as
  // a tenant that simply had nothing to report.
  it("shows a failed first load as an error, not as no data", async () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: null,
      isLoading: false,
      isError: true,
      error: new Error("HTTP 500"),
      dataUpdatedAt: 0,
      refetch: vi.fn(),
    });

    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByTestId("error-state")).toBeInTheDocument();
    });

    expect(screen.getByText("Failed to load health data")).toBeInTheDocument();
    expect(
      screen.queryByText("No health data available"),
    ).not.toBeInTheDocument();
  });

  // TanStack Query clears isError on every refetch of a query that never
  // loaded, so each retry of a failed first load swapped the error for the
  // loading skeleton. errorUpdatedAt still says the last request failed.
  it("keeps a failed first load as an error while it is retried", async () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: undefined,
      error: null,
      isError: false,
      isLoading: true,
      isFetching: true,
      status: "pending",
      dataUpdatedAt: 0,
      errorUpdatedAt: 1_000,
      refetch: vi.fn(),
    });

    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByTestId("error-state")).toBeInTheDocument();
    });
    expect(screen.getByText("Failed to load health data")).toBeInTheDocument();
  });

  // Server error text is for the logs: it can be unclear and can reveal
  // internal detail.
  it("shows a fixed message, not the server's error text, when the first load fails", async () => {
    const refetch = vi.fn();
    mockUseKeycloakDashboard.mockReturnValue({
      data: null,
      isLoading: false,
      isError: true,
      error: new Error("injected failure"),
      dataUpdatedAt: 0,
      refetch,
    });

    const { rerender } = render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByTestId("error-state")).toBeInTheDocument();
    });
    expect(
      screen.getByText("The health data could not be loaded. Try again."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/injected failure/)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Try Again" }));
    expect(refetch).toHaveBeenCalledTimes(1);

    mockUseKeycloakDashboard.mockReturnValue({
      data: mockKeycloakDashboard,
      isLoading: false,
      isError: false,
      error: null,
      dataUpdatedAt: Date.now(),
      refetch,
    });
    rerender(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText("UP")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("error-state")).not.toBeInTheDocument();
  });

  // The page refreshes every five seconds and keeps the last answer. A failed
  // refresh used to leave a green "UP" behind with no sign of its age.
  it("marks the health data as out of date when a refresh fails", async () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: mockKeycloakDashboard,
      isLoading: false,
      isError: true,
      error: new Error("HTTP 500"),
      dataUpdatedAt: new Date("2026-10-12T08:30:00Z").getTime(),
      refetch: vi.fn(),
    });

    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByTestId("alert-banner")).toBeInTheDocument();
    });

    expect(screen.getByText("Health data is out of date")).toBeInTheDocument();
    expect(
      screen.getByText(/The last refresh failed\. Showing the answer from /),
    ).toBeInTheDocument();

    const chip = screen.getByText("UP").closest(".MuiChip-root");
    expect(chip?.className).toContain("colorWarning");
    expect(chip?.className).not.toContain("colorSuccess");
  });

  it("keeps a healthy status green while the request answers", async () => {
    render(<HealthPage />);

    await waitFor(() => {
      expect(screen.getByText("UP")).toBeInTheDocument();
    });

    const chip = screen.getByText("UP").closest(".MuiChip-root");
    expect(chip?.className).toContain("colorSuccess");
    expect(screen.queryByTestId("alert-banner")).not.toBeInTheDocument();
  });
});
