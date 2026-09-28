import type { UserInfo, AuthConfig } from "@/shared/types/auth.types";

// Always use /auth - the web server proxies to the backend
const AUTH_BASE_URL = "/auth";

class AuthService {
  /**
   * Get auth configuration (which methods are enabled)
   */
  async getAuthConfig(): Promise<AuthConfig> {
    const response = await fetch(`${AUTH_BASE_URL}/config`);
    if (!response.ok) {
      throw new Error("Failed to get auth config");
    }
    return response.json();
  }

  /**
   * Login with username/password (simple auth)
   */
  async loginSimple(username: string, password: string): Promise<UserInfo> {
    const response = await fetch(`${AUTH_BASE_URL}/login/simple`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ username, password }),
      credentials: "include",
    });

    const data = await response.json();

    if (!response.ok || !data.success) {
      throw new Error(data.message || "Login failed");
    }

    return data.user;
  }

  /**
   * Redirect to OAuth2 login page (initiates OAuth2 flow)
   */
  loginOAuth2(): void {
    window.location.href = `${AUTH_BASE_URL}/login/oauth2`;
  }

  /**
   * Logout and clear session
   */
  logout(): void {
    try {
      const keysToRemove: string[] = [];
      for (let i = 0; i < sessionStorage.length; i++) {
        const key = sessionStorage.key(i);
        if (key && key.startsWith("selectedRealm_")) {
          keysToRemove.push(key);
        }
      }
      keysToRemove.forEach((key) => sessionStorage.removeItem(key));
    } catch {
      // Silently fail if sessionStorage is unavailable
    }

    window.location.href = `${AUTH_BASE_URL}/logout`;
  }

  /**
   * Get current user info with RBAC data
   */
  async getUserInfo(): Promise<UserInfo> {
    const response = await fetch(`/api/rbac/me`, {
      credentials: "include",
    });

    if (!response.ok) {
      throw new Error("Not authenticated");
    }

    return response.json();
  }

  /**
   * Check if auth is enabled on the backend
   */
  async isAuthEnabled(): Promise<boolean> {
    try {
      const response = await fetch(`${AUTH_BASE_URL}/userinfo`, {
        credentials: "include",
      });
      if (response.status === 404) {
        return false;
      }
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if user is authenticated
   */
  async isAuthenticated(): Promise<boolean> {
    try {
      await this.getUserInfo();
      return true;
    } catch {
      return false;
    }
  }
}

export const authService = new AuthService();
