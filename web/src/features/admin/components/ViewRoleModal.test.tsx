import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ViewRoleModal } from "./ViewRoleModal";
import { ThemeProvider, createTheme } from "@mui/material";

const theme = createTheme();

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

describe("ViewRoleModal", () => {
  const mockOnClose = vi.fn();

  const mockRole = {
    id: 1,
    name: "admin",
    display_name: "Admin",
    description: "Administrator role with full access",
    is_system: true,
    permissions: [
      {
        id: 1,
        name: "users:read",
        display_name: "Read Users",
        description: "View user information",
        resource: "users",
        action: "read",
      },
      {
        id: 2,
        name: "users:write",
        display_name: "Write Users",
        description: "Create and update users",
        resource: "users",
        action: "write",
      },
      {
        id: 3,
        name: "roles:read",
        display_name: "Read Roles",
        description: "View roles",
        resource: "roles",
        action: "read",
      },
    ],
  };

  const defaultProps = {
    open: true,
    role: mockRole,
    onClose: mockOnClose,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render role display name", () => {
    renderWithTheme(<ViewRoleModal {...defaultProps} />);
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("should render role description", () => {
    renderWithTheme(<ViewRoleModal {...defaultProps} />);
    expect(
      screen.getByText("Administrator role with full access"),
    ).toBeInTheDocument();
  });

  it("should render role name badge", () => {
    renderWithTheme(<ViewRoleModal {...defaultProps} />);
    expect(screen.getByText("admin")).toBeInTheDocument();
  });

  it("should render system role badge when role is system", () => {
    renderWithTheme(<ViewRoleModal {...defaultProps} />);
    expect(screen.getByText("System Role")).toBeInTheDocument();
  });

  it("should not render system role badge when role is not system", () => {
    const nonSystemRole = { ...mockRole, is_system: false };
    renderWithTheme(<ViewRoleModal {...defaultProps} role={nonSystemRole} />);
    expect(screen.queryByText("System Role")).not.toBeInTheDocument();
  });

  it("should display permissions count", () => {
    renderWithTheme(<ViewRoleModal {...defaultProps} />);
    expect(screen.getByText("Permissions (3)")).toBeInTheDocument();
  });

  it("should group permissions by resource", () => {
    renderWithTheme(<ViewRoleModal {...defaultProps} />);
    expect(screen.getByText("users")).toBeInTheDocument();
    expect(screen.getByText("roles")).toBeInTheDocument();
  });

  it("should display all permissions", () => {
    renderWithTheme(<ViewRoleModal {...defaultProps} />);
    expect(screen.getByText("Read Users")).toBeInTheDocument();
    expect(screen.getByText("Write Users")).toBeInTheDocument();
    expect(screen.getByText("Read Roles")).toBeInTheDocument();
  });

  it("should display permission names", () => {
    renderWithTheme(<ViewRoleModal {...defaultProps} />);
    expect(screen.getByText("users:read")).toBeInTheDocument();
    expect(screen.getByText("users:write")).toBeInTheDocument();
    expect(screen.getByText("roles:read")).toBeInTheDocument();
  });

  it("should render close button", () => {
    renderWithTheme(<ViewRoleModal {...defaultProps} />);
    const closeButton = screen.getByRole("button");
    expect(closeButton).toBeInTheDocument();
  });

  it("should call onClose when close button is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(<ViewRoleModal {...defaultProps} />);

    const closeButton = screen.getByRole("button");
    await user.click(closeButton);

    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it("should show message when role has no permissions", () => {
    const roleWithoutPermissions = { ...mockRole, permissions: [] };
    renderWithTheme(
      <ViewRoleModal {...defaultProps} role={roleWithoutPermissions} />,
    );
    expect(
      screen.getByText("No permissions assigned to this role"),
    ).toBeInTheDocument();
  });

  it("should not render when role is null", () => {
    const { container } = renderWithTheme(
      <ViewRoleModal {...defaultProps} role={null} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
