import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { UserEventsFilters } from "./UserEventsFilters";

vi.mock("@/shared/components/TimeSelector", () => ({
  TimeSelector: ({
    onTimeRangeChange,
  }: {
    onTimeRangeChange: (range: { start: number; end: number }) => void;
  }) => (
    <button onClick={() => onTimeRangeChange({ start: 0, end: 100 })}>
      Time Selector
    </button>
  ),
}));

describe("UserEventsFilters", () => {
  const mockOnSearchChange = vi.fn();
  const mockOnSeverityChange = vi.fn();
  const mockOnSourceChange = vi.fn();
  const mockOnLimitChange = vi.fn();
  const mockOnReset = vi.fn();
  const mockOnTimeRangeChange = vi.fn();

  const defaultProps = {
    searchTerm: "",
    severityFilter: "",
    sourceFilter: "",
    limit: 50,
    onSearchChange: mockOnSearchChange,
    onSeverityChange: mockOnSeverityChange,
    onSourceChange: mockOnSourceChange,
    onLimitChange: mockOnLimitChange,
    onReset: mockOnReset,
    onTimeRangeChange: mockOnTimeRangeChange,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render TimeSelector component", () => {
    render(<UserEventsFilters {...defaultProps} />);

    expect(screen.getByText("Time Selector")).toBeInTheDocument();
  });

  it("should render limit selector", () => {
    render(<UserEventsFilters {...defaultProps} />);

    expect(screen.getByLabelText(/show/i)).toBeInTheDocument();
  });

  it("should render search input", () => {
    render(<UserEventsFilters {...defaultProps} />);

    expect(
      screen.getByPlaceholderText(/Type, description.../i),
    ).toBeInTheDocument();
  });

  it("should render severity filter", () => {
    render(<UserEventsFilters {...defaultProps} />);

    expect(screen.getByLabelText(/severity/i)).toBeInTheDocument();
  });

  it("should render source filter", () => {
    render(<UserEventsFilters {...defaultProps} />);

    expect(
      screen.getByPlaceholderText(/Filter by source.../i),
    ).toBeInTheDocument();
  });

  it("should render reset filters button", () => {
    render(<UserEventsFilters {...defaultProps} />);

    expect(
      screen.getByRole("button", { name: /reset filters/i }),
    ).toBeInTheDocument();
  });

  it("should call onSearchChange when search input changes", async () => {
    const user = userEvent.setup();
    render(<UserEventsFilters {...defaultProps} />);

    const searchInput = screen.getByPlaceholderText(/Type, description.../i);
    await user.type(searchInput, "test");

    expect(mockOnSearchChange).toHaveBeenCalled();
    // user.type calls onChange for each character
    expect(mockOnSearchChange).toHaveBeenCalledTimes(4); // "t", "e", "s", "t"
  });

  it("should call onSeverityChange when severity filter changes", async () => {
    const user = userEvent.setup();
    render(<UserEventsFilters {...defaultProps} />);

    const severitySelect = screen.getByLabelText(/severity/i);
    await user.click(severitySelect);
    await user.click(screen.getByRole("option", { name: /error/i }));

    expect(mockOnSeverityChange).toHaveBeenCalledWith("error");
  });

  it("should call onSourceChange when source filter changes", async () => {
    const user = userEvent.setup();
    render(<UserEventsFilters {...defaultProps} />);

    const sourceInput = screen.getByPlaceholderText(/Filter by source.../i);
    await user.type(sourceInput, "keycloak");

    expect(mockOnSourceChange).toHaveBeenCalled();
    // user.type calls onChange for each character
    expect(mockOnSourceChange).toHaveBeenCalledTimes(8); // "keycloak" has 8 characters
  });

  it("should call onLimitChange when limit selector changes", async () => {
    const user = userEvent.setup();
    render(<UserEventsFilters {...defaultProps} />);

    const limitSelect = screen.getByLabelText(/show/i);
    await user.click(limitSelect);
    await user.click(screen.getByRole("option", { name: /100 events/i }));

    expect(mockOnLimitChange).toHaveBeenCalledWith(100);
  });

  it("should call onReset when reset button is clicked", async () => {
    const user = userEvent.setup();
    render(<UserEventsFilters {...defaultProps} />);

    await user.click(screen.getByRole("button", { name: /reset filters/i }));

    expect(mockOnReset).toHaveBeenCalledTimes(1);
  });

  it("should call onTimeRangeChange when time range changes", async () => {
    const user = userEvent.setup();
    render(<UserEventsFilters {...defaultProps} />);

    await user.click(screen.getByText("Time Selector"));

    expect(mockOnTimeRangeChange).toHaveBeenCalledWith({ start: 0, end: 100 });
  });

  it("should display current search term", () => {
    render(<UserEventsFilters {...defaultProps} searchTerm="test-search" />);

    const searchInput = screen.getByPlaceholderText(/Type, description.../i);
    expect(searchInput).toHaveValue("test-search");
  });

  it("should display current source filter", () => {
    render(<UserEventsFilters {...defaultProps} sourceFilter="keycloak" />);

    const sourceInput = screen.getByPlaceholderText(/Filter by source.../i);
    expect(sourceInput).toHaveValue("keycloak");
  });

  it("should display current limit value", async () => {
    const user = userEvent.setup();
    render(<UserEventsFilters {...defaultProps} limit={100} />);

    const limitSelect = screen.getByLabelText(/show/i);
    await user.click(limitSelect);

    const option100 = screen.getByRole("option", { name: /100 events/i });
    expect(option100).toHaveClass("Mui-selected");
  });

  it("should render severity filter options", async () => {
    const user = userEvent.setup();
    render(<UserEventsFilters {...defaultProps} />);

    const severitySelect = screen.getByLabelText(/severity/i);
    await user.click(severitySelect);

    expect(
      screen.getByRole("option", { name: /all severities/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /^info$/i })).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: /warning/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /error/i })).toBeInTheDocument();
  });

  it("should render limit options", async () => {
    const user = userEvent.setup();
    render(<UserEventsFilters {...defaultProps} />);

    const limitSelect = screen.getByLabelText(/show/i);
    await user.click(limitSelect);

    expect(
      screen.getByRole("option", { name: /25 events/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: /50 events/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: /100 events/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: /200 events/i }),
    ).toBeInTheDocument();
  });
});
