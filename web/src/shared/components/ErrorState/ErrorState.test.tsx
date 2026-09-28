import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@/test-utils";
import { ErrorState } from "./ErrorState";

describe("ErrorState", () => {
  it("should render with default title", () => {
    render(<ErrorState error={null} />);

    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
  });

  it("should render with custom title", () => {
    render(<ErrorState error={null} title="Custom error title" />);

    expect(screen.getByText("Custom error title")).toBeInTheDocument();
  });

  it("should render string error message", () => {
    render(<ErrorState error="Custom error message" />);

    expect(screen.getByText("Custom error message")).toBeInTheDocument();
  });

  it("should render Error object with sanitized message", () => {
    const error = new Error("Network request failed");
    render(<ErrorState error={error} />);

    // Should show error message (sanitized via getErrorMessage)
    expect(screen.getByText(/failed/i)).toBeInTheDocument();
  });

  it("should render default message when error is null", () => {
    render(<ErrorState error={null} />);

    expect(screen.getByText(/failed to load data/i)).toBeInTheDocument();
  });

  it("should render error icon", () => {
    const { container } = render(<ErrorState error="Error" />);

    const icon = container.querySelector('[data-testid="ErrorOutlineIcon"]');
    expect(icon).toBeInTheDocument();
  });

  it("should render retry button when onRetry is provided", () => {
    const onRetry = vi.fn();
    render(<ErrorState error="Error" onRetry={onRetry} />);

    expect(
      screen.getByRole("button", { name: /try again/i }),
    ).toBeInTheDocument();
  });

  it("should not render retry button when onRetry is not provided", () => {
    render(<ErrorState error="Error" />);

    expect(
      screen.queryByRole("button", { name: /try again/i }),
    ).not.toBeInTheDocument();
  });

  it("should call onRetry when retry button is clicked", () => {
    const onRetry = vi.fn();
    render(<ErrorState error="Error" onRetry={onRetry} />);

    fireEvent.click(screen.getByRole("button", { name: /try again/i }));

    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("should show error details when showDetails is true", () => {
    const error = new Error("Detailed error");
    error.stack = "Error: Detailed error\n    at Component";

    render(<ErrorState error={error} showDetails />);

    expect(screen.getByText(/Error: Detailed error/)).toBeInTheDocument();
  });

  it("should not show error details by default", () => {
    const error = new Error("Detailed error");
    error.stack = "Error: Detailed error\n    at Component";

    render(<ErrorState error={error} />);

    expect(screen.queryByText(/at Component/)).not.toBeInTheDocument();
  });

  it("should not show error details for string errors", () => {
    render(<ErrorState error="String error" showDetails />);

    // String errors don't have stack traces
    expect(screen.queryByRole("code")).not.toBeInTheDocument();
  });

  it("should apply default minHeight", () => {
    const { container } = render(<ErrorState error="Error" />);

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ minHeight: "200px" });
  });

  it("should apply custom minHeight as number", () => {
    const { container } = render(<ErrorState error="Error" minHeight={400} />);

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ minHeight: "400px" });
  });

  it("should apply custom minHeight as string", () => {
    const { container } = render(<ErrorState error="Error" minHeight="50vh" />);

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ minHeight: "50vh" });
  });

  it("should apply custom sx props", () => {
    const { container } = render(
      <ErrorState error="Error" sx={{ padding: 4 }} />,
    );

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ padding: "32px" });
  });

  it("should render centered layout", () => {
    const { container } = render(<ErrorState error="Error" />);

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({
      display: "flex",
      flexDirection: "column",
      alignItems: "center",
      justifyContent: "center",
      textAlign: "center",
    });
  });

  it("should render title with h6 variant", () => {
    render(<ErrorState error="Error" title="Custom Title" />);

    const title = screen.getByText("Custom Title");
    expect(title).toHaveClass("MuiTypography-h6");
  });

  it("should render message with body2 variant", () => {
    render(<ErrorState error="Custom message" />);

    const message = screen.getByText("Custom message");
    expect(message).toHaveClass("MuiTypography-body2");
  });

  it("should render with all props combined", () => {
    const onRetry = vi.fn();
    const error = new Error("Test error");
    error.stack = "Error: Test error\n    at TestComponent";

    const { container } = render(
      <ErrorState
        error={error}
        title="Operation Failed"
        onRetry={onRetry}
        showDetails
        minHeight={300}
        sx={{ padding: 2 }}
      />,
    );

    expect(screen.getByText("Operation Failed")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /try again/i }),
    ).toBeInTheDocument();
    // Error details section shows the stack trace
    expect(screen.getByText(/at TestComponent/)).toBeInTheDocument();

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ minHeight: "300px", padding: "16px" });
  });
});
