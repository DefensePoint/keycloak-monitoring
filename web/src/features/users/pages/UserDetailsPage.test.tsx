import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { UserDetailsPage } from "./UserDetailsPage";
import { useTenant } from "@/shared/context";

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
  }: {
    onReset: () => void;
    onTimeRangeChange: (start: Date | null, end: Date | null) => void;
  }) => (
    <div data-testid="user-events-filters">
      <button onClick={onReset}>Reset</button>
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
