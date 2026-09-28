import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AssignRoleModal } from "./AssignRoleModal";
import { ThemeProvider, createTheme } from "@mui/material";

const theme = createTheme();

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

describe("AssignRoleModal", () => {
  const mockOnRoleChange = vi.fn();
  const mockOnTenantChange = vi.fn();
  const mockOnSubmit = vi.fn((e) => e.preventDefault());
  const mockOnClose = vi.fn();

  const mockUser = {
    id: 1,
    name: "John Doe",
    username: "johndoe",
    email: "john@example.com",
    roleAssignments: [
      {
        id: 1,
        role: { id: 1, name: "admin", display_name: "Admin" },
        tenant_id: null,
      },
    ],
  };

  const mockRoles = [
    { id: 1, name: "admin", display_name: "Admin", description: "Admin role" },
    {
      id: 2,
      name: "operator",
      display_name: "Operator",
      description: "Operator role",
    },
  ];

  const mockTenants = [
    { tenant_id: "tenant1", name: "Tenant 1" },
    { tenant_id: "tenant2", name: "Tenant 2" },
  ];

  const defaultProps = {
    open: true,
    user: mockUser,
    roles: mockRoles,
    tenants: mockTenants,
    selectedRoleId: 0,
    selectedTenantId: null,
    onRoleChange: mockOnRoleChange,
    onTenantChange: mockOnTenantChange,
    onSubmit: mockOnSubmit,
    onClose: mockOnClose,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render modal with user name in title", () => {
    renderWithTheme(<AssignRoleModal {...defaultProps} />);
    expect(screen.getByText("Assign Role to John Doe")).toBeInTheDocument();
  });

  it("should render role select dropdown", () => {
    renderWithTheme(<AssignRoleModal {...defaultProps} />);
    expect(screen.getByText("Select Role")).toBeInTheDocument();
  });

  it("should render tenant scope dropdown", () => {
    renderWithTheme(<AssignRoleModal {...defaultProps} />);
    expect(
      screen.getByRole("combobox", { name: /tenant scope/i }),
    ).toBeInTheDocument();
  });

  it("should display current roles for user", () => {
    renderWithTheme(<AssignRoleModal {...defaultProps} />);
    expect(
      screen.getByText("Current roles for this user:"),
    ).toBeInTheDocument();
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("should show 'No roles assigned' when user has no roles", () => {
    const userWithoutRoles = { ...mockUser, roleAssignments: [] };
    renderWithTheme(
      <AssignRoleModal {...defaultProps} user={userWithoutRoles} />,
    );
    expect(screen.getByText("No roles assigned")).toBeInTheDocument();
  });

  it("should call onRoleChange when role is selected", async () => {
    const user = userEvent.setup();
    renderWithTheme(<AssignRoleModal {...defaultProps} />);

    const roleSelect = screen.getByRole("combobox", { name: /select role/i });
    await user.click(roleSelect);

    const operatorOption = await screen.findByRole("option", {
      name: /operator/i,
    });
    await user.click(operatorOption);

    expect(mockOnRoleChange).toHaveBeenCalled();
  });

  it("should call onSubmit when form is submitted", async () => {
    const user = userEvent.setup();
    renderWithTheme(<AssignRoleModal {...defaultProps} selectedRoleId={1} />);

    const submitButton = screen.getByRole("button", { name: /assign role/i });
    await user.click(submitButton);

    expect(mockOnSubmit).toHaveBeenCalledTimes(1);
  });

  it("should call onClose and reset selections when cancel is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(<AssignRoleModal {...defaultProps} />);

    const cancelButton = screen.getByRole("button", { name: /cancel/i });
    await user.click(cancelButton);

    expect(mockOnRoleChange).toHaveBeenCalledWith(0);
    expect(mockOnTenantChange).toHaveBeenCalledWith(null);
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it("should not render when user is null", () => {
    const { container } = renderWithTheme(
      <AssignRoleModal {...defaultProps} user={null} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("should render submit and cancel buttons", () => {
    renderWithTheme(<AssignRoleModal {...defaultProps} />);
    expect(screen.getByRole("button", { name: /cancel/i })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /assign role/i }),
    ).toBeInTheDocument();
  });
});
