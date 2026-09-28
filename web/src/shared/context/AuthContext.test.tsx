import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, renderHook } from "@/test-utils";
import { AuthProvider, useAuth } from "./AuthContext";
import { authService } from "@/shared/services/authService";
import type { UserInfo, AuthConfig } from "@/shared/types";

vi.mock("@/shared/services/authService", () => ({
  authService: {
    getAuthConfig: vi.fn(),
    getUserInfo: vi.fn(),
    loginSimple: vi.fn(),
    loginOAuth2: vi.fn(),
    logout: vi.fn(),
  },
}));

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
        wrapper: AuthProvider,
      });

      expect(result.current.isLoading).toBe(true);
      expect(result.current.user).toBe(null);
      expect(result.current.isAuthenticated).toBe(false);
    });

    it("should load auth config and user info on mount", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockResolvedValue(mockUserInfo);

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthProvider,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.authConfig).toEqual(mockAuthConfig);
      expect(result.current.user).toEqual(mockUserInfo);
      expect(result.current.isAuthenticated).toBe(true);
    });

    it("should handle auth config failure", async () => {
      vi.mocked(authService.getAuthConfig).mockRejectedValue(
        new Error("Failed to get config"),
      );

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthProvider,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.authConfig).toBe(null);
      expect(result.current.user).toBe(null);
      expect(console.error).toHaveBeenCalledWith(
        "Failed to initialize auth:",
        expect.any(Error),
      );
    });

    it("should handle user not authenticated on mount", async () => {
      vi.mocked(authService.getAuthConfig).mockResolvedValue(mockAuthConfig);
      vi.mocked(authService.getUserInfo).mockRejectedValue(
        new Error("Not authenticated"),
      );

      const { result } = renderHook(() => useAuth(), {
        wrapper: AuthProvider,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.authConfig).toEqual(mockAuthConfig);
      expect(result.current.user).toBe(null);
      expect(result.current.isAuthenticated).toBe(false);
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
        wrapper: AuthProvider,
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
        wrapper: AuthProvider,
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
        wrapper: AuthProvider,
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
        wrapper: AuthProvider,
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
        wrapper: AuthProvider,
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
        wrapper: AuthProvider,
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
        <AuthProvider>
          <TestComponent />
        </AuthProvider>,
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
        wrapper: AuthProvider,
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
        wrapper: AuthProvider,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.isAuthenticated).toBe(false);
    });
  });
});
