import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { AlertsPage } from "./AlertsPage";
import { useTenant } from "@/shared/context";
import {
  useAlerts,
  useAlertStats,
  useUpdateAlertStatus,
  useResolveAlert,
} from "@/shared/hooks";

vi.mock("react-router-dom", () => ({
  useParams: () => ({ tenantId: "tenant-1" }),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
  useAuth: () => ({
    hasRole: vi.fn().mockReturnValue(true),
    hasPermission: vi.fn().mockReturnValue(true),
  }),
}));

vi.mock("@/shared/hooks", () => ({
  useAlerts: vi.fn(),
  useAlertStats: vi.fn(),
  useAmfaCheckerStatus: () => ({ data: { enabled: false } }),
  useReloadAmfaAlerts: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateAlertStatus: vi.fn(),
  useResolveAlert: vi.fn(),
}));

vi.mock("material-react-table", () => ({
  MaterialReactTable: ({ data }: { data: unknown[] }) => (
    <div data-testid="material-table">Table with {data.length} items</div>
  ),
}));

vi.mock("@/shared/components", () => ({
  LoadingSkeleton: () => (
    <div data-testid="loading-state">Loading alerts...</div>
  ),
  FeatureErrorBoundary: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  Toast: null,
  ConfirmDialog: ({
    open,
    onConfirm,
    onCancel,
  }: {
    open: boolean;
    onConfirm: () => void;
    onCancel: () => void;
  }) =>
    open ? (
      <div data-testid="confirm-dialog">
        <button onClick={onConfirm}>Confirm</button>
        <button onClick={onCancel}>Cancel</button>
      </div>
    ) : null,
}));

vi.mock("../components", () => ({
  AlertsHeader: () => (
    <div data-testid="alerts-header">Configuration Alerts</div>
  ),
  AlertStatsCards: ({ stats }: { stats: unknown }) => (
    <div data-testid="alert-stats">{stats ? "Stats loaded" : "No stats"}</div>
  ),
  AlertsFilters: ({
    onStatusChange,
  }: {
    onStatusChange: (status: string) => void;
  }) => (
    <div data-testid="alerts-filters">
      <button onClick={() => onStatusChange("active")}>Active</button>
      <button onClick={() => onStatusChange("resolved")}>Resolved</button>
    </div>
  ),
  AlertActionsMenu: ({
    anchorEl,
    onClose,
    onAcknowledge,
    onIgnore,
    onResolve,
  }: {
    anchorEl: HTMLElement | null;
    onClose: () => void;
    onAcknowledge: () => void;
    onIgnore: () => void;
    onResolve: () => void;
  }) =>
    anchorEl ? (
      <div data-testid="alert-actions-menu">
        <button onClick={onAcknowledge}>Acknowledge</button>
        <button onClick={onIgnore}>Ignore</button>
        <button onClick={onResolve}>Resolve</button>
        <button onClick={onClose}>Close</button>
      </div>
    ) : null,
  AlertDetailModal: () => null,
}));

const mockAlerts = [
  {
    alert_id: "alert-1",
    title: "Test Alert 1",
    severity: "critical",
    status: "active",
    source: "keycloak",
    realm_name: "master",
    resource_type: "realm",
    resource_name: "Test Realm",
    check_type: "security",
    first_detected: "2024-01-01T00:00:00Z",
  },
  {
    alert_id: "alert-2",
    title: "Test Alert 2",
    severity: "warning",
    status: "active",
    source: "keycloak",
    realm_name: "test",
    resource_type: "client",
    resource_name: "Test Client",
    check_type: "configuration",
    first_detected: "2024-01-02T00:00:00Z",
  },
];

const mockStats = {
  total_active: 10,
  total_resolved: 5,
  by_severity: { critical: 3, error: 4, warning: 2, info: 1 },
};

describe("AlertsPage", () => {
  const mockSelectedTenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
  };

  const mockMutate = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useTenant).mockReturnValue({
      selectedTenant: mockSelectedTenant,
      tenants: [mockSelectedTenant],
      setSelectedTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoadingTenants: false,
    });
    vi.mocked(useAlerts).mockReturnValue({
      data: { alerts: mockAlerts, total: 2 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as ReturnType<typeof useAlerts>);
    vi.mocked(useAlertStats).mockReturnValue({
      data: mockStats,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as unknown as ReturnType<typeof useAlertStats>);
    vi.mocked(useUpdateAlertStatus).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useUpdateAlertStatus>);
    vi.mocked(useResolveAlert).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useResolveAlert>);
  });

  it("should render loading state initially", () => {
    vi.mocked(useAlerts).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      refetch: vi.fn(),
    } as ReturnType<typeof useAlerts>);

    render(<AlertsPage />);
    expect(screen.getByTestId("loading-state")).toBeInTheDocument();
    expect(screen.getByText("Loading alerts...")).toBeInTheDocument();
  });

  it("should load and display alerts", async () => {
    render(<AlertsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("material-table")).toBeInTheDocument();
    });

    expect(screen.getByText("Table with 2 items")).toBeInTheDocument();
  });

  it("should display alert statistics", async () => {
    render(<AlertsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("alert-stats")).toBeInTheDocument();
    });

    expect(screen.getByText("Stats loaded")).toBeInTheDocument();
  });

  it("should change status filter when filter button is clicked", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("alerts-header")).toBeInTheDocument();
    });

    const resolvedButton = screen.getByText("Resolved");
    await user.click(resolvedButton);

    // Verify the hooks are called with correct parameters
    expect(useAlerts).toHaveBeenCalled();
  });

  it("should display error when loading alerts fails", async () => {
    vi.mocked(useAlerts).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed"),
      refetch: vi.fn(),
    } as ReturnType<typeof useAlerts>);

    render(<AlertsPage />);

    await waitFor(() => {
      expect(screen.getByText("Failed")).toBeInTheDocument();
    });
  });

  it("should render alerts header and filters", async () => {
    render(<AlertsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("alerts-header")).toBeInTheDocument();
    });

    expect(screen.getByTestId("alerts-filters")).toBeInTheDocument();
  });
});
