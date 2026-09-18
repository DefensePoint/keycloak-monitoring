import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import { SettingsPage } from "./SettingsPage";
import { useAuth, useTenant } from "@/shared/context";

const mockUseVersion = vi.fn();

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/tenant-1/settings" }),
  useParams: () => ({ tenantId: "tenant-1" }),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useAuth: vi.fn(),
  useTenant: vi.fn(),
}));

vi.mock("@/shared/hooks", () => ({
  useVersion: () => mockUseVersion(),
}));

const mockVersionInfo = {
  version: "1.0.0",
  git_commit: "abc123def",
  build_date: "2024-01-01T00:00:00Z",
  go_version: "go1.21.0",
  platform: "linux/amd64",
};

describe("SettingsPage", () => {
  const mockUser = {
    name: "Test User",
    email: "test@example.com",
    preferred_username: "testuser",
  };

  const mockSelectedTenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useAuth).mockReturnValue({
      user: mockUser,
      authConfig: null,
      loginSimple: vi.fn(),
      loginOAuth2: vi.fn(),
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(true),
      hasAnyPermission: vi.fn().mockReturnValue(true),
      hasRole: vi.fn().mockReturnValue(true),
      isAdmin: vi.fn().mockReturnValue(true),
      hasAccessToTenant: vi.fn().mockReturnValue(true),
    });

    vi.mocked(useTenant).mockReturnValue({
      selectedTenant: mockSelectedTenant,
      tenants: [mockSelectedTenant],
      setSelectedTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoadingTenants: false,
    });

    mockUseVersion.mockReturnValue({
      data: mockVersionInfo,
      isLoading: false,
      isError: false,
    });
  });

  it("should render settings page header", () => {
    render(<SettingsPage />);

    // "Settings" appears in breadcrumb and title
    expect(screen.getAllByText("Settings").length).toBeGreaterThanOrEqual(1);
    expect(
      screen.getByText("Configure application preferences and user settings"),
    ).toBeInTheDocument();
  });

  it("should display user profile section", () => {
    render(<SettingsPage />);

    expect(screen.getByText("User Profile")).toBeInTheDocument();
    expect(screen.getByText("Full Name")).toBeInTheDocument();
    expect(screen.getByText("Test User")).toBeInTheDocument();
    expect(screen.getByText("Email")).toBeInTheDocument();
    expect(screen.getByText("test@example.com")).toBeInTheDocument();
    expect(screen.getByText("Username")).toBeInTheDocument();
    expect(screen.getByText("testuser")).toBeInTheDocument();
  });

  it("should display display settings section", () => {
    render(<SettingsPage />);

    expect(screen.getByText("Display Settings")).toBeInTheDocument();
    expect(screen.getByText("Theme")).toBeInTheDocument();
    expect(screen.getByText("Current: Dark (Default)")).toBeInTheDocument();
    expect(screen.getByText("Timezone")).toBeInTheDocument();
  });

  it("should show coming soon badges for unavailable features", () => {
    render(<SettingsPage />);

    const comingSoonBadges = screen.getAllByText("Coming Soon");
    expect(comingSoonBadges.length).toBeGreaterThan(0);
  });

  it("should display notification settings", () => {
    render(<SettingsPage />);

    expect(screen.getByText("Notifications")).toBeInTheDocument();
    expect(screen.getByText("Email Notifications")).toBeInTheDocument();
    expect(screen.getByText("Receive alerts via email")).toBeInTheDocument();
    expect(screen.getByText("Slack Integration")).toBeInTheDocument();
  });

  it("should display security settings", () => {
    render(<SettingsPage />);

    expect(screen.getByText("Security")).toBeInTheDocument();
    expect(screen.getByText("Session Timeout")).toBeInTheDocument();
    expect(screen.getByText("24 hours")).toBeInTheDocument();
    expect(screen.getByText("Two-Factor Authentication")).toBeInTheDocument();
  });

  it("should display monitoring configuration", () => {
    render(<SettingsPage />);

    expect(screen.getByText("Monitoring Configuration")).toBeInTheDocument();
    expect(screen.getByText("Event Collection Interval")).toBeInTheDocument();
    expect(screen.getByText("30 seconds")).toBeInTheDocument();
    expect(screen.getByText("Metrics Refresh Interval")).toBeInTheDocument();
    expect(screen.getByText("10 seconds")).toBeInTheDocument();
  });

  it("should display about section with version info", async () => {
    render(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText("About")).toBeInTheDocument();
    });

    expect(screen.getByText("Keycloak Monitoring Tool")).toBeInTheDocument();
    expect(screen.getByText("1.0.0")).toBeInTheDocument();
    expect(screen.getByText("abc123def")).toBeInTheDocument();
    expect(screen.getByText("go1.21.0")).toBeInTheDocument();
    expect(screen.getByText("linux/amd64")).toBeInTheDocument();
  });

  it("should display default version when version info not available", () => {
    mockUseVersion.mockReturnValue({
      data: null,
      isLoading: false,
      isError: false,
    });

    render(<SettingsPage />);

    expect(screen.getByText("0.1.0")).toBeInTheDocument();
  });

  it("should display auto-detected timezone", () => {
    render(<SettingsPage />);

    const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    expect(screen.getByText(`Current: ${timezone}`)).toBeInTheDocument();
    expect(screen.getByText("Auto-detected")).toBeInTheDocument();
  });

  it("should handle user without name", () => {
    vi.mocked(useAuth).mockReturnValue({
      user: { email: "test@example.com", preferred_username: "testuser" },
      authConfig: null,
      loginSimple: vi.fn(),
      loginOAuth2: vi.fn(),
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(true),
      hasAnyPermission: vi.fn().mockReturnValue(true),
      hasRole: vi.fn().mockReturnValue(true),
      isAdmin: vi.fn().mockReturnValue(true),
      hasAccessToTenant: vi.fn().mockReturnValue(true),
    });

    render(<SettingsPage />);

    expect(screen.queryByText("Full Name")).not.toBeInTheDocument();
    expect(screen.getByText("test@example.com")).toBeInTheDocument();
  });
});
