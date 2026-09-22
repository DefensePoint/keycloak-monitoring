import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { UserRolesDisplay, UserRoleBadge } from "./UserRolesDisplay";
import * as usePermissionHooks from "@/shared/hooks";

vi.mock("@/shared/hooks", () => ({
  useUserRoles: vi.fn(() => ({
    roles: [],
    isLoading: false,
  })),
  useUserPermissions: vi.fn(() => ({
    permissions: [],
    isLoading: false,
  })),
  useIsAdmin: vi.fn(() => ({
    isAdmin: false,
  })),
}));

describe("UserRolesDisplay", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should show loading skeleton when loading", () => {
    vi.mocked(usePermissionHooks.useUserRoles).mockReturnValue({
      roles: [],
      isLoading: true,
    });
    vi.mocked(usePermissionHooks.useUserPermissions).mockReturnValue({
      permissions: [],
      isLoading: false,
    });

    const { container } = render(<UserRolesDisplay />);

    // MUI Skeleton uses span elements with the MuiSkeleton class
    const skeletons = container.querySelectorAll(".MuiSkeleton-root");
    expect(skeletons).toHaveLength(2);
  });

  it("should display no roles message when user has no roles", () => {
    vi.mocked(usePermissionHooks.useUserRoles).mockReturnValue({
      roles: [],
      isLoading: false,
    });
    vi.mocked(usePermissionHooks.useUserPermissions).mockReturnValue({
      permissions: [],
      isLoading: false,
    });

    render(<UserRolesDisplay />);

    expect(screen.getByText("No roles assigned")).toBeInTheDocument();
  });

  it("should display user roles", () => {
    vi.mocked(usePermissionHooks.useUserRoles).mockReturnValue({
      roles: [
        {
          id: "role-1",
          role: { name: "viewer", display_name: "Viewer" },
          tenant_id: null,
        },
        {
          id: "role-2",
          role: { name: "editor", display_name: "Editor" },
          tenant_id: "tenant-1",
        },
      ],
      isLoading: false,
    });

    render(<UserRolesDisplay />);

    expect(screen.getByText("Viewer")).toBeInTheDocument();
    expect(screen.getByText("Editor")).toBeInTheDocument();
    expect(screen.getByText("(tenant-1)")).toBeInTheDocument();
  });

  it("should display admin badge when user is admin", () => {
    vi.mocked(usePermissionHooks.useUserRoles).mockReturnValue({
      roles: [
        {
          id: "role-1",
          role: { name: "admin", display_name: "Admin" },
          tenant_id: null,
        },
      ],
      isLoading: false,
    });
    vi.mocked(usePermissionHooks.useIsAdmin).mockReturnValue({ isAdmin: true });

    render(<UserRolesDisplay />);

    expect(screen.getByText("ADMIN")).toBeInTheDocument();
  });

  it("should display no permissions message when user has no permissions", () => {
    render(<UserRolesDisplay />);

    expect(screen.getByText("Your Permissions (0)")).toBeInTheDocument();
  });

  it("should display user permissions in accordion", async () => {
    const user = userEvent.setup();
    vi.mocked(usePermissionHooks.useUserPermissions).mockReturnValue({
      permissions: ["alerts:read", "alerts:write", "users:read"],
      isLoading: false,
    });

    render(<UserRolesDisplay />);

    expect(screen.getByText("Your Permissions (3)")).toBeInTheDocument();

    // Expand accordion
    await user.click(screen.getByText("Your Permissions (3)"));

    expect(screen.getByText("alerts:read")).toBeInTheDocument();
    expect(screen.getByText("alerts:write")).toBeInTheDocument();
    expect(screen.getByText("users:read")).toBeInTheDocument();
  });

  it("should show role name when display_name is not available", () => {
    vi.mocked(usePermissionHooks.useUserRoles).mockReturnValue({
      roles: [
        {
          id: "role-1",
          role: { name: "custom-role" },
          tenant_id: null,
        },
      ],
      isLoading: false,
    });

    render(<UserRolesDisplay />);

    expect(screen.getByText("custom-role")).toBeInTheDocument();
  });
});

describe("UserRoleBadge", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should not render when user has no roles", () => {
    vi.mocked(usePermissionHooks.useUserRoles).mockReturnValue({
      roles: [],
      isLoading: false,
    });
    vi.mocked(usePermissionHooks.useIsAdmin).mockReturnValue({
      isAdmin: false,
    });

    const { container } = render(<UserRoleBadge />);

    expect(container.firstChild).toBeNull();
  });

  it("should display admin badge when user is admin", () => {
    vi.mocked(usePermissionHooks.useUserRoles).mockReturnValue({
      roles: [
        {
          id: "role-1",
          role: { name: "admin", display_name: "Admin" },
          tenant_id: null,
        },
      ],
    });
    vi.mocked(usePermissionHooks.useIsAdmin).mockReturnValue({ isAdmin: true });

    render(<UserRoleBadge />);

    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("should display primary role when user is not admin", () => {
    vi.mocked(usePermissionHooks.useUserRoles).mockReturnValue({
      roles: [
        {
          id: "role-1",
          role: { name: "viewer", display_name: "Viewer" },
          tenant_id: null,
        },
        {
          id: "role-2",
          role: { name: "editor", display_name: "Editor" },
          tenant_id: null,
        },
      ],
      isLoading: false,
    });
    vi.mocked(usePermissionHooks.useIsAdmin).mockReturnValue({
      isAdmin: false,
    });

    render(<UserRoleBadge />);

    expect(screen.getByText("Viewer")).toBeInTheDocument();
    expect(screen.queryByText("Editor")).not.toBeInTheDocument();
  });

  it("should show role name when display_name is not available", () => {
    vi.mocked(usePermissionHooks.useUserRoles).mockReturnValue({
      roles: [
        {
          id: "role-1",
          role: { name: "custom-role" },
          tenant_id: null,
        },
      ],
      isLoading: false,
    });
    vi.mocked(usePermissionHooks.useIsAdmin).mockReturnValue({
      isAdmin: false,
    });

    render(<UserRoleBadge />);

    expect(screen.getByText("custom-role")).toBeInTheDocument();
  });
});
