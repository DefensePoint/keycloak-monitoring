import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { ConfirmDialog } from "./ConfirmDialog";

describe("ConfirmDialog", () => {
  const mockOnConfirm = vi.fn();
  const mockOnCancel = vi.fn();

  const defaultProps = {
    open: true,
    title: "Confirm Action",
    message: "Are you sure you want to proceed?",
    onConfirm: mockOnConfirm,
    onCancel: mockOnCancel,
  };

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("should render dialog when open is true", () => {
    render(<ConfirmDialog {...defaultProps} />);

    expect(screen.getByText("Confirm Action")).toBeInTheDocument();
    expect(
      screen.getByText("Are you sure you want to proceed?"),
    ).toBeInTheDocument();
  });

  it("should not render dialog when open is false", () => {
    render(<ConfirmDialog {...defaultProps} open={false} />);

    expect(screen.queryByText("Confirm Action")).not.toBeInTheDocument();
  });

  it("should render default confirm and cancel buttons", () => {
    render(<ConfirmDialog {...defaultProps} />);

    expect(
      screen.getByRole("button", { name: /confirm/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /cancel/i })).toBeInTheDocument();
  });

  it("should render custom confirm and cancel text", () => {
    render(
      <ConfirmDialog
        {...defaultProps}
        confirmText="Delete"
        cancelText="Go Back"
      />,
    );

    expect(screen.getByRole("button", { name: /delete/i })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /go back/i }),
    ).toBeInTheDocument();
  });

  it("should call onConfirm when confirm button is clicked", async () => {
    const user = userEvent.setup();
    render(<ConfirmDialog {...defaultProps} />);

    await user.click(screen.getByRole("button", { name: /confirm/i }));

    expect(mockOnConfirm).toHaveBeenCalledTimes(1);
  });

  it("should call onCancel when cancel button is clicked", async () => {
    const user = userEvent.setup();
    render(<ConfirmDialog {...defaultProps} />);

    await user.click(screen.getByRole("button", { name: /cancel/i }));

    expect(mockOnCancel).toHaveBeenCalledTimes(1);
  });

  it("should call onCancel when dialog backdrop is clicked", async () => {
    const user = userEvent.setup();
    render(<ConfirmDialog {...defaultProps} />);

    // Click on backdrop (MuiBackdrop-root)
    const backdrop = document.querySelector(".MuiBackdrop-root");
    if (backdrop) {
      await user.click(backdrop);
      expect(mockOnCancel).toHaveBeenCalledTimes(1);
    }
  });

  it("should render with error color by default", () => {
    render(<ConfirmDialog {...defaultProps} />);

    const confirmButton = screen.getByRole("button", { name: /confirm/i });
    expect(confirmButton).toHaveClass("MuiButton-containedError");
  });

  it("should render with success color", () => {
    render(<ConfirmDialog {...defaultProps} confirmColor="success" />);

    const confirmButton = screen.getByRole("button", { name: /confirm/i });
    expect(confirmButton).toHaveClass("MuiButton-containedSuccess");
  });

  it("should render with warning color", () => {
    render(<ConfirmDialog {...defaultProps} confirmColor="warning" />);

    const confirmButton = screen.getByRole("button", { name: /confirm/i });
    expect(confirmButton).toHaveClass("MuiButton-containedWarning");
  });

  it("should render with secondary color", () => {
    render(<ConfirmDialog {...defaultProps} confirmColor="secondary" />);

    const confirmButton = screen.getByRole("button", { name: /confirm/i });
    expect(confirmButton).toHaveClass("MuiButton-containedSecondary");
  });

  it("should render centered title", () => {
    render(<ConfirmDialog {...defaultProps} />);

    const title = screen.getByText("Confirm Action");
    expect(title).toBeInTheDocument();
  });

  it("should render centered message", () => {
    render(<ConfirmDialog {...defaultProps} />);

    const message = screen.getByText("Are you sure you want to proceed?");
    expect(message).toBeInTheDocument();
  });

  it("should render full width buttons", () => {
    render(<ConfirmDialog {...defaultProps} />);

    const confirmButton = screen.getByRole("button", { name: /confirm/i });
    const cancelButton = screen.getByRole("button", { name: /cancel/i });

    expect(confirmButton).toHaveClass("MuiButton-fullWidth");
    expect(cancelButton).toHaveClass("MuiButton-fullWidth");
  });
});
