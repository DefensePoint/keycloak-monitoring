import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ThemeProvider, createTheme } from "@mui/material";
import { OperatorSummaryCards } from "./OperatorSummaryCards";

const theme = createTheme();

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

describe("OperatorSummaryCards", () => {
  const defaultProps = {
    totalOperators: 5,
    totalAlertsHandled: 150,
    totalHours: 42.5,
    avgResponseTime: "2.5min",
  };

  it("should render all summary cards", () => {
    renderWithTheme(<OperatorSummaryCards {...defaultProps} />);

    expect(screen.getByText("Total Operators")).toBeInTheDocument();
    expect(screen.getByText("Total Alerts")).toBeInTheDocument();
    expect(screen.getByText("Total Hours")).toBeInTheDocument();
    expect(screen.getByText("Avg Response")).toBeInTheDocument();
  });

  it("should display total operators count", () => {
    renderWithTheme(<OperatorSummaryCards {...defaultProps} />);

    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("should display total alerts handled count", () => {
    renderWithTheme(<OperatorSummaryCards {...defaultProps} />);

    expect(screen.getByText("150")).toBeInTheDocument();
  });

  it("should display total hours with decimal", () => {
    renderWithTheme(<OperatorSummaryCards {...defaultProps} />);

    expect(screen.getByText("42.5")).toBeInTheDocument();
  });

  it("should display average response time", () => {
    renderWithTheme(<OperatorSummaryCards {...defaultProps} />);

    expect(screen.getByText("2.5min")).toBeInTheDocument();
  });

  it("should render People icon for total operators", () => {
    const { container } = renderWithTheme(
      <OperatorSummaryCards {...defaultProps} />,
    );

    const peopleIcons = container.querySelectorAll(
      'svg[data-testid="PeopleIcon"]',
    );
    expect(peopleIcons.length).toBeGreaterThan(0);
  });

  it("should render Notifications icon for total alerts", () => {
    const { container } = renderWithTheme(
      <OperatorSummaryCards {...defaultProps} />,
    );

    const notificationIcons = container.querySelectorAll(
      'svg[data-testid="NotificationsIcon"]',
    );
    expect(notificationIcons.length).toBeGreaterThan(0);
  });

  it("should render AccessTime icon for total hours", () => {
    const { container } = renderWithTheme(
      <OperatorSummaryCards {...defaultProps} />,
    );

    const timeIcons = container.querySelectorAll(
      'svg[data-testid="AccessTimeIcon"]',
    );
    expect(timeIcons.length).toBeGreaterThan(0);
  });

  it("should render FlashOn icon for avg response", () => {
    const { container } = renderWithTheme(
      <OperatorSummaryCards {...defaultProps} />,
    );

    const flashIcons = container.querySelectorAll(
      'svg[data-testid="FlashOnIcon"]',
    );
    expect(flashIcons.length).toBeGreaterThan(0);
  });

  it("should handle zero operators", () => {
    renderWithTheme(
      <OperatorSummaryCards {...defaultProps} totalOperators={0} />,
    );

    expect(screen.getByText("0")).toBeInTheDocument();
  });

  it("should format total hours with one decimal place", () => {
    renderWithTheme(
      <OperatorSummaryCards {...defaultProps} totalHours={10.123} />,
    );

    expect(screen.getByText("10.1")).toBeInTheDocument();
  });
});
