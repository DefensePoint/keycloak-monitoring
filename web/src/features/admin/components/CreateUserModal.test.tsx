import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { CreateUserModal } from "./CreateUserModal";

describe("CreateUserModal", () => {
  const mockOnSubmit = vi.fn();
  const mockOnClose = vi.fn();

  const defaultProps = {
    open: true,
    onSubmit: mockOnSubmit,
    onClose: mockOnClose,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render modal title", () => {
    render(<CreateUserModal {...defaultProps} />);
    expect(screen.getByText("Create New User")).toBeInTheDocument();
  });

  it("should render all form fields", () => {
    render(<CreateUserModal {...defaultProps} />);
    expect(
      screen.getByRole("textbox", { name: /username/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /email/i })).toBeInTheDocument();
    expect(
      screen.getByRole("textbox", { name: /full name/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("Password")).toBeInTheDocument();
  });

  it("should allow typing in all form fields", async () => {
    const user = userEvent.setup();
    render(<CreateUserModal {...defaultProps} />);

    const usernameInput = screen.getByRole("textbox", { name: /username/i });
    const emailInput = screen.getByRole("textbox", { name: /email/i });
    const nameInput = screen.getByRole("textbox", { name: /full name/i });
    const passwordInput = screen.getByLabelText(/password/i);

    await user.type(usernameInput, "johndoe");
    await user.type(emailInput, "john@example.com");
    await user.type(nameInput, "John Doe");
    await user.type(passwordInput, "SecurePassword123!");

    expect(usernameInput).toHaveValue("johndoe");
    expect(emailInput).toHaveValue("john@example.com");
    expect(nameInput).toHaveValue("John Doe");
    expect(passwordInput).toHaveValue("SecurePassword123!");
  });

  it("should call onSubmit with form data when form is submitted with valid data", async () => {
    const user = userEvent.setup();
    render(<CreateUserModal {...defaultProps} />);

    const usernameInput = screen.getByRole("textbox", { name: /username/i });
    const emailInput = screen.getByRole("textbox", { name: /email/i });
    const nameInput = screen.getByRole("textbox", { name: /full name/i });
    const passwordInput = screen.getByLabelText(/password/i);

    await user.type(usernameInput, "testuser");
    await user.type(emailInput, "test@example.com");
    await user.type(nameInput, "Test User");
    await user.type(passwordInput, "SecurePassword123!");

    const submitButton = screen.getByRole("button", { name: /create user/i });
    await user.click(submitButton);

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledWith({
        username: "testuser",
        email: "test@example.com",
        name: "Test User",
        password: "SecurePassword123!",
      });
    });
  });

  it("should call onClose when cancel is clicked", async () => {
    const user = userEvent.setup();
    render(<CreateUserModal {...defaultProps} />);

    const cancelButton = screen.getByRole("button", { name: /cancel/i });
    await user.click(cancelButton);

    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it("should have password field with correct type", () => {
    render(<CreateUserModal {...defaultProps} />);
    const passwordInput = screen.getByLabelText(/password/i);
    expect(passwordInput).toHaveAttribute("type", "password");
  });

  it("should display password helper text", () => {
    render(<CreateUserModal {...defaultProps} />);
    expect(
      screen.getByText(
        /Minimum 12 characters with uppercase, lowercase, number, and special character/i,
      ),
    ).toBeInTheDocument();
  });

  it("should have email field with email type", () => {
    render(<CreateUserModal {...defaultProps} />);
    const emailInput = screen.getByRole("textbox", { name: /email/i });
    expect(emailInput).toHaveAttribute("type", "email");
  });

  it("should mark all fields as required", () => {
    render(<CreateUserModal {...defaultProps} />);
    expect(screen.getByRole("textbox", { name: /username/i })).toBeRequired();
    expect(screen.getByRole("textbox", { name: /email/i })).toBeRequired();
    expect(screen.getByRole("textbox", { name: /full name/i })).toBeRequired();
    expect(screen.getByLabelText(/password/i)).toBeRequired();
  });

  it("should render submit and cancel buttons", () => {
    render(<CreateUserModal {...defaultProps} />);
    expect(screen.getByRole("button", { name: /cancel/i })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /create user/i }),
    ).toBeInTheDocument();
  });
});
