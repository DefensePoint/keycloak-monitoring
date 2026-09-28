import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { AdminRolesPage } from "./AdminRolesPage";
import {
  useRoles,
  useRole,
  usePermissions,
  useCreateRole,
  useDeleteRole,
} from "../hooks";

vi.mock("../hooks", () => ({
  useRoles: vi.fn(),
  useRole: vi.fn(),
  usePermissions: vi.fn(),
  useCreateRole: vi.fn(),
  useDeleteRole: vi.fn(),
}));

const mockShowToast = vi.fn();
vi.mock("@/shared/context", () => ({
  useToast: () => ({ showToast: mockShowToast }),
}));

vi.mock("@/shared/components", () => ({
  ConfirmDialog: ({
    open,
    onConfirm,
    onCancel,
  }: {
    open: boolean;
    onConfirm: () => void;
    onCancel: () => void;
  }) =>
    open ? (
      <div data-testid="confirm-dialog">
        <button onClick={onConfirm}>Confirm</button>
        <button onClick={onCancel}>Cancel</button>
      </div>
    ) : null,
  LoadingSkeleton: () => (
    <div data-testid="loading-state">Loading roles...</div>
  ),
}));

vi.mock("../components", () => ({
  AdminRolesPageHeader: ({
    rolesCount,
    onCreateClick,
  }: {
    rolesCount: number;
    onCreateClick: () => void;
  }) => (
    <div data-testid="roles-header">
      <span>Roles: {rolesCount}</span>
      <button onClick={onCreateClick}>Create Role</button>
    </div>
  ),
  RolesGrid: ({
    roles,
    onView,
    onDelete,
  }: {
    roles: unknown[];
    onView: (role: { id: number }) => void;
    onDelete: (id: number, isSystem: boolean) => void;
  }) => (
    <div data-testid="roles-grid">
      {roles.map((role: { id: number; name: string; is_system: boolean }) => (
        <div key={role.id}>
          <span>{role.name}</span>
          <button onClick={() => onView(role)}>View</button>
          <button onClick={() => onDelete(role.id, role.is_system)}>
            Delete
          </button>
        </div>
      ))}
    </div>
  ),
  CreateRoleModal: ({
    open,
    onSubmit,
    onClose,
  }: {
    open: boolean;
    onSubmit: (e: React.FormEvent) => void;
    onClose: () => void;
  }) =>
    open ? (
      <div data-testid="create-role-modal">
        <button onClick={onSubmit}>Submit</button>
        <button onClick={onClose}>Close</button>
      </div>
    ) : null,
  ViewRoleModal: ({ open, onClose }: { open: boolean; onClose: () => void }) =>
    open ? (
      <div data-testid="view-role-modal">
        <button onClick={onClose}>Close</button>
      </div>
    ) : null,
}));

const mockRoles = [
  { id: 1, name: "admin", display_name: "Admin", is_system: true },
  { id: 2, name: "operator", display_name: "Operator", is_system: true },
  { id: 3, name: "custom", display_name: "Custom", is_system: false },
];

const mockPermissions = [
  { id: 1, name: "users:read", description: "Read users" },
  { id: 2, name: "users:write", description: "Write users" },
];

describe("AdminRolesPage", () => {
  const mockMutate = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useRoles).mockReturnValue({
      data: mockRoles,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useRoles>);
    vi.mocked(usePermissions).mockReturnValue({
      data: mockPermissions,
      isLoading: false,
      error: null,
    } as ReturnType<typeof usePermissions>);
    vi.mocked(useRole).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useRole>);
    vi.mocked(useCreateRole).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useCreateRole>);
    vi.mocked(useDeleteRole).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useDeleteRole>);
  });

  it("should render loading state initially", () => {
    vi.mocked(useRoles).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as ReturnType<typeof useRoles>);

    render(<AdminRolesPage />);
    expect(screen.getByTestId("loading-state")).toBeInTheDocument();
    expect(screen.getByText("Loading roles...")).toBeInTheDocument();
  });

  it("should load and display roles", async () => {
    render(<AdminRolesPage />);

    await waitFor(() => {
      expect(screen.getByTestId("roles-header")).toBeInTheDocument();
    });

    expect(screen.getByText("Roles: 3")).toBeInTheDocument();
    expect(screen.getByTestId("roles-grid")).toBeInTheDocument();
  });

  it("should open create role modal when create button is clicked", async () => {
    const user = userEvent.setup();
    render(<AdminRolesPage />);

    await waitFor(() => {
      expect(screen.getByTestId("roles-header")).toBeInTheDocument();
    });

    const createButton = screen.getByText("Create Role");
    await user.click(createButton);

    expect(screen.getByTestId("create-role-modal")).toBeInTheDocument();
  });

  it("should call createRole when form is submitted", async () => {
    const user = userEvent.setup();
    render(<AdminRolesPage />);

    await waitFor(() => {
      expect(screen.getByTestId("roles-header")).toBeInTheDocument();
    });

    const createButton = screen.getByText("Create Role");
    await user.click(createButton);

    const submitButton = screen.getByText("Submit");
    await user.click(submitButton);

    expect(mockMutate).toHaveBeenCalled();
  });

  it("should open view role modal when view button is clicked", async () => {
    const user = userEvent.setup();
    const mockFullRole = {
      ...mockRoles[0],
      permissions: mockPermissions,
    };
    vi.mocked(useRole).mockReturnValue({
      data: mockFullRole,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useRole>);

    render(<AdminRolesPage />);

    await waitFor(() => {
      expect(screen.getByTestId("roles-grid")).toBeInTheDocument();
    });

    const viewButtons = screen.getAllByText("View");
    await user.click(viewButtons[0]);

    await waitFor(() => {
      expect(screen.getByTestId("view-role-modal")).toBeInTheDocument();
    });
  });

  it("should show error when trying to delete system role", async () => {
    const user = userEvent.setup();
    render(<AdminRolesPage />);

    await waitFor(() => {
      expect(screen.getByTestId("roles-grid")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByText("Delete");
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(mockShowToast).toHaveBeenCalledWith({
        message: "Cannot delete system roles (admin, operator, viewer)",
        type: "warning",
      });
    });
    expect(screen.queryByTestId("confirm-dialog")).not.toBeInTheDocument();
  });

  it("should show confirm dialog for non-system role deletion", async () => {
    const user = userEvent.setup();
    render(<AdminRolesPage />);

    await waitFor(() => {
      expect(screen.getByTestId("roles-grid")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByText("Delete");
    await user.click(deleteButtons[2]);

    await waitFor(() => {
      expect(screen.getByTestId("confirm-dialog")).toBeInTheDocument();
    });
  });

  it("should call deleteRole when confirmed", async () => {
    const user = userEvent.setup();
    render(<AdminRolesPage />);

    await waitFor(() => {
      expect(screen.getByTestId("roles-grid")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByText("Delete");
    await user.click(deleteButtons[2]);

    await waitFor(() => {
      expect(screen.getByTestId("confirm-dialog")).toBeInTheDocument();
    });

    const confirmButton = screen.getByText("Confirm");
    await user.click(confirmButton);

    expect(mockMutate).toHaveBeenCalled();
  });

  it("should show warning toast when trying to delete system role", async () => {
    const user = userEvent.setup();
    render(<AdminRolesPage />);

    await waitFor(() => {
      expect(screen.getByTestId("roles-grid")).toBeInTheDocument();
    });

    // Try to delete a system role (admin)
    const deleteButtons = screen.getAllByText("Delete");
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(mockShowToast).toHaveBeenCalledWith({
        message: "Cannot delete system roles (admin, operator, viewer)",
        type: "warning",
      });
    });
  });
});
