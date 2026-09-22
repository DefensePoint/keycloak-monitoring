import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { UsersTable } from "./UsersTable";
import { ThemeProvider, createTheme } from "@mui/material";

const theme = createTheme();

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

vi.mock("@/shared/components", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/shared/components")>();
  return {
    ...actual,
    PermissionGate: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
  };
});

vi.mock("@/shared/hooks", () => ({
  usePermission: () => ({ hasPermission: true, isLoading: false }),
}));

interface Column {
  header: string;
}

interface UserData {
  id: number;
  name: string;
  email: string;
}

vi.mock("material-react-table", () => ({
  MaterialReactTable: ({
    columns,
    data,
  }: {
    columns: Column[];
    data: UserData[];
  }) => (
    <div data-testid="users-table">
      <table>
        <thead>
          <tr>
            {columns.map((col) => (
              <th key={col.header}>{col.header}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map((user) => (
            <tr key={user.id}>
              <td>{user.name}</td>
              <td>{user.email}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  ),
}));

describe("UsersTable", () => {
  const mockOnEdit = vi.fn();
  const mockOnDelete = vi.fn();
  const mockOnManageRoles = vi.fn();
  const mockOnRevokeRole = vi.fn();

  const mockUsers = [
    {
      id: 1,
      name: "John Doe",
      preferred_username: "johndoe",
      email: "john@example.com",
      auth_method: "local",
      email_verified: true,
      is_active: true,
      is_blocked: false,
      must_change_password: false,
      last_accessed: "2024-01-01T00:00:00Z",
      roleAssignments: [
        {
          id: 1,
          role_id: 1,
          tenant_id: null,
          role: { id: 1, name: "admin", display_name: "Admin" },
        },
      ],
    },
    {
      id: 2,
      name: "Jane Doe",
      preferred_username: "janedoe",
      email: "jane@example.com",
      auth_method: "oauth",
      email_verified: false,
      is_active: true,
      is_blocked: false,
      must_change_password: false,
      last_accessed: null,
      roleAssignments: [],
    },
  ];

  const mockTenants = [
    { tenant_id: "tenant1", name: "Tenant 1" },
    { tenant_id: "tenant2", name: "Tenant 2" },
  ];

  const defaultProps = {
    users: mockUsers,
    tenants: mockTenants,
    onEdit: mockOnEdit,
    onDelete: mockOnDelete,
    onManageRoles: mockOnManageRoles,
    onRevokeRole: mockOnRevokeRole,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render table with users data", () => {
    renderWithTheme(<UsersTable {...defaultProps} />);
    expect(screen.getByTestId("users-table")).toBeInTheDocument();
  });

  it("should render user data in table", () => {
    renderWithTheme(<UsersTable {...defaultProps} />);
    expect(screen.getByText("John Doe")).toBeInTheDocument();
    expect(screen.getByText("Jane Doe")).toBeInTheDocument();
    expect(screen.getByText("john@example.com")).toBeInTheDocument();
    expect(screen.getByText("jane@example.com")).toBeInTheDocument();
  });

  it("should render table headers", () => {
    renderWithTheme(<UsersTable {...defaultProps} />);
    expect(screen.getByText("User")).toBeInTheDocument();
    expect(screen.getByText("Email")).toBeInTheDocument();
  });

  it("should render empty table when no users provided", () => {
    renderWithTheme(<UsersTable {...defaultProps} users={[]} />);
    expect(screen.getByTestId("users-table")).toBeInTheDocument();
  });
});
