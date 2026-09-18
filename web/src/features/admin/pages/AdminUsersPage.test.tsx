import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { AdminUsersPage } from "./AdminUsersPage";
import { useAuth, useTenant } from "@/shared/context";
import {
  useUsers,
  useRoles,
  useCreateUser,
  useUpdateUser,
  useDeleteUser,
  useAssignRole,
  useRevokeRole,
} from "../hooks";

vi.mock("@/shared/context", () => ({
  useAuth: vi.fn(),
  useTenant: vi.fn(),
  useToast: () => ({ showToast: vi.fn() }),
}));

vi.mock("../hooks", () => ({
  useUsers: vi.fn(),
  useRoles: vi.fn(),
  useCreateUser: vi.fn(),
  useUpdateUser: vi.fn(),
  useDeleteUser: vi.fn(),
  useUsersRoles: vi.fn().mockReturnValue([]),
  useAssignRole: vi.fn(),
  useRevokeRole: vi.fn(),
}));

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual("@tanstack/react-query");
  return {
    ...actual,
    useQueries: vi.fn().mockReturnValue([]),
  };
});

vi.mock("../services", () => ({
  rbacService: {
    getUserRoles: vi.fn().mockResolvedValue([]),
  },
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
    <div data-testid="loading-state">Loading users...</div>
  ),
}));

vi.mock("../components", () => ({
  AdminUsersPageHeader: ({
    usersCount,
    onCreateClick,
  }: {
    usersCount: number;
    onCreateClick: () => void;
  }) => (
    <div data-testid="users-header">
      <span>Users: {usersCount}</span>
      <button onClick={onCreateClick}>Create User</button>
    </div>
  ),
  UsersTable: ({
    users,
    onEdit,
    onDelete,
    onManageRoles,
  }: {
    users: unknown[];
    onEdit: (user: { id: number }) => void;
    onDelete: (id: number) => void;
    onManageRoles: (user: { id: number }) => void;
  }) => (
    <div data-testid="users-table">
      {users.map((user: { id: number; preferred_username: string }) => (
        <div key={user.id}>
          <span>{user.preferred_username}</span>
          <button onClick={() => onEdit(user)}>Edit</button>
          <button onClick={() => onDelete(user.id)}>Delete</button>
          <button onClick={() => onManageRoles(user)}>Manage Roles</button>
        </div>
      ))}
    </div>
  ),
  CreateUserModal: ({
    open,
    onSubmit,
    onClose,
  }: {
    open: boolean;
    onSubmit: (e: React.FormEvent) => void;
    onClose: () => void;
  }) =>
    open ? (
      <div data-testid="create-user-modal">
        <button onClick={onSubmit}>Submit</button>
        <button onClick={onClose}>Close</button>
      </div>
    ) : null,
  EditUserModal: ({
    open,
    onSubmit,
    onClose,
  }: {
    open: boolean;
    onSubmit: (e: React.FormEvent) => void;
    onClose: () => void;
  }) =>
    open ? (
      <div data-testid="edit-user-modal">
        <button onClick={onSubmit}>Submit</button>
        <button onClick={onClose}>Close</button>
      </div>
    ) : null,
  AssignRoleModal: ({
    open,
    onSubmit,
    onClose,
  }: {
    open: boolean;
    onSubmit: (e: React.FormEvent) => void;
    onClose: () => void;
  }) =>
    open ? (
      <div data-testid="assign-role-modal">
        <button onClick={onSubmit}>Submit</button>
        <button onClick={onClose}>Close</button>
      </div>
    ) : null,
}));

const mockUsers = [
  {
    id: 1,
    preferred_username: "testuser",
    email: "test@example.com",
    name: "Test User",
    is_active: true,
    is_blocked: false,
    roleAssignments: [],
  },
  {
    id: 2,
    preferred_username: "admin",
    email: "admin@example.com",
    name: "Admin User",
    is_active: true,
    is_blocked: false,
    roleAssignments: [],
  },
];

const mockRoles = [
  { id: 1, name: "admin", display_name: "Admin" },
  { id: 2, name: "operator", display_name: "Operator" },
];

const mockTenants = [
  { tenant_id: "tenant1", name: "Tenant 1" },
  { tenant_id: "tenant2", name: "Tenant 2" },
];

describe("AdminUsersPage", () => {
  const mockMutate = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useAuth).mockReturnValue({
      user: { email: "admin@example.com", name: "Admin" },
      authConfig: null,
      loginSimple: vi.fn(),
      loginOAuth2: vi.fn(),
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
      refreshUser: vi.fn(),
      hasPermission: vi.fn().mockReturnValue(true),
      hasAnyPermission: vi.fn().mockReturnValue(true),
      hasRole: vi.fn().mockReturnValue(true),
      isAdmin: vi.fn().mockReturnValue(true),
      hasAccessToTenant: vi.fn().mockReturnValue(true),
    });
    vi.mocked(useTenant).mockReturnValue({
      tenants: mockTenants,
      selectedTenant: mockTenants[0],
      setSelectedTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoadingTenants: false,
    });
    vi.mocked(useUsers).mockReturnValue({
      data: mockUsers,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useUsers>);
    vi.mocked(useRoles).mockReturnValue({
      data: mockRoles,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useRoles>);
    vi.mocked(useCreateUser).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useCreateUser>);
    vi.mocked(useUpdateUser).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useUpdateUser>);
    vi.mocked(useDeleteUser).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useDeleteUser>);
    vi.mocked(useAssignRole).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useAssignRole>);
    vi.mocked(useRevokeRole).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useRevokeRole>);
  });

  it("should render loading state initially", () => {
    vi.mocked(useUsers).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as ReturnType<typeof useUsers>);

    render(<AdminUsersPage />);
    expect(screen.getByTestId("loading-state")).toBeInTheDocument();
    expect(screen.getByText("Loading users...")).toBeInTheDocument();
  });

  it("should load and display users", async () => {
    render(<AdminUsersPage />);

    await waitFor(() => {
      expect(screen.getByTestId("users-header")).toBeInTheDocument();
    });

    expect(screen.getByText("Users: 2")).toBeInTheDocument();
    expect(screen.getByTestId("users-table")).toBeInTheDocument();
  });

  it("should open create user modal when create button is clicked", async () => {
    const user = userEvent.setup();
    render(<AdminUsersPage />);

    await waitFor(() => {
      expect(screen.getByTestId("users-header")).toBeInTheDocument();
    });

    const createButton = screen.getByText("Create User");
    await user.click(createButton);

    expect(screen.getByTestId("create-user-modal")).toBeInTheDocument();
  });

  it("should call createUser when form is submitted", async () => {
    const user = userEvent.setup();
    render(<AdminUsersPage />);

    await waitFor(() => {
      expect(screen.getByTestId("users-header")).toBeInTheDocument();
    });

    const createButton = screen.getByText("Create User");
    await user.click(createButton);

    const submitButton = screen.getByText("Submit");
    await user.click(submitButton);

    expect(mockMutate).toHaveBeenCalled();
  });

  it("should open edit user modal when edit button is clicked", async () => {
    const user = userEvent.setup();
    render(<AdminUsersPage />);

    await waitFor(() => {
      expect(screen.getByTestId("users-table")).toBeInTheDocument();
    });

    const editButtons = screen.getAllByText("Edit");
    await user.click(editButtons[0]);

    expect(screen.getByTestId("edit-user-modal")).toBeInTheDocument();
  });

  it("should call updateUser when edit form is submitted", async () => {
    const user = userEvent.setup();
    render(<AdminUsersPage />);

    await waitFor(() => {
      expect(screen.getByTestId("users-table")).toBeInTheDocument();
    });

    const editButtons = screen.getAllByText("Edit");
    await user.click(editButtons[0]);

    const submitButton = screen.getByText("Submit");
    await user.click(submitButton);

    expect(mockMutate).toHaveBeenCalled();
  });

  it("should show confirm dialog when delete button is clicked", async () => {
    const user = userEvent.setup();
    render(<AdminUsersPage />);

    await waitFor(() => {
      expect(screen.getByTestId("users-table")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByText("Delete");
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByTestId("confirm-dialog")).toBeInTheDocument();
    });
  });

  it("should call deleteUser when confirmed", async () => {
    const user = userEvent.setup();
    render(<AdminUsersPage />);

    await waitFor(() => {
      expect(screen.getByTestId("users-table")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByText("Delete");
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByTestId("confirm-dialog")).toBeInTheDocument();
    });

    const confirmButton = screen.getByText("Confirm");
    await user.click(confirmButton);

    expect(mockMutate).toHaveBeenCalled();
  });

  it("should open assign role modal when manage roles is clicked", async () => {
    const user = userEvent.setup();
    render(<AdminUsersPage />);

    await waitFor(() => {
      expect(screen.getByTestId("users-table")).toBeInTheDocument();
    });

    const manageButtons = screen.getAllByText("Manage Roles");
    await user.click(manageButtons[0]);

    expect(screen.getByTestId("assign-role-modal")).toBeInTheDocument();
  });
});
