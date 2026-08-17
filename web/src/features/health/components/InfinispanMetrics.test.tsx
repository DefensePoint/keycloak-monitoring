import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import { InfinispanMetrics } from "./InfinispanMetrics";
import { useInfinispanMetrics } from "../hooks";

vi.mock("../hooks", () => ({
  useInfinispanMetrics: vi.fn(),
}));

vi.mock("@/shared/utils", () => ({
  formatBytes: (bytes: number) => `${bytes}B`,
  formatNumber: (num: number) => num.toString(),
}));

vi.mock("@/shared/components", () => ({
  AlertBanner: ({ message }: { message: string }) => (
    <div role="alert">{message}</div>
  ),
}));

const mockUseInfinispanMetrics = vi.mocked(useInfinispanMetrics);

describe("InfinispanMetrics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseInfinispanMetrics.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);
  });

  it("should show loading state when fetching metrics", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    const { container } = render(<InfinispanMetrics tenantId="tenant-1" />);

    expect(screen.getByText("Infinispan Cache")).toBeInTheDocument();
    // MUI Skeleton components don't have role="progressbar", check for skeleton elements
    const skeletons = container.querySelectorAll(".MuiSkeleton-root");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("should show error alert when metrics fetch fails", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed to fetch"),
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<InfinispanMetrics tenantId="tenant-1" />);

    expect(screen.getByRole("alert")).toHaveTextContent(
      /Infinispan metrics not available/i,
    );
  });

  it("should display healthy status when metrics are healthy", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: true,
        cluster_size: 3,
        required_minimum_nodes: 2,
        replication_count: 100,
        replication_failures: 0,
        jvm_memory_used_bytes: 1000000,
        jvm_memory_committed_bytes: 2000000,
        jvm_memory_used_percent: 50.0,
        process_cpu_usage: 0.25,
        cache_stats: {},
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<InfinispanMetrics tenantId="tenant-1" />);

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
        jvm_memory_used_bytes: 1800000,
        jvm_memory_committed_bytes: 2000000,
        jvm_memory_used_percent: 90.0,
        process_cpu_usage: 0.85,
        cache_stats: {},
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<InfinispanMetrics tenantId="tenant-1" />);

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
        jvm_memory_used_bytes: 1000000,
        jvm_memory_committed_bytes: 2000000,
        jvm_memory_used_percent: 50.0,
        process_cpu_usage: 0.25,
        cache_stats: {},
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<InfinispanMetrics tenantId="tenant-1" />);

    expect(screen.getByText("Cluster Size")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("Min required: 2")).toBeInTheDocument();
  });

  it("should display JVM memory metrics", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: true,
        cluster_size: 3,
        required_minimum_nodes: 2,
        replication_count: 100,
        replication_failures: 0,
        jvm_memory_used_bytes: 1000000,
        jvm_memory_committed_bytes: 2000000,
        jvm_memory_used_percent: 50.5,
        process_cpu_usage: 0.25,
        cache_stats: {},
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<InfinispanMetrics tenantId="tenant-1" />);

    expect(screen.getByText("JVM Memory")).toBeInTheDocument();
    expect(screen.getByText("50.5%")).toBeInTheDocument();
    expect(screen.getByText(/1000000B \/ 2000000B/i)).toBeInTheDocument();
  });

  it("should display CPU usage metrics", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: true,
        cluster_size: 3,
        required_minimum_nodes: 2,
        replication_count: 100,
        replication_failures: 0,
        jvm_memory_used_bytes: 1000000,
        jvm_memory_committed_bytes: 2000000,
        jvm_memory_used_percent: 50.0,
        process_cpu_usage: 0.35,
        cache_stats: {},
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<InfinispanMetrics tenantId="tenant-1" />);

    expect(screen.getByText("CPU Usage")).toBeInTheDocument();
    expect(screen.getByText("35.0%")).toBeInTheDocument();
  });

  it("should display replication count and failures", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: false,
        cluster_size: 3,
        required_minimum_nodes: 2,
        replication_count: 100,
        replication_failures: 5,
        jvm_memory_used_bytes: 1000000,
        jvm_memory_committed_bytes: 2000000,
        jvm_memory_used_percent: 50.0,
        process_cpu_usage: 0.25,
        cache_stats: {},
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<InfinispanMetrics tenantId="tenant-1" />);

    expect(screen.getByText("Replication")).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
    expect(screen.getByText("5 failures")).toBeInTheDocument();
  });

  it("should display cache statistics when available", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: true,
        cluster_size: 3,
        required_minimum_nodes: 2,
        replication_count: 100,
        replication_failures: 0,
        jvm_memory_used_bytes: 1000000,
        jvm_memory_committed_bytes: 2000000,
        jvm_memory_used_percent: 50.0,
        process_cpu_usage: 0.25,
        cache_stats: {
          sessions: {
            cache_name: "sessions",
            approximate_entries: 500,
            hits: 1000,
            misses: 50,
            hit_ratio: 95.2,
            evictions: 10,
            hit_time_ms: 2.5,
          },
          users: {
            cache_name: "users",
            approximate_entries: 200,
            hits: 800,
            misses: 100,
            hit_ratio: 88.8,
            evictions: 5,
            hit_time_ms: 1.8,
          },
        },
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<InfinispanMetrics tenantId="tenant-1" />);

    expect(screen.getByText("Cache Statistics")).toBeInTheDocument();
    expect(screen.getByText("sessions")).toBeInTheDocument();
    expect(screen.getByText("users")).toBeInTheDocument();
    expect(screen.getByText("Hit Ratio: 95.2%")).toBeInTheDocument();
    expect(screen.getByText("Hit Ratio: 88.8%")).toBeInTheDocument();
  });

  it("should not display cache statistics section when no caches available", () => {
    mockUseInfinispanMetrics.mockReturnValue({
      data: {
        is_healthy: true,
        cluster_size: 3,
        required_minimum_nodes: 2,
        replication_count: 100,
        replication_failures: 0,
        jvm_memory_used_bytes: 1000000,
        jvm_memory_committed_bytes: 2000000,
        jvm_memory_used_percent: 50.0,
        process_cpu_usage: 0.25,
        cache_stats: {},
      },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useInfinispanMetrics>);

    render(<InfinispanMetrics tenantId="tenant-1" />);

    expect(screen.getByText("Healthy")).toBeInTheDocument();
    expect(screen.queryByText("Cache Statistics")).not.toBeInTheDocument();
  });
});
