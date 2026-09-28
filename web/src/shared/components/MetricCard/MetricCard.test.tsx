import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { MetricCard } from "./MetricCard";
import { People } from "@mui/icons-material";

describe("MetricCard", () => {
  it("should render label and string value", () => {
    render(<MetricCard label="Active Users" value="1,234" />);

    expect(screen.getByText("Active Users")).toBeInTheDocument();
    expect(screen.getByText("1,234")).toBeInTheDocument();
  });

  it("should render label and number value", () => {
    render(<MetricCard label="Total Sessions" value={5678} />);

    expect(screen.getByText("Total Sessions")).toBeInTheDocument();
    // Number formatting can vary by locale
    expect(screen.getByText(/5[,.]?678/)).toBeInTheDocument();
  });

  it("should format number values with locale string", () => {
    render(<MetricCard label="Events" value={1000000} />);

    // Number formatting can vary by locale
    expect(screen.getByText(/1[,.]000[,.]000/)).toBeInTheDocument();
  });

  it("should render icon when provided", () => {
    const { container } = render(
      <MetricCard label="Users" value={100} icon={<People />} />,
    );

    const icon = container.querySelector("svg");
    expect(icon).toBeInTheDocument();
  });

  it("should not render icon when not provided", () => {
    const { container } = render(<MetricCard label="Users" value={100} />);

    const icon = container.querySelector("svg");
    expect(icon).not.toBeInTheDocument();
  });

  it("should render subtitle when provided", () => {
    render(<MetricCard label="Active" value={567} subtitle="Last 24 hours" />);

    expect(screen.getByText("Last 24 hours")).toBeInTheDocument();
  });

  it("should not render subtitle when not provided", () => {
    render(<MetricCard label="Count" value={100} />);

    expect(screen.queryByText("Last 24 hours")).not.toBeInTheDocument();
  });

  it("should render subtitle with caption variant", () => {
    render(<MetricCard label="Active" value={567} subtitle="Last 24 hours" />);

    const subtitle = screen.getByText("Last 24 hours");
    expect(subtitle).toHaveClass("MuiTypography-caption");
  });

  it("should not be clickable when onClick is not provided", () => {
    const { container } = render(<MetricCard label="Users" value={100} />);

    const cardActionArea = container.querySelector(".MuiCardActionArea-root");
    expect(cardActionArea).not.toBeInTheDocument();
  });

  it("should be clickable when onClick is provided", () => {
    const mockOnClick = vi.fn();
    const { container } = render(
      <MetricCard label="Users" value={100} onClick={mockOnClick} />,
    );

    const cardActionArea = container.querySelector(".MuiCardActionArea-root");
    expect(cardActionArea).toBeInTheDocument();
  });

  it("should call onClick when card is clicked", async () => {
    const user = userEvent.setup();
    const mockOnClick = vi.fn();
    render(<MetricCard label="Events" value={1234} onClick={mockOnClick} />);

    const cardActionArea = screen.getByRole("button");
    await user.click(cardActionArea);
    expect(mockOnClick).toHaveBeenCalledTimes(1);
  });

  it("should apply custom sx props", () => {
    const { container } = render(
      <MetricCard label="Test" value={100} sx={{ margin: 2 }} />,
    );

    const card = container.querySelector(".MuiCard-root");
    expect(card).toHaveStyle({ margin: "16px" });
  });

  it("should render value with h3 variant", () => {
    render(<MetricCard label="Count" value={100} />);

    const value = screen.getByText("100");
    expect(value).toHaveClass("MuiTypography-h3");
  });

  it("should render label with overline variant", () => {
    render(<MetricCard label="Total Users" value={100} />);

    const label = screen.getByText("Total Users");
    expect(label).toHaveClass("MuiTypography-overline");
  });

  it("should render label with uppercase styling", () => {
    render(<MetricCard label="Active Sessions" value={50} />);

    const label = screen.getByText("Active Sessions");
    expect(label).toHaveClass("MuiTypography-overline");
  });

  it("should align text left when no icon is provided", () => {
    const { container } = render(<MetricCard label="Users" value={100} />);

    const textBox = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(textBox).toHaveStyle({ textAlign: "left" });
  });

  it("should align text right when icon is provided", () => {
    render(<MetricCard label="Users" value={100} icon={<People />} />);

    expect(screen.getByText("Users")).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
  });

  it("should handle zero as value", () => {
    render(<MetricCard label="Errors" value={0} />);

    expect(screen.getByText("0")).toBeInTheDocument();
  });

  it("should handle all props combined", () => {
    const mockOnClick = vi.fn();
    const { container } = render(
      <MetricCard
        label="Active Users"
        value={1234}
        icon={<People />}
        subtitle="Last hour"
        onClick={mockOnClick}
        sx={{ margin: 2 }}
      />,
    );

    expect(screen.getByText("Active Users")).toBeInTheDocument();
    // Number formatting can vary by locale
    expect(screen.getByText(/1[,.]?234/)).toBeInTheDocument();
    expect(screen.getByText("Last hour")).toBeInTheDocument();
    expect(container.querySelector("svg")).toBeInTheDocument();
    expect(
      container.querySelector(".MuiCardActionArea-root"),
    ).toBeInTheDocument();

    const card = container.querySelector(".MuiCard-root");
    expect(card).toHaveStyle({ margin: "16px" });
  });
});
