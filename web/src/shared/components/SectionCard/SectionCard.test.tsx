import { describe, it, expect } from "vitest";
import { render, screen } from "@/test-utils";
import { SectionCard } from "./SectionCard";
import { Button, Typography } from "@mui/material";

describe("SectionCard", () => {
  it("should render title and children", () => {
    render(
      <SectionCard title="Test Section">
        <Typography>Content goes here</Typography>
      </SectionCard>,
    );

    expect(screen.getByText("Test Section")).toBeInTheDocument();
    expect(screen.getByText("Content goes here")).toBeInTheDocument();
  });

  it("should render subtitle when provided", () => {
    render(
      <SectionCard title="System Health" subtitle="Last 24 hours">
        <div>Content</div>
      </SectionCard>,
    );

    expect(screen.getByText("System Health")).toBeInTheDocument();
    expect(screen.getByText("Last 24 hours")).toBeInTheDocument();
  });

  it("should not render subtitle when not provided", () => {
    render(
      <SectionCard title="Test">
        <div>Content</div>
      </SectionCard>,
    );

    expect(screen.queryByText("Last 24 hours")).not.toBeInTheDocument();
  });

  it("should render action button when provided", () => {
    render(
      <SectionCard
        title="Recent Activity"
        action={<Button size="small">View All</Button>}
      >
        <div>Content</div>
      </SectionCard>,
    );

    expect(
      screen.getByRole("button", { name: /view all/i }),
    ).toBeInTheDocument();
  });

  it("should not render action when not provided", () => {
    render(
      <SectionCard title="Test">
        <div>Content</div>
      </SectionCard>,
    );

    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("should render title with uppercase styling", () => {
    render(
      <SectionCard title="Test Title">
        <div>Content</div>
      </SectionCard>,
    );

    const title = screen.getByText("Test Title");
    expect(title).toHaveStyle({ textTransform: "uppercase" });
  });

  it("should render title with h6 variant", () => {
    render(
      <SectionCard title="Test">
        <div>Content</div>
      </SectionCard>,
    );

    const title = screen.getByText("Test");
    expect(title).toHaveClass("MuiTypography-h6");
  });

  it("should render subtitle with body2 variant", () => {
    render(
      <SectionCard title="Test" subtitle="Subtitle text">
        <div>Content</div>
      </SectionCard>,
    );

    const subtitle = screen.getByText("Subtitle text");
    expect(subtitle).toHaveClass("MuiTypography-body2");
  });

  it("should apply custom sx props", () => {
    const { container } = render(
      <SectionCard title="Test" sx={{ margin: 2 }}>
        <div>Content</div>
      </SectionCard>,
    );

    const card = container.querySelector(".MuiCard-root");
    expect(card).toHaveStyle({
      margin: "16px",
    });
  });

  it("should render header with border bottom", () => {
    const { container } = render(
      <SectionCard title="Test">
        <div>Content</div>
      </SectionCard>,
    );

    // Find the header box
    const boxes = container.querySelectorAll(".MuiBox-root");
    const headerBox = Array.from(boxes).find((box) =>
      box.textContent?.includes("Test"),
    );

    expect(headerBox).toBeDefined();
  });

  it("should render with flexbox header layout", () => {
    const { container } = render(
      <SectionCard title="Test" action={<Button>Action</Button>}>
        <div>Content</div>
      </SectionCard>,
    );

    const boxes = container.querySelectorAll(".MuiBox-root");
    const headerBox = Array.from(boxes).find((box) =>
      box.textContent?.includes("Test"),
    );

    expect(headerBox).toHaveStyle({
      display: "flex",
      alignItems: "flex-start",
      justifyContent: "space-between",
    });
  });

  it("should render multiple children", () => {
    render(
      <SectionCard title="Test">
        <Typography>First paragraph</Typography>
        <Typography>Second paragraph</Typography>
        <Button>Action Button</Button>
      </SectionCard>,
    );

    expect(screen.getByText("First paragraph")).toBeInTheDocument();
    expect(screen.getByText("Second paragraph")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /action button/i }),
    ).toBeInTheDocument();
  });

  it("should render complex children", () => {
    render(
      <SectionCard title="User List">
        <div>
          <ul>
            <li>User 1</li>
            <li>User 2</li>
            <li>User 3</li>
          </ul>
        </div>
      </SectionCard>,
    );

    expect(screen.getByText("User 1")).toBeInTheDocument();
    expect(screen.getByText("User 2")).toBeInTheDocument();
    expect(screen.getByText("User 3")).toBeInTheDocument();
  });

  it("should render with all props combined", () => {
    const { container } = render(
      <SectionCard
        title="System Metrics"
        subtitle="Real-time data"
        action={<Button size="small">Refresh</Button>}
        sx={{ margin: 2 }}
      >
        <Typography>Metric data goes here</Typography>
      </SectionCard>,
    );

    expect(screen.getByText("System Metrics")).toBeInTheDocument();
    expect(screen.getByText("Real-time data")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /refresh/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("Metric data goes here")).toBeInTheDocument();

    const card = container.querySelector(".MuiCard-root");
    expect(card).toHaveStyle({ margin: "16px" });
  });

  it("should apply height 100% by default", () => {
    const { container } = render(
      <SectionCard title="Test">
        <div>Content</div>
      </SectionCard>,
    );

    const card = container.querySelector(".MuiCard-root");
    expect(card).toHaveStyle({ height: "100%" });
  });

  it("should handle long titles", () => {
    const longTitle = "This is a very long section title that might wrap";
    render(
      <SectionCard title={longTitle}>
        <div>Content</div>
      </SectionCard>,
    );

    expect(screen.getByText(longTitle)).toBeInTheDocument();
  });

  it("should handle long subtitles", () => {
    const longSubtitle = "This is a very long subtitle with lots of text";
    render(
      <SectionCard title="Test" subtitle={longSubtitle}>
        <div>Content</div>
      </SectionCard>,
    );

    expect(screen.getByText(longSubtitle)).toBeInTheDocument();
  });
});
