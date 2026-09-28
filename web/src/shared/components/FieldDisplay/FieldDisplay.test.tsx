import { describe, it, expect } from "vitest";
import { render, screen } from "@/test-utils";
import { FieldDisplay } from "./FieldDisplay";
import { Chip } from "@mui/material";

describe("FieldDisplay", () => {
  it("should render label and string value", () => {
    render(<FieldDisplay label="Name" value="John Doe" />);

    expect(screen.getByText("Name")).toBeInTheDocument();
    expect(screen.getByText("John Doe")).toBeInTheDocument();
  });

  it("should render label and number value", () => {
    render(<FieldDisplay label="Total Users" value={1234} />);

    expect(screen.getByText("Total Users")).toBeInTheDocument();
    // Number formatting can vary by locale, so we check the number is present
    expect(screen.getByText(/1[,.]?234/)).toBeInTheDocument();
  });

  it("should format number values with locale string", () => {
    render(<FieldDisplay label="Count" value={1000000} />);

    // Number formatting can vary by locale
    expect(screen.getByText(/1[,.]000[,.]000/)).toBeInTheDocument();
  });

  it("should render ReactNode as value", () => {
    render(
      <FieldDisplay
        label="Status"
        value={<Chip label="Active" color="success" />}
      />,
    );

    expect(screen.getByText("Status")).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("should render with small size", () => {
    render(<FieldDisplay label="Name" value="Test" size="small" />);

    const value = screen.getByText("Test");
    expect(value).toHaveClass("MuiTypography-body2");
  });

  it("should render with medium size by default", () => {
    render(<FieldDisplay label="Name" value="Test" />);

    const value = screen.getByText("Test");
    expect(value).toHaveClass("MuiTypography-h6");
  });

  it("should render with medium size explicitly", () => {
    render(<FieldDisplay label="Name" value="Test" size="medium" />);

    const value = screen.getByText("Test");
    expect(value).toHaveClass("MuiTypography-h6");
  });

  it("should render with large size", () => {
    render(<FieldDisplay label="Name" value="Test" size="large" />);

    const value = screen.getByText("Test");
    expect(value).toHaveClass("MuiTypography-h4");
  });

  it("should render label with uppercase styling", () => {
    const { container } = render(
      <FieldDisplay label="Field Label" value="Value" />,
    );

    const label = container.querySelector(".MuiTypography-overline");
    expect(label).toHaveStyle({ textTransform: "uppercase" });
  });

  it("should render label with overline variant", () => {
    render(<FieldDisplay label="Test" value="Value" />);

    const label = screen.getByText("Test");
    expect(label).toHaveClass("MuiTypography-overline");
  });

  it("should apply custom sx props", () => {
    const { container } = render(
      <FieldDisplay label="Test" value="Value" sx={{ padding: 2 }} />,
    );

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({
      padding: "16px",
    });
  });

  it("should handle zero as value", () => {
    render(<FieldDisplay label="Count" value={0} />);

    expect(screen.getByText("0")).toBeInTheDocument();
  });

  it("should handle empty string as value", () => {
    render(<FieldDisplay label="Description" value="" />);

    expect(screen.getByText("Description")).toBeInTheDocument();
  });

  it("should handle negative numbers", () => {
    render(<FieldDisplay label="Balance" value={-500} />);

    expect(screen.getByText("-500")).toBeInTheDocument();
  });

  it("should handle decimal numbers", () => {
    render(<FieldDisplay label="Price" value={99.99} />);

    // Number formatting can vary by locale
    expect(screen.getByText(/99[.,]99/)).toBeInTheDocument();
  });

  it("should render multiple FieldDisplay components", () => {
    render(
      <>
        <FieldDisplay label="First" value="Value 1" />
        <FieldDisplay label="Second" value={123} />
        <FieldDisplay label="Third" value={<Chip label="Active" />} />
      </>,
    );

    expect(screen.getByText("First")).toBeInTheDocument();
    expect(screen.getByText("Value 1")).toBeInTheDocument();
    expect(screen.getByText("Second")).toBeInTheDocument();
    expect(screen.getByText(/123/)).toBeInTheDocument();
    expect(screen.getByText("Third")).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("should handle long labels", () => {
    const longLabel = "This is a very long field label";
    render(<FieldDisplay label={longLabel} value="Test" />);

    expect(screen.getByText(longLabel)).toBeInTheDocument();
  });

  it("should handle long string values", () => {
    const longValue =
      "This is a very long value that might wrap to multiple lines";
    render(<FieldDisplay label="Description" value={longValue} />);

    expect(screen.getByText(longValue)).toBeInTheDocument();
  });
});
