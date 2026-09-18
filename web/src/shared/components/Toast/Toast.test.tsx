import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { Toast } from "./Toast";

describe("Toast", () => {
  const mockOnClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
  });

  it("should render with success type", () => {
    render(
      <Toast message="Success message" type="success" onClose={mockOnClose} />,
    );

    expect(screen.getByText("Success message")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveClass("MuiAlert-standardSuccess");
  });

  it("should render with error type", () => {
    render(
      <Toast message="Error message" type="error" onClose={mockOnClose} />,
    );

    expect(screen.getByText("Error message")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveClass("MuiAlert-standardError");
  });

  it("should render with info type", () => {
    render(<Toast message="Info message" type="info" onClose={mockOnClose} />);

    expect(screen.getByText("Info message")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveClass("MuiAlert-standardInfo");
  });

  it("should call onClose when close button is clicked", async () => {
    vi.useRealTimers();
    const user = userEvent.setup({ delay: null });
    render(
      <Toast message="Test message" type="success" onClose={mockOnClose} />,
    );

    const closeButton = screen.getByRole("button", { name: /close/i });
    await user.click(closeButton);

    expect(mockOnClose).toHaveBeenCalledTimes(1);
    vi.useFakeTimers();
  });

  it("should auto hide after default duration", () => {
    render(
      <Toast message="Auto hide message" type="info" onClose={mockOnClose} />,
    );

    expect(mockOnClose).not.toHaveBeenCalled();

    vi.advanceTimersByTime(3000);

    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it("should auto hide after custom duration", () => {
    render(
      <Toast
        message="Custom duration"
        type="success"
        onClose={mockOnClose}
        duration={5000}
      />,
    );

    vi.advanceTimersByTime(3000);
    expect(mockOnClose).not.toHaveBeenCalled();

    vi.advanceTimersByTime(2000);
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it("should render at top-right position", () => {
    const { container } = render(
      <Toast message="Position test" type="info" onClose={mockOnClose} />,
    );

    const snackbar = container.querySelector(".MuiSnackbar-root");
    expect(snackbar).toHaveClass("MuiSnackbar-anchorOriginTopRight");
  });
});
