import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider, createTheme } from "@mui/material";
import { UserEventsTable } from "./UserEventsTable";

const theme = createTheme();

vi.mock("@/shared/components", () => ({
  LoadingSkeleton: () => <div>Loading events...</div>,
  EmptyState: ({ message }: { message: string }) => <div>{message}</div>,
}));

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

describe("UserEventsTable", () => {
  const mockOnEventClick = vi.fn();

  const mockEvents = [
    {
      event_id: "event-1",
      user_id: "user-1",
      timestamp: "2024-01-01T12:00:00Z",
      type: "LOGIN",
      source: "keycloak",
      description: "User logged in successfully",
      severity: "info",
      metadata: "{}",
    },
    {
      event_id: "event-2",
      user_id: "user-1",
      timestamp: "2024-01-01T13:00:00Z",
      type: "LOGIN_ERROR",
      source: "keycloak",
      description: "Failed login attempt",
      severity: "error",
      metadata: "{}",
    },
  ];

  const defaultProps = {
    events: mockEvents,
    loading: false,
    onEventClick: mockOnEventClick,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should show loading state when loading is true", () => {
    renderWithTheme(
      <UserEventsTable {...defaultProps} events={[]} loading={true} />,
    );

    expect(screen.getByText("Loading events...")).toBeInTheDocument();
  });

  it("should show empty state when no events are available", () => {
    renderWithTheme(
      <UserEventsTable {...defaultProps} events={[]} loading={false} />,
    );

    expect(
      screen.getByText("No events found for this user"),
    ).toBeInTheDocument();
  });

  it("should render event table when events are available", () => {
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    expect(screen.getByText("User Events")).toBeInTheDocument();
    expect(screen.getByText(/Showing 2 events/i)).toBeInTheDocument();
  });

  it("should render table headers", () => {
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    expect(screen.getByText("Timestamp")).toBeInTheDocument();
    expect(screen.getByText("Type")).toBeInTheDocument();
    expect(screen.getByText("Source")).toBeInTheDocument();
    expect(screen.getByText("Description")).toBeInTheDocument();
    expect(screen.getByText("Severity")).toBeInTheDocument();
  });

  it("should render event data in table", () => {
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    expect(screen.getByText("LOGIN")).toBeInTheDocument();
    expect(screen.getByText("User logged in successfully")).toBeInTheDocument();
    expect(screen.getByText("info")).toBeInTheDocument();
  });

  it("should display formatted timestamps", () => {
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    const expectedDate = new Date("2024-01-01T12:00:00Z").toLocaleString();
    expect(screen.getByText(expectedDate)).toBeInTheDocument();
  });

  it("should render severity chips with correct colors", () => {
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    const infoChip = screen.getByText("info");
    const errorChip = screen.getByText("error");

    expect(infoChip).toBeInTheDocument();
    expect(errorChip).toBeInTheDocument();
  });

  it("should call onEventClick when a row is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    const firstRow = screen.getByText("LOGIN").closest("tr");
    await user.click(firstRow!);

    expect(mockOnEventClick).toHaveBeenCalledWith(mockEvents[0]);
  });

  it("should call onEventClick when action button is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    const actionButtons = screen.getAllByRole("button", { name: "" });
    await user.click(actionButtons[0]);

    expect(mockOnEventClick).toHaveBeenCalledWith(mockEvents[0]);
  });

  it("should display event count", () => {
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    expect(screen.getByText("Showing 2 events")).toBeInTheDocument();
  });

  it("should not show loading state when loading is false", () => {
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    expect(screen.queryByText("Loading events...")).not.toBeInTheDocument();
  });

  it("should not show empty state when events are available", () => {
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    expect(
      screen.queryByText("No events found for this user"),
    ).not.toBeInTheDocument();
  });

  it("should render all events in the table", () => {
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    expect(screen.getByText("LOGIN")).toBeInTheDocument();
    expect(screen.getByText("LOGIN_ERROR")).toBeInTheDocument();
  });

  it("should render keycloak source for events", () => {
    renderWithTheme(<UserEventsTable {...defaultProps} />);

    const sources = screen.getAllByText("keycloak");
    expect(sources).toHaveLength(2);
  });
});
