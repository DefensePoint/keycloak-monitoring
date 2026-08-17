import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { OperatorDateRangeFilter } from "./OperatorDateRangeFilter";

describe("OperatorDateRangeFilter", () => {
  const mockOnStartDateChange = vi.fn();
  const mockOnEndDateChange = vi.fn();
  const mockOnRefresh = vi.fn();

  const defaultProps = {
    startDate: "2024-01-01",
    endDate: "2024-01-31",
    onStartDateChange: mockOnStartDateChange,
    onEndDateChange: mockOnEndDateChange,
    onRefresh: mockOnRefresh,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render start date and end date inputs", () => {
    render(<OperatorDateRangeFilter {...defaultProps} />);

    expect(screen.getByLabelText(/start date/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/end date/i)).toBeInTheDocument();
  });

  it("should render refresh button", () => {
    render(<OperatorDateRangeFilter {...defaultProps} />);

    expect(
      screen.getByRole("button", { name: /refresh/i }),
    ).toBeInTheDocument();
  });

  it("should display current start date value", () => {
    render(<OperatorDateRangeFilter {...defaultProps} />);

    const startDateInput = screen.getByLabelText(/start date/i);
    expect(startDateInput).toHaveValue("2024-01-01");
  });

  it("should display current end date value", () => {
    render(<OperatorDateRangeFilter {...defaultProps} />);

    const endDateInput = screen.getByLabelText(/end date/i);
    expect(endDateInput).toHaveValue("2024-01-31");
  });

  it("should call onStartDateChange when start date changes", () => {
    render(<OperatorDateRangeFilter {...defaultProps} />);

    const startDateInput = screen.getByLabelText(/start date/i);
    fireEvent.change(startDateInput, { target: { value: "2024-02-01" } });

    expect(mockOnStartDateChange).toHaveBeenCalledWith("2024-02-01");
  });

  it("should call onEndDateChange when end date changes", () => {
    render(<OperatorDateRangeFilter {...defaultProps} />);

    const endDateInput = screen.getByLabelText(/end date/i);
    fireEvent.change(endDateInput, { target: { value: "2024-02-28" } });

    expect(mockOnEndDateChange).toHaveBeenCalledWith("2024-02-28");
  });

  it("should call onRefresh when refresh button is clicked", async () => {
    const user = userEvent.setup();
    render(<OperatorDateRangeFilter {...defaultProps} />);

    await user.click(screen.getByRole("button", { name: /refresh/i }));

    expect(mockOnRefresh).toHaveBeenCalledTimes(1);
  });

  it("should have date input type for start date", () => {
    render(<OperatorDateRangeFilter {...defaultProps} />);

    const startDateInput = screen.getByLabelText(/start date/i);
    expect(startDateInput).toHaveAttribute("type", "date");
  });

  it("should have date input type for end date", () => {
    render(<OperatorDateRangeFilter {...defaultProps} />);

    const endDateInput = screen.getByLabelText(/end date/i);
    expect(endDateInput).toHaveAttribute("type", "date");
  });
});
