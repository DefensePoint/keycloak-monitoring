import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { TimeSelector } from "./TimeSelector";

describe("TimeSelector", () => {
  const mockOnTimeRangeChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    sessionStorage.clear();
  });

  it("should render time selector button with default label", async () => {
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    await waitFor(() => {
      expect(screen.getByText("Last 7 days")).toBeInTheDocument();
    });
  });

  it("should call onTimeRangeChange on mount with default 7 days range", async () => {
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    await waitFor(() => {
      expect(mockOnTimeRangeChange).toHaveBeenCalled();
    });
  });

  it("should open popover when button is clicked", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    expect(screen.getByText("Relative time ranges")).toBeInTheDocument();
    expect(screen.getByText("Absolute time range")).toBeInTheDocument();
  });

  it("should render all quick range options", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    expect(screen.getByText("Last 5 minutes")).toBeInTheDocument();
    expect(screen.getByText("Last 15 minutes")).toBeInTheDocument();
    expect(screen.getByText("Last 30 minutes")).toBeInTheDocument();
    expect(screen.getByText("Last 1 hour")).toBeInTheDocument();
    expect(screen.getByText("Last 3 hours")).toBeInTheDocument();
    expect(screen.getByText("Last 6 hours")).toBeInTheDocument();
    expect(screen.getByText("Last 12 hours")).toBeInTheDocument();
    expect(screen.getByText("Last 24 hours")).toBeInTheDocument();
    expect(screen.getAllByText("Last 7 days")[0]).toBeInTheDocument();
    expect(screen.getByText("Last 30 days")).toBeInTheDocument();
  });

  it("should call onTimeRangeChange when a quick range is selected", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    mockOnTimeRangeChange.mockClear();

    const range = screen.getByText("Last 15 minutes");
    await user.click(range);

    expect(mockOnTimeRangeChange).toHaveBeenCalledTimes(1);
  });

  it("should update button label when a quick range is selected", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    const range = screen.getByText("Last 24 hours");
    await user.click(range);

    await waitFor(() => {
      expect(screen.getByText("Last 24 hours")).toBeInTheDocument();
    });
  });

  it("should close popover after selecting a quick range", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    const range = screen.getByText("Last 5 minutes");
    await user.click(range);

    await waitFor(() => {
      expect(
        screen.queryByText("Relative time ranges"),
      ).not.toBeInTheDocument();
    });
  });

  it("should switch to absolute time range tab", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    const absoluteTab = screen.getByRole("tab", {
      name: /absolute time range/i,
    });
    await user.click(absoluteTab);

    expect(screen.getByLabelText(/from/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/to/i)).toBeInTheDocument();
  });

  it("should render datetime inputs in absolute tab", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    const absoluteTab = screen.getByRole("tab", {
      name: /absolute time range/i,
    });
    await user.click(absoluteTab);

    const fromInput = screen.getByLabelText(/from/i);
    const toInput = screen.getByLabelText(/to/i);

    expect(fromInput).toHaveAttribute("type", "datetime-local");
    expect(toInput).toHaveAttribute("type", "datetime-local");
  });

  it("should have apply and cancel buttons in absolute tab", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    const absoluteTab = screen.getByRole("tab", {
      name: /absolute time range/i,
    });
    await user.click(absoluteTab);

    expect(
      screen.getByRole("button", { name: /apply time range/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /cancel/i })).toBeInTheDocument();
  });

  it("should close popover when cancel is clicked in absolute tab", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    const absoluteTab = screen.getByRole("tab", {
      name: /absolute time range/i,
    });
    await user.click(absoluteTab);

    const cancelButton = screen.getByRole("button", { name: /cancel/i });
    await user.click(cancelButton);

    await waitFor(() => {
      expect(screen.queryByText("Absolute time range")).not.toBeInTheDocument();
    });
  });

  it("should apply custom className", () => {
    const { container } = render(
      <TimeSelector
        onTimeRangeChange={mockOnTimeRangeChange}
        className="custom-class"
      />,
    );

    const wrapper = container.querySelector(".custom-class");
    expect(wrapper).toBeInTheDocument();
  });

  it("should render with initial start and end dates", async () => {
    const initialStart = new Date("2024-01-01T10:00:00");
    const initialEnd = new Date("2024-01-01T12:00:00");

    render(
      <TimeSelector
        onTimeRangeChange={mockOnTimeRangeChange}
        initialStart={initialStart}
        initialEnd={initialEnd}
      />,
    );

    await waitFor(() => {
      expect(mockOnTimeRangeChange).toHaveBeenCalledWith(
        initialStart,
        initialEnd,
      );
    });
  });

  it("should render AccessTime icon", () => {
    const { container } = render(
      <TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />,
    );

    const icon = container.querySelector("svg[data-testid='AccessTimeIcon']");
    expect(icon).toBeInTheDocument();
  });

  it("should render ExpandMore icon", () => {
    const { container } = render(
      <TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />,
    );

    const icon = container.querySelector("svg[data-testid='ExpandMoreIcon']");
    expect(icon).toBeInTheDocument();
  });

  it("should rotate ExpandMore icon when popover is open", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    // Verify popover is open
    expect(screen.getByText("Relative time ranges")).toBeInTheDocument();
  });

  it("should have two tabs in popover", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    const tabs = screen.getAllByRole("tab");
    expect(tabs).toHaveLength(2);
  });

  it("should start with relative time tab selected", async () => {
    const user = userEvent.setup();
    render(<TimeSelector onTimeRangeChange={mockOnTimeRangeChange} />);

    const button = screen.getByRole("button", { name: /last 7 days/i });
    await user.click(button);

    const relativeTab = screen.getByRole("tab", {
      name: /relative time ranges/i,
    });
    expect(relativeTab).toHaveAttribute("aria-selected", "true");
  });
});
