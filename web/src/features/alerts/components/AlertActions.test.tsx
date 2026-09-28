import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { AlertActions } from "./AlertActions";

vi.mock("@/shared/components", () => ({
  SectionCard: ({
    title,
    children,
  }: {
    title: string;
    children: React.ReactNode;
  }) => (
    <div>
      <h3>{title}</h3>
      {children}
    </div>
  ),
}));

describe("AlertActions", () => {
  const mockOnAcknowledge = vi.fn();
  const mockOnIgnore = vi.fn();
  const mockOnResolve = vi.fn();

  const defaultProps = {
    onAcknowledge: mockOnAcknowledge,
    onIgnore: mockOnIgnore,
    onResolve: mockOnResolve,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render Actions section title", () => {
    render(<AlertActions status="active" {...defaultProps} />);
    expect(screen.getByText("Actions")).toBeInTheDocument();
  });

  describe("when status is active", () => {
    it("should render acknowledge, ignore, and resolve buttons", () => {
      render(<AlertActions status="active" {...defaultProps} />);

      expect(
        screen.getByRole("button", { name: /acknowledge/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /ignore/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /resolve/i }),
      ).toBeInTheDocument();
    });

    it("should call onAcknowledge when acknowledge button is clicked", async () => {
      const user = userEvent.setup();
      render(<AlertActions status="active" {...defaultProps} />);

      await user.click(screen.getByRole("button", { name: /acknowledge/i }));
      expect(mockOnAcknowledge).toHaveBeenCalledTimes(1);
    });

    it("should call onIgnore when ignore button is clicked", async () => {
      const user = userEvent.setup();
      render(<AlertActions status="active" {...defaultProps} />);

      await user.click(screen.getByRole("button", { name: /ignore/i }));
      expect(mockOnIgnore).toHaveBeenCalledTimes(1);
    });

    it("should call onResolve when resolve button is clicked", async () => {
      const user = userEvent.setup();
      render(<AlertActions status="active" {...defaultProps} />);

      await user.click(screen.getByRole("button", { name: /resolve/i }));
      expect(mockOnResolve).toHaveBeenCalledTimes(1);
    });
  });

  describe("when status is acknowledged", () => {
    it("should not render acknowledge button", () => {
      render(<AlertActions status="acknowledged" {...defaultProps} />);

      expect(
        screen.queryByRole("button", { name: /acknowledge/i }),
      ).not.toBeInTheDocument();
    });

    it("should render ignore and resolve buttons", () => {
      render(<AlertActions status="acknowledged" {...defaultProps} />);

      expect(
        screen.getByRole("button", { name: /ignore/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /resolve/i }),
      ).toBeInTheDocument();
    });
  });

  describe("when status is resolved", () => {
    it("should not render any action buttons", () => {
      render(<AlertActions status="resolved" {...defaultProps} />);

      expect(
        screen.queryByRole("button", { name: /acknowledge/i }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /ignore/i }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /resolve/i }),
      ).not.toBeInTheDocument();
    });

    it("should display resolved message", () => {
      render(<AlertActions status="resolved" {...defaultProps} />);

      expect(
        screen.getByText(/This alert has been resolved/i),
      ).toBeInTheDocument();
    });
  });

  describe("when status is ignored", () => {
    it("should not render any action buttons", () => {
      render(<AlertActions status="ignored" {...defaultProps} />);

      expect(
        screen.queryByRole("button", { name: /acknowledge/i }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /ignore/i }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /resolve/i }),
      ).not.toBeInTheDocument();
    });

    it("should display ignored message", () => {
      render(<AlertActions status="ignored" {...defaultProps} />);

      expect(
        screen.getByText(/This alert has been ignored/i),
      ).toBeInTheDocument();
    });
  });
});
