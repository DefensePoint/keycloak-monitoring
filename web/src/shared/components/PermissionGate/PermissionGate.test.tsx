import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import { PermissionGate } from "./PermissionGate";
import * as usePermissionHooks from "@/shared/hooks";
import { PERMISSIONS } from "@/shared/constants";

vi.mock("@/shared/hooks", async () => {
  const { PERMISSIONS } = await import("@/shared/constants");
  return {
    usePermission: vi.fn((permission) => ({
      hasPermission: permission === PERMISSIONS.ALERTS.WRITE,
      isLoading: false,
    })),
    useRole: vi.fn((role) => ({
      hasRole: role === "admin",
      isLoading: false,
    })),
    useIsAdmin: vi.fn(() => ({
      isAdmin: false,
      isLoading: false,
    })),
  };
});

describe("PermissionGate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render children when user has required permission", () => {
    render(
      <PermissionGate permission={PERMISSIONS.ALERTS.WRITE}>
        <div>Protected Content</div>
      </PermissionGate>,
    );

    expect(screen.getByText("Protected Content")).toBeInTheDocument();
  });

  it("should not render children when user lacks required permission", () => {
    render(
      <PermissionGate permission={PERMISSIONS.ALERTS.DELETE}>
        <div>Protected Content</div>
      </PermissionGate>,
    );

    expect(screen.queryByText("Protected Content")).not.toBeInTheDocument();
  });

  it("should render fallback when user lacks permission and fallback is provided", () => {
    render(
      <PermissionGate
        permission={PERMISSIONS.ALERTS.DELETE}
        fallback={<div>Access Denied</div>}
      >
        <div>Protected Content</div>
      </PermissionGate>,
    );

    expect(screen.getByText("Access Denied")).toBeInTheDocument();
    expect(screen.queryByText("Protected Content")).not.toBeInTheDocument();
  });

  it("should render children when user has required role", () => {
    render(
      <PermissionGate role="admin">
        <div>Admin Content</div>
      </PermissionGate>,
    );

    expect(screen.getByText("Admin Content")).toBeInTheDocument();
  });

  it("should not render children when user lacks required role", () => {
    render(
      <PermissionGate role="superuser">
        <div>Admin Content</div>
      </PermissionGate>,
    );

    expect(screen.queryByText("Admin Content")).not.toBeInTheDocument();
  });

  it("should render children when no restrictions are specified", () => {
    render(
      <PermissionGate>
        <div>Public Content</div>
      </PermissionGate>,
    );

    expect(screen.getByText("Public Content")).toBeInTheDocument();
  });

  describe("admin bypass", () => {
    beforeEach(() => {
      vi.mocked(usePermissionHooks.useIsAdmin).mockReturnValue({
        isAdmin: true,
        isLoading: false,
      });
    });

    it("should render children for admin even without permission", () => {
      render(
        <PermissionGate permission={PERMISSIONS.ALERTS.DELETE}>
          <div>Protected Content</div>
        </PermissionGate>,
      );

      expect(screen.getByText("Protected Content")).toBeInTheDocument();
    });

    it("should render children for admin even without role", () => {
      render(
        <PermissionGate role="superuser">
          <div>Admin Content</div>
        </PermissionGate>,
      );

      expect(screen.getByText("Admin Content")).toBeInTheDocument();
    });
  });

  describe("loading states", () => {
    it("should not render children when permission check is loading", () => {
      vi.mocked(usePermissionHooks.useIsAdmin).mockReturnValue({
        isAdmin: false,
        isLoading: false,
      });
      vi.mocked(usePermissionHooks.usePermission).mockReturnValue({
        hasPermission: false,
        isLoading: true,
      });

      render(
        <PermissionGate permission={PERMISSIONS.ALERTS.WRITE}>
          <div>Protected Content</div>
        </PermissionGate>,
      );

      expect(screen.queryByText("Protected Content")).not.toBeInTheDocument();
    });

    it("should not render children when role check is loading", () => {
      vi.mocked(usePermissionHooks.useIsAdmin).mockReturnValue({
        isAdmin: false,
        isLoading: false,
      });
      vi.mocked(usePermissionHooks.useRole).mockReturnValue({
        hasRole: false,
        isLoading: true,
      });

      render(
        <PermissionGate role="admin">
          <div>Admin Content</div>
        </PermissionGate>,
      );

      expect(screen.queryByText("Admin Content")).not.toBeInTheDocument();
    });
  });

  describe("requireAdmin prop", () => {
    it("should render children when user is admin and requireAdmin is true", () => {
      vi.mocked(usePermissionHooks.useIsAdmin).mockReturnValue({
        isAdmin: true,
        isLoading: false,
      });

      render(
        <PermissionGate requireAdmin>
          <div>Admin Only</div>
        </PermissionGate>,
      );

      expect(screen.getByText("Admin Only")).toBeInTheDocument();
    });

    it("should render fallback when user is not admin and requireAdmin is true", () => {
      vi.mocked(usePermissionHooks.useIsAdmin).mockReturnValue({
        isAdmin: false,
        isLoading: false,
      });

      render(
        <PermissionGate requireAdmin fallback={<div>Not Admin</div>}>
          <div>Admin Only</div>
        </PermissionGate>,
      );

      expect(screen.getByText("Not Admin")).toBeInTheDocument();
      expect(screen.queryByText("Admin Only")).not.toBeInTheDocument();
    });
  });
});
