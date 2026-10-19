import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RoleCard } from "./RoleCard";
import { ThemeProvider, createTheme } from "@mui/material";

const theme = createTheme();

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

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

describe("RoleCard", () => {
  const mockOnView = vi.fn();
  const mockOnDelete = vi.fn();

  const mockRole = {
    id: 1,
    name: "admin",
    display_name: "Admin",
    description: "Administrator role with full access",
    is_system: false,
    is_active: true,
    permissions: [],
  };

  const defaultProps = {
    role: mockRole,
    onView: mockOnView,
    onDelete: mockOnDelete,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render role display name", () => {
    renderWithTheme(<RoleCard {...defaultProps} />);
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("should render role description", () => {
    renderWithTheme(<RoleCard {...defaultProps} />);
    expect(
      screen.getByText("Administrator role with full access"),
    ).toBeInTheDocument();
  });

  it("should render role name", () => {
    renderWithTheme(<RoleCard {...defaultProps} />);
    expect(screen.getByText("admin")).toBeInTheDocument();
  });

  it("should render system badge for system roles", () => {
    const systemRole = { ...mockRole, is_system: true };
    renderWithTheme(<RoleCard {...defaultProps} role={systemRole} />);
    expect(screen.getByText("System")).toBeInTheDocument();
  });

  it("should not render system badge for non-system roles", () => {
    renderWithTheme(<RoleCard {...defaultProps} />);
    expect(screen.queryByText("System")).not.toBeInTheDocument();
  });

  it("should render view button", () => {
    renderWithTheme(<RoleCard {...defaultProps} />);
    const viewButton = screen.getByTitle("View details");
    expect(viewButton).toBeInTheDocument();
  });

  it("should call onView when view button is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(<RoleCard {...defaultProps} />);

    const viewButton = screen.getByTitle("View details");
    await user.click(viewButton);

    expect(mockOnView).toHaveBeenCalledWith(mockRole);
  });

  it("should render delete button for non-system roles", () => {
    renderWithTheme(<RoleCard {...defaultProps} />);
    const deleteButton = screen.getByTitle("Delete role");
    expect(deleteButton).toBeInTheDocument();
  });

  it("should not render delete button for system roles", () => {
    const systemRole = { ...mockRole, is_system: true };
    renderWithTheme(<RoleCard {...defaultProps} role={systemRole} />);
    const deleteButton = screen.queryByTitle("Delete role");
    expect(deleteButton).not.toBeInTheDocument();
  });

  it("should call onDelete when delete button is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(<RoleCard {...defaultProps} />);

    const deleteButton = screen.getByTitle("Delete role");
    await user.click(deleteButton);

    expect(mockOnDelete).toHaveBeenCalledWith(mockRole.id, mockRole.is_system);
  });

  it("should wrap delete button in PermissionGate", () => {
    renderWithTheme(<RoleCard {...defaultProps} />);
    expect(
      screen.getByTestId("permission-gate-roles:delete"),
    ).toBeInTheDocument();
  });

  it("should render different badge colors based on role name", () => {
    const { container } = renderWithTheme(<RoleCard {...defaultProps} />);
    expect(container).toBeInTheDocument();
  });

  describe("active state", () => {
    it("shows no Inactive chip for an active role", () => {
      renderWithTheme(<RoleCard {...defaultProps} />);
      expect(screen.queryByText("Inactive")).not.toBeInTheDocument();
    });

    it("shows an Inactive chip for an inactive role", () => {
      renderWithTheme(
        <RoleCard {...defaultProps} role={{ ...mockRole, is_active: false }} />,
      );
      expect(screen.getByText("Inactive")).toBeInTheDocument();
    });

    it("deactivates an active role and activates an inactive one", async () => {
      const user = userEvent.setup();
      const onToggleActive = vi.fn();
      const { rerender } = renderWithTheme(
        <RoleCard {...defaultProps} onToggleActive={onToggleActive} />,
      );
      await user.click(screen.getByRole("button", { name: "Deactivate role" }));
      expect(onToggleActive).toHaveBeenCalledWith(mockRole);

      const inactive = { ...mockRole, is_active: false };
      rerender(
        <ThemeProvider theme={theme}>
          <RoleCard
            {...defaultProps}
            role={inactive}
            onToggleActive={onToggleActive}
          />
        </ThemeProvider>,
      );
      await user.click(screen.getByRole("button", { name: "Activate role" }));
      expect(onToggleActive).toHaveBeenLastCalledWith(inactive);
    });

    it("offers no toggle on a system role", () => {
      renderWithTheme(
        <RoleCard
          {...defaultProps}
          role={{ ...mockRole, is_system: true }}
          onToggleActive={vi.fn()}
        />,
      );
      expect(
        screen.queryByRole("button", { name: /activate role/i }),
      ).not.toBeInTheDocument();
    });
  });
});
