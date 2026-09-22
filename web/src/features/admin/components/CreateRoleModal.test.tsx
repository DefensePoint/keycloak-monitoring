import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { CreateRoleModal } from "./CreateRoleModal";

vi.mock("./PermissionsSelector", () => ({
  PermissionsSelector: ({
    selectedPermissionIds,
    onTogglePermission,
  }: {
    selectedPermissionIds: number[];
    onTogglePermission: (id: number) => void;
  }) => (
    <div data-testid="permissions-selector">
      <button onClick={() => onTogglePermission(1)}>Toggle Permission 1</button>
      <div data-testid="selected-permissions">
        {selectedPermissionIds.join(",")}
      </div>
    </div>
  ),
}));

describe("CreateRoleModal", () => {
  const mockOnSubmit = vi.fn();
  const mockOnClose = vi.fn();

  const mockPermissions = [
    {
      id: 1,
      name: "users:read",
      display_name: "Read Users",
      description: "Read users",
      resource: "users",
      action: "read",
    },
    {
      id: 2,
      name: "users:write",
      display_name: "Write Users",
      description: "Write users",
      resource: "users",
      action: "write",
    },
  ];

  const defaultProps = {
    open: true,
    permissions: mockPermissions,
    onSubmit: mockOnSubmit,
    onClose: mockOnClose,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render modal title", () => {
    render(<CreateRoleModal {...defaultProps} />);
    expect(screen.getByText("Create New Role")).toBeInTheDocument();
  });

  it("should render all form fields", () => {
    render(<CreateRoleModal {...defaultProps} />);
    expect(
      screen.getByRole("textbox", { name: /role name/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: /display name/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: /description/i }),
    ).toBeInTheDocument();
  });

  it("should render PermissionsSelector component", () => {
    render(<CreateRoleModal {...defaultProps} />);
    expect(screen.getByTestId("permissions-selector")).toBeInTheDocument();
  });

  it("should allow typing in role name field", async () => {
    const user = userEvent.setup();
    render(<CreateRoleModal {...defaultProps} />);

    const roleNameInput = screen.getByRole("textbox", { name: /role name/i });
    await user.type(roleNameInput, "incident_responder");

    expect(roleNameInput).toHaveValue("incident_responder");
  });

  it("should allow typing in display name field", async () => {
    const user = userEvent.setup();
    render(<CreateRoleModal {...defaultProps} />);

    const displayNameInput = screen.getByRole("textbox", {
      name: /display name/i,
    });
    await user.type(displayNameInput, "Incident Responder");

    expect(displayNameInput).toHaveValue("Incident Responder");
  });

  it("should allow typing in description field", async () => {
    const user = userEvent.setup();
    render(<CreateRoleModal {...defaultProps} />);

    const descriptionInput = screen.getByRole("textbox", {
      name: /description/i,
    });
    await user.click(descriptionInput);
    await user.paste("Can respond to incidents");

    expect(descriptionInput).toHaveValue("Can respond to incidents");
  });

  it("should update selected permissions when permission is toggled", async () => {
    const user = userEvent.setup();
    render(<CreateRoleModal {...defaultProps} />);

    const toggleButton = screen.getByText("Toggle Permission 1");
    await user.click(toggleButton);

    await waitFor(() => {
      expect(screen.getByTestId("selected-permissions")).toHaveTextContent("1");
    });
  });

  it("should call onSubmit with form data when form is submitted with valid data", async () => {
    const user = userEvent.setup();
    render(<CreateRoleModal {...defaultProps} />);

    // First toggle permission to enable submit button
    await user.click(screen.getByText("Toggle Permission 1"));

    // Then fill in the form fields
    await user.type(
      screen.getByRole("textbox", { name: /role name/i }),
      "incident_responder",
    );
    await user.type(
      screen.getByRole("textbox", { name: /display name/i }),
      "Incident Responder",
    );
    const descriptionInput = screen.getByRole("textbox", {
      name: /description/i,
    });
    await user.click(descriptionInput);
    await user.paste("Can respond to incidents");

    // Submit the form
    await user.click(screen.getByRole("button", { name: /create role/i }));

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledWith({
        name: "incident_responder",
        display_name: "Incident Responder",
        description: "Can respond to incidents",
        permission_ids: [1],
      });
    });
  });

  it("should disable submit button when no permissions are selected", () => {
    render(<CreateRoleModal {...defaultProps} />);
    const submitButton = screen.getByRole("button", { name: /create role/i });
    expect(submitButton).toBeDisabled();
  });

  it("should enable submit button when permissions are selected", async () => {
    const user = userEvent.setup();
    render(<CreateRoleModal {...defaultProps} />);

    const toggleButton = screen.getByText("Toggle Permission 1");
    await user.click(toggleButton);

    await waitFor(() => {
      const submitButton = screen.getByRole("button", { name: /create role/i });
      expect(submitButton).not.toBeDisabled();
    });
  });

  it("should call onClose when cancel is clicked", async () => {
    const user = userEvent.setup();
    render(<CreateRoleModal {...defaultProps} />);

    const cancelButton = screen.getByRole("button", { name: /cancel/i });
    await user.click(cancelButton);

    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it("should render submit and cancel buttons", () => {
    render(<CreateRoleModal {...defaultProps} />);
    expect(screen.getByRole("button", { name: /cancel/i })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /create role/i }),
    ).toBeInTheDocument();
  });
});
