import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  authService,
  NotAuthenticatedError,
  ServerUnavailableError,
} from "./authService";

// Logout used to navigate to GET /auth/logout, so any page could end the
// session by linking there. It is a POST now.
describe("authService.logout", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
    delete (window as unknown as { location: unknown }).location;
    window.location = { href: "" } as unknown as Location;
  });

  it("sends a POST with the session cookie, then goes home", async () => {
    (global.fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValue(
      new Response(JSON.stringify({ success: true }), { status: 200 }),
    );

    authService.logout();

    await vi.waitFor(() => expect(window.location.href).toBe("/"));
    expect(fetch).toHaveBeenCalledWith("/auth/logout", {
      method: "POST",
      credentials: "include",
    });
  });

  it("never navigates to the logout URL itself", async () => {
    (global.fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValue(
      new Response("{}", { status: 200 }),
    );

    authService.logout();

    await vi.waitFor(() => expect(window.location.href).toBe("/"));
    expect(window.location.href).not.toContain("/auth/logout");
  });

  it("still goes home when the request fails", async () => {
    (global.fetch as unknown as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("network down"),
    );

    authService.logout();

    await vi.waitFor(() => expect(window.location.href).toBe("/"));
  });
});

// At startup a failed call used to read as "signed out", so a backend that
// was down showed an empty sign-in card that never recovered. Only a 401
// means signed out; anything else means the server could not be reached.
describe("authService startup calls", () => {
  const mockFetch = () => global.fetch as unknown as ReturnType<typeof vi.fn>;

  beforeEach(() => {
    global.fetch = vi.fn();
  });

  it("reports a 401 from /api/rbac/me as signed out", async () => {
    mockFetch().mockResolvedValue(new Response("{}", { status: 401 }));

    const err = await authService.getUserInfo().catch((e: unknown) => e);

    expect(err).toBeInstanceOf(NotAuthenticatedError);
    expect(err).not.toBeInstanceOf(ServerUnavailableError);
  });

  it.each([502, 503, 500])(
    "reports a %i from /api/rbac/me as the server being unavailable",
    async (status) => {
      mockFetch().mockResolvedValue(new Response("Bad Gateway", { status }));

      await expect(authService.getUserInfo()).rejects.toBeInstanceOf(
        ServerUnavailableError,
      );
    },
  );

  it("reports a network failure of /api/rbac/me as the server being unavailable", async () => {
    mockFetch().mockRejectedValue(new TypeError("Failed to fetch"));

    await expect(authService.getUserInfo()).rejects.toBeInstanceOf(
      ServerUnavailableError,
    );
  });

  it.each([
    ["a 502", () => Promise.resolve(new Response("", { status: 502 }))],
    [
      "a network failure",
      () => Promise.reject(new TypeError("Failed to fetch")),
    ],
  ])(
    "reports %s of /auth/config as the server being unavailable",
    async (_, answer) => {
      mockFetch().mockImplementation(answer);

      await expect(authService.getAuthConfig()).rejects.toBeInstanceOf(
        ServerUnavailableError,
      );
    },
  );
});

// A backend that accepts connections but never answers left startup on its
// loading spinner for as long as the proxies in front kept the request open
// (90 seconds through the web server, as the browser resent it twice). The
// startup calls give up on their own after 10 seconds.
describe("authService startup calls on a server that does not answer", () => {
  const hang = (_: RequestInfo | URL, init?: RequestInit) =>
    new Promise<Response>((_, reject) => {
      init?.signal?.addEventListener("abort", () =>
        reject(new DOMException("The operation was aborted.", "AbortError")),
      );
    });

  beforeEach(() => {
    vi.useFakeTimers();
    global.fetch = vi.fn(hang) as unknown as typeof fetch;
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it.each([
    ["/auth/config", () => authService.getAuthConfig()],
    ["/api/rbac/me", () => authService.getUserInfo()],
  ])(
    "gives up on %s after 10 seconds as the server being unavailable",
    async (_, call) => {
      let settled: unknown = "pending";
      void call().then(
        () => (settled = "resolved"),
        (e: unknown) => (settled = e),
      );

      await vi.advanceTimersByTimeAsync(9999);
      expect(settled).toBe("pending");
      await vi.advanceTimersByTimeAsync(1);

      expect(settled).toBeInstanceOf(ServerUnavailableError);
    },
  );
});
