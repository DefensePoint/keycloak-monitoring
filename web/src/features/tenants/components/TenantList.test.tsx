import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { TenantsList } from "./TenantsList";

vi.mock("@/shared/components", () => ({
  EmptyState: ({
    message,
    action,
  }: {
    message: string;
    action?: React.ReactNode;
  }) => (
    <div>
      <div>{message}</div>
      {action}
    </div>
  ),
}));

interface Tenant {
  tenant_id: string;
  name: string;
}

vi.mock("./TenantCard", () => ({
  TenantCard: ({
    tenant,
    onEdit,
    onDelete,
    onToggleEnabled,
  }: {
    tenant: Tenant;
    onEdit: (tenant: Tenant) => void;
    onDelete: (id: string) => void;
    onToggleEnabled: (tenant: Tenant) => void;
  }) => (
    <div>
      <span>TenantCard: {tenant.name}</span>
      <button onClick={() => onEdit(tenant)}>Edit</button>
      <button onClick={() => onDelete(tenant.tenant_id)}>Delete</button>
      <button onClick={() => onToggleEnabled(tenant)}>Toggle</button>
    </div>
  ),
}));

describe("TenantsList", () => {
  const mockOnEdit = vi.fn();
  const mockOnDelete = vi.fn();
  const mockOnToggleEnabled = vi.fn();
  const mockOnCreateClick = vi.fn();

  const mockTenants = [
    {
      tenant_id: "tenant-1",
      name: "Tenant 1",
      description: "Description 1",
      server_url: "https://keycloak1.example.com",
      enabled: true,
      is_default: false,
      health_status: "healthy",
      last_health_check: null,
      last_error: null,
      admin_realm: "master",
      client_id: "admin-cli",
      client_secret: null,
      default_realm: null,
    },
    {
      tenant_id: "tenant-2",
      name: "Tenant 2",
      description: "Description 2",
      server_url: "https://keycloak2.example.com",
      enabled: false,
      is_default: true,
      health_status: "unhealthy",
      last_health_check: null,
      last_error: null,
      admin_realm: "master",
      client_id: "admin-cli",
      client_secret: null,
      default_realm: null,
    },
  ];

  const defaultProps = {
    tenants: mockTenants,
    onEdit: mockOnEdit,
    onDelete: mockOnDelete,
    onToggleEnabled: mockOnToggleEnabled,
    onCreateClick: mockOnCreateClick,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render tenant cards when tenants are available", () => {
    render(<TenantsList {...defaultProps} />);

    expect(screen.getByText("TenantCard: Tenant 1")).toBeInTheDocument();
    expect(screen.getByText("TenantCard: Tenant 2")).toBeInTheDocument();
  });

  it("should call onEdit when edit button is clicked", async () => {
    const user = userEvent.setup();
    render(<TenantsList {...defaultProps} />);

    const editButtons = screen.getAllByRole("button", { name: /edit/i });
    await user.click(editButtons[0]);

    expect(mockOnEdit).toHaveBeenCalledWith(mockTenants[0]);
  });

  it("should call onDelete when delete button is clicked", async () => {
    const user = userEvent.setup();
    render(<TenantsList {...defaultProps} />);

    const deleteButtons = screen.getAllByRole("button", { name: /delete/i });
    await user.click(deleteButtons[0]);

    expect(mockOnDelete).toHaveBeenCalledWith("tenant-1");
  });

  it("should call onToggleEnabled when toggle button is clicked", async () => {
    const user = userEvent.setup();
    render(<TenantsList {...defaultProps} />);

    const toggleButtons = screen.getAllByRole("button", { name: /toggle/i });
    await user.click(toggleButtons[0]);

    expect(mockOnToggleEnabled).toHaveBeenCalledWith(mockTenants[0]);
  });

  it("should show empty state when no tenants are available", () => {
    render(<TenantsList {...defaultProps} tenants={[]} />);

    expect(screen.getByText("No tenants configured")).toBeInTheDocument();
  });

  it("should render create button in empty state", () => {
    render(<TenantsList {...defaultProps} tenants={[]} />);

    expect(
      screen.getByRole("button", { name: /create your first tenant/i }),
    ).toBeInTheDocument();
  });

  it("should call onCreateClick when create button is clicked in empty state", async () => {
    const user = userEvent.setup();
    render(<TenantsList {...defaultProps} tenants={[]} />);

    await user.click(
      screen.getByRole("button", { name: /create your first tenant/i }),
    );

    expect(mockOnCreateClick).toHaveBeenCalledTimes(1);
  });

  it("should render all tenants in the list", () => {
    render(<TenantsList {...defaultProps} />);

    expect(screen.getByText("TenantCard: Tenant 1")).toBeInTheDocument();
    expect(screen.getByText("TenantCard: Tenant 2")).toBeInTheDocument();
  });

  it("should not show empty state when tenants are available", () => {
    render(<TenantsList {...defaultProps} />);

    expect(screen.queryByText("No tenants configured")).not.toBeInTheDocument();
  });
});
