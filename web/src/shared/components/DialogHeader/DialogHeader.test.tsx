import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { DialogHeader } from "./DialogHeader";

describe("DialogHeader", () => {
  const mockOnClose = vi.fn();

  const defaultProps = {
    title: "Test Dialog",
    onClose: mockOnClose,
  };

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("should render title", () => {
    render(<DialogHeader {...defaultProps} />);

    expect(screen.getByText("Test Dialog")).toBeInTheDocument();
  });

  it("should render subtitle when provided", () => {
    render(<DialogHeader {...defaultProps} subtitle="Test Subtitle" />);

    expect(screen.getByText("Test Dialog")).toBeInTheDocument();
    expect(screen.getByText("Test Subtitle")).toBeInTheDocument();
  });

  it("should not render subtitle when not provided", () => {
    render(<DialogHeader {...defaultProps} />);

    expect(screen.getByText("Test Dialog")).toBeInTheDocument();
    expect(screen.queryByText("Test Subtitle")).not.toBeInTheDocument();
  });

  it("should render close button", () => {
    render(<DialogHeader {...defaultProps} />);

    const closeButton = screen.getByRole("button", { name: /close/i });
    expect(closeButton).toBeInTheDocument();
  });

  it("should call onClose when close button is clicked", async () => {
    const user = userEvent.setup();
    render(<DialogHeader {...defaultProps} />);

    const closeButton = screen.getByRole("button", { name: /close/i });
    await user.click(closeButton);

    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it("should render close icon", () => {
    const { container } = render(<DialogHeader {...defaultProps} />);

    const closeIcon = container.querySelector("svg");
    expect(closeIcon).toBeInTheDocument();
  });

  it("should render title with uppercase styling", () => {
    render(<DialogHeader {...defaultProps} />);

    const title = screen.getByText("Test Dialog");
    expect(title).toHaveStyle({ textTransform: "uppercase" });
  });

  it("should render title with h4 variant", () => {
    render(<DialogHeader {...defaultProps} />);

    const title = screen.getByText("Test Dialog");
    expect(title).toHaveClass("MuiTypography-h4");
  });

  it("should render subtitle with body2 variant", () => {
    render(<DialogHeader {...defaultProps} subtitle="Test Subtitle" />);

    const subtitle = screen.getByText("Test Subtitle");
    expect(subtitle).toHaveClass("MuiTypography-body2");
  });

  it("should render with flexbox layout", () => {
    const { container } = render(<DialogHeader {...defaultProps} />);

    const dialogTitle = container.querySelector(".MuiDialogTitle-root");
    expect(dialogTitle).toHaveStyle({
      display: "flex",
      alignItems: "flex-start",
      justifyContent: "space-between",
    });
  });

  it("should handle long titles", () => {
    const longTitle = "This is a very long dialog title that should wrap";
    render(<DialogHeader {...defaultProps} title={longTitle} />);

    expect(screen.getByText(longTitle)).toBeInTheDocument();
  });

  it("should handle long subtitles", () => {
    const longSubtitle = "This is a very long subtitle that might wrap";
    render(<DialogHeader {...defaultProps} subtitle={longSubtitle} />);

    expect(screen.getByText(longSubtitle)).toBeInTheDocument();
  });
});
