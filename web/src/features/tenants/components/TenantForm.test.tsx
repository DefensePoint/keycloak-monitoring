import { describe, it, expect, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { TenantForm } from "./TenantForm";

describe("TenantForm", () => {
  const mockOnSubmit = vi.fn();
  const mockOnCancel = vi.fn();

  const defaultProps = {
    open: true,
    isCreating: true,
    editingTenant: null,
    realms: [],
    error: null,
    isSubmitting: false,
    onSubmit: mockOnSubmit,
    onCancel: mockOnCancel,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render create form title when creating", () => {
    render(<TenantForm {...defaultProps} />);

    expect(screen.getByText("Create New Tenant")).toBeInTheDocument();
  });

  it("should render edit form title when editing", () => {
    const editingTenant = {
      tenant_id: "tenant-1",
      name: "Test Tenant",
      description: "",
      server_url: "https://keycloak.example.com",
      admin_realm: "master",
      client_id: "",
      enabled: true,
      is_default: false,
      default_realm: "",
    };
    render(
      <TenantForm
        {...defaultProps}
        isCreating={false}
        editingTenant={editingTenant}
      />,
    );

    expect(screen.getByText("Edit Tenant")).toBeInTheDocument();
  });

  it("should render tenant ID field when creating", () => {
    render(<TenantForm {...defaultProps} />);

    expect(screen.getByLabelText(/tenant id/i)).toBeInTheDocument();
  });

  it("should not render tenant ID field when editing", () => {
    const editingTenant = {
      tenant_id: "tenant-1",
      name: "Test Tenant",
      description: "",
      server_url: "https://keycloak.example.com",
      admin_realm: "master",
      client_id: "",
      enabled: true,
      is_default: false,
      default_realm: "",
    };
    render(
      <TenantForm
        {...defaultProps}
        isCreating={false}
        editingTenant={editingTenant}
      />,
    );

    expect(screen.queryByLabelText(/tenant id/i)).not.toBeInTheDocument();
  });

  it("should render all required form fields", () => {
    render(<TenantForm {...defaultProps} />);

    expect(screen.getByLabelText(/^name/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/description/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/server url/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/admin realm/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/client id/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/client secret/i)).toBeInTheDocument();
  });

  it("should allow typing in name field", async () => {
    const user = userEvent.setup();
    render(<TenantForm {...defaultProps} />);

    const nameInput = screen.getByLabelText(/^name/i);
    await user.type(nameInput, "New Tenant");

    expect(nameInput).toHaveValue("New Tenant");
  });

  it("should call onSubmit with form data when form is submitted with valid data", async () => {
    const user = userEvent.setup();
    render(<TenantForm {...defaultProps} />);

    fireEvent.change(screen.getByLabelText(/tenant id/i), {
      target: { value: "test-tenant" },
    });
    fireEvent.change(screen.getByLabelText(/^name/i), {
      target: { value: "Test Tenant" },
    });
    fireEvent.change(screen.getByLabelText(/server url/i), {
      target: { value: "https://keycloak.example.com" },
    });
    fireEvent.change(screen.getByLabelText(/admin realm/i), {
      target: { value: "master" },
    });
    fireEvent.change(screen.getByLabelText(/client id/i), {
      target: { value: "monitoring-service" },
    });
    fireEvent.change(screen.getByLabelText(/client secret/i), {
      target: { value: "client-secret" },
    });

    const submitButton = screen.getByRole("button", {
      name: /create tenant/i,
    });
    await user.click(submitButton);

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          tenant_id: "test-tenant",
          name: "Test Tenant",
          server_url: "https://keycloak.example.com",
          admin_realm: "master",
          client_id: "monitoring-service",
          client_secret: "client-secret",
          enabled: true,
          is_default: false,
        }),
      );
    });
  });

  describe("client_secret on edit", () => {
    const editingTenant = {
      tenant_id: "tenant-1",
      name: "Test Tenant",
      description: "",
      server_url: "https://keycloak.example.com",
      admin_realm: "master",
      client_id: "monitoring-service",
      enabled: true,
      is_default: false,
      default_realm: "",
    };

    it("should omit client_secret when the secret field is left blank", async () => {
      const user = userEvent.setup();
      render(
        <TenantForm
          {...defaultProps}
          isCreating={false}
          editingTenant={editingTenant}
        />,
      );

      // The stored secret is never rendered, so the field starts blank.
      expect(screen.getByLabelText(/client secret/i)).toHaveValue("");

      await user.click(screen.getByRole("button", { name: /save changes/i }));

      await waitFor(() => {
        expect(mockOnSubmit).toHaveBeenCalled();
      });

      const payload = mockOnSubmit.mock.calls[0][0];
      expect(payload.client_secret).toBeUndefined();
      // apiClient serialises with JSON.stringify, which drops undefined keys,
      // so the field must not reach the server at all.
      expect(JSON.stringify(payload)).not.toContain("client_secret");
    });

    it("should send client_secret when a new secret is entered", async () => {
      const user = userEvent.setup();
      render(
        <TenantForm
          {...defaultProps}
          isCreating={false}
          editingTenant={editingTenant}
        />,
      );

      fireEvent.change(screen.getByLabelText(/client secret/i), {
        target: { value: "rotated-secret" },
      });
      await user.click(screen.getByRole("button", { name: /save changes/i }));

      await waitFor(() => {
        expect(mockOnSubmit).toHaveBeenCalledWith(
          expect.objectContaining({ client_secret: "rotated-secret" }),
        );
      });
    });
  });

  it("should call onCancel when cancel button is clicked", async () => {
    const user = userEvent.setup();
    render(<TenantForm {...defaultProps} />);

    const cancelButton = screen.getByRole("button", { name: /cancel/i });
    await user.click(cancelButton);

    expect(mockOnCancel).toHaveBeenCalledTimes(1);
  });

  it("should disable submit button when submitting", () => {
    render(<TenantForm {...defaultProps} isSubmitting={true} />);

    const submitButton = screen.getByRole("button", { name: /saving.../i });
    expect(submitButton).toBeDisabled();
  });

  it("should disable cancel button when submitting", () => {
    render(<TenantForm {...defaultProps} isSubmitting={true} />);

    const cancelButton = screen.getByRole("button", { name: /cancel/i });
    expect(cancelButton).toBeDisabled();
  });

  it("should display error message when error is present", () => {
    render(<TenantForm {...defaultProps} error="An error occurred" />);

    expect(screen.getByText("An error occurred")).toBeInTheDocument();
  });

  it("should not display error message when error is null", () => {
    render(<TenantForm {...defaultProps} />);

    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("should render enabled checkbox", () => {
    render(<TenantForm {...defaultProps} />);

    expect(
      screen.getByLabelText(/enable monitoring for this tenant/i),
    ).toBeInTheDocument();
  });

  it("should render is_default checkbox", () => {
    render(<TenantForm {...defaultProps} />);

    expect(screen.getByLabelText(/set as default tenant/i)).toBeInTheDocument();
  });

  it("should toggle enabled checkbox correctly", async () => {
    const user = userEvent.setup();
    render(<TenantForm {...defaultProps} />);

    const enabledCheckbox = screen.getByLabelText(
      /enable monitoring for this tenant/i,
    );
    expect(enabledCheckbox).toBeChecked();

    await user.click(enabledCheckbox);
    expect(enabledCheckbox).not.toBeChecked();
  });

  it("should render default realm selector when editing with realms", () => {
    const realms = [{ realm_name: "realm-1" }, { realm_name: "realm-2" }];
    const editingTenant = {
      tenant_id: "tenant-1",
      name: "Test Tenant",
      description: "",
      server_url: "https://keycloak.example.com",
      admin_realm: "master",
      client_id: "",
      enabled: true,
      is_default: false,
      default_realm: "",
    };

    render(
      <TenantForm
        {...defaultProps}
        isCreating={false}
        editingTenant={editingTenant}
        realms={realms}
      />,
    );

    const defaultRealmElements = screen.getAllByText("Default Realm");
    expect(defaultRealmElements.length).toBeGreaterThan(0);
  });

  it("should not render default realm selector when creating", () => {
    const realms = [{ realm_name: "realm-1" }, { realm_name: "realm-2" }];

    render(<TenantForm {...defaultProps} realms={realms} />);

    expect(screen.queryByLabelText(/default realm/i)).not.toBeInTheDocument();
  });

  it("should show Create Tenant button text when creating", () => {
    render(<TenantForm {...defaultProps} />);

    expect(
      screen.getByRole("button", { name: /create tenant/i }),
    ).toBeInTheDocument();
  });

  it("should show Save Changes button text when editing", () => {
    const editingTenant = {
      tenant_id: "tenant-1",
      name: "Test Tenant",
      description: "",
      server_url: "https://keycloak.example.com",
      admin_realm: "master",
      client_id: "",
      enabled: true,
      is_default: false,
      default_realm: "",
    };
    render(
      <TenantForm
        {...defaultProps}
        isCreating={false}
        editingTenant={editingTenant}
      />,
    );

    expect(
      screen.getByRole("button", { name: /save changes/i }),
    ).toBeInTheDocument();
  });

  it("should show Saving... button text when submitting", () => {
    render(<TenantForm {...defaultProps} isSubmitting={true} />);

    expect(
      screen.getByRole("button", { name: /saving.../i }),
    ).toBeInTheDocument();
  });

  it("should render client ID and client secret fields", () => {
    render(<TenantForm {...defaultProps} />);

    expect(screen.getByLabelText(/client id/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/client secret/i)).toBeInTheDocument();
  });

  it("should display tenant data values when editing", () => {
    const editingTenant = {
      tenant_id: "test-tenant",
      name: "Test Tenant",
      description: "Test Description",
      server_url: "https://keycloak.example.com",
      admin_realm: "master",
      client_id: "admin-cli",
      enabled: true,
      is_default: false,
      default_realm: "",
    };

    render(
      <TenantForm
        {...defaultProps}
        isCreating={false}
        editingTenant={editingTenant}
      />,
    );

    expect(screen.getByLabelText(/^name/i)).toHaveValue("Test Tenant");
    expect(screen.getByLabelText(/description/i)).toHaveValue(
      "Test Description",
    );
    expect(screen.getByLabelText(/server url/i)).toHaveValue(
      "https://keycloak.example.com",
    );
  });
});
