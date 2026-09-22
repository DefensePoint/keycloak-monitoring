import { describe, it, expect } from "vitest";
import { render, screen } from "@/test-utils";
import { SeverityIndicator } from "./SeverityIndicator";

describe("SeverityIndicator", () => {
  it("should render with critical severity", () => {
    const { container } = render(<SeverityIndicator severity="critical" />);

    const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(dot).toBeInTheDocument();
  });

  it("should render with error severity", () => {
    const { container } = render(<SeverityIndicator severity="error" />);

    const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(dot).toBeInTheDocument();
  });

  it("should render with warning severity", () => {
    const { container } = render(<SeverityIndicator severity="warning" />);

    const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(dot).toBeInTheDocument();
  });

  it("should render with info severity", () => {
    const { container } = render(<SeverityIndicator severity="info" />);

    const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(dot).toBeInTheDocument();
  });

  it("should render with success severity", () => {
    const { container } = render(<SeverityIndicator severity="success" />);

    const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(dot).toBeInTheDocument();
  });

  it("should render with default size of 8", () => {
    const { container } = render(<SeverityIndicator severity="error" />);

    const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(dot).toHaveStyle({ width: "8px", height: "8px" });
  });

  it("should render with custom size", () => {
    const { container } = render(
      <SeverityIndicator severity="warning" size={16} />,
    );

    const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(dot).toHaveStyle({ width: "16px", height: "16px" });
  });

  it("should render with dot variant by default", () => {
    const { container } = render(<SeverityIndicator severity="info" />);

    const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(dot).toHaveStyle({ borderRadius: "50%" });
  });

  it("should render with dot variant explicitly", () => {
    const { container } = render(
      <SeverityIndicator severity="success" variant="dot" />,
    );

    const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(dot).toHaveStyle({ borderRadius: "50%" });
  });

  it("should render with icon variant", () => {
    const { container } = render(
      <SeverityIndicator severity="error" variant="icon" />,
    );

    const icon = container.querySelector("svg");
    expect(icon).toBeInTheDocument();
  });

  it("should render label when provided", () => {
    render(<SeverityIndicator severity="critical" label="Critical Error" />);

    expect(screen.getByText("Critical Error")).toBeInTheDocument();
  });

  it("should not render label when not provided", () => {
    render(<SeverityIndicator severity="info" />);

    expect(screen.queryByText("Info")).not.toBeInTheDocument();
  });

  it("should render label with icon variant", () => {
    render(
      <SeverityIndicator
        severity="warning"
        variant="icon"
        label="Warning Message"
      />,
    );

    expect(screen.getByText("Warning Message")).toBeInTheDocument();
  });

  it("should apply custom sx props", () => {
    const { container } = render(
      <SeverityIndicator severity="success" sx={{ padding: 2 }} />,
    );

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({
      padding: "16px",
    });
  });

  it("should render with flexbox layout", () => {
    const { container } = render(<SeverityIndicator severity="info" />);

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({
      display: "flex",
      alignItems: "center",
    });
  });

  it("should render circular dot", () => {
    const { container } = render(<SeverityIndicator severity="error" />);

    const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(dot).toHaveStyle({
      borderRadius: "50%",
    });
  });

  it("should render icon with custom size", () => {
    const { container } = render(
      <SeverityIndicator severity="warning" variant="icon" size={24} />,
    );

    const icon = container.querySelector("svg");
    expect(icon).toHaveStyle({ fontSize: "24px" });
  });

  it("should handle all severities with dot variant", () => {
    const severities: Array<
      "critical" | "error" | "warning" | "info" | "success"
    > = ["critical", "error", "warning", "info", "success"];

    severities.forEach((severity) => {
      const { container } = render(<SeverityIndicator severity={severity} />);
      const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
      expect(dot).toBeInTheDocument();
      expect(dot).toHaveStyle({ borderRadius: "50%" });
    });
  });

  it("should handle all severities with icon variant", () => {
    const severities: Array<
      "critical" | "error" | "warning" | "info" | "success"
    > = ["critical", "error", "warning", "info", "success"];

    severities.forEach((severity) => {
      const { container } = render(
        <SeverityIndicator severity={severity} variant="icon" />,
      );
      const icon = container.querySelector("svg");
      expect(icon).toBeInTheDocument();
    });
  });

  it("should render with label and custom size", () => {
    const { container } = render(
      <SeverityIndicator severity="error" size={12} label="Error occurred" />,
    );

    expect(screen.getByText("Error occurred")).toBeInTheDocument();
    const dot = container.querySelector(".MuiBox-root > .MuiBox-root");
    expect(dot).toHaveStyle({ width: "12px", height: "12px" });
  });

  it("should render all props combined", () => {
    const { container } = render(
      <SeverityIndicator
        severity="critical"
        variant="icon"
        size={20}
        label="Critical Issue"
        sx={{ margin: 1 }}
      />,
    );

    expect(screen.getByText("Critical Issue")).toBeInTheDocument();
    const icon = container.querySelector("svg");
    expect(icon).toHaveStyle({ fontSize: "20px" });

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ margin: "8px" });
  });
});
