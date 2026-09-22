import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider, createTheme } from "@mui/material";
import { RealmsGrid } from "./RealmsGrid";

const theme = createTheme();

vi.mock("@/shared/components", () => ({
  LoadingSkeleton: () => <div>Loading realms...</div>,
  EmptyState: ({
    message,
    description,
  }: {
    message: string;
    description?: string;
  }) => (
    <div>
      <div>{message}</div>
      {description && <div>{description}</div>}
    </div>
  ),
}));

vi.mock("./RealmCard", () => ({
  RealmCard: ({
    realm,
    onClick,
  }: {
    realm: { realm_name: string };
    onClick: () => void;
  }) => (
    <div onClick={onClick}>
      <span>RealmCard: {realm.realm_name}</span>
    </div>
  ),
}));

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

describe("RealmsGrid", () => {
  const mockOnRealmClick = vi.fn();

  const mockRealms = [
    {
      realm_name: "realm-1",
      enabled: true,
      is_healthy: true,
      events_enabled: true,
      events_listeners: ["listener-1"],
      metrics: {
        total_users: 100,
        enabled_users: 95,
        active_sessions: 50,
        total_clients: 10,
        login_events: 500,
        failed_login_events: 25,
        logout_events: 300,
      },
    },
    {
      realm_name: "realm-2",
      enabled: true,
      is_healthy: false,
      events_enabled: false,
      events_listeners: [],
      metrics: {
        total_users: 50,
        enabled_users: 45,
        active_sessions: 20,
        total_clients: 5,
        login_events: 200,
        failed_login_events: 15,
        logout_events: 100,
      },
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should show loading state when loading is true", () => {
    renderWithTheme(
      <RealmsGrid realms={[]} onRealmClick={mockOnRealmClick} loading={true} />,
    );

    expect(screen.getByText("Loading realms...")).toBeInTheDocument();
  });

  it("should show empty state when no realms are available", () => {
    renderWithTheme(
      <RealmsGrid
        realms={[]}
        onRealmClick={mockOnRealmClick}
        loading={false}
      />,
    );

    expect(
      screen.getByText("No realms match your filters"),
    ).toBeInTheDocument();
  });

  it("should render realm cards when realms are available", () => {
    renderWithTheme(
      <RealmsGrid
        realms={mockRealms}
        onRealmClick={mockOnRealmClick}
        loading={false}
      />,
    );

    expect(screen.getByText("RealmCard: realm-1")).toBeInTheDocument();
    expect(screen.getByText("RealmCard: realm-2")).toBeInTheDocument();
  });

  it("should call onRealmClick with realm name when realm card is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(
      <RealmsGrid
        realms={mockRealms}
        onRealmClick={mockOnRealmClick}
        loading={false}
      />,
    );

    await user.click(screen.getByText("RealmCard: realm-1"));

    expect(mockOnRealmClick).toHaveBeenCalledWith("realm-1");
  });

  it("should render all realms in the grid", () => {
    renderWithTheme(
      <RealmsGrid
        realms={mockRealms}
        onRealmClick={mockOnRealmClick}
        loading={false}
      />,
    );

    expect(screen.getByText("RealmCard: realm-1")).toBeInTheDocument();
    expect(screen.getByText("RealmCard: realm-2")).toBeInTheDocument();
  });

  it("should not show loading state when loading is false", () => {
    renderWithTheme(
      <RealmsGrid
        realms={mockRealms}
        onRealmClick={mockOnRealmClick}
        loading={false}
      />,
    );

    expect(screen.queryByText("Loading realms...")).not.toBeInTheDocument();
  });

  it("should not show empty state when realms are available", () => {
    renderWithTheme(
      <RealmsGrid
        realms={mockRealms}
        onRealmClick={mockOnRealmClick}
        loading={false}
      />,
    );

    expect(
      screen.queryByText("No realms match your filters"),
    ).not.toBeInTheDocument();
  });

  it("shows a connection-error state instead of the empty state when connectionBroken", () => {
    renderWithTheme(
      <RealmsGrid
        realms={[]}
        onRealmClick={mockOnRealmClick}
        loading={false}
        connectionBroken
        connectionError="connect: connection refused"
      />,
    );

    expect(screen.getByText(/can't connect to keycloak/i)).toBeInTheDocument();
    expect(screen.getByText(/connection refused/i)).toBeInTheDocument();
    expect(
      screen.queryByText(/no realms match your filters/i),
    ).not.toBeInTheDocument();
  });

  it("shows the genuine empty state when not broken and zero realms", () => {
    renderWithTheme(
      <RealmsGrid
        realms={[]}
        onRealmClick={mockOnRealmClick}
        loading={false}
      />,
    );

    expect(
      screen.getByText(/no realms match your filters/i),
    ).toBeInTheDocument();
  });
});
