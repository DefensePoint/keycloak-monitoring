import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { AdminRolesPageHeader } from "./AdminRolesPageHeader";

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/admin/roles" }),
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

describe("AdminRolesPageHeader", () => {
  const mockOnCreateClick = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render page title and description", () => {
    render(
      <AdminRolesPageHeader rolesCount={5} onCreateClick={mockOnCreateClick} />,
    );

    expect(screen.getByText("Role Management")).toBeInTheDocument();
    expect(
      screen.getByText("Manage roles and their permissions"),
    ).toBeInTheDocument();
  });

  it("should display roles count", () => {
    render(
      <AdminRolesPageHeader
        rolesCount={15}
        onCreateClick={mockOnCreateClick}
      />,
    );

    expect(screen.getByText("Total Roles:")).toBeInTheDocument();
    expect(screen.getByText("15")).toBeInTheDocument();
  });

  it("should render create role button", () => {
    render(
      <AdminRolesPageHeader rolesCount={5} onCreateClick={mockOnCreateClick} />,
    );

    expect(
      screen.getByRole("button", { name: /create role/i }),
    ).toBeInTheDocument();
  });

  it("should call onCreateClick when create button is clicked", async () => {
    const user = userEvent.setup();
    render(
      <AdminRolesPageHeader rolesCount={5} onCreateClick={mockOnCreateClick} />,
    );

    await user.click(screen.getByRole("button", { name: /create role/i }));
    expect(mockOnCreateClick).toHaveBeenCalledTimes(1);
  });

  it("should wrap create button in PermissionGate with correct permission", () => {
    render(
      <AdminRolesPageHeader rolesCount={5} onCreateClick={mockOnCreateClick} />,
    );

    expect(
      screen.getByTestId("permission-gate-roles:write"),
    ).toBeInTheDocument();
  });

  it("should display 0 when rolesCount is 0", () => {
    render(
      <AdminRolesPageHeader rolesCount={0} onCreateClick={mockOnCreateClick} />,
    );

    expect(screen.getByText("0")).toBeInTheDocument();
  });
});
