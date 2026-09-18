import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { PermissionsSelector } from "./PermissionsSelector";
import { ThemeProvider, createTheme } from "@mui/material";

const theme = createTheme();

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

describe("PermissionsSelector", () => {
  const mockOnTogglePermission = vi.fn();

  const mockPermissions = [
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
  ];

  const defaultProps = {
    permissions: mockPermissions,
    selectedPermissionIds: [],
    onTogglePermission: mockOnTogglePermission,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render permissions header", () => {
    renderWithTheme(<PermissionsSelector {...defaultProps} />);
    expect(
      screen.getByText("Permissions * (Select at least one)"),
    ).toBeInTheDocument();
  });

  it("should group permissions by resource", () => {
    renderWithTheme(<PermissionsSelector {...defaultProps} />);
    expect(screen.getByText("users")).toBeInTheDocument();
    expect(screen.getByText("roles")).toBeInTheDocument();
  });

  it("should display all permission options", () => {
    renderWithTheme(<PermissionsSelector {...defaultProps} />);
    expect(screen.getByText("Read Users")).toBeInTheDocument();
    expect(screen.getByText("Write Users")).toBeInTheDocument();
    expect(screen.getByText("Read Roles")).toBeInTheDocument();
  });

  it("should display permission descriptions", () => {
    renderWithTheme(<PermissionsSelector {...defaultProps} />);
    expect(screen.getByText("View user information")).toBeInTheDocument();
    expect(screen.getByText("Create and update users")).toBeInTheDocument();
    expect(screen.getByText("View roles")).toBeInTheDocument();
  });

  it("should display permission names", () => {
    renderWithTheme(<PermissionsSelector {...defaultProps} />);
    expect(screen.getByText("users:read")).toBeInTheDocument();
    expect(screen.getByText("users:write")).toBeInTheDocument();
    expect(screen.getByText("roles:read")).toBeInTheDocument();
  });

  it("should check selected permissions", () => {
    renderWithTheme(
      <PermissionsSelector {...defaultProps} selectedPermissionIds={[1, 2]} />,
    );
    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes[0]).toBeChecked();
    expect(checkboxes[1]).toBeChecked();
    expect(checkboxes[2]).not.toBeChecked();
  });

  it("should call onTogglePermission when checkbox is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(<PermissionsSelector {...defaultProps} />);

    const checkboxes = screen.getAllByRole("checkbox");
    await user.click(checkboxes[0]);

    expect(mockOnTogglePermission).toHaveBeenCalledWith(1);
  });

  it("should display count of selected permissions", () => {
    renderWithTheme(
      <PermissionsSelector {...defaultProps} selectedPermissionIds={[1, 3]} />,
    );
    expect(screen.getByText("Selected: 2 permission(s)")).toBeInTheDocument();
  });

  it("should display zero count when no permissions selected", () => {
    renderWithTheme(<PermissionsSelector {...defaultProps} />);
    expect(screen.getByText("Selected: 0 permission(s)")).toBeInTheDocument();
  });
});
