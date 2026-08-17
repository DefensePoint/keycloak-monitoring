import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { AdminUsersPageHeader } from "./AdminUsersPageHeader";

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/admin/users" }),
  useParams: () => ({}),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useTenant: () => ({
    selectedTenant: null,
  }),
}));

vi.mock("@/shared/components", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/shared/components")>();
  return {
    ...actual,
    PermissionGate: ({
      children,
      permission,
    }: {
      children: React.ReactNode;
      permission: string;
    }) => <div data-testid={`permission-gate-${permission}`}>{children}</div>,
  };
});

describe("AdminUsersPageHeader", () => {
  const mockOnCreateClick = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render page title and description", () => {
    render(
      <AdminUsersPageHeader
        usersCount={10}
        onCreateClick={mockOnCreateClick}
      />,
    );

    expect(screen.getByText("User Management")).toBeInTheDocument();
    expect(
      screen.getByText("Manage platform users and their role assignments"),
    ).toBeInTheDocument();
  });

  it("should display users count", () => {
    render(
      <AdminUsersPageHeader
        usersCount={42}
        onCreateClick={mockOnCreateClick}
      />,
    );

    expect(screen.getByText("Total Users:")).toBeInTheDocument();
    expect(screen.getByText("42")).toBeInTheDocument();
  });

  it("should render create user button", () => {
    render(
      <AdminUsersPageHeader
        usersCount={10}
        onCreateClick={mockOnCreateClick}
      />,
    );

    expect(
      screen.getByRole("button", { name: /create user/i }),
    ).toBeInTheDocument();
  });

  it("should call onCreateClick when create button is clicked", async () => {
    const user = userEvent.setup();
    render(
      <AdminUsersPageHeader
        usersCount={10}
        onCreateClick={mockOnCreateClick}
      />,
    );

    await user.click(screen.getByRole("button", { name: /create user/i }));
    expect(mockOnCreateClick).toHaveBeenCalledTimes(1);
  });

  it("should wrap create button in PermissionGate", () => {
    render(
      <AdminUsersPageHeader
        usersCount={10}
        onCreateClick={mockOnCreateClick}
      />,
    );

    expect(
      screen.getByTestId("permission-gate-platform_users:write"),
    ).toBeInTheDocument();
  });

  it("should display 0 when usersCount is 0", () => {
    render(
      <AdminUsersPageHeader usersCount={0} onCreateClick={mockOnCreateClick} />,
    );

    expect(screen.getByText("0")).toBeInTheDocument();
  });
});
