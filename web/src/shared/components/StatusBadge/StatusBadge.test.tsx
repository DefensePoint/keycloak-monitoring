import { describe, it, expect } from "vitest";
import { render, screen } from "@/test-utils";
import { StatusBadge } from "./StatusBadge";

describe("StatusBadge", () => {
  it("should render label with success status", () => {
    render(<StatusBadge status="success" label="Active" />);

    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("should render label with error status", () => {
    render(<StatusBadge status="error" label="Failed" />);

    expect(screen.getByText("Failed")).toBeInTheDocument();
  });

  it("should render label with warning status", () => {
    render(<StatusBadge status="warning" label="Pending" />);

    expect(screen.getByText("Pending")).toBeInTheDocument();
  });

  it("should render label with info status", () => {
    render(<StatusBadge status="info" label="Processing" />);

    expect(screen.getByText("Processing")).toBeInTheDocument();
  });

  it("should render label with default status", () => {
    render(<StatusBadge status="default" label="Unknown" />);

    expect(screen.getByText("Unknown")).toBeInTheDocument();
  });

  it("should render with success color", () => {
    const { container } = render(
      <StatusBadge status="success" label="Active" />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-colorSuccess");
  });

  it("should render with error color", () => {
    const { container } = render(<StatusBadge status="error" label="Failed" />);

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-colorError");
  });

  it("should render with warning color", () => {
    const { container } = render(
      <StatusBadge status="warning" label="Pending" />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-colorWarning");
  });

  it("should render with info color", () => {
    const { container } = render(<StatusBadge status="info" label="Info" />);

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-colorInfo");
  });

  it("should render with default color", () => {
    const { container } = render(
      <StatusBadge status="default" label="Default" />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-colorDefault");
  });

  it("should render with small size by default", () => {
    const { container } = render(
      <StatusBadge status="success" label="Active" />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-sizeSmall");
  });

  it("should render with small size explicitly", () => {
    const { container } = render(
      <StatusBadge status="success" label="Active" size="small" />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-sizeSmall");
  });

  it("should render with medium size", () => {
    const { container } = render(
      <StatusBadge status="error" label="Failed" size="medium" />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-sizeMedium");
  });

  it("should render with filled variant by default", () => {
    const { container } = render(
      <StatusBadge status="success" label="Active" />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-filled");
  });

  it("should render with filled variant explicitly", () => {
    const { container } = render(
      <StatusBadge status="success" label="Active" variant="filled" />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-filled");
  });

  it("should render with outlined variant", () => {
    const { container } = render(
      <StatusBadge status="warning" label="Pending" variant="outlined" />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-outlined");
  });

  it("should render with uppercase styling", () => {
    const { container } = render(
      <StatusBadge status="info" label="processing" />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveStyle({
      textTransform: "uppercase",
    });
  });

  it("should apply fontWeight and letterSpacing styles", () => {
    const { container } = render(
      <StatusBadge status="success" label="Active" />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveStyle({
      fontWeight: "600",
      letterSpacing: "0.05em",
    });
  });

  it("should apply custom sx props via chipProps", () => {
    const { container } = render(
      <StatusBadge
        status="error"
        label="Failed"
        chipProps={{ sx: { margin: 2 } }}
      />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveStyle({
      margin: "16px",
    });
  });

  it("should merge custom sx with default sx", () => {
    const { container } = render(
      <StatusBadge
        status="success"
        label="Active"
        chipProps={{ sx: { padding: 1 } }}
      />,
    );

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveStyle({
      padding: "8px",
    });
  });

  it("should handle all status types", () => {
    const statuses: Array<
      "success" | "error" | "warning" | "info" | "default"
    > = ["success", "error", "warning", "info", "default"];

    statuses.forEach((status) => {
      const { container } = render(
        <StatusBadge status={status} label={status} />,
      );
      const chip = container.querySelector(".MuiChip-root");
      expect(chip).toBeInTheDocument();
    });
  });

  it("should render with all props combined", () => {
    const { container } = render(
      <StatusBadge
        status="warning"
        label="Pending Approval"
        size="medium"
        variant="outlined"
        chipProps={{ sx: { margin: 1 } }}
      />,
    );

    expect(screen.getByText("Pending Approval")).toBeInTheDocument();

    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toHaveClass("MuiChip-colorWarning");
    expect(chip).toHaveClass("MuiChip-sizeMedium");
    expect(chip).toHaveClass("MuiChip-outlined");
    expect(chip).toHaveStyle({
      margin: "8px",
    });
  });

  it("should handle long labels", () => {
    const longLabel = "This is a very long status label";
    render(<StatusBadge status="info" label={longLabel} />);

    expect(screen.getByText(longLabel)).toBeInTheDocument();
  });

  it("should handle empty label", () => {
    render(<StatusBadge status="success" label="" />);

    const { container } = render(<StatusBadge status="success" label="" />);
    const chip = container.querySelector(".MuiChip-root");
    expect(chip).toBeInTheDocument();
  });
});
