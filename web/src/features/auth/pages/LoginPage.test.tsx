import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { LoginPage } from "./LoginPage";
import { useAuth } from "@/shared/context";

vi.mock("@/shared/context", () => ({
  useAuth: vi.fn(),
}));

const mockLoginMutate = vi.fn();
const mockUseLoginSimple = vi.fn();

vi.mock("../hooks", () => ({
  useLoginSimple: () => mockUseLoginSimple(),
}));

// Default mock implementation
const defaultLoginSimpleMock = {
  mutate: mockLoginMutate,
  isPending: false,
  error: null,
};

describe("LoginPage", () => {
  const mockLoginSimple = vi.fn();
  const mockLoginOAuth2 = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    mockUseLoginSimple.mockReturnValue(defaultLoginSimpleMock);
  });

  it("should render login form when simple auth is enabled", () => {
    vi.mocked(useAuth).mockReturnValue({
      authConfig: {
        simple_enabled: true,
        oauth2_enabled: false,
        auth_required: true,
      },
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    expect(screen.getByLabelText(/username/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /sign in/i }),
    ).toBeInTheDocument();
  });

  it("should render OAuth2 button when OAuth2 is enabled", () => {
    vi.mocked(useAuth).mockReturnValue({
      authConfig: {
        simple_enabled: false,
        oauth2_enabled: true,
        auth_required: true,
      },
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    expect(screen.getByText(/sign in with sso/i)).toBeInTheDocument();
  });

  it("should render both auth methods when both are enabled", () => {
    vi.mocked(useAuth).mockReturnValue({
      authConfig: {
        simple_enabled: true,
        oauth2_enabled: true,
        auth_required: true,
      },
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    expect(screen.getByLabelText(/username/i)).toBeInTheDocument();
    expect(screen.getByText(/sign in with sso/i)).toBeInTheDocument();
    // Check for OR divider - look for exact text to avoid matching "organization's"
    expect(screen.getByText("OR")).toBeInTheDocument();
  });

  it("should call login mutation when form is submitted", async () => {
    const user = userEvent.setup();

    vi.mocked(useAuth).mockReturnValue({
      authConfig: {
        simple_enabled: true,
        oauth2_enabled: false,
        auth_required: true,
      },
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    const usernameInput = screen.getByLabelText(/username/i);
    const passwordInput = screen.getByLabelText(/password/i);
    const submitButton = screen.getByRole("button", { name: /sign in/i });

    await user.type(usernameInput, "testuser");
    await user.type(passwordInput, "password123");
    await user.click(submitButton);

    await waitFor(() => {
      expect(mockLoginMutate).toHaveBeenCalledWith({
        username: "testuser",
        password: "password123",
      });
    });
  });

  it("should display error message on login failure", () => {
    mockUseLoginSimple.mockReturnValue({
      mutate: mockLoginMutate,
      isPending: false,
      error: new Error("Invalid credentials"),
    });

    vi.mocked(useAuth).mockReturnValue({
      authConfig: {
        simple_enabled: true,
        oauth2_enabled: false,
        auth_required: true,
      },
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    expect(screen.getByText("Invalid credentials")).toBeInTheDocument();
  });

  it("should show loading state during login", () => {
    mockUseLoginSimple.mockReturnValue({
      mutate: mockLoginMutate,
      isPending: true,
      error: null,
    });

    vi.mocked(useAuth).mockReturnValue({
      authConfig: {
        simple_enabled: true,
        oauth2_enabled: false,
        auth_required: true,
      },
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    const submitButton = screen.getByRole("button", { name: /signing in/i });
    expect(submitButton).toBeDisabled();
  });

  it("should disable inputs during loading", () => {
    mockUseLoginSimple.mockReturnValue({
      mutate: mockLoginMutate,
      isPending: true,
      error: null,
    });

    vi.mocked(useAuth).mockReturnValue({
      authConfig: {
        simple_enabled: true,
        oauth2_enabled: false,
        auth_required: true,
      },
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    const usernameInput = screen.getByLabelText(/username/i);
    const passwordInput = screen.getByLabelText(/password/i);

    expect(usernameInput).toBeDisabled();
    expect(passwordInput).toBeDisabled();
  });

  it("should call loginOAuth2 when SSO button is clicked", async () => {
    const user = userEvent.setup();

    vi.mocked(useAuth).mockReturnValue({
      authConfig: {
        simple_enabled: false,
        oauth2_enabled: true,
        auth_required: true,
      },
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    const ssoButton = screen.getByText(/sign in with sso/i);
    await user.click(ssoButton);

    expect(mockLoginOAuth2).toHaveBeenCalled();
  });

  it("should render logo and branding", () => {
    vi.mocked(useAuth).mockReturnValue({
      authConfig: {
        simple_enabled: true,
        oauth2_enabled: false,
        auth_required: true,
      },
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    expect(screen.getByAltText("DefensePoint")).toBeInTheDocument();
    expect(screen.getByText("Keycloak")).toBeInTheDocument();
    expect(screen.getByText("MONITORING TOOL")).toBeInTheDocument();
  });

  it("should handle null auth config gracefully", () => {
    vi.mocked(useAuth).mockReturnValue({
      authConfig: null,
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    // Should not crash and both methods should be disabled by default
    expect(screen.queryByLabelText(/username/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/sign in with sso/i)).not.toBeInTheDocument();
  });

  it("should show error from hook when present", () => {
    mockUseLoginSimple.mockReturnValue({
      mutate: mockLoginMutate,
      isPending: false,
      error: new Error("Network error"),
    });

    vi.mocked(useAuth).mockReturnValue({
      authConfig: {
        simple_enabled: true,
        oauth2_enabled: false,
        auth_required: true,
      },
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    expect(screen.getByText("Network error")).toBeInTheDocument();
  });

  it("should not show error when no error present", () => {
    mockUseLoginSimple.mockReturnValue({
      mutate: mockLoginMutate,
      isPending: false,
      error: null,
    });

    vi.mocked(useAuth).mockReturnValue({
      authConfig: {
        simple_enabled: true,
        oauth2_enabled: false,
        auth_required: true,
      },
      loginSimple: mockLoginSimple,
      loginOAuth2: mockLoginOAuth2,
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(false),
      hasAnyPermission: vi.fn().mockReturnValue(false),
      hasRole: vi.fn().mockReturnValue(false),
      isAdmin: vi.fn().mockReturnValue(false),
      hasAccessToTenant: vi.fn().mockReturnValue(false),
    });

    render(<LoginPage />);

    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
