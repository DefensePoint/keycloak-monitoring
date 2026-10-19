import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { UserDetailsPage } from "./UserDetailsPage";
import { useTenant } from "@/shared/context";
import { eventService } from "../services";

const mockUseQuery = vi.fn();
const mockUseNavigate = vi.fn();

vi.mock("react-router-dom", () => ({
  useParams: () => ({ realmName: "master", userId: "user-123" }),
  useNavigate: () => mockUseNavigate,
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: (config: { queryKey: string[]; queryFn: () => Promise<unknown> }) =>
    mockUseQuery(config),
}));

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
}));

vi.mock("../services", () => ({
  eventService: { getEvents: vi.fn() },
  userService: { getUserDetails: vi.fn() },
}));

vi.mock("../components", () => ({
  UserDetailsHeader: () => (
    <div data-testid="user-header">User Details Header</div>
  ),
  UserInfoCard: ({
    userDetails,
    loading,
  }: {
    userDetails: unknown;
    loading: boolean;
  }) => (
    <div data-testid="user-info-card">
      {loading ? "Loading..." : userDetails ? "User Info" : "No user"}
    </div>
  ),
  UserEventsFilters: ({
    onReset,
    onTimeRangeChange,
    onSeverityChange,
  }: {
    onReset: () => void;
    onTimeRangeChange: (start: Date | null, end: Date | null) => void;
    onSeverityChange: (severity: string) => void;
  }) => (
    <div data-testid="user-events-filters">
      <button onClick={onReset}>Reset</button>
      <button onClick={() => onSeverityChange("error")}>Set Severity</button>
      <button onClick={() => onTimeRangeChange(new Date(), new Date())}>
        Set Time Range
      </button>
    </div>
  ),
  UserEventsTable: ({
    events,
    loading,
    onEventClick,
  }: {
    events: unknown[];
    loading: boolean;
    onEventClick: (event: { event_id: string }) => void;
  }) => (
    <div data-testid="user-events-table">
      {loading ? (
        "Loading events..."
      ) : (
        <>
          <div>Events: {events.length}</div>
          {events.map((event: { event_id: string; type: string }) => (
            <div key={event.event_id} onClick={() => onEventClick(event)}>
              {event.type}
            </div>
          ))}
        </>
      )}
    </div>
  ),
}));

const mockUserDetailsResponse = {
  user: {
    id: "user-123",
    username: "testuser",
    email: "test@example.com",
    firstName: "Test",
    lastName: "User",
    enabled: true,
    emailVerified: true,
    createdTimestamp: 1640000000000,
  },
  groups: [
    {
      id: "group-1",
      name: "test-group",
      path: "/test-group",
    },
  ],
  roleMappings: {
    realmRoles: ["user", "offline_access"],
    clientRoles: {
      "test-client": ["client-role-1"],
    },
  },
};

const mockEventsResponse = {
  events: [
    {
      event_id: "event-1",
      type: "LOGIN",
      timestamp: "2024-01-01T00:00:00Z",
      severity: "info",
      description: "User logged in",
      source: "keycloak",
      user_id: "user-123",
      username: "testuser",
      client_id: "admin-cli",
    },
    {
      event_id: "event-2",
      type: "UPDATE_PROFILE",
      timestamp: "2024-01-02T00:00:00Z",
      severity: "info",
      description: "User updated profile",
      source: "keycloak",
      user_id: "user-123",
      username: "testuser",
      client_id: "account",
    },
    {
      event_id: "event-3",
      type: "LOGIN",
      timestamp: "2024-01-03T00:00:00Z",
      severity: "info",
      description: "Another user logged in",
      source: "keycloak",
      user_id: "user-456",
      username: "otheruser",
      client_id: "admin-cli",
    },
  ],
  total: 3,
};

describe("UserDetailsPage", () => {
  const mockSelectedTenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useTenant).mockReturnValue({
      selectedTenant: mockSelectedTenant,
      tenants: [mockSelectedTenant],
      setSelectedTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoadingTenants: false,
    });

    mockUseQuery.mockImplementation(({ queryKey }) => {
      if (queryKey[0] === "user-details") {
        return {
          data: mockUserDetailsResponse,
          isLoading: false,
          isError: false,
        };
      }
      if (queryKey[0] === "user-events") {
        return {
          data: mockEventsResponse,
          isLoading: false,
          isError: false,
        };
      }
      return { data: null, isLoading: false, isError: false };
    });
  });

  it("should render user details header", async () => {
    render(<UserDetailsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("user-header")).toBeInTheDocument();
    });
  });

  it("should display user info card", async () => {
    render(<UserDetailsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("user-info-card")).toBeInTheDocument();
    });

    expect(screen.getByText("User Info")).toBeInTheDocument();
  });

  it("should display user events filters", async () => {
    render(<UserDetailsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("user-events-filters")).toBeInTheDocument();
    });
  });

  it("should display user events table", async () => {
    render(<UserDetailsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("user-events-table")).toBeInTheDocument();
    });
  });

  it("should filter events for the specific user", async () => {
    render(<UserDetailsPage />);

    await waitFor(() => {
      expect(screen.getByText("Events: 2")).toBeInTheDocument();
    });

    // Should only show events for user-123, not user-456
    expect(screen.getByText("LOGIN")).toBeInTheDocument();
    expect(screen.getByText("UPDATE_PROFILE")).toBeInTheDocument();
  });

  it("should show loading state while fetching user details", () => {
    mockUseQuery.mockImplementation(({ queryKey }) => {
      if (queryKey[0] === "user-details") {
        return {
          data: null,
          isLoading: true,
          isError: false,
        };
      }
      return {
        data: mockEventsResponse,
        isLoading: false,
        isError: false,
      };
    });

    render(<UserDetailsPage />);

    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("should show loading state while fetching events", () => {
    mockUseQuery.mockImplementation(({ queryKey }) => {
      if (queryKey[0] === "user-events") {
        return {
          data: null,
          isLoading: true,
          isError: false,
        };
      }
      return {
        data: mockUserDetailsResponse,
        isLoading: false,
        isError: false,
      };
    });

    render(<UserDetailsPage />);

    expect(screen.getByText("Loading events...")).toBeInTheDocument();
  });

  it("should reset filters when reset button is clicked", async () => {
    const user = userEvent.setup();
    render(<UserDetailsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("user-events-filters")).toBeInTheDocument();
    });

    const resetButton = screen.getByText("Reset");
    await user.click(resetButton);

    // Events should still be displayed after reset
    await waitFor(() => {
      expect(screen.getByText("Events: 2")).toBeInTheDocument();
    });
  });

  it("should handle time range change", async () => {
    const user = userEvent.setup();
    render(<UserDetailsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("user-events-filters")).toBeInTheDocument();
    });

    const timeRangeButton = screen.getByText("Set Time Range");
    await user.click(timeRangeButton);

    // Query should be refetched with new time range
    await waitFor(() => {
      expect(mockUseQuery).toHaveBeenCalled();
    });
  });

  it("should navigate to events page when event is clicked", async () => {
    const user = userEvent.setup();
    render(<UserDetailsPage />);

    await waitFor(() => {
      expect(screen.getByText("LOGIN")).toBeInTheDocument();
    });

    const loginEvent = screen.getByText("LOGIN");
    await user.click(loginEvent);

    expect(mockUseNavigate).toHaveBeenCalledWith(
      "/tenant-1/events?event=event-1",
    );
  });

  it("should handle empty events list", async () => {
    mockUseQuery.mockImplementation(({ queryKey }) => {
      if (queryKey[0] === "user-events") {
        return {
          data: { events: [], total: 0 },
          isLoading: false,
          isError: false,
        };
      }
      return {
        data: mockUserDetailsResponse,
        isLoading: false,
        isError: false,
      };
    });

    render(<UserDetailsPage />);

    await waitFor(() => {
      expect(screen.getByText("Events: 0")).toBeInTheDocument();
    });
  });

  it("should filter events by user_id only showing user-specific events", async () => {
    render(<UserDetailsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("user-events-table")).toBeInTheDocument();
    });

    // Should show 2 events (event-1 and event-2), not 3
    expect(screen.getByText("Events: 2")).toBeInTheDocument();
  });

  describe("severity filter", () => {
    const lastUserEventsQuery = () =>
      mockUseQuery.mock.calls
        .map(([config]) => config)
        .filter((config) => config.queryKey[0] === "user-events")
        .pop();

    it("is sent to the server rather than applied to the loaded rows", async () => {
      const user = userEvent.setup();
      render(<UserDetailsPage />);

      await user.click(screen.getByText("Set Severity"));

      await waitFor(() => {
        expect(lastUserEventsQuery().queryKey).toContain("error");
      });
      await lastUserEventsQuery().queryFn();
      expect(eventService.getEvents).toHaveBeenLastCalledWith(
        "tenant-1",
        1000,
        0,
        undefined,
        undefined,
        { severity: "error", userId: "user-123", realm: "master" },
      );

      // The mocked rows are all "info". The server already applied the
      // filter, so the page must not drop them a second time.
      expect(screen.getByText("Events: 2")).toBeInTheDocument();
    });
  });

  describe("user scoping", () => {
    const userEventsQuery = () =>
      mockUseQuery.mock.calls
        .map(([config]) => config)
        .filter((config) => config.queryKey[0] === "user-events")
        .pop();

    const userRows = (n: number) =>
      Array.from({ length: n }, (_, i) => ({
        event_id: `own-${i}`,
        type: "LOGIN",
        timestamp: "2024-01-01T00:00:00Z",
        severity: "info",
        description: `own event ${i}`,
        source: "keycloak:master",
        user_id: "user-123",
      }));

    const respondWith = (events: unknown[], total: number) =>
      mockUseQuery.mockImplementation(({ queryKey }) => {
        if (queryKey[0] === "user-details") {
          return { data: mockUserDetailsResponse, isLoading: false };
        }
        if (queryKey[0] === "user-events") {
          return { data: { events, total }, isLoading: false };
        }
        return { data: null, isLoading: false };
      });

    it("asks the server for this user's events only", async () => {
      render(<UserDetailsPage />);

      await userEventsQuery().queryFn();
      expect(eventService.getEvents).toHaveBeenLastCalledWith(
        "tenant-1",
        1000,
        0,
        undefined,
        undefined,
        { severity: undefined, userId: "user-123", realm: "master" },
      );
    });

    it("asks for the realm named in the URL, not every realm", async () => {
      render(<UserDetailsPage />);

      // The realm is part of the query key, so moving to the same user ID in
      // another realm fetches again instead of reusing this realm's rows.
      expect(userEventsQuery().queryKey).toContain("master");
      await userEventsQuery().queryFn();
      const filters = vi.mocked(eventService.getEvents).mock.lastCall?.[5];
      expect(filters?.realm).toBe("master");
    });

    it("hands every loaded event to the table, not just the first page's worth", async () => {
      respondWith(userRows(60), 60);
      render(<UserDetailsPage />);

      expect(await screen.findByText("Events: 60")).toBeInTheDocument();
    });

    it("says so when the user has more events than were loaded", async () => {
      respondWith(userRows(60), 1500);
      render(<UserDetailsPage />);

      expect(
        await screen.findByText(/Showing the newest 60 of 1500 events/),
      ).toBeInTheDocument();
    });

    it("makes no count claim when the server ignored user_id", async () => {
      // An API that predates the user_id filter answers with the tenant's
      // events, so its total is the tenant's, not this user's. Found end to
      // end: the banner read "newest 1000 of 1141 events for this user"
      // above a single row.
      const tenantWide = [
        ...userRows(1),
        ...Array.from({ length: 999 }, (_, i) => ({
          ...userRows(1)[0],
          event_id: `other-${i}`,
          user_id: "user-999",
        })),
      ];
      respondWith(tenantWide, 1141);
      render(<UserDetailsPage />);

      expect(await screen.findByText("Events: 1")).toBeInTheDocument();
      expect(screen.queryByText(/of 1141 events/)).not.toBeInTheDocument();
      expect(screen.getByText(/could not filter by user/i)).toBeInTheDocument();
    });

    it("still warns when the count failed and the list hit the cap", async () => {
      // /events answers total 0 when its count query fails but the list
      // succeeds. A capped list with an unknown total may be partial, so the
      // page must not let it look complete, nor claim "of 0".
      respondWith(userRows(1000), 0);
      render(<UserDetailsPage />);

      expect(
        await screen.findByText(
          /Showing the newest 1000 events for this user; there may be more/,
        ),
      ).toBeInTheDocument();
      expect(screen.queryByText(/of 0 events/)).not.toBeInTheDocument();
    });

    it("stays quiet when the count failed but the list was not capped", async () => {
      respondWith(userRows(5), 0);
      render(<UserDetailsPage />);

      await screen.findByText("Events: 5");
      expect(screen.queryByText(/Showing the newest/)).not.toBeInTheDocument();
    });

    it("shows no truncation notice when everything was loaded", async () => {
      respondWith(userRows(60), 60);
      render(<UserDetailsPage />);

      await screen.findByText("Events: 60");
      expect(screen.queryByText(/Showing the newest/)).not.toBeInTheDocument();
    });
  });

  it("should handle missing user details", () => {
    mockUseQuery.mockImplementation(({ queryKey }) => {
      if (queryKey[0] === "user-details") {
        return {
          data: null,
          isLoading: false,
          isError: false,
        };
      }
      return {
        data: mockEventsResponse,
        isLoading: false,
        isError: false,
      };
    });

    render(<UserDetailsPage />);

    expect(screen.getByText("No user")).toBeInTheDocument();
  });
});
