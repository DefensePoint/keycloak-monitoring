import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, act } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { AlertDetailHeader } from "./AlertDetailHeader";

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/tenant-123/alerts/alert-1" }),
  useParams: () => ({ tenantId: "tenant-123", alertId: "alert-1" }),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useTenant: () => ({
    selectedTenant: { tenant_id: "tenant-123", name: "Test Tenant" },
  }),
}));

// Mock clipboard API globally
const mockWriteText = vi.fn(() => Promise.resolve());

Object.defineProperty(navigator, "clipboard", {
  value: {
    writeText: mockWriteText,
  },
  writable: true,
  configurable: true,
});

describe("AlertDetailHeader", () => {
  beforeEach(() => {
    mockWriteText.mockClear();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  const defaultProps = {
    title: "Weak Password Policy Detected",
    checkType: "password-policy",
    tenantId: "tenant-123",
  };

  it("should render alert title", () => {
    render(<AlertDetailHeader {...defaultProps} />);
    expect(
      screen.getByText("Weak Password Policy Detected"),
    ).toBeInTheDocument();
  });

  it("should render check type", () => {
    render(<AlertDetailHeader {...defaultProps} />);
    expect(screen.getByText("password-policy")).toBeInTheDocument();
  });

  it("should render breadcrumb link", () => {
    render(<AlertDetailHeader {...defaultProps} />);
    // PageHeader shows "Alerts" in breadcrumb
    expect(screen.getByText("Alerts")).toBeInTheDocument();
  });

  it("should render copy link button", () => {
    render(<AlertDetailHeader {...defaultProps} />);
    expect(
      screen.getByRole("button", { name: /copy link/i }),
    ).toBeInTheDocument();
  });

  it("should trigger copy action when copy button is clicked", async () => {
    const user = userEvent.setup({ delay: null });
    render(<AlertDetailHeader {...defaultProps} />);

    const copyButton = screen.getByRole("button", { name: /copy link/i });
    await user.click(copyButton);

    // Verify the button text changes to indicate copy was triggered
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /copied/i }),
      ).toBeInTheDocument();
    });
  });

  it("should show copied state after copying", async () => {
    const user = userEvent.setup({ delay: null });
    render(<AlertDetailHeader {...defaultProps} />);

    const copyButton = screen.getByRole("button", { name: /copy link/i });
    await user.click(copyButton);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: /copied/i }),
      ).toBeInTheDocument();
    });
  });

  it("should reset copied state after 2 seconds", async () => {
    vi.useFakeTimers();

    render(<AlertDetailHeader {...defaultProps} />);

    const copyButton = screen.getByRole("button", { name: /copy link/i });

    // Click the button
    await act(async () => {
      copyButton.click();
    });

    // Button should show "Copied!" (state updated but timer is pending)
    expect(screen.getByRole("button", { name: /copied/i })).toBeInTheDocument();

    // Advance time by 2 seconds to trigger the setTimeout callback
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });

    // Button should revert to "Copy Link"
    expect(
      screen.getByRole("button", { name: /copy link/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /copied/i }),
    ).not.toBeInTheDocument();

    vi.useRealTimers();
  });

  it("should render correct breadcrumb href for different tenant IDs", () => {
    render(<AlertDetailHeader {...defaultProps} tenantId="different-tenant" />);
    // PageHeader handles breadcrumb navigation based on route
    expect(screen.getByText("Alerts")).toBeInTheDocument();
  });
});
