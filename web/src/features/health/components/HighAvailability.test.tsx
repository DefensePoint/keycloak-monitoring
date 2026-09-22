import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import { HighAvailability } from "./HighAvailability";
import { useInfinispanMetrics } from "../hooks";

vi.mock("../hooks", () => ({
  useInfinispanMetrics: vi.fn(),
}));

vi.mock("@/shared/utils", () => ({
  formatNumber: (num: number) => num.toString(),
}));

const mockUseInfinispanMetrics = vi.mocked(useInfinispanMetrics);

describe("HighAvailability", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseInfinispanMetrics.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);
  });

  it("should show informational message when InfiniSpan is not enabled", () => {
    render(<HighAvailability tenantId="tenant-1" infinispanEnabled={false} />);

    expect(screen.getByText("High Availability")).toBeInTheDocument();
    expect(
      screen.getByText(/High Availability features are not configured/i),
    ).toBeInTheDocument();
  });

  it("should show InfiniSpan benefits when not enabled", () => {
    render(<HighAvailability tenantId="tenant-1" infinispanEnabled={false} />);

    expect(
      screen.getByText(
        /InfiniSpan Clustering: Recommended for High Availability/i,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(/Zero Downtime:/i)).toBeInTheDocument();
    expect(screen.getByText(/Session Continuity:/i)).toBeInTheDocument();
    expect(screen.getByText(/Load Distribution:/i)).toBeInTheDocument();
    expect(screen.getByText(/Data Redundancy:/i)).toBeInTheDocument();
  });

  it("should show loading state when fetching metrics", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    const { container } = render(
      <HighAvailability tenantId="tenant-1" infinispanEnabled={true} />,
    );

    // MUI Skeleton components don't have role="progressbar", check for skeleton elements
    const skeletons = container.querySelectorAll(".MuiSkeleton-root");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("should show error state when metrics fetch fails", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed to fetch"),
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<HighAvailability tenantId="tenant-1" infinispanEnabled={true} />);

    expect(
      screen.getByText(/InfiniSpan Metrics Unavailable/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/Possible Issues:/i)).toBeInTheDocument();
  });

  it("should display healthy status when metrics are healthy", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: true,
        cluster_size: 3,
        required_minimum_nodes: 2,
        replication_count: 100,
        replication_failures: 0,
        average_replication_time_ms: 5.2,
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<HighAvailability tenantId="tenant-1" infinispanEnabled={true} />);

    expect(screen.getByText("Healthy")).toBeInTheDocument();
  });

  it("should display degraded status when metrics are unhealthy", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: false,
        cluster_size: 1,
        required_minimum_nodes: 2,
        replication_count: 50,
        replication_failures: 10,
        average_replication_time_ms: 25.5,
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<HighAvailability tenantId="tenant-1" infinispanEnabled={true} />);

    expect(screen.getByText("Degraded")).toBeInTheDocument();
  });

  it("should display cluster metrics", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: true,
        cluster_size: 3,
        required_minimum_nodes: 2,
        replication_count: 100,
        replication_failures: 0,
        average_replication_time_ms: 5.2,
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<HighAvailability tenantId="tenant-1" infinispanEnabled={true} />);

    expect(screen.getByText("Cluster Overview")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("Min required: 2")).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
    expect(screen.getByText("5.2ms")).toBeInTheDocument();
  });

  it("should show warning when cluster size is below minimum", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: false,
        cluster_size: 1,
        required_minimum_nodes: 2,
        replication_count: 50,
        replication_failures: 0,
        average_replication_time_ms: 5.2,
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<HighAvailability tenantId="tenant-1" infinispanEnabled={true} />);

    expect(screen.getByText(/Below minimum threshold/i)).toBeInTheDocument();
  });

  it("should show warning when replication failures occur", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: false,
        cluster_size: 3,
        required_minimum_nodes: 2,
        replication_count: 100,
        replication_failures: 5,
        average_replication_time_ms: 5.2,
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<HighAvailability tenantId="tenant-1" infinispanEnabled={true} />);

    expect(screen.getByText("5 failures")).toBeInTheDocument();
  });

  it("should display failover status based on cluster size", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: true,
        cluster_size: 3,
        required_minimum_nodes: 2,
        replication_count: 100,
        replication_failures: 0,
        average_replication_time_ms: 5.2,
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<HighAvailability tenantId="tenant-1" infinispanEnabled={true} />);

    expect(screen.getByText("Ready")).toBeInTheDocument();
  });

  it("should show limited failover when cluster size is inadequate", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: false,
        cluster_size: 1,
        required_minimum_nodes: 2,
        replication_count: 50,
        replication_failures: 0,
        average_replication_time_ms: 5.2,
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<HighAvailability tenantId="tenant-1" infinispanEnabled={true} />);

    expect(screen.getByText("Limited")).toBeInTheDocument();
  });
});
