import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { EditUserModal } from "./EditUserModal";

describe("EditUserModal", () => {
  const mockOnSubmit = vi.fn();
  const mockOnClose = vi.fn();

  const mockLocalUser = {
    id: 1,
    preferred_username: "johndoe",
    email: "john@example.com",
    name: "John Doe",
    auth_method: "local",
    is_active: true,
    is_blocked: false,
  };

  const mockOAuthUser = {
    id: 2,
    preferred_username: "janedoe",
    email: "jane@example.com",
    name: "Jane Doe",
    auth_method: "oauth",
    is_active: true,
    is_blocked: false,
  };

  const defaultProps = {
    open: true,
    user: mockLocalUser,
    onSubmit: mockOnSubmit,
    onClose: mockOnClose,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render modal title with username", () => {
    render(<EditUserModal {...defaultProps} />);
    expect(screen.getByText("Edit User: johndoe")).toBeInTheDocument();
  });

  it("should not render when user is null", () => {
    const { container } = render(
      <EditUserModal {...defaultProps} user={null} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  describe("Local Auth User", () => {
    it("should render all editable form fields for local user", () => {
      render(<EditUserModal {...defaultProps} />);
      expect(
        screen.getByRole("textbox", { name: /username/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("textbox", { name: /email/i }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("textbox", { name: /full name/i }),
      ).toBeInTheDocument();
      expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
    });

    it("should display password placeholder text", () => {
      render(<EditUserModal {...defaultProps} />);
      const passwordInput = screen.getByLabelText(/password/i);
      expect(passwordInput).toHaveAttribute(
        "placeholder",
        "Leave empty to keep current password",
      );
    });

    it("should display password helper text", () => {
      render(<EditUserModal {...defaultProps} />);
      expect(
        screen.getByText(
          /Leave empty to keep current password. If changing, minimum 12 characters required./i,
        ),
      ).toBeInTheDocument();
    });

    it("should allow typing in username field", async () => {
      const user = userEvent.setup();
      render(<EditUserModal {...defaultProps} />);

      const usernameInput = screen.getByRole("textbox", { name: /username/i });
      await user.clear(usernameInput);
      await user.type(usernameInput, "newusername");

      expect(usernameInput).toHaveValue("newusername");
    });

    it("should render checkboxes for active and blocked status", () => {
      render(<EditUserModal {...defaultProps} />);
      expect(screen.getByLabelText("Active")).toBeInTheDocument();
      expect(screen.getByLabelText("Blocked")).toBeInTheDocument();
    });
  });

  describe("OAuth User", () => {
    it("should display OAuth info alert for OAuth user", () => {
      render(<EditUserModal {...defaultProps} user={mockOAuthUser} />);
      expect(screen.getByText("OAuth Authenticated User")).toBeInTheDocument();
      expect(
        screen.getByText(/Identity fields.*are managed by the OAuth provider/i),
      ).toBeInTheDocument();
    });

    it("should disable identity fields for OAuth user", () => {
      render(<EditUserModal {...defaultProps} user={mockOAuthUser} />);
      expect(screen.getByLabelText("Username")).toBeDisabled();
      expect(screen.getByLabelText("Email")).toBeDisabled();
      expect(screen.getByLabelText("Full Name")).toBeDisabled();
    });

    it("should not render password field for OAuth user", () => {
      render(<EditUserModal {...defaultProps} user={mockOAuthUser} />);
      expect(screen.queryByLabelText("Password")).not.toBeInTheDocument();
    });

    it("should still render status checkboxes for OAuth user", () => {
      render(<EditUserModal {...defaultProps} user={mockOAuthUser} />);
      expect(screen.getByLabelText("Active")).toBeInTheDocument();
      expect(screen.getByLabelText("Blocked")).toBeInTheDocument();
    });
  });

  it("should toggle active checkbox correctly", async () => {
    const user = userEvent.setup();
    render(<EditUserModal {...defaultProps} />);

    const activeCheckbox = screen.getByLabelText("Active");
    expect(activeCheckbox).toBeChecked();

    await user.click(activeCheckbox);
    expect(activeCheckbox).not.toBeChecked();
  });

  it("should toggle blocked checkbox correctly", async () => {
    const user = userEvent.setup();
    render(<EditUserModal {...defaultProps} />);

    const blockedCheckbox = screen.getByLabelText("Blocked");
    expect(blockedCheckbox).not.toBeChecked();

    await user.click(blockedCheckbox);
    expect(blockedCheckbox).toBeChecked();
  });

  it("should call onSubmit with form data when form is submitted with valid data", async () => {
    const user = userEvent.setup();
    render(<EditUserModal {...defaultProps} />);

    const usernameInput = screen.getByRole("textbox", { name: /username/i });
    const emailInput = screen.getByRole("textbox", { name: /email/i });
    const nameInput = screen.getByRole("textbox", { name: /full name/i });
    const passwordInput = screen.getByLabelText(/password/i);

    await user.clear(usernameInput);
    await user.type(usernameInput, "updateuser");
    await user.clear(emailInput);
    await user.type(emailInput, "updated@example.com");
    await user.clear(nameInput);
    await user.type(nameInput, "Updated User");
    await user.type(passwordInput, "NewPassword123!");

    const submitButton = screen.getByRole("button", {
      name: /save changes/i,
    });
    await user.click(submitButton);

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledWith({
        username: "updateuser",
        email: "updated@example.com",
        name: "Updated User",
        password: "NewPassword123!",
        is_active: true,
        is_blocked: false,
      });
    });
  });

  it("should call onClose when cancel is clicked", async () => {
    const user = userEvent.setup();
    render(<EditUserModal {...defaultProps} />);

    const cancelButton = screen.getByRole("button", { name: /cancel/i });
    await user.click(cancelButton);

    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it("should render submit and cancel buttons", () => {
    render(<EditUserModal {...defaultProps} />);
    expect(screen.getByRole("button", { name: /cancel/i })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /save changes/i }),
    ).toBeInTheDocument();
  });
});
