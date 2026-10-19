import type { UserInfo, AuthConfig } from "@/shared/types/auth.types";

// Always use /auth - the web server proxies to the backend
const AUTH_BASE_URL = "/auth";

/** The server answered that there is no signed-in session (401). */
export class NotAuthenticatedError extends Error {
  constructor(message = "Not authenticated") {
    super(message);
    this.name = "NotAuthenticatedError";
  }
}

/**
 * The server could not be reached, or answered with an error other than 401,
 * as the web server's proxy does while the backend is down. That says nothing
 * about the session, so the caller should try again rather than show the
 * user as signed out.
 */
export class ServerUnavailableError extends Error {
  constructor(message = "Server unavailable") {
    super(message);
    this.name = "ServerUnavailableError";
  }
}

// A startup call that has not been answered after this long counts as the
// server being unavailable. Without it a backend that accepts connections but
// never answers left startup waiting on whatever timeouts the proxies in front
// have, 90 seconds through the web server.
const STARTUP_REQUEST_TIMEOUT_MS = 10000;

// fetchOrUnavailable turns a network failure, or no answer within
// STARTUP_REQUEST_TIMEOUT_MS, into ServerUnavailableError.
async function fetchOrUnavailable(
  input: string,
  init?: RequestInit,
): Promise<Response> {
  const controller = new AbortController();
  const timer = setTimeout(
    () => controller.abort(),
    STARTUP_REQUEST_TIMEOUT_MS,
  );
  try {
    return await fetch(input, { ...init, signal: controller.signal });
  } catch {
    throw new ServerUnavailableError(`Could not reach ${input}`);
  } finally {
    clearTimeout(timer);
  }
}

class AuthService {
  /**
   * Get auth configuration (which methods are enabled)
   */
  async getAuthConfig(): Promise<AuthConfig> {
    const response = await fetchOrUnavailable(`${AUTH_BASE_URL}/config`);
    if (!response.ok) {
      throw new ServerUnavailableError(
        `Failed to get auth config: HTTP ${response.status}`,
      );
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

    // Logout is a POST: as a GET, any page could end the session just by
    // linking to it. Go home whatever the outcome, so the button never seems
    // to do nothing; a failed logout leaves the session, which the next page
    // load shows.
    void fetch(`${AUTH_BASE_URL}/logout`, {
      method: "POST",
      credentials: "include",
    })
      .catch(() => {
        // Network failure: still leave the page
      })
      .finally(() => {
        window.location.href = "/";
      });
  }

  /**
   * Get current user info with RBAC data
   */
  async getUserInfo(): Promise<UserInfo> {
    const response = await fetchOrUnavailable(`/api/rbac/me`, {
      credentials: "include",
    });

    if (response.status === 401) {
      throw new NotAuthenticatedError();
    }
    if (!response.ok) {
      throw new ServerUnavailableError(
        `Failed to get user info: HTTP ${response.status}`,
      );
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
