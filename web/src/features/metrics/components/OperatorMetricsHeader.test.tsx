import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import { OperatorMetricsHeader } from "./OperatorMetricsHeader";

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/tenant-1/metrics" }),
  useParams: () => ({ tenantId: "tenant-1" }),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useTenant: () => ({
    selectedTenant: { tenant_id: "tenant-1", name: "Test Tenant" },
  }),
}));

describe("OperatorMetricsHeader", () => {
  it("should render header title", () => {
    render(<OperatorMetricsHeader />);

    // "Metrics" appears in breadcrumb and title
    expect(screen.getAllByText("Metrics").length).toBeGreaterThanOrEqual(1);
  });

  it("should render header description", () => {
    render(<OperatorMetricsHeader />);

    expect(
      screen.getByText(
        /Track operator performance, response times, and workload distribution/i,
      ),
    ).toBeInTheDocument();
  });

  it("should render title with correct styling", () => {
    render(<OperatorMetricsHeader />);

    // Get the h3 title specifically
    const title = screen.getByRole("heading", { level: 3, name: "Metrics" });
    expect(title).toHaveClass("MuiTypography-h3");
  });

  it("should render description with correct styling", () => {
    render(<OperatorMetricsHeader />);

    const description = screen.getByText(
      /Track operator performance, response times, and workload distribution/i,
    );
    expect(description).toHaveClass("MuiTypography-body2");
  });
});
