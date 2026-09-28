import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider, createTheme } from "@mui/material";
import { RealmCard } from "./RealmCard";

const theme = createTheme();

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

describe("RealmCard", () => {
  const mockOnClick = vi.fn();

  const mockRealm = {
    realm_name: "test-realm",
    enabled: true,
    is_healthy: true,
    events_enabled: true,
    events_listeners: ["event-listener-1"],
    metrics: {
      total_users: 100,
      enabled_users: 95,
      active_sessions: 50,
      total_clients: 10,
      login_events: 500,
      failed_login_events: 25,
      logout_events: 300,
    },
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render realm name", () => {
    renderWithTheme(<RealmCard realm={mockRealm} onClick={mockOnClick} />);

    expect(screen.getByText("test-realm")).toBeInTheDocument();
  });

  it("should render healthy badge when realm is healthy", () => {
    renderWithTheme(<RealmCard realm={mockRealm} onClick={mockOnClick} />);

    expect(screen.getByText("Healthy")).toBeInTheDocument();
  });

  it("should render unhealthy badge when realm is not healthy", () => {
    const unhealthyRealm = { ...mockRealm, is_healthy: false };
    renderWithTheme(<RealmCard realm={unhealthyRealm} onClick={mockOnClick} />);

    expect(screen.getByText("Unhealthy")).toBeInTheDocument();
  });

  it("should show warning icon when events are not configured", () => {
    const realmWithoutEvents = {
      ...mockRealm,
      events_enabled: false,
    };
    renderWithTheme(
      <RealmCard realm={realmWithoutEvents} onClick={mockOnClick} />,
    );

    expect(
      screen.getByLabelText(/Event collection is not properly configured/i),
    ).toBeInTheDocument();
  });

  it("should show warning icon when event listeners are empty", () => {
    const realmWithoutListeners = {
      ...mockRealm,
      events_listeners: [],
    };
    renderWithTheme(
      <RealmCard realm={realmWithoutListeners} onClick={mockOnClick} />,
    );

    expect(
      screen.getByLabelText(/Event collection is not properly configured/i),
    ).toBeInTheDocument();
  });

  it("should display metrics when available", () => {
    renderWithTheme(<RealmCard realm={mockRealm} onClick={mockOnClick} />);

    expect(screen.getByText(/100 \(95 enabled\)/i)).toBeInTheDocument();
    expect(screen.getByText(/50 active/i)).toBeInTheDocument();
    expect(screen.getByText("10")).toBeInTheDocument();
    expect(screen.getByText("500")).toBeInTheDocument();
    expect(screen.getByText("25")).toBeInTheDocument();
    expect(screen.getByText("300")).toBeInTheDocument();
  });

  it("should call onClick when card is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(<RealmCard realm={mockRealm} onClick={mockOnClick} />);

    const card = screen.getByText("test-realm").closest("div");
    await user.click(card!);

    expect(mockOnClick).toHaveBeenCalledTimes(1);
  });

  it("should not show metrics section when metrics are not available", () => {
    const realmWithoutMetrics = {
      ...mockRealm,
      metrics: undefined,
    };
    renderWithTheme(
      <RealmCard realm={realmWithoutMetrics} onClick={mockOnClick} />,
    );

    expect(screen.queryByText("Users")).not.toBeInTheDocument();
  });

  it("should render unhealthy badge for disabled realm", () => {
    const disabledRealm = {
      ...mockRealm,
      enabled: false,
      is_healthy: false,
    };
    renderWithTheme(<RealmCard realm={disabledRealm} onClick={mockOnClick} />);

    expect(screen.getByText("Unhealthy")).toBeInTheDocument();
  });
});
