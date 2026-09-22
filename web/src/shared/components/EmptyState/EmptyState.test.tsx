import { describe, it, expect } from "vitest";
import { render, screen } from "@/test-utils";
import { EmptyState } from "./EmptyState";
import { Button } from "@mui/material";
import { SearchOff } from "@mui/icons-material";

describe("EmptyState", () => {
  it("should render with message", () => {
    render(<EmptyState message="No data available" />);

    expect(screen.getByText("No data available")).toBeInTheDocument();
  });

  it("should render with description", () => {
    render(
      <EmptyState
        message="No events found"
        description="Try adjusting your filters"
      />,
    );

    expect(screen.getByText("No events found")).toBeInTheDocument();
    expect(screen.getByText("Try adjusting your filters")).toBeInTheDocument();
  });

  it("should render without description when not provided", () => {
    render(<EmptyState message="No data" />);

    expect(screen.getByText("No data")).toBeInTheDocument();
  });

  it("should render default InboxOutlined icon", () => {
    const { container } = render(<EmptyState message="Empty" />);

    const icon = container.querySelector("svg");
    expect(icon).toBeInTheDocument();
  });

  it("should render custom icon", () => {
    const { container } = render(
      <EmptyState message="No results" icon={<SearchOff />} />,
    );

    const icon = container.querySelector("svg");
    expect(icon).toBeInTheDocument();
  });

  it("should not render icon when null is provided", () => {
    const { container } = render(<EmptyState message="Empty" icon={null} />);

    const icon = container.querySelector("svg");
    expect(icon).not.toBeInTheDocument();
  });

  it("should render action button", () => {
    render(
      <EmptyState message="No items" action={<Button>Add Item</Button>} />,
    );

    expect(
      screen.getByRole("button", { name: /add item/i }),
    ).toBeInTheDocument();
  });

  it("should not render action when not provided", () => {
    render(<EmptyState message="Empty" />);

    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("should apply default minHeight", () => {
    const { container } = render(<EmptyState message="Empty" />);

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ minHeight: "200px" });
  });

  it("should apply custom minHeight as number", () => {
    const { container } = render(
      <EmptyState message="Empty" minHeight={400} />,
    );

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ minHeight: "400px" });
  });

  it("should apply custom minHeight as string", () => {
    const { container } = render(
      <EmptyState message="Empty" minHeight="50vh" />,
    );

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ minHeight: "50vh" });
  });

  it("should apply custom sx props", () => {
    const { container } = render(
      <EmptyState message="Empty" sx={{ padding: 2 }} />,
    );

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ padding: "16px" });
  });

  it("should render centered layout", () => {
    const { container } = render(<EmptyState message="Empty" />);

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({
      display: "flex",
      flexDirection: "column",
      alignItems: "center",
      justifyContent: "center",
      textAlign: "center",
    });
  });

  it("should render message with body1 variant", () => {
    render(<EmptyState message="No data" />);

    const message = screen.getByText("No data");
    expect(message).toHaveClass("MuiTypography-body1");
  });

  it("should render description with body2 variant", () => {
    render(<EmptyState message="No data" description="Try again later" />);

    const description = screen.getByText("Try again later");
    expect(description).toHaveClass("MuiTypography-body2");
  });

  it("should render with all props combined", () => {
    const { container } = render(
      <EmptyState
        message="No search results"
        description="Try different keywords"
        icon={<SearchOff />}
        action={<Button>Clear Filters</Button>}
        minHeight={300}
        sx={{ padding: 4 }}
      />,
    );

    expect(screen.getByText("No search results")).toBeInTheDocument();
    expect(screen.getByText("Try different keywords")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /clear filters/i }),
    ).toBeInTheDocument();
    expect(container.querySelector("svg")).toBeInTheDocument();

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ minHeight: "300px", padding: "32px" });
  });
});
