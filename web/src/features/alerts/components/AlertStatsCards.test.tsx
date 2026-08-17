import { describe, it, expect } from "vitest";
import { render, screen } from "@/test-utils";
import { AlertStatsCards } from "./AlertStatsCards";
import type { AlertStats } from "../types";

describe("AlertStatsCards", () => {
  const mockStats: AlertStats = {
    total_active: 42,
    by_severity: {
      critical: 5,
      error: 10,
      warning: 15,
      info: 12,
    },
    by_source: {
      keycloak: 30,
      operator: 12,
    },
    by_status: {
      active: 25,
      acknowledged: 10,
      resolved: 5,
      ignored: 2,
    },
  };

  it("should render null when stats is null", () => {
    const { container } = render(<AlertStatsCards stats={null} />);
    expect(container.firstChild).toBeNull();
  });

  it("should render total active alerts count", () => {
    render(<AlertStatsCards stats={mockStats} />);
    expect(screen.getByText("Total Active")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();
  });

  it("should render all severity cards", () => {
    render(<AlertStatsCards stats={mockStats} />);

    expect(screen.getByText("Critical")).toBeInTheDocument();
    expect(screen.getByText("Error")).toBeInTheDocument();
    expect(screen.getByText("Warning")).toBeInTheDocument();
    expect(screen.getByText("Info")).toBeInTheDocument();
  });

  it("should display correct counts for each severity level", () => {
    render(<AlertStatsCards stats={mockStats} />);

    expect(screen.getByText("5")).toBeInTheDocument(); // critical
    expect(screen.getByText("10")).toBeInTheDocument(); // error
    expect(screen.getByText("15")).toBeInTheDocument(); // warning
    expect(screen.getByText("12")).toBeInTheDocument(); // info
  });

  it("should display 0 when severity data is missing", () => {
    const statsWithoutSeverity: AlertStats = {
      total_active: 10,
      by_severity: {},
      by_source: {},
      by_status: {},
    };

    render(<AlertStatsCards stats={statsWithoutSeverity} />);

    // All severity counts should show 0
    const zeroElements = screen.getAllByText("0");
    expect(zeroElements.length).toBeGreaterThanOrEqual(4);
  });

  it("should display 0 for total active when undefined", () => {
    const statsWithoutTotal: AlertStats = {
      total_active: undefined as unknown as number,
      by_severity: {
        critical: 5,
        error: 10,
        warning: 15,
        info: 12,
      },
      by_source: {},
      by_status: {},
    };

    render(<AlertStatsCards stats={statsWithoutTotal} />);

    expect(screen.getByText("Total Active")).toBeInTheDocument();
    expect(screen.getByText("0")).toBeInTheDocument();
  });

  it("should handle partial severity data", () => {
    const partialStats: AlertStats = {
      total_active: 20,
      by_severity: {
        critical: 5,
        // error missing
        warning: 10,
        // info missing
      },
      by_source: {},
      by_status: {},
    };

    render(<AlertStatsCards stats={partialStats} />);

    expect(screen.getByText("5")).toBeInTheDocument(); // critical
    expect(screen.getByText("10")).toBeInTheDocument(); // warning
    // Missing severities should show 0
    const zeroElements = screen.getAllByText("0");
    expect(zeroElements.length).toBeGreaterThanOrEqual(2);
  });
});
