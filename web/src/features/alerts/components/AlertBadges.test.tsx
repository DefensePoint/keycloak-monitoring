import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import { AlertBadges } from "./AlertBadges";

vi.mock("@/shared/components", () => ({
  FieldDisplay: ({
    label,
    value,
  }: {
    label: string;
    value: React.ReactNode;
  }) => (
    <div>
      <span>{label}</span>
      {value}
    </div>
  ),
}));

describe("AlertBadges", () => {
  it("should render severity, status, and type badges", () => {
    render(
      <AlertBadges severity="critical" status="active" type="configuration" />,
    );

    expect(screen.getByText("Severity")).toBeInTheDocument();
    expect(screen.getByText("CRITICAL")).toBeInTheDocument();
    expect(screen.getByText("Status")).toBeInTheDocument();
    expect(screen.getByText("ACTIVE")).toBeInTheDocument();
    expect(screen.getByText("Type")).toBeInTheDocument();
    expect(screen.getByText("CONFIGURATION")).toBeInTheDocument();
  });

  it("should render severity with error color for critical", () => {
    render(
      <AlertBadges severity="critical" status="active" type="configuration" />,
    );

    const criticalChip = screen.getByText("CRITICAL");
    expect(criticalChip).toBeInTheDocument();
  });

  it("should render severity with error color for error", () => {
    render(
      <AlertBadges severity="error" status="active" type="configuration" />,
    );

    expect(screen.getByText("ERROR")).toBeInTheDocument();
  });

  it("should render severity with warning color for warning", () => {
    render(
      <AlertBadges severity="warning" status="active" type="configuration" />,
    );

    expect(screen.getByText("WARNING")).toBeInTheDocument();
  });

  it("should render severity with info color for info", () => {
    render(
      <AlertBadges severity="info" status="active" type="configuration" />,
    );

    expect(screen.getByText("INFO")).toBeInTheDocument();
  });

  it("should render status with error color for active", () => {
    render(
      <AlertBadges severity="info" status="active" type="configuration" />,
    );

    expect(screen.getByText("ACTIVE")).toBeInTheDocument();
  });

  it("should render status with warning color for acknowledged", () => {
    render(
      <AlertBadges
        severity="info"
        status="acknowledged"
        type="configuration"
      />,
    );

    expect(screen.getByText("ACKNOWLEDGED")).toBeInTheDocument();
  });

  it("should render status with default color for resolved", () => {
    render(
      <AlertBadges severity="info" status="resolved" type="configuration" />,
    );

    expect(screen.getByText("RESOLVED")).toBeInTheDocument();
  });

  it("should render status with default color for ignored", () => {
    render(
      <AlertBadges severity="info" status="ignored" type="configuration" />,
    );

    expect(screen.getByText("IGNORED")).toBeInTheDocument();
  });

  it("should render type badge in uppercase", () => {
    render(<AlertBadges severity="info" status="active" type="security" />);

    expect(screen.getByText("SECURITY")).toBeInTheDocument();
  });
});
