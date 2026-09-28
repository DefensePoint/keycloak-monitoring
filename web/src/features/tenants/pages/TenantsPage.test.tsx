import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { TenantsPage } from "./TenantsPage";
import { useTenant } from "@/shared/context";
import { useKeycloakDashboard } from "@/shared/hooks";
import { useCreateTenant, useUpdateTenant, useDeleteTenant } from "../hooks";

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
  useToast: () => ({ showToast: vi.fn() }),
}));

vi.mock("@/shared/hooks", () => ({
  useKeycloakDashboard: vi.fn(),
}));

vi.mock("../hooks", () => ({
  useCreateTenant: vi.fn(),
  useUpdateTenant: vi.fn(),
  useDeleteTenant: vi.fn(),
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
}));

vi.mock("../components", () => ({
  TenantsPageHeader: ({ onAddClick }: { onAddClick: () => void }) => (
    <div data-testid="tenants-header">
      <button onClick={onAddClick}>Add Tenant</button>
    </div>
  ),
  TenantsList: ({
    tenants,
    onEdit,
    onDelete,
    onToggleEnabled,
    onCreateClick,
  }: {
    tenants: unknown[];
    onEdit: (tenant: { tenant_id: string }) => void;
    onDelete: (tenantId: string) => void;
    onToggleEnabled: (tenant: { tenant_id: string; enabled: boolean }) => void;
    onCreateClick: () => void;
  }) => (
    <div data-testid="tenants-list">
      {tenants.length === 0 ? (
        <button onClick={onCreateClick}>Create First Tenant</button>
      ) : (
        tenants.map(
          (tenant: { tenant_id: string; name: string; enabled: boolean }) => (
            <div key={tenant.tenant_id}>
              <span>{tenant.name}</span>
              <button onClick={() => onEdit(tenant)}>Edit</button>
              <button onClick={() => onDelete(tenant.tenant_id)}>Delete</button>
              <button onClick={() => onToggleEnabled(tenant)}>Toggle</button>
            </div>
          ),
        )
      )}
    </div>
  ),
  TenantForm: ({
    open,
    isCreating,
    onCancel,
    onSubmit,
    error,
  }: {
    open: boolean;
    isCreating: boolean;
    onCancel: () => void;
    onSubmit: (e: React.FormEvent) => void;
    error?: string | null;
  }) =>
    open ? (
      <div data-testid="tenant-form">
        <h2>{isCreating ? "Create Tenant" : "Edit Tenant"}</h2>
        {error && <div>{error}</div>}
        <button onClick={onSubmit}>Submit</button>
        <button onClick={onCancel}>Cancel</button>
      </div>
    ) : null,
}));

const mockTenants = [
  {
    tenant_id: "tenant-1",
    name: "Tenant 1",
    description: "Test Tenant 1",
    enabled: true,
    is_default: false,
    server_url: "https://keycloak1.example.com",
    admin_realm: "master",
    client_id: "admin-cli",
    default_realm: "master",
    owner: "owner1",
  },
  {
    tenant_id: "tenant-2",
    name: "Tenant 2",
    description: "Test Tenant 2",
    enabled: false,
    is_default: false,
    server_url: "https://keycloak2.example.com",
    admin_realm: "master",
    client_id: "admin-cli",
    default_realm: "test",
    owner: "owner2",
  },
];

describe("TenantsPage", () => {
  const mockRefreshTenants = vi.fn();
  const mockMutate = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useTenant).mockReturnValue({
      tenants: mockTenants,
      selectedTenant: mockTenants[0],
      selectTenant: vi.fn(),
      refreshTenants: mockRefreshTenants,
      isLoading: false,
      error: null,
    });
    vi.mocked(useKeycloakDashboard).mockReturnValue({
      data: { realms: [] },
      isLoading: false,
      error: null,
    } as ReturnType<typeof useKeycloakDashboard>);
    vi.mocked(useCreateTenant).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useCreateTenant>);
    vi.mocked(useUpdateTenant).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useUpdateTenant>);
    vi.mocked(useDeleteTenant).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useDeleteTenant>);
  });

  it("should render tenants page with header", () => {
    render(<TenantsPage />);

    expect(screen.getByTestId("tenants-header")).toBeInTheDocument();
  });

  it("should display list of tenants", () => {
    render(<TenantsPage />);

    expect(screen.getByTestId("tenants-list")).toBeInTheDocument();
    expect(screen.getByText("Tenant 1")).toBeInTheDocument();
    expect(screen.getByText("Tenant 2")).toBeInTheDocument();
  });

  it("should open create form when add button is clicked", async () => {
    const user = userEvent.setup();
    render(<TenantsPage />);

    const addButton = screen.getByText("Add Tenant");
    await user.click(addButton);

    expect(screen.getByTestId("tenant-form")).toBeInTheDocument();
    expect(screen.getByText("Create Tenant")).toBeInTheDocument();
  });

  it("should call createTenant when form is submitted", async () => {
    const user = userEvent.setup();
    render(<TenantsPage />);

    const addButton = screen.getByText("Add Tenant");
    await user.click(addButton);

    const submitButton = screen.getByText("Submit");
    await user.click(submitButton);

    expect(mockMutate).toHaveBeenCalled();
  });

  it("should open edit form when edit button is clicked", async () => {
    const user = userEvent.setup();
    render(<TenantsPage />);

    const editButtons = screen.getAllByText("Edit");
    await user.click(editButtons[0]);

    expect(screen.getByTestId("tenant-form")).toBeInTheDocument();
    expect(screen.getByText("Edit Tenant")).toBeInTheDocument();
  });

  it("should call updateTenant when edit form is submitted", async () => {
    const user = userEvent.setup();
    render(<TenantsPage />);

    const editButtons = screen.getAllByText("Edit");
    await user.click(editButtons[0]);

    const submitButton = screen.getByText("Submit");
    await user.click(submitButton);

    expect(mockMutate).toHaveBeenCalled();
  });

  it("should show confirm dialog when delete button is clicked", async () => {
    const user = userEvent.setup();
    render(<TenantsPage />);

    const deleteButtons = screen.getAllByText("Delete");
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByTestId("confirm-dialog")).toBeInTheDocument();
    });
  });

  it("should call deleteTenant when confirmed", async () => {
    const user = userEvent.setup();
    render(<TenantsPage />);

    const deleteButtons = screen.getAllByText("Delete");
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByTestId("confirm-dialog")).toBeInTheDocument();
    });

    const confirmButton = screen.getByText("Confirm");
    await user.click(confirmButton);

    expect(mockMutate).toHaveBeenCalled();
  });

  it("should call updateTenant for toggle enabled", async () => {
    const user = userEvent.setup();
    render(<TenantsPage />);

    const toggleButtons = screen.getAllByText("Toggle");
    await user.click(toggleButtons[0]);

    expect(mockMutate).toHaveBeenCalled();
  });

  it("should cancel form and return to list", async () => {
    const user = userEvent.setup();
    render(<TenantsPage />);

    const addButton = screen.getByText("Add Tenant");
    await user.click(addButton);

    expect(screen.getByTestId("tenant-form")).toBeInTheDocument();

    const cancelButton = screen.getByText("Cancel");
    await user.click(cancelButton);

    await waitFor(() => {
      expect(screen.queryByTestId("tenant-form")).not.toBeInTheDocument();
      expect(screen.getByTestId("tenants-list")).toBeInTheDocument();
    });
  });
});
