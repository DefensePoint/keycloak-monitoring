import { describe, it, expect } from "vitest";
import { render, screen } from "@/test-utils";
import { FilterCard } from "./FilterCard";
import { Typography, TextField } from "@mui/material";

describe("FilterCard", () => {
  it("should render children content", () => {
    render(
      <FilterCard>
        <TextField label="Search" data-testid="search-field" />
      </FilterCard>,
    );

    expect(screen.getByTestId("search-field")).toBeInTheDocument();
  });

  it("should render footer when provided", () => {
    render(
      <FilterCard footer={<Typography>Showing 10 of 50 items</Typography>}>
        <div>Filters</div>
      </FilterCard>,
    );

    expect(screen.getByText("Showing 10 of 50 items")).toBeInTheDocument();
  });

  it("should not render footer section when not provided", () => {
    const { container } = render(
      <FilterCard>
        <div>Filters only</div>
      </FilterCard>,
    );

    expect(screen.getByText("Filters only")).toBeInTheDocument();
    // Footer box should not exist
    const footerBox = container.querySelectorAll(".MuiBox-root");
    expect(footerBox.length).toBe(0);
  });

  it("should apply custom sx props", () => {
    const { container } = render(
      <FilterCard sx={{ margin: 2 }}>
        <div>Content</div>
      </FilterCard>,
    );

    const card = container.querySelector(".MuiCard-root");
    expect(card).toHaveStyle({ margin: "16px" });
  });

  it("should render multiple children", () => {
    render(
      <FilterCard>
        <TextField label="Field 1" data-testid="field-1" />
        <TextField label="Field 2" data-testid="field-2" />
        <TextField label="Field 3" data-testid="field-3" />
      </FilterCard>,
    );

    expect(screen.getByTestId("field-1")).toBeInTheDocument();
    expect(screen.getByTestId("field-2")).toBeInTheDocument();
    expect(screen.getByTestId("field-3")).toBeInTheDocument();
  });

  it("should render complex footer content", () => {
    render(
      <FilterCard
        footer={
          <>
            <Typography>Results: 10</Typography>
            <button>Clear All</button>
          </>
        }
      >
        <div>Filters</div>
      </FilterCard>,
    );

    expect(screen.getByText("Results: 10")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Clear All" }),
    ).toBeInTheDocument();
  });
});
