import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider, createTheme } from "@mui/material";
import { TenantCard } from "./TenantCard";

const theme = createTheme();

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

describe("TenantCard", () => {
  const mockOnEdit = vi.fn();
  const mockOnDelete = vi.fn();
  const mockOnToggleEnabled = vi.fn();

  const mockTenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
    description: "Test description",
    server_url: "https://keycloak.example.com",
    enabled: true,
    is_default: false,
    health_status: "healthy",
    last_health_check: "2024-01-01T12:00:00Z",
    last_error: null,
    admin_realm: "master",
    client_id: "admin-cli",
    client_secret: null,
    default_realm: null,
  };

  const defaultProps = {
    tenant: mockTenant,
    onEdit: mockOnEdit,
    onDelete: mockOnDelete,
    onToggleEnabled: mockOnToggleEnabled,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render tenant name", () => {
    renderWithTheme(<TenantCard {...defaultProps} />);

    expect(screen.getByText("Test Tenant")).toBeInTheDocument();
  });

  it("should render server URL", () => {
    renderWithTheme(<TenantCard {...defaultProps} />);

    expect(
      screen.getByText("https://keycloak.example.com"),
    ).toBeInTheDocument();
  });

  it("should render Server URL label", () => {
    renderWithTheme(<TenantCard {...defaultProps} />);

    expect(screen.getByText("Server URL")).toBeInTheDocument();
  });

  it("should render health status", () => {
    renderWithTheme(<TenantCard {...defaultProps} />);

    expect(screen.getByText(/healthy/i)).toBeInTheDocument();
  });

  it("should render last health check date", () => {
    renderWithTheme(<TenantCard {...defaultProps} />);

    const expectedDate = new Date("2024-01-01T12:00:00Z").toLocaleString();
    expect(screen.getByText(expectedDate)).toBeInTheDocument();
  });

  it("should display default badge when tenant is default", () => {
    const defaultTenant = { ...mockTenant, is_default: true };
    renderWithTheme(<TenantCard {...defaultProps} tenant={defaultTenant} />);

    expect(screen.getByText("Default")).toBeInTheDocument();
  });

  it("should display disabled badge when tenant is disabled", () => {
    const disabledTenant = {
      ...mockTenant,
      enabled: false,
      health_status: null,
    };
    renderWithTheme(<TenantCard {...defaultProps} tenant={disabledTenant} />);

    expect(screen.getByText("Disabled")).toBeInTheDocument();
  });

  it("should render edit button", () => {
    renderWithTheme(<TenantCard {...defaultProps} />);

    expect(screen.getByRole("button", { name: /edit/i })).toBeInTheDocument();
  });

  it("should render delete button", () => {
    renderWithTheme(<TenantCard {...defaultProps} />);

    expect(screen.getByRole("button", { name: /delete/i })).toBeInTheDocument();
  });

  it("should render disable button when tenant is enabled", () => {
    renderWithTheme(<TenantCard {...defaultProps} />);

    expect(
      screen.getByRole("button", { name: /disable/i }),
    ).toBeInTheDocument();
  });

  it("should render enable button when tenant is disabled", () => {
    const disabledTenant = { ...mockTenant, enabled: false };
    renderWithTheme(<TenantCard {...defaultProps} tenant={disabledTenant} />);

    expect(screen.getByRole("button", { name: /enable/i })).toBeInTheDocument();
  });

  it("should call onEdit when edit button is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(<TenantCard {...defaultProps} />);

    await user.click(screen.getByRole("button", { name: /edit/i }));

    expect(mockOnEdit).toHaveBeenCalledWith(mockTenant);
  });

  it("should call onDelete when delete button is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(<TenantCard {...defaultProps} />);

    await user.click(screen.getByRole("button", { name: /delete/i }));

    expect(mockOnDelete).toHaveBeenCalledWith("tenant-1");
  });

  it("should call onToggleEnabled when disable button is clicked", async () => {
    const user = userEvent.setup();
    renderWithTheme(<TenantCard {...defaultProps} />);

    await user.click(screen.getByRole("button", { name: /disable/i }));

    expect(mockOnToggleEnabled).toHaveBeenCalledWith(mockTenant);
  });

  it("should display error chip when error is present", () => {
    const tenantWithError = {
      ...mockTenant,
      last_error: "Connection failed",
    };
    renderWithTheme(<TenantCard {...defaultProps} tenant={tenantWithError} />);

    expect(screen.getByText("Error")).toBeInTheDocument();
  });

  it("should not display error chip when no error", () => {
    renderWithTheme(<TenantCard {...defaultProps} />);

    expect(screen.queryByText("Error")).not.toBeInTheDocument();
  });

  it("should display unhealthy status with correct color", () => {
    const unhealthyTenant = { ...mockTenant, health_status: "unhealthy" };
    renderWithTheme(<TenantCard {...defaultProps} tenant={unhealthyTenant} />);

    expect(screen.getByText(/unhealthy/i)).toBeInTheDocument();
  });

  it("should display unhealthy status when health status is unknown", () => {
    const unknownTenant = { ...mockTenant, health_status: "unknown" };
    renderWithTheme(<TenantCard {...defaultProps} tenant={unknownTenant} />);

    // When health_status is not "healthy", the chip shows "Unhealthy" for enabled tenants
    expect(screen.getByText(/unhealthy/i)).toBeInTheDocument();
  });

  describe("config-defined tenant", () => {
    const configDefinedTenant = { ...mockTenant, is_config_defined: true };

    it("should display a read-only badge", () => {
      renderWithTheme(
        <TenantCard {...defaultProps} tenant={configDefinedTenant} />,
      );

      expect(screen.getByText("Read-only")).toBeInTheDocument();
    });

    it("should not display a read-only badge for a UI-created tenant", () => {
      renderWithTheme(<TenantCard {...defaultProps} />);

      expect(screen.queryByText("Read-only")).not.toBeInTheDocument();
    });

    it("should disable the edit, delete, and toggle buttons", () => {
      renderWithTheme(
        <TenantCard {...defaultProps} tenant={configDefinedTenant} />,
      );

      expect(screen.getByRole("button", { name: /edit/i })).toBeDisabled();
      expect(screen.getByRole("button", { name: /delete/i })).toBeDisabled();
      expect(screen.getByRole("button", { name: /disable/i })).toBeDisabled();
    });

    it("should not call onEdit when the card body is clicked", async () => {
      const user = userEvent.setup();
      renderWithTheme(
        <TenantCard {...defaultProps} tenant={configDefinedTenant} />,
      );

      await user.click(screen.getByText("Test Tenant"));

      expect(mockOnEdit).not.toHaveBeenCalled();
    });

    it("should reject pointer interaction on the disabled action buttons", async () => {
      const user = userEvent.setup();
      renderWithTheme(
        <TenantCard {...defaultProps} tenant={configDefinedTenant} />,
      );

      // A disabled MUI IconButton sets pointer-events: none, so the click
      // itself must fail here, not just leave the handler unfired.
      await expect(
        user.click(screen.getByRole("button", { name: /edit/i })),
      ).rejects.toThrow(/pointer-events: none/);
      expect(mockOnEdit).not.toHaveBeenCalled();
    });
  });
});
