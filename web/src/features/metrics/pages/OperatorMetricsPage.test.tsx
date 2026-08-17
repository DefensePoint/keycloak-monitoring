import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { OperatorMetricsPage } from "./OperatorMetricsPage";
import { useTenant } from "@/shared/context";
import { useOperatorMetrics } from "../hooks";

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/tenant-1/metrics" }),
  useParams: () => ({ tenantId: "tenant-1" }),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
}));

vi.mock("../hooks", () => ({
  useOperatorMetrics: vi.fn(),
}));

vi.mock("material-react-table", () => ({
  MaterialReactTable: ({
    data,
    renderTopToolbarCustomActions,
  }: {
    data: unknown[];
    renderTopToolbarCustomActions: () => React.ReactElement;
  }) => (
    <div data-testid="metrics-table">
      {renderTopToolbarCustomActions()}
      <div>Table with {data.length} operators</div>
    </div>
  ),
}));

vi.mock("@/shared/components", () => ({
  LoadingSkeleton: () => (
    <div data-testid="loading-state">Loading operator metrics...</div>
  ),
  AlertBanner: ({ message }: { message: string }) => (
    <div data-testid="alert-banner">{message}</div>
  ),
}));

vi.mock("../components", () => ({
  OperatorMetricsHeader: () => (
    <div data-testid="operator-header">Metrics Header</div>
  ),
  OperatorDateRangeFilter: ({ onRefresh }: { onRefresh: () => void }) => (
    <button onClick={onRefresh} data-testid="refresh-button">
      Refresh
    </button>
  ),
  OperatorSummaryCards: () => <div data-testid="summary-cards">Summary</div>,
}));

const mockMetrics = [
  {
    operator_email: "op1@example.com",
    operator_name: "Operator 1",
    total_alerts_handled: 10,
    alerts_acknowledged: 5,
    alerts_resolved: 3,
    alerts_ignored: 2,
    critical_alerts_handled: 1,
    avg_response_time: 300,
    total_work_time_hours: 8,
    avg_time_to_acknowledge: 120,
    avg_time_to_resolve: 600,
    avg_time_to_ignore: 180,
    avg_acknowledge_to_resolve_time: 480,
    avg_acknowledge_to_ignore_time: 60,
  },
  {
    operator_email: "op2@example.com",
    operator_name: "Operator 2",
    total_alerts_handled: 15,
    alerts_acknowledged: 8,
    alerts_resolved: 5,
    alerts_ignored: 2,
    critical_alerts_handled: 2,
    avg_response_time: 250,
    total_work_time_hours: 12,
    avg_time_to_acknowledge: 100,
    avg_time_to_resolve: 500,
    avg_time_to_ignore: 150,
    avg_acknowledge_to_resolve_time: 400,
    avg_acknowledge_to_ignore_time: 50,
  },
];

describe("OperatorMetricsPage", () => {
  const mockSelectedTenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
  };

  const mockRefetch = vi.fn();

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
  });

  it("should render loading state when data is loading", () => {
    vi.mocked(useOperatorMetrics).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      refetch: mockRefetch,
    } as ReturnType<typeof useOperatorMetrics>);

    render(<OperatorMetricsPage />);
    expect(screen.getByTestId("loading-state")).toBeInTheDocument();
    expect(screen.getByText("Loading operator metrics...")).toBeInTheDocument();
  });

  it("should load and display operator metrics", async () => {
    vi.mocked(useOperatorMetrics).mockReturnValue({
      data: mockMetrics,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
    } as ReturnType<typeof useOperatorMetrics>);

    render(<OperatorMetricsPage />);

    expect(screen.getByTestId("operator-header")).toBeInTheDocument();
    // There are 2 tables, so we expect 2 elements with this text
    expect(screen.getAllByText("Table with 2 operators")).toHaveLength(2);
  });

  it("should display summary cards when metrics loaded", () => {
    vi.mocked(useOperatorMetrics).mockReturnValue({
      data: mockMetrics,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
    } as ReturnType<typeof useOperatorMetrics>);

    render(<OperatorMetricsPage />);

    expect(screen.getByTestId("summary-cards")).toBeInTheDocument();
  });

  it("should display operator performance table header", () => {
    vi.mocked(useOperatorMetrics).mockReturnValue({
      data: mockMetrics,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
    } as ReturnType<typeof useOperatorMetrics>);

    render(<OperatorMetricsPage />);

    expect(screen.getByText("Operator Performance")).toBeInTheDocument();
  });

  it("should display action timing breakdown table header", () => {
    vi.mocked(useOperatorMetrics).mockReturnValue({
      data: mockMetrics,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
    } as ReturnType<typeof useOperatorMetrics>);

    render(<OperatorMetricsPage />);

    expect(screen.getByText("Action Timing Breakdown")).toBeInTheDocument();
  });

  it("should display metrics explanation in info banner", () => {
    vi.mocked(useOperatorMetrics).mockReturnValue({
      data: mockMetrics,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
    } as ReturnType<typeof useOperatorMetrics>);

    render(<OperatorMetricsPage />);

    // The info banner contains explanation about metrics
    expect(screen.getByText(/Time to X/)).toBeInTheDocument();
    expect(screen.getByText(/Ack → Resolve/)).toBeInTheDocument();
  });

  it("should call refetch when refresh button clicked", async () => {
    const user = userEvent.setup();
    vi.mocked(useOperatorMetrics).mockReturnValue({
      data: mockMetrics,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
    } as ReturnType<typeof useOperatorMetrics>);

    render(<OperatorMetricsPage />);

    const refreshButton = screen.getByTestId("refresh-button");
    await user.click(refreshButton);

    expect(mockRefetch).toHaveBeenCalled();
  });

  it("should display error when fetching metrics fails", () => {
    vi.mocked(useOperatorMetrics).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed to fetch"),
      refetch: mockRefetch,
    } as ReturnType<typeof useOperatorMetrics>);

    render(<OperatorMetricsPage />);

    expect(screen.getByTestId("alert-banner")).toBeInTheDocument();
    expect(screen.getByText("Failed to fetch")).toBeInTheDocument();
  });

  it("should handle empty metrics response", () => {
    vi.mocked(useOperatorMetrics).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      refetch: mockRefetch,
    } as ReturnType<typeof useOperatorMetrics>);

    render(<OperatorMetricsPage />);

    // There are 2 tables, so we expect 2 elements with this text
    expect(screen.getAllByText("Table with 0 operators")).toHaveLength(2);
    // Summary cards should not be shown when no metrics
    expect(screen.queryByTestId("summary-cards")).not.toBeInTheDocument();
  });

  it("should call useOperatorMetrics with correct params", () => {
    vi.mocked(useOperatorMetrics).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      refetch: mockRefetch,
    } as ReturnType<typeof useOperatorMetrics>);

    render(<OperatorMetricsPage />);

    expect(useOperatorMetrics).toHaveBeenCalledWith(
      expect.objectContaining({
        tenantId: "tenant-1",
        startDate: expect.any(String),
        endDate: expect.any(String),
      }),
    );
  });
});
