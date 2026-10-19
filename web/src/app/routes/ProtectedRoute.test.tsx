import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@/test-utils";
import { ProtectedRoute } from "./ProtectedRoute";

const mockUseAuth = vi.fn();

vi.mock("@/shared/context", () => ({
  useAuth: () => mockUseAuth(),
  TenantProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

vi.mock("@/features/auth", () => ({
  LoginPage: () => <div>Sign-in form</div>,
}));

vi.mock("@/shared/components/Layout", () => ({
  Layout: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

const signedOut = {
  isAuthenticated: false,
  isLoading: false,
  serverUnavailable: false,
  nextRetryAt: null,
  retryNow: vi.fn(),
};

// A backend that was down when the page loaded used to leave a bare sign-in
// card with no way to sign in. The page now says the server can't be reached,
// counts down to the next attempt and offers to try at once.
describe("ProtectedRoute", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-14T12:00:00Z"));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("says the server can't be reached, and when it will try again", () => {
    mockUseAuth.mockReturnValue({
      ...signedOut,
      serverUnavailable: true,
      nextRetryAt: Date.now() + 8000,
    });

    render(<ProtectedRoute>app</ProtectedRoute>);

    expect(screen.getByText("Can't reach the KMT server.")).toBeInTheDocument();
    expect(screen.getByText("Retrying in 8s…")).toBeInTheDocument();
    expect(screen.queryByText("Sign-in form")).not.toBeInTheDocument();
    expect(screen.queryByText("app")).not.toBeInTheDocument();
  });

  it("retries at once on Retry now", () => {
    const retryNow = vi.fn();
    mockUseAuth.mockReturnValue({
      ...signedOut,
      serverUnavailable: true,
      nextRetryAt: Date.now() + 30000,
      retryNow,
    });

    render(<ProtectedRoute>app</ProtectedRoute>);
    fireEvent.click(screen.getByRole("button", { name: "Retry now" }));

    expect(retryNow).toHaveBeenCalledTimes(1);
  });

  it("shows the sign-in form to a signed-out user once the server answers", () => {
    mockUseAuth.mockReturnValue(signedOut);

    render(<ProtectedRoute>app</ProtectedRoute>);

    expect(screen.getByText("Sign-in form")).toBeInTheDocument();
    expect(
      screen.queryByText("Can't reach the KMT server."),
    ).not.toBeInTheDocument();
  });
});
