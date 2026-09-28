import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { TenantSelector } from "./TenantSelector";
import { useTenant } from "@/shared/context";
import type { Tenant } from "@/shared/types";

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
}));

vi.mock("react-router-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router-dom")>();
  return {
    ...actual,
    Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
      <a href={to}>{children}</a>
    ),
  };
});

describe("TenantSelector", () => {
  const mockSelectTenant = vi.fn();

  const mockTenants: Tenant[] = [
    {
      tenant_id: "tenant-1",
      name: "Tenant 1",
      server_url: "https://tenant1.example.com",
      enabled: true,
      is_default: true,
      health_status: "healthy",
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
    },
    {
      tenant_id: "tenant-2",
      name: "Tenant 2",
      server_url: "https://tenant2.example.com",
      enabled: true,
      is_default: false,
      health_status: "unhealthy",
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
    },
    {
      tenant_id: "tenant-3",
      name: "Disabled Tenant",
      server_url: "https://tenant3.example.com",
      enabled: false,
      is_default: false,
      health_status: "unknown",
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should show loading state", () => {
    vi.mocked(useTenant).mockReturnValue({
      tenants: [],
      selectedTenant: null,
      selectTenant: mockSelectTenant,
      isLoading: true,
      error: null,
      refreshTenants: vi.fn(),
    });

    render(<TenantSelector />);

    expect(screen.getByText("Loading tenants...")).toBeInTheDocument();
  });

  it("should show empty state when no tenants", () => {
    vi.mocked(useTenant).mockReturnValue({
      tenants: [],
      selectedTenant: null,
      selectTenant: mockSelectTenant,
      isLoading: false,
      error: null,
      refreshTenants: vi.fn(),
    });

    render(<TenantSelector />);

    expect(screen.getByText("No tenants configured")).toBeInTheDocument();
    expect(screen.getByText("Add Tenant")).toBeInTheDocument();
  });

  it("should display selected tenant name", () => {
    vi.mocked(useTenant).mockReturnValue({
      tenants: mockTenants,
      selectedTenant: mockTenants[0],
      selectTenant: mockSelectTenant,
      isLoading: false,
      error: null,
      refreshTenants: vi.fn(),
    });

    render(<TenantSelector />);

    expect(screen.getByText("Tenant 1")).toBeInTheDocument();
    expect(screen.getByText("Current Tenant")).toBeInTheDocument();
  });

  it("should display 'Select Tenant' when no tenant selected", () => {
    vi.mocked(useTenant).mockReturnValue({
      tenants: mockTenants,
      selectedTenant: null,
      selectTenant: mockSelectTenant,
      isLoading: false,
      error: null,
      refreshTenants: vi.fn(),
    });

    render(<TenantSelector />);

    expect(screen.getByText("Select Tenant")).toBeInTheDocument();
  });

  it("should open menu when button is clicked", async () => {
    const user = userEvent.setup();
    vi.mocked(useTenant).mockReturnValue({
      tenants: mockTenants,
      selectedTenant: mockTenants[0],
      selectTenant: mockSelectTenant,
      isLoading: false,
      error: null,
      refreshTenants: vi.fn(),
    });

    render(<TenantSelector />);

    const button = screen.getByRole("button");
    await user.click(button);

    // Menu items should be visible
    expect(screen.getByText("Tenant 2")).toBeInTheDocument();
    expect(screen.getByText("Disabled Tenant")).toBeInTheDocument();
    expect(screen.getByText("Manage Tenants")).toBeInTheDocument();
  });

  it("should call selectTenant when a tenant is clicked", async () => {
    const user = userEvent.setup();
    vi.mocked(useTenant).mockReturnValue({
      tenants: mockTenants,
      selectedTenant: mockTenants[0],
      selectTenant: mockSelectTenant,
      isLoading: false,
      error: null,
      refreshTenants: vi.fn(),
    });

    render(<TenantSelector />);

    const button = screen.getByRole("button");
    await user.click(button);

    const tenant2MenuItem = screen.getByText("Tenant 2");
    await user.click(tenant2MenuItem);

    expect(mockSelectTenant).toHaveBeenCalledWith(mockTenants[1]);
  });

  it("should show Default chip for default tenant", async () => {
    const user = userEvent.setup();
    vi.mocked(useTenant).mockReturnValue({
      tenants: mockTenants,
      selectedTenant: null,
      selectTenant: mockSelectTenant,
      isLoading: false,
      error: null,
      refreshTenants: vi.fn(),
    });

    render(<TenantSelector />);

    const button = screen.getByRole("button");
    await user.click(button);

    expect(screen.getByText("Default")).toBeInTheDocument();
  });

  it("should show Disabled label for disabled tenants", async () => {
    const user = userEvent.setup();
    vi.mocked(useTenant).mockReturnValue({
      tenants: mockTenants,
      selectedTenant: null,
      selectTenant: mockSelectTenant,
      isLoading: false,
      error: null,
      refreshTenants: vi.fn(),
    });

    render(<TenantSelector />);

    const button = screen.getByRole("button");
    await user.click(button);

    expect(screen.getByText("Disabled")).toBeInTheDocument();
  });

  it("should display tenant server URL in menu", async () => {
    const user = userEvent.setup();
    vi.mocked(useTenant).mockReturnValue({
      tenants: mockTenants,
      selectedTenant: mockTenants[0],
      selectTenant: mockSelectTenant,
      isLoading: false,
      error: null,
      refreshTenants: vi.fn(),
    });

    render(<TenantSelector />);

    const button = screen.getByRole("button");
    await user.click(button);

    expect(screen.getByText("https://tenant1.example.com")).toBeInTheDocument();
  });

  it("should have link to manage tenants page", async () => {
    const user = userEvent.setup();
    vi.mocked(useTenant).mockReturnValue({
      tenants: mockTenants,
      selectedTenant: mockTenants[0],
      selectTenant: mockSelectTenant,
      isLoading: false,
      error: null,
      refreshTenants: vi.fn(),
    });

    render(<TenantSelector />);

    const button = screen.getByRole("button");
    await user.click(button);

    const manageLink = screen.getByText("Manage Tenants");
    expect(manageLink.closest("a")).toHaveAttribute("href", "/tenants");
  });
});
