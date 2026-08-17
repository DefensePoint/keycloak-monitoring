import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { StatsCardGrid, StatsCardItem } from "./StatsCardGrid";
import {
  People,
  Notifications,
  Warning,
  CheckCircle,
} from "@mui/icons-material";

describe("StatsCardGrid", () => {
  const mockItems: StatsCardItem[] = [
    { icon: People, label: "Total Users", value: 1234, color: "#1976d2" },
    { icon: Notifications, label: "Alerts", value: 56, color: "#ed6c02" },
  ];

  it("should render all items", () => {
    render(<StatsCardGrid items={mockItems} />);

    expect(screen.getByText("Total Users")).toBeInTheDocument();
    expect(screen.getByText("Alerts")).toBeInTheDocument();
  });

  it("should render values for each item", () => {
    render(<StatsCardGrid items={mockItems} />);

    expect(screen.getByText(/1[,.]?234/)).toBeInTheDocument();
    expect(screen.getByText("56")).toBeInTheDocument();
  });

  it("should render icons for each item", () => {
    const { container } = render(<StatsCardGrid items={mockItems} />);

    const icons = container.querySelectorAll("svg");
    expect(icons.length).toBeGreaterThanOrEqual(2);
  });

  it("should render with default columns configuration", () => {
    const { container } = render(<StatsCardGrid items={mockItems} />);

    const grid = container.firstChild;
    expect(grid).toHaveStyle({ display: "grid" });
  });

  it("should render footer when provided in item", () => {
    const itemsWithFooter: StatsCardItem[] = [
      {
        icon: People,
        label: "Users",
        value: 100,
        color: "#1976d2",
        footer: <span>Last updated: now</span>,
      },
    ];

    render(<StatsCardGrid items={itemsWithFooter} />);

    expect(screen.getByText("Last updated: now")).toBeInTheDocument();
  });

  it("should handle click on items with onClick", async () => {
    const user = userEvent.setup();
    const mockOnClick = vi.fn();
    const clickableItems: StatsCardItem[] = [
      {
        icon: People,
        label: "Clickable Card",
        value: 50,
        color: "#1976d2",
        onClick: mockOnClick,
      },
    ];

    render(<StatsCardGrid items={clickableItems} />);

    const button = screen.getByRole("button");
    await user.click(button);

    expect(mockOnClick).toHaveBeenCalledTimes(1);
  });

  it("should render with custom spacing", () => {
    const { container } = render(
      <StatsCardGrid items={mockItems} spacing={4} />,
    );

    const grid = container.firstChild;
    expect(grid).toHaveStyle({ gap: "32px" });
  });

  it("should apply custom sx props", () => {
    const { container } = render(
      <StatsCardGrid items={mockItems} sx={{ margin: 2 }} />,
    );

    const grid = container.firstChild;
    expect(grid).toHaveStyle({ margin: "16px" });
  });

  it("should render empty when no items provided", () => {
    const { container } = render(<StatsCardGrid items={[]} />);

    const grid = container.firstChild;
    expect(grid?.childNodes.length).toBe(0);
  });

  it("should render four items correctly", () => {
    const fourItems: StatsCardItem[] = [
      { icon: People, label: "Users", value: 100, color: "#1976d2" },
      { icon: Notifications, label: "Alerts", value: 20, color: "#ed6c02" },
      { icon: Warning, label: "Warnings", value: 5, color: "#ff9800" },
      { icon: CheckCircle, label: "Resolved", value: 15, color: "#2e7d32" },
    ];

    render(<StatsCardGrid items={fourItems} />);

    expect(screen.getByText("Users")).toBeInTheDocument();
    expect(screen.getByText("Alerts")).toBeInTheDocument();
    expect(screen.getByText("Warnings")).toBeInTheDocument();
    expect(screen.getByText("Resolved")).toBeInTheDocument();
  });

  it("should render with string values", () => {
    const stringValueItems: StatsCardItem[] = [
      { icon: People, label: "Status", value: "Active", color: "#1976d2" },
    ];

    render(<StatsCardGrid items={stringValueItems} />);

    expect(screen.getByText("Active")).toBeInTheDocument();
  });
});
