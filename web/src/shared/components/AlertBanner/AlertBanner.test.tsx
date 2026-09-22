import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { AlertBanner } from "./AlertBanner";

describe("AlertBanner", () => {
  it("should render with error severity", () => {
    render(<AlertBanner severity="error" message="Error message" />);

    expect(screen.getByText("Error message")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveClass("MuiAlert-standardError");
  });

  it("should render with warning severity", () => {
    render(<AlertBanner severity="warning" message="Warning message" />);

    expect(screen.getByText("Warning message")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveClass("MuiAlert-standardWarning");
  });

  it("should render with info severity", () => {
    render(<AlertBanner severity="info" message="Info message" />);

    expect(screen.getByText("Info message")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveClass("MuiAlert-standardInfo");
  });

  it("should render with success severity", () => {
    render(<AlertBanner severity="success" message="Success message" />);

    expect(screen.getByText("Success message")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveClass("MuiAlert-standardSuccess");
  });

  it("should render with title", () => {
    render(
      <AlertBanner
        severity="warning"
        title="Warning Title"
        message="Warning message"
      />,
    );

    expect(screen.getByText("Warning Title")).toBeInTheDocument();
    expect(screen.getByText("Warning message")).toBeInTheDocument();
  });

  it("should not render close button by default", () => {
    render(<AlertBanner severity="info" message="Info message" />);

    expect(
      screen.queryByRole("button", { name: /close/i }),
    ).not.toBeInTheDocument();
  });

  it("should render close button when closable is true", () => {
    const mockOnClose = vi.fn();
    render(
      <AlertBanner
        severity="info"
        message="Info message"
        closable
        onClose={mockOnClose}
      />,
    );

    expect(screen.getByRole("button", { name: /close/i })).toBeInTheDocument();
  });

  it("should call onClose when close button is clicked", async () => {
    const user = userEvent.setup();
    const mockOnClose = vi.fn();

    render(
      <AlertBanner
        severity="success"
        message="Success message"
        closable
        onClose={mockOnClose}
      />,
    );

    await user.click(screen.getByRole("button", { name: /close/i }));
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it("should not render close button when closable is true but onClose is not provided", () => {
    render(<AlertBanner severity="info" message="Info message" closable />);

    expect(
      screen.queryByRole("button", { name: /close/i }),
    ).not.toBeInTheDocument();
  });

  it("should render ReactNode as message", () => {
    render(
      <AlertBanner
        severity="info"
        message={
          <div>
            <strong>Bold text</strong> and normal text
          </div>
        }
      />,
    );

    expect(screen.getByText("Bold text")).toBeInTheDocument();
    expect(screen.getByText(/and normal text/i)).toBeInTheDocument();
  });

  it("should apply custom sx props", () => {
    const { container } = render(
      <AlertBanner
        severity="info"
        message="Test message"
        sx={{ marginTop: 2 }}
      />,
    );

    const alert = container.querySelector(".MuiAlert-root");
    expect(alert).toHaveStyle({ marginTop: "16px" });
  });

  it("should render without title when not provided", () => {
    render(<AlertBanner severity="info" message="Message only" />);

    expect(screen.getByText("Message only")).toBeInTheDocument();
    expect(screen.queryByRole("heading")).not.toBeInTheDocument();
  });
});
