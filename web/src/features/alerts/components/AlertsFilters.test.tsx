import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { AlertsFilters } from "./AlertsFilters";

describe("AlertsFilters", () => {
  const mockOnStatusChange = vi.fn();
  const mockOnSeverityChange = vi.fn();
  const mockOnRealmChange = vi.fn();
  const mockOnResourceChange = vi.fn();
  const mockOnClearFilters = vi.fn();
  const mockOnRefresh = vi.fn();

  const defaultProps = {
    statusFilter: "active",
    severityFilter: "all",
    realmFilter: "all",
    resourceFilter: "all",
    uniqueRealms: ["realm1", "realm2"],
    uniqueResourceTypes: ["user", "client"],
    filteredCount: 10,
    totalCount: 20,
    onStatusChange: mockOnStatusChange,
    onSeverityChange: mockOnSeverityChange,
    onRealmChange: mockOnRealmChange,
    onResourceChange: mockOnResourceChange,
    onClearFilters: mockOnClearFilters,
    onRefresh: mockOnRefresh,
    showReloadAmfa: false,
    onReloadAmfa: vi.fn(),
    reloadingAmfa: false,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render all filter dropdowns", () => {
    render(<AlertsFilters {...defaultProps} />);

    expect(screen.getByLabelText(/severity/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/realm/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/resource type/i)).toBeInTheDocument();
  });

  it("should display filtered count and total count", () => {
    render(<AlertsFilters {...defaultProps} />);

    expect(
      screen.getByText(/Showing 10 of 20 matching alerts/i),
    ).toBeInTheDocument();
  });

  it("should call onSeverityChange when severity filter changes", async () => {
    const user = userEvent.setup();
    render(<AlertsFilters {...defaultProps} />);

    const severitySelect = screen.getByRole("combobox", { name: /severity/i });
    await user.click(severitySelect);
    await user.click(screen.getByRole("option", { name: /critical/i }));

    expect(mockOnSeverityChange).toHaveBeenCalledWith("critical");
  });

  it("should call onRealmChange when realm filter changes", async () => {
    const user = userEvent.setup();
    render(<AlertsFilters {...defaultProps} />);

    const realmSelect = screen.getByRole("combobox", { name: /realm/i });
    await user.click(realmSelect);
    await user.click(screen.getByRole("option", { name: "realm1" }));

    expect(mockOnRealmChange).toHaveBeenCalledWith("realm1");
  });

  it("should call onResourceChange when resource filter changes", async () => {
    const user = userEvent.setup();
    render(<AlertsFilters {...defaultProps} />);

    const resourceSelect = screen.getByRole("combobox", {
      name: /resource type/i,
    });
    await user.click(resourceSelect);
    await user.click(screen.getByRole("option", { name: "user" }));

    expect(mockOnResourceChange).toHaveBeenCalledWith("user");
  });

  it("should render unique realms in dropdown", async () => {
    const user = userEvent.setup();
    render(<AlertsFilters {...defaultProps} />);

    const realmSelect = screen.getByRole("combobox", { name: /realm/i });
    await user.click(realmSelect);

    expect(screen.getByRole("option", { name: "realm1" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "realm2" })).toBeInTheDocument();
  });

  it("should render unique resource types in dropdown", async () => {
    const user = userEvent.setup();
    render(<AlertsFilters {...defaultProps} />);

    const resourceSelect = screen.getByRole("combobox", {
      name: /resource type/i,
    });
    await user.click(resourceSelect);

    expect(screen.getByRole("option", { name: "user" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "client" })).toBeInTheDocument();
  });

  it("should show reset button when filters are active", () => {
    render(<AlertsFilters {...defaultProps} severityFilter="critical" />);

    const resetButton = screen.getByRole("button", { name: /reset/i });
    expect(resetButton).toBeInTheDocument();
    expect(resetButton).not.toBeDisabled();
  });

  it("should disable reset button when no filters are active", () => {
    render(<AlertsFilters {...defaultProps} />);

    const resetButton = screen.getByRole("button", { name: /reset/i });
    expect(resetButton).toBeDisabled();
  });

  it("should call onClearFilters when reset button is clicked", async () => {
    const user = userEvent.setup();
    render(<AlertsFilters {...defaultProps} severityFilter="critical" />);

    await user.click(screen.getByRole("button", { name: /reset/i }));

    expect(mockOnClearFilters).toHaveBeenCalledTimes(1);
  });

  it("should enable reset button when realm filter is active", () => {
    render(<AlertsFilters {...defaultProps} realmFilter="realm1" />);

    const resetButton = screen.getByRole("button", { name: /reset/i });
    expect(resetButton).not.toBeDisabled();
  });

  it("should enable reset button when resource filter is active", () => {
    render(<AlertsFilters {...defaultProps} resourceFilter="user" />);

    const resetButton = screen.getByRole("button", { name: /reset/i });
    expect(resetButton).not.toBeDisabled();
  });

  it("hides the Reload AMFA button when showReloadAmfa is false", () => {
    render(<AlertsFilters {...defaultProps} showReloadAmfa={false} />);
    expect(
      screen.queryByRole("button", { name: /reload amfa/i }),
    ).not.toBeInTheDocument();
  });

  it("shows and fires the Reload AMFA button when enabled", async () => {
    const user = userEvent.setup();
    const onReloadAmfa = vi.fn();
    render(
      <AlertsFilters
        {...defaultProps}
        showReloadAmfa={true}
        onReloadAmfa={onReloadAmfa}
      />,
    );
    const btn = screen.getByRole("button", { name: /reload amfa/i });
    await user.click(btn);
    expect(onReloadAmfa).toHaveBeenCalledTimes(1);
  });

  it("disables the Reload AMFA button while reloading", () => {
    render(
      <AlertsFilters
        {...defaultProps}
        showReloadAmfa={true}
        reloadingAmfa={true}
      />,
    );
    expect(screen.getByRole("button", { name: /reload amfa/i })).toBeDisabled();
  });
});
