import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import { RoleManagement } from "./RoleManagement";
import { useRoles, usePermissions } from "../hooks";

vi.mock("../hooks", () => ({
  useRoles: vi.fn(),
  usePermissions: vi.fn(),
  useUserRoles: vi.fn(),
  useAssignRole: vi.fn(),
  useRevokeRole: vi.fn(),
}));

vi.mock("@/shared/components", () => ({
  LoadingSkeleton: () => (
    <div data-testid="loading-state">Loading roles and permissions...</div>
  ),
  AlertBanner: ({ message }: { message: React.ReactNode }) => (
    <div data-testid="alert-banner">{message}</div>
  ),
  EmptyState: ({ message }: { message: string }) => (
    <div data-testid="empty-state">{message}</div>
  ),
  PermissionGate: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

describe("RoleManagement", () => {
  const mockRoles = [
    {
      id: 1,
      name: "admin",
      display_name: "Admin",
      description: "Admin role",
      is_system: true,
      permissions: [],
    },
    {
      id: 2,
      name: "viewer",
      display_name: "Viewer",
      description: "Viewer role",
      is_system: false,
      permissions: [],
    },
  ];

  const mockPermissions = [
    {
      id: 1,
      name: "users:read",
      display_name: "Read Users",
      description: "Can read users",
      resource: "users",
      action: "read",
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should show loading state when roles are loading", () => {
    vi.mocked(useRoles).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      refetch: vi.fn(),
    } as ReturnType<typeof useRoles>);

    vi.mocked(usePermissions).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as ReturnType<typeof usePermissions>);

    render(<RoleManagement />);
    expect(screen.getByTestId("loading-state")).toBeInTheDocument();
    expect(
      screen.getByText("Loading roles and permissions..."),
    ).toBeInTheDocument();
  });

  it("should display roles and permissions when loaded", () => {
    vi.mocked(useRoles).mockReturnValue({
      data: mockRoles,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as ReturnType<typeof useRoles>);

    vi.mocked(usePermissions).mockReturnValue({
      data: mockPermissions,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as ReturnType<typeof usePermissions>);

    render(<RoleManagement />);

    expect(screen.getByText("Role Management")).toBeInTheDocument();
    expect(screen.getByText("Roles (2)")).toBeInTheDocument();
    expect(screen.getByText("All Permissions (1)")).toBeInTheDocument();
    expect(screen.getByText("Admin")).toBeInTheDocument();
    expect(screen.getByText("Viewer")).toBeInTheDocument();
  });

  it("should display error when fetch fails", () => {
    vi.mocked(useRoles).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed to fetch roles"),
      refetch: vi.fn(),
    } as ReturnType<typeof useRoles>);

    vi.mocked(usePermissions).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as ReturnType<typeof usePermissions>);

    render(<RoleManagement />);

    expect(screen.getByTestId("alert-banner")).toBeInTheDocument();
  });

  it("should display empty state when no role is selected", () => {
    vi.mocked(useRoles).mockReturnValue({
      data: mockRoles,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as ReturnType<typeof useRoles>);

    vi.mocked(usePermissions).mockReturnValue({
      data: mockPermissions,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as ReturnType<typeof usePermissions>);

    render(<RoleManagement />);

    expect(
      screen.getByText("Select a role to view details"),
    ).toBeInTheDocument();
  });
});
