import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { RolesGrid } from "./RolesGrid";
import { ThemeProvider, createTheme } from "@mui/material";

const theme = createTheme();

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

interface RoleCardProps {
  role: { id: number; display_name: string; is_system: boolean };
  onView: (role: RoleCardProps["role"]) => void;
  onDelete: (id: number, isSystem: boolean) => void;
}

vi.mock("./RoleCard", () => ({
  RoleCard: ({ role, onView, onDelete }: RoleCardProps) => (
    <div data-testid={`role-card-${role.id}`}>
      <span>{role.display_name}</span>
      <button onClick={() => onView(role)}>View</button>
      <button onClick={() => onDelete(role.id, role.is_system)}>Delete</button>
    </div>
  ),
}));

vi.mock("@/shared/components", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/shared/components")>();
  return {
    ...actual,
    PermissionGate: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
  };
});

describe("RolesGrid", () => {
  const mockOnView = vi.fn();
  const mockOnDelete = vi.fn();

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
      name: "operator",
      display_name: "Operator",
      description: "Operator role",
      is_system: false,
      permissions: [],
    },
    {
      id: 3,
      name: "viewer",
      display_name: "Viewer",
      description: "Viewer role",
      is_system: false,
      permissions: [],
    },
  ];

  const defaultProps = {
    roles: mockRoles,
    onView: mockOnView,
    onDelete: mockOnDelete,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render all role cards", () => {
    renderWithTheme(<RolesGrid {...defaultProps} />);
    expect(screen.getByTestId("role-card-1")).toBeInTheDocument();
    expect(screen.getByTestId("role-card-2")).toBeInTheDocument();
    expect(screen.getByTestId("role-card-3")).toBeInTheDocument();
  });

  it("should pass role data to RoleCard components", () => {
    renderWithTheme(<RolesGrid {...defaultProps} />);
    expect(screen.getByText("Admin")).toBeInTheDocument();
    expect(screen.getByText("Operator")).toBeInTheDocument();
    expect(screen.getByText("Viewer")).toBeInTheDocument();
  });

  it("should pass onView handler to RoleCard components", async () => {
    const user = await import("@testing-library/user-event").then((m) =>
      m.default.setup(),
    );
    renderWithTheme(<RolesGrid {...defaultProps} />);

    const viewButtons = screen.getAllByText("View");
    await user.click(viewButtons[0]);

    expect(mockOnView).toHaveBeenCalledWith(mockRoles[0]);
  });

  it("should pass onDelete handler to RoleCard components", async () => {
    const user = await import("@testing-library/user-event").then((m) =>
      m.default.setup(),
    );
    renderWithTheme(<RolesGrid {...defaultProps} />);

    const deleteButtons = screen.getAllByText("Delete");
    await user.click(deleteButtons[1]);

    expect(mockOnDelete).toHaveBeenCalledWith(
      mockRoles[1].id,
      mockRoles[1].is_system,
    );
  });

  it("should render empty grid when no roles provided", () => {
    const { container } = renderWithTheme(
      <RolesGrid {...defaultProps} roles={[]} />,
    );
    const roleCards = container.querySelectorAll("[data-testid^='role-card-']");
    expect(roleCards).toHaveLength(0);
  });
});
