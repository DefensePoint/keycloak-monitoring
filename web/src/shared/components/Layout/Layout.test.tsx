import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import { BrowserRouter } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Layout } from "./Layout";
import { useAuth, useTenant } from "@/shared/context";
import { useIsAdmin } from "@/shared/hooks";
import type { UserInfo } from "@/shared/types";

vi.mock("@/shared/context", () => ({
  useAuth: vi.fn(),
  useTenant: vi.fn(),
}));

vi.mock("@/shared/hooks", () => ({
  useIsAdmin: vi.fn(),
}));

vi.mock("../TenantSelector", () => ({
  TenantSelector: () => (
    <div data-testid="tenant-selector">Tenant Selector</div>
  ),
}));

const mockUser: UserInfo = {
  subject: "test-subject-123",
  email: "test@example.com",
  email_verified: true,
  name: "Test User",
  given_name: "Test",
  family_name: "User",
  preferred_username: "testuser",
  locale: "en",
  updated_at: new Date().toISOString(),
};

const mockTenant = {
  id: 1,
  tenant_id: "test-tenant",
  name: "Test Tenant",
  enabled: true,
  server_url: "http://test.com",
  admin_realm: "master",
  client_id: "admin-cli",
  health_status: "healthy",
  created_at: "2023-01-01T00:00:00Z",
  updated_at: "2023-01-01T00:00:00Z",
  last_health_check: "2023-01-01T00:00:00Z",
};

describe("Layout", () => {
  let queryClient: QueryClient;
  const mockLogout = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });
    vi.mocked(useTenant).mockReturnValue({
      tenants: [mockTenant],
      selectedTenant: mockTenant,
      selectTenant: vi.fn(),
      isLoading: false,
      error: null,
      refreshTenants: vi.fn(),
    });
    vi.mocked(useIsAdmin).mockReturnValue({ isAdmin: false, isLoading: false });
  });

  const mockAuthContext = (user: UserInfo | null = mockUser) => ({
    user,
    logout: mockLogout,
    isAuthenticated: !!user,
    authConfig: null,
    isLoading: false,
    loginSimple: vi.fn(),
    loginOAuth2: vi.fn(),
    refreshUser: vi.fn(),
    hasPermission: vi.fn().mockReturnValue(false),
    hasAnyPermission: vi.fn().mockReturnValue(false),
    hasRole: vi.fn().mockReturnValue(false),
    isAdmin: vi.fn().mockReturnValue(false),
    hasAccessToTenant: vi.fn().mockReturnValue(false),
  });

  const renderWithRouter = (children: React.ReactNode, path = "/") => {
    window.history.pushState({}, "Test page", path);
    return render(
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>{children}</BrowserRouter>
      </QueryClientProvider>,
    );
  };

  it("should render children content", () => {
    vi.mocked(useAuth).mockReturnValue(mockAuthContext());

    renderWithRouter(
      <Layout>
        <div data-testid="test-content">Test Content</div>
      </Layout>,
    );

    expect(screen.getByTestId("test-content")).toBeInTheDocument();
  });

  it("should render logo and navigation", () => {
    vi.mocked(useAuth).mockReturnValue(mockAuthContext());

    renderWithRouter(
      <Layout>
        <div />
      </Layout>,
    );

    expect(screen.getByAltText("DefensePoint")).toBeInTheDocument();
    expect(screen.getByText("Keycloak")).toBeInTheDocument();
    expect(screen.getByText("Monitoring Tool")).toBeInTheDocument();
  });

  it("should render all navigation items", () => {
    vi.mocked(useAuth).mockReturnValue(mockAuthContext());

    renderWithRouter(
      <Layout>
        <div />
      </Layout>,
    );

    expect(screen.getByText("Dashboard")).toBeInTheDocument();
    expect(screen.getByText("Realms")).toBeInTheDocument();
    expect(screen.getByText("Events")).toBeInTheDocument();
    expect(screen.getByText("Health")).toBeInTheDocument();
    expect(screen.getByText("Settings")).toBeInTheDocument();
  });

  it("should display user info when user is present", () => {
    vi.mocked(useAuth).mockReturnValue(mockAuthContext());

    renderWithRouter(
      <Layout>
        <div />
      </Layout>,
    );

    expect(screen.getByText("Test User")).toBeInTheDocument();
    expect(screen.getByText("test@example.com")).toBeInTheDocument();
  });

  it("should display email when name is not present", () => {
    const userWithoutName: UserInfo = {
      subject: "test-subject-123",
      email: "test@example.com",
      email_verified: true,
      name: "",
      given_name: "",
      family_name: "",
      preferred_username: "testuser",
      locale: "en",
      updated_at: new Date().toISOString(),
    };

    vi.mocked(useAuth).mockReturnValue(mockAuthContext(userWithoutName));

    renderWithRouter(
      <Layout>
        <div />
      </Layout>,
    );

    const emailElements = screen.getAllByText("test@example.com");
    expect(emailElements.length).toBeGreaterThan(0);
  });

  it("should render logout button when user is present", () => {
    vi.mocked(useAuth).mockReturnValue(mockAuthContext());

    renderWithRouter(
      <Layout>
        <div />
      </Layout>,
    );

    expect(screen.getByText("Logout")).toBeInTheDocument();
  });

  it("should call logout when logout button is clicked", async () => {
    const user = userEvent.setup();
    vi.mocked(useAuth).mockReturnValue(mockAuthContext());

    renderWithRouter(
      <Layout>
        <div />
      </Layout>,
    );

    await user.click(screen.getByText("Logout"));

    expect(mockLogout).toHaveBeenCalled();
  });

  it("should not display user info when user is null", () => {
    vi.mocked(useAuth).mockReturnValue(mockAuthContext(null));

    renderWithRouter(
      <Layout>
        <div />
      </Layout>,
    );

    expect(screen.queryByText("Logout")).not.toBeInTheDocument();
  });

  it("should highlight active navigation item for root path", () => {
    vi.mocked(useAuth).mockReturnValue(mockAuthContext());

    renderWithRouter(
      <Layout>
        <div />
      </Layout>,
      "/test-tenant",
    );

    const dashboardButton = screen.getByText("Dashboard").closest("a");
    expect(dashboardButton).toHaveClass("Mui-selected");
  });

  it("should highlight active navigation item for nested paths", () => {
    vi.mocked(useAuth).mockReturnValue(mockAuthContext());

    renderWithRouter(
      <Layout>
        <div />
      </Layout>,
      "/test-tenant/events",
    );

    const eventsButton = screen.getByText("Events").closest("a");
    expect(eventsButton).toHaveClass("Mui-selected");
  });
});
