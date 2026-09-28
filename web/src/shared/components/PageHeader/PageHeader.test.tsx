import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import { PageHeader } from "./PageHeader";
import { Button } from "@mui/material";

// Mock react-router-dom
vi.mock("react-router-dom", () => ({
  useLocation: vi.fn(() => ({ pathname: "/tenant-1/alerts" })),
  useParams: vi.fn(() => ({ tenantId: "tenant-1" })),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

// Mock useTenant
vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(() => ({
    selectedTenant: { tenant_id: "tenant-1", name: "Test Tenant" },
  })),
}));

describe("PageHeader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render title", () => {
    render(<PageHeader title="Configuration Alerts" />);

    expect(screen.getByText("Configuration Alerts")).toBeInTheDocument();
  });

  it("should render title with h3 variant", () => {
    render(<PageHeader title="Test Title" />);

    const title = screen.getByText("Test Title");
    expect(title).toHaveClass("MuiTypography-h3");
  });

  it("should render subtitle when provided", () => {
    render(
      <PageHeader
        title="Alerts"
        subtitle="Monitor and manage configuration issues"
      />,
    );

    expect(
      screen.getByText("Monitor and manage configuration issues"),
    ).toBeInTheDocument();
  });

  it("should not render subtitle when not provided", () => {
    render(<PageHeader title="Alerts" />);

    expect(
      screen.queryByText("Monitor and manage configuration issues"),
    ).not.toBeInTheDocument();
  });

  it("should render children in actions slot", () => {
    render(
      <PageHeader title="Alerts">
        <Button data-testid="action-button">Add Alert</Button>
      </PageHeader>,
    );

    expect(screen.getByTestId("action-button")).toBeInTheDocument();
    expect(screen.getByText("Add Alert")).toBeInTheDocument();
  });

  it("should render multiple children", () => {
    render(
      <PageHeader title="Alerts">
        <Button data-testid="btn-1">Button 1</Button>
        <Button data-testid="btn-2">Button 2</Button>
      </PageHeader>,
    );

    expect(screen.getByTestId("btn-1")).toBeInTheDocument();
    expect(screen.getByTestId("btn-2")).toBeInTheDocument();
  });

  it("should render breadcrumb by default", () => {
    render(<PageHeader title="Alerts" />);

    // Should have breadcrumb navigation
    expect(screen.getByRole("navigation")).toBeInTheDocument();
  });

  it("should hide breadcrumb when showBreadcrumb is false", () => {
    render(<PageHeader title="Alerts" showBreadcrumb={false} />);

    expect(screen.queryByRole("navigation")).not.toBeInTheDocument();
  });

  it("should render tenant name in breadcrumb", () => {
    render(<PageHeader title="Alerts" />);

    expect(screen.getByText("Test Tenant")).toBeInTheDocument();
  });

  it("should render section name in breadcrumb", () => {
    render(<PageHeader title="Configuration Alerts" />);

    expect(screen.getByText("Alerts")).toBeInTheDocument();
  });

  it("should apply correct styling to subtitle", () => {
    render(<PageHeader title="Title" subtitle="Subtitle text" />);

    const subtitle = screen.getByText("Subtitle text");
    expect(subtitle).toHaveClass("MuiTypography-body2");
  });
});
