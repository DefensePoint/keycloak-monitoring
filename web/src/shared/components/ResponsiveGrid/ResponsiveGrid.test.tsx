import { describe, it, expect } from "vitest";
import { render, screen } from "@/test-utils";
import { ResponsiveGrid } from "./ResponsiveGrid";
import { Box } from "@mui/material";

describe("ResponsiveGrid", () => {
  it("should render children", () => {
    render(
      <ResponsiveGrid>
        <div>Item 1</div>
        <div>Item 2</div>
        <div>Item 3</div>
      </ResponsiveGrid>,
    );

    expect(screen.getByText("Item 1")).toBeInTheDocument();
    expect(screen.getByText("Item 2")).toBeInTheDocument();
    expect(screen.getByText("Item 3")).toBeInTheDocument();
  });

  it("should use default columns when not provided", () => {
    const { container } = render(
      <ResponsiveGrid>
        <div>Item</div>
      </ResponsiveGrid>,
    );

    const grid = container.firstChild as HTMLElement;
    expect(grid).toHaveStyle({ display: "grid" });
  });

  it("should apply custom columns for xs breakpoint", () => {
    render(
      <ResponsiveGrid columns={{ xs: 2 }}>
        <div>Item 1</div>
        <div>Item 2</div>
      </ResponsiveGrid>,
    );

    expect(screen.getByText("Item 1")).toBeInTheDocument();
    expect(screen.getByText("Item 2")).toBeInTheDocument();
  });

  it("should apply custom columns for all breakpoints", () => {
    render(
      <ResponsiveGrid columns={{ xs: 1, sm: 2, md: 3, lg: 4, xl: 5 }}>
        <div>Item</div>
      </ResponsiveGrid>,
    );

    expect(screen.getByText("Item")).toBeInTheDocument();
  });

  it("should use default spacing of 2", () => {
    const { container } = render(
      <ResponsiveGrid>
        <div>Item</div>
      </ResponsiveGrid>,
    );

    const grid = container.firstChild as HTMLElement;
    expect(grid).toHaveStyle({ gap: "16px" });
  });

  it("should apply custom spacing", () => {
    const { container } = render(
      <ResponsiveGrid spacing={4}>
        <div>Item</div>
      </ResponsiveGrid>,
    );

    const grid = container.firstChild as HTMLElement;
    expect(grid).toHaveStyle({ gap: "32px" });
  });

  it("should apply spacing of 0", () => {
    const { container } = render(
      <ResponsiveGrid spacing={0}>
        <div>Item</div>
      </ResponsiveGrid>,
    );

    const grid = container.firstChild as HTMLElement;
    expect(grid).toHaveStyle({ gap: "0px" });
  });

  it("should apply custom sx props", () => {
    const { container } = render(
      <ResponsiveGrid sx={{ padding: 2 }}>
        <div>Item</div>
      </ResponsiveGrid>,
    );

    const grid = container.firstChild as HTMLElement;
    expect(grid).toHaveStyle({
      padding: "16px",
    });
  });

  it("should render grid layout", () => {
    const { container } = render(
      <ResponsiveGrid>
        <div>Item</div>
      </ResponsiveGrid>,
    );

    const grid = container.firstChild as HTMLElement;
    expect(grid).toHaveStyle({ display: "grid" });
  });

  it("should render multiple items in grid", () => {
    render(
      <ResponsiveGrid columns={{ xs: 2, md: 3 }}>
        <Box>Item 1</Box>
        <Box>Item 2</Box>
        <Box>Item 3</Box>
        <Box>Item 4</Box>
        <Box>Item 5</Box>
      </ResponsiveGrid>,
    );

    expect(screen.getByText("Item 1")).toBeInTheDocument();
    expect(screen.getByText("Item 2")).toBeInTheDocument();
    expect(screen.getByText("Item 3")).toBeInTheDocument();
    expect(screen.getByText("Item 4")).toBeInTheDocument();
    expect(screen.getByText("Item 5")).toBeInTheDocument();
  });

  it("should handle single column layout", () => {
    render(
      <ResponsiveGrid columns={{ xs: 1 }}>
        <div>Item 1</div>
        <div>Item 2</div>
      </ResponsiveGrid>,
    );

    expect(screen.getByText("Item 1")).toBeInTheDocument();
    expect(screen.getByText("Item 2")).toBeInTheDocument();
  });

  it("should handle partial breakpoint configuration", () => {
    render(
      <ResponsiveGrid columns={{ xs: 1, lg: 4 }}>
        <div>Item</div>
      </ResponsiveGrid>,
    );

    expect(screen.getByText("Item")).toBeInTheDocument();
  });

  it("should render empty grid with no children", () => {
    const { container } = render(<ResponsiveGrid>{[]}</ResponsiveGrid>);

    const grid = container.firstChild as HTMLElement;
    expect(grid).toBeInTheDocument();
    expect(grid).toHaveStyle({ display: "grid" });
  });

  it("should handle complex children", () => {
    render(
      <ResponsiveGrid columns={{ xs: 2, md: 3 }}>
        <Box sx={{ padding: 2 }}>
          <div>Complex Item 1</div>
          <span>Nested content</span>
        </Box>
        <Box>Item 2</Box>
      </ResponsiveGrid>,
    );

    expect(screen.getByText("Complex Item 1")).toBeInTheDocument();
    expect(screen.getByText("Nested content")).toBeInTheDocument();
    expect(screen.getByText("Item 2")).toBeInTheDocument();
  });

  it("should combine spacing and sx props", () => {
    const { container } = render(
      <ResponsiveGrid spacing={3} columns={{ xs: 2 }} sx={{ margin: 2 }}>
        <div>Item</div>
      </ResponsiveGrid>,
    );

    const grid = container.firstChild as HTMLElement;
    expect(grid).toHaveStyle({
      gap: "24px",
      margin: "16px",
    });
  });
});
