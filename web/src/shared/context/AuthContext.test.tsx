import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, renderHook, act } from "@/test-utils";
import { AuthProvider, useAuth } from "./AuthContext";
import {
  authService,
  NotAuthenticatedError,
  ServerUnavailableError,
} from "@/shared/services/authService";
import { SESSION_EXPIRED_EVENT } from "@/shared/lib/apiClient";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import type { UserInfo, AuthConfig } from "@/shared/types";

vi.mock("@/shared/services/authService", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/shared/services/authService")>()),
  authService: {
    getAuthConfig: vi.fn(),
    getUserInfo: vi.fn(),
    loginSimple: vi.fn(),
    loginOAuth2: vi.fn(),
    logout: vi.fn(),
  },
}));

let queryClient: QueryClient;

const AuthWrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={queryClient}>
    <AuthProvider>{children}</AuthProvider>
  </QueryClientProvider>
);

describe("AuthContext", () => {
  const mockUserInfo: UserInfo = {
    subject: "test-subject-123",
    email: "test@example.com",
    email_verified: true,
    name: "Test User",
    given_name: "Test",
    family_name: "User",
    preferred_username: "testuser",
    locale: "en",
    updated_at: new Date().toISOString(),
  };

  const mockAuthConfig: AuthConfig = {
    simple_enabled: true,
    oauth2_enabled: true,
    auth_required: true,
  };

  beforeEach(() => {
    queryClient = new QueryClient();
    vi.clearAllMocks();
    // Suppress console.error for expected errors in tests
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("AuthProvider", () => {
    it("should initialize with loading state", () => {
      vi.mocked(authService.getAuthConfig).mockImplementation(
        () => new Promise(() => {}), // Never resolves
      );

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      expect(result.current.isLoading).toBe(true);
      expect(result.current.user).toBe(null);
      expect(result.current.isAuthenticated).toBe(false);
    });

    it("should load auth config and user info on mount", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockResolvedValue(mockUserInfo);

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.authConfig).toEqual(mockAuthConfig);
      expect(result.current.user).toEqual(mockUserInfo);
      expect(result.current.isAuthenticated).toBe(true);
    });

    it("reports an unreachable server instead of an empty sign-in", async () => {
      vi.mocked(authService.getAuthConfig).mockRejectedValue(
        new ServerUnavailableError("Failed to get auth config"),
      );

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.serverUnavailable).toBe(true);
      expect(result.current.authConfig).toBe(null);
      expect(result.current.user).toBe(null);
    });

    it("should handle user not authenticated on mount", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockRejectedValue(
        new Error("Not authenticated"),
      );

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.authConfig).toEqual(mockAuthConfig);
      expect(result.current.user).toBe(null);
      expect(result.current.isAuthenticated).toBe(false);
    });
  });

  // At startup the config and the session are read once. A backend that was
  // down then left an empty sign-in card for good; now startup is retried
  // with backoff until the server answers, and only a 401 means signed out.
  describe("when the server can't be reached at startup", () => {
    beforeEach(() => {
      vi.useFakeTimers();
    });
    afterEach(() => {
      vi.useRealTimers();
    });

    const unavailable = () =>
      new ServerUnavailableError("Failed to get auth config");
    const advance = async (ms: number) => {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(ms);
      });
    };

    it("keeps retrying the config and continues once the server answers", async () => {
      vi.mocked(authService.getAuthConfig)
        .mockRejectedValueOnce(unavailable())
        .mockRejectedValueOnce(unavailable())
        .mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockRejectedValue(
        new NotAuthenticatedError(),
      );

      const { result } = renderHook(() => useAuth(), { wrapper: AuthWrapper });
      await advance(0);
      expect(result.current.isLoading).toBe(false);
      expect(result.current.serverUnavailable).toBe(true);

      await advance(2000);
      expect(result.current.serverUnavailable).toBe(true);
      await advance(4000);

      expect(result.current.serverUnavailable).toBe(false);
      expect(result.current.authConfig).toEqual(mockAuthConfig);
      expect(result.current.isAuthenticated).toBe(false);
      expect(authService.getAuthConfig).toHaveBeenCalledTimes(3);
    });

    it("does not take an unreachable server for a signed-out user", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo)
        .mockRejectedValueOnce(new ServerUnavailableError("502"))
        .mockResolvedValue(mockUserInfo);

      const { result } = renderHook(() => useAuth(), { wrapper: AuthWrapper });
      await advance(0);
      expect(result.current.serverUnavailable).toBe(true);
      expect(result.current.isAuthenticated).toBe(false);

      await advance(2000);

      expect(result.current.serverUnavailable).toBe(false);
      expect(result.current.isAuthenticated).toBe(true);
    });

    it("backs off from 2 seconds up to 30 and stays there", async () => {
      vi.mocked(authService.getAuthConfig).mockRejectedValue(unavailable());
      const calls = () =>
        vi.mocked(authService.getAuthConfig).mock.calls.length;

      const { result } = renderHook(() => useAuth(), { wrapper: AuthWrapper });
      await advance(0);
      const attempts = [calls()];
      for (const wait of [2000, 4000, 8000, 16000, 30000, 30000]) {
        await advance(wait - 1);
        expect(calls()).toBe(attempts.length);
        await advance(1);
        attempts.push(calls());
      }

      expect(attempts).toEqual([1, 2, 3, 4, 5, 6, 7]);
      expect(result.current.serverUnavailable).toBe(true);
      expect(result.current.nextRetryAt).toBe(Date.now() + 30000);
    });

    it("retries at once on Retry now", async () => {
      vi.mocked(authService.getAuthConfig)
        .mockRejectedValueOnce(unavailable())
        .mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockResolvedValue(mockUserInfo);

      const { result } = renderHook(() => useAuth(), { wrapper: AuthWrapper });
      await advance(0);
      expect(result.current.serverUnavailable).toBe(true);

      act(() => {
        result.current.retryNow();
      });
      await advance(0);

      expect(result.current.serverUnavailable).toBe(false);
      expect(result.current.isAuthenticated).toBe(true);
    });

    it.each([
      [
        "the browser comes back online",
        () => window.dispatchEvent(new Event("online")),
      ],
      [
        "the tab is shown again",
        () => document.dispatchEvent(new Event("visibilitychange")),
      ],
    ])("retries at once when %s", async (_, trigger) => {
      vi.mocked(authService.getAuthConfig)
        .mockRejectedValueOnce(unavailable())
        .mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockResolvedValue(mockUserInfo);

      const { result } = renderHook(() => useAuth(), { wrapper: AuthWrapper });
      await advance(0);
      expect(result.current.serverUnavailable).toBe(true);

      act(() => {
        trigger();
      });
      await advance(0);

      expect(result.current.isAuthenticated).toBe(true);
    });

    it("does not retry once the server has answered that the user is signed out", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockRejectedValue(
        new NotAuthenticatedError(),
      );

      const { result } = renderHook(() => useAuth(), { wrapper: AuthWrapper });
      await advance(0);
      await advance(60000);
      act(() => {
        window.dispatchEvent(new Event("online"));
      });
      await advance(0);

      expect(result.current.serverUnavailable).toBe(false);
      expect(authService.getAuthConfig).toHaveBeenCalledTimes(1);
    });

    it("stops retrying when the app goes away", async () => {
      vi.mocked(authService.getAuthConfig).mockRejectedValue(unavailable());

      const { unmount } = renderHook(() => useAuth(), { wrapper: AuthWrapper });
      await advance(0);
      unmount();
      await advance(120000);

      expect(authService.getAuthConfig).toHaveBeenCalledTimes(1);
    });
  });

  describe("an expired session", () => {
    it("signs the user out in place and says the session expired", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockResolvedValue(mockUserInfo);
      vi.mocked(authService.loginSimple).mockResolvedValue(mockUserInfo);

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });
      await waitFor(() => {
        expect(result.current.isAuthenticated).toBe(true);
      });
      expect(result.current.sessionExpired).toBe(false);

      act(() => {
        window.dispatchEvent(new Event(SESSION_EXPIRED_EVENT));
      });

      expect(result.current.isAuthenticated).toBe(false);
      expect(result.current.sessionExpired).toBe(true);

      await act(async () => {
        await result.current.loginSimple("testuser", "password123");
      });

      expect(result.current.isAuthenticated).toBe(true);
      expect(result.current.sessionExpired).toBe(false);
    });

    it("leaves nothing of the previous user's data to the next one", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockResolvedValue(mockUserInfo);
      vi.mocked(authService.loginSimple).mockResolvedValue(mockUserInfo);

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });
      await waitFor(() => {
        expect(result.current.isAuthenticated).toBe(true);
      });
      queryClient.setQueryData(["alerts", "demo1"], ["an alert in a realm"]);

      act(() => {
        window.dispatchEvent(new Event(SESSION_EXPIRED_EVENT));
      });
      await act(async () => {
        await result.current.loginSimple("someone-else", "password123");
      });

      expect(queryClient.getQueryData(["alerts", "demo1"])).toBeUndefined();
    });

    it("is not reported to someone who was never signed in", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockRejectedValue(
        new Error("Not authenticated"),
      );

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });
      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      act(() => {
        window.dispatchEvent(new Event(SESSION_EXPIRED_EVENT));
      });

      expect(result.current.isAuthenticated).toBe(false);
      expect(result.current.sessionExpired).toBe(false);
    });
  });

  describe("loginSimple", () => {
    it("should login user successfully", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo)
        .mockRejectedValueOnce(new Error("Not authenticated")) // First call during mount
        .mockResolvedValueOnce(mockUserInfo); // Second call after login
      vi.mocked(authService.loginSimple).mockResolvedValue(mockUserInfo);

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      await result.current.loginSimple("testuser", "password123");

      expect(authService.loginSimple).toHaveBeenCalledWith(
        "testuser",
        "password123",
      );

      await waitFor(() => {
        expect(result.current.user).toEqual(mockUserInfo);
      });

      expect(result.current.isAuthenticated).toBe(true);
    });

    it("should throw error on login failure", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockRejectedValue(
        new Error("Not authenticated"),
      );
      vi.mocked(authService.loginSimple).mockRejectedValue(
        new Error("Invalid credentials"),
      );

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      await expect(
        result.current.loginSimple("testuser", "wrongpass"),
      ).rejects.toThrow("Invalid credentials");
      expect(result.current.user).toBe(null);
    });
  });

  describe("loginOAuth2", () => {
    it("should call authService.loginOAuth2", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockRejectedValue(
        new Error("Not authenticated"),
      );

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      result.current.loginOAuth2();

      expect(authService.loginOAuth2).toHaveBeenCalled();
    });
  });

  describe("logout", () => {
    it("should call authService.logout", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockResolvedValue(mockUserInfo);

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      result.current.logout();

      expect(authService.logout).toHaveBeenCalled();
    });
  });

  describe("refreshUser", () => {
    it("should refresh user info successfully", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo)
        .mockRejectedValueOnce(new Error("Not authenticated"))
        .mockResolvedValueOnce(mockUserInfo);

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.user).toBe(null);

      await result.current.refreshUser();

      await waitFor(() => {
        expect(result.current.user).toEqual(mockUserInfo);
      });

      expect(result.current.isAuthenticated).toBe(true);
    });

    it("should handle refresh failure", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo)
        .mockResolvedValueOnce(mockUserInfo)
        .mockRejectedValueOnce(new Error("Session expired"));

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.user).toEqual(mockUserInfo);

      await result.current.refreshUser();

      await waitFor(() => {
        expect(result.current.user).toBe(null);
      });

      expect(result.current.isAuthenticated).toBe(false);
    });
  });

  describe("useAuth hook", () => {
    it("should throw error when used outside AuthProvider", () => {
      // Temporarily override console.error to silence expected error
      const consoleError = console.error;
      console.error = vi.fn();

      expect(() => {
        renderHook(() => useAuth());
      }).toThrow("useAuth must be used within an AuthProvider");

      console.error = consoleError;
    });

    it("should provide auth context when used within AuthProvider", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockResolvedValue(mockUserInfo);

      const TestComponent = () => {
        const auth = useAuth();
        return <div data-testid="test">{auth.user?.preferred_username}</div>;
      };

      render(
        <AuthWrapper>
          <TestComponent />
        </AuthWrapper>,
      );

      await waitFor(() => {
        expect(screen.getByTestId("test")).toHaveTextContent("testuser");
      });
    });
  });

  describe("isAuthenticated", () => {
    it("should be true when user is present", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockResolvedValue(mockUserInfo);

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.isAuthenticated).toBe(true);
    });

    it("should be false when user is null", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockRejectedValue(
        new Error("Not authenticated"),
      );

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthWrapper,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.isAuthenticated).toBe(false);
    });
  });

  describe("isAdmin", () => {
    // Mirrors rbac.IsAdmin: /api/rbac/me returns every role row, so the
    // client must drop the ones the server does not count.
    const withRoles = async (roles: NonNullable<UserInfo["roles"]>) => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockResolvedValue({
        ...mockUserInfo,
        roles,
      });
      const { result } = renderHook(() => useAuth(), { wrapper: AuthWrapper });
      await waitFor(() => expect(result.current.isLoading).toBe(false));
      return result.current.isAdmin();
    };
    const role = (name: string, extra: Record<string, unknown> = {}) =>
      ({
        id: 1,
        user_id: 1,
        role_id: 1,
        role: { id: 1, name },
        assigned_at: "2026-01-01T00:00:00Z",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
        ...extra,
      }) as unknown as NonNullable<UserInfo["roles"]>[number];
    const inAnHour = new Date(Date.now() + 3_600_000).toISOString();
    const anHourAgo = new Date(Date.now() - 3_600_000).toISOString();

    it.each([
      ["a global admin grant", true, [role("admin", { tenant_id: null })]],
      ["a global grant sent with tenant_id omitted", true, [role("admin")]],
      [
        "a grant that expires later",
        true,
        [role("admin", { expires_at: inAnHour })],
      ],
      ["an expired grant", false, [role("admin", { expires_at: anHourAgo })]],
      [
        "a tenant-scoped admin grant",
        false,
        [role("admin", { tenant_id: "tenant-1" })],
      ],
      ["a non-admin role", false, [role("viewer", { tenant_id: null })]],
      [
        "an expired grant beside a live one",
        true,
        [role("admin", { expires_at: anHourAgo }), role("admin")],
      ],
    ])("%s -> %s", async (_label, want, roles) => {
      expect(await withRoles(roles)).toBe(want);
    });
  });
  // /api/rbac/me sends a global role row with tenant_id omitted, not null;
  // the server grants such a role in every tenant, and so must the UI.
  describe("tenant access and roles", () => {
    const signedIn = async (roles: NonNullable<UserInfo["roles"]>) => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockResolvedValue({
        ...mockUserInfo,
        roles,
      });
      const { result } = renderHook(() => useAuth(), { wrapper: AuthWrapper });
      await waitFor(() => expect(result.current.isLoading).toBe(false));
      return result.current;
    };
    const role = (name: string, extra: Record<string, unknown> = {}) =>
      ({
        id: 1,
        user_id: 1,
        role_id: 1,
        role: { id: 1, name },
        assigned_at: "2026-01-01T00:00:00Z",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
        ...extra,
      }) as unknown as NonNullable<UserInfo["roles"]>[number];

    it.each([
      [
        "a global role sent with tenant_id omitted",
        true,
        true,
        [role("viewer")],
      ],
      [
        "a global role sent as null",
        true,
        true,
        [role("viewer", { tenant_id: null })],
      ],
      [
        "a role for tenant-1 only",
        true,
        false,
        [role("viewer", { tenant_id: "tenant-1" })],
      ],
      [
        "a role for tenant-1 and a global one",
        true,
        true,
        [role("viewer", { tenant_id: "tenant-1" }), role("operator")],
      ],
      ["no roles", false, false, []],
    ])(
      "%s: tenant-1 %s, tenant-2 %s",
      async (_label, tenant1, tenant2, roles) => {
        const auth = await signedIn(roles);
        expect(auth.hasAccessToTenant("tenant-1")).toBe(tenant1);
        expect(auth.hasAccessToTenant("tenant-2")).toBe(tenant2);
      },
    );

    it("counts a global role as held in every tenant", async () => {
      const auth = await signedIn([
        role("viewer"),
        role("operator", { tenant_id: "tenant-1" }),
      ]);
      expect(auth.hasRole("viewer", "tenant-2")).toBe(true);
      expect(auth.hasRole("operator", "tenant-1")).toBe(true);
      expect(auth.hasRole("operator", "tenant-2")).toBe(false);
    });
  });
});
