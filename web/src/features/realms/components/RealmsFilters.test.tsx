import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { RealmsFilters } from "./RealmsFilters";

describe("RealmsFilters", () => {
  const mockOnSearchChange = vi.fn();
  const mockOnStatusChange = vi.fn();
  const mockOnHealthChange = vi.fn();
  const mockOnEventWarningChange = vi.fn();
  const mockOnReset = vi.fn();
  const mockOnRealmChange = vi.fn();

  const mockRealms = [
    {
      realm_name: "master",
      enabled: true,
      is_healthy: true,
      events_enabled: true,
    },
    {
      realm_name: "test",
      enabled: false,
      is_healthy: false,
      events_enabled: false,
    },
  ];

  const defaultProps = {
    searchTerm: "",
    statusFilter: "",
    healthFilter: "",
    eventWarningFilter: null as boolean | null,
    onSearchChange: mockOnSearchChange,
    onStatusChange: mockOnStatusChange,
    onHealthChange: mockOnHealthChange,
    onEventWarningChange: mockOnEventWarningChange,
    onReset: mockOnReset,
    selectedRealm: "all",
    onRealmChange: mockOnRealmChange,
    realms: mockRealms,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render search input", () => {
    render(<RealmsFilters {...defaultProps} />);

    expect(screen.getByPlaceholderText(/Realm name.../i)).toBeInTheDocument();
  });

  it("should render status filter", () => {
    render(<RealmsFilters {...defaultProps} />);

    expect(screen.getAllByText("Status").length).toBeGreaterThanOrEqual(1);
  });

  it("should render health filter", () => {
    render(<RealmsFilters {...defaultProps} />);

    expect(screen.getAllByText("Health").length).toBeGreaterThanOrEqual(1);
  });

  it("should render event config filter", () => {
    render(<RealmsFilters {...defaultProps} />);

    expect(screen.getAllByText("Events").length).toBeGreaterThanOrEqual(1);
  });

  it("should render reset filters button", () => {
    render(<RealmsFilters {...defaultProps} searchTerm="test" />);

    expect(screen.getByRole("button", { name: /reset/i })).toBeInTheDocument();
  });

  it("should call onSearchChange when search input changes", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} />);

    const searchInput = screen.getByPlaceholderText(/Realm name.../i);
    await user.type(searchInput, "test");

    expect(mockOnSearchChange).toHaveBeenCalled();
    // user.type calls onChange for each character
    expect(mockOnSearchChange).toHaveBeenCalledTimes(4); // "test" has 4 characters
  });

  it("should call onStatusChange when status filter changes", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} />);

    // Find the Status label (first match is the label element)
    const statusLabels = screen.getAllByText("Status");
    const statusLabel = statusLabels.find((el) => el.tagName === "LABEL")!;
    const statusSelect =
      statusLabel.parentElement?.querySelector('[role="combobox"]');
    expect(statusSelect).toBeInTheDocument();
    await user.click(statusSelect!);
    await user.click(screen.getByRole("option", { name: /enabled/i }));

    expect(mockOnStatusChange).toHaveBeenCalledWith("enabled");
  });

  it("should call onHealthChange when health filter changes", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} />);

    const healthLabels = screen.getAllByText("Health");
    const healthLabel = healthLabels.find((el) => el.tagName === "LABEL")!;
    const healthSelect =
      healthLabel.parentElement?.querySelector('[role="combobox"]');
    expect(healthSelect).toBeInTheDocument();
    await user.click(healthSelect!);
    const healthyOptions = screen.getAllByRole("option", {
      name: /^healthy$/i,
    });
    await user.click(healthyOptions[0]);

    expect(mockOnHealthChange).toHaveBeenCalledWith("healthy");
  });

  it("should call onEventWarningChange when event config filter changes", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} />);

    const eventsLabels = screen.getAllByText("Events");
    const eventsLabel = eventsLabels.find((el) => el.tagName === "LABEL")!;
    const eventConfigSelect =
      eventsLabel.parentElement?.querySelector('[role="combobox"]');
    expect(eventConfigSelect).toBeInTheDocument();
    await user.click(eventConfigSelect!);
    await user.click(screen.getByRole("option", { name: /configured/i }));

    expect(mockOnEventWarningChange).toHaveBeenCalledWith(false);
  });

  it("should call onEventWarningChange with true for warning option", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} />);

    const eventsLabels = screen.getAllByText("Events");
    const eventsLabel = eventsLabels.find((el) => el.tagName === "LABEL")!;
    const eventConfigSelect =
      eventsLabel.parentElement?.querySelector('[role="combobox"]');
    expect(eventConfigSelect).toBeInTheDocument();
    await user.click(eventConfigSelect!);
    await user.click(screen.getByRole("option", { name: /warning/i }));

    expect(mockOnEventWarningChange).toHaveBeenCalledWith(true);
  });

  it("should call onEventWarningChange with null for all option", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} eventWarningFilter={true} />);

    const eventsLabels = screen.getAllByText("Events");
    const eventsLabel = eventsLabels.find((el) => el.tagName === "LABEL")!;
    const eventConfigSelect =
      eventsLabel.parentElement?.querySelector('[role="combobox"]');
    expect(eventConfigSelect).toBeInTheDocument();
    await user.click(eventConfigSelect!);
    const allOptions = screen.getAllByRole("option", { name: /^all$/i });
    await user.click(allOptions[allOptions.length - 1]); // Get the last "All" option

    expect(mockOnEventWarningChange).toHaveBeenCalledWith(null);
  });

  it("should call onReset when reset button is clicked", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} searchTerm="test" />);

    await user.click(screen.getByRole("button", { name: /reset/i }));

    expect(mockOnReset).toHaveBeenCalledTimes(1);
  });

  it("should display current search term", () => {
    render(<RealmsFilters {...defaultProps} searchTerm="test-realm" />);

    const searchInput = screen.getByPlaceholderText(/Realm name.../i);
    expect(searchInput).toHaveValue("test-realm");
  });

  it("should display current status filter", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} statusFilter="enabled" />);

    const statusLabels = screen.getAllByText("Status");
    const statusLabel = statusLabels.find((el) => el.tagName === "LABEL")!;
    const statusSelect =
      statusLabel.parentElement?.querySelector('[role="combobox"]');
    await user.click(statusSelect!);

    const enabledOption = screen.getByRole("option", { name: /^enabled$/i });
    expect(enabledOption).toHaveClass("Mui-selected");
  });

  it("should display current health filter", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} healthFilter="healthy" />);

    const healthLabels = screen.getAllByText("Health");
    const healthLabel = healthLabels.find((el) => el.tagName === "LABEL")!;
    const healthSelect =
      healthLabel.parentElement?.querySelector('[role="combobox"]');
    await user.click(healthSelect!);

    const healthyOption = screen.getByRole("option", { name: /^healthy$/i });
    expect(healthyOption).toHaveClass("Mui-selected");
  });

  it("should render status filter options", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} />);

    const statusLabels = screen.getAllByText("Status");
    const statusLabel = statusLabels.find((el) => el.tagName === "LABEL")!;
    const statusSelect =
      statusLabel.parentElement?.querySelector('[role="combobox"]');
    await user.click(statusSelect!);

    expect(screen.getByRole("option", { name: /^all$/i })).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: /^enabled$/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: /disabled/i }),
    ).toBeInTheDocument();
  });

  it("should render health filter options", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} />);

    const healthLabels = screen.getAllByText("Health");
    const healthLabel = healthLabels.find((el) => el.tagName === "LABEL")!;
    const healthSelect =
      healthLabel.parentElement?.querySelector('[role="combobox"]');
    await user.click(healthSelect!);

    expect(
      screen.getByRole("option", { name: /^healthy$/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: /unhealthy/i }),
    ).toBeInTheDocument();
  });

  it("should render event config filter options", async () => {
    const user = userEvent.setup();
    render(<RealmsFilters {...defaultProps} />);

    const eventsLabels = screen.getAllByText("Events");
    const eventsLabel = eventsLabels.find((el) => el.tagName === "LABEL")!;
    const eventConfigSelect =
      eventsLabel.parentElement?.querySelector('[role="combobox"]');
    await user.click(eventConfigSelect!);

    expect(
      screen.getByRole("option", { name: /configured/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: /warning/i }),
    ).toBeInTheDocument();
  });
});
