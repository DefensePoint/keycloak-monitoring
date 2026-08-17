import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { apiClient, ApiClient } from "./apiClient";

describe("ApiClient", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
    delete (window as unknown as { location: unknown }).location;
    window.location = { reload: vi.fn(), href: "" } as unknown as Location;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("get", () => {
    it("should make GET request with correct headers", async () => {
      const mockData = { id: 1, name: "Test" };
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(mockData),
      });

      const result = await apiClient.get("/test");

      expect(fetch).toHaveBeenCalledWith("/api/test", {
        method: "GET",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
        },
      });
      expect(result).toEqual(mockData);
    });

    it("should include query parameters in URL", async () => {
      const mockData = { items: [] };
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(mockData),
      });

      await apiClient.get("/items", { page: 1, limit: 10, active: true });

      expect(fetch).toHaveBeenCalledWith(
        "/api/items?page=1&limit=10&active=true",
        expect.any(Object),
      );
    });

    it("should skip undefined query parameters", async () => {
      const mockData = { items: [] };
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(mockData),
      });

      await apiClient.get("/items", { page: 1, filter: undefined });

      expect(fetch).toHaveBeenCalledWith(
        "/api/items?page=1",
        expect.any(Object),
      );
    });

    it("should redirect to login on 401 status", async () => {
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: false,
        status: 401,
      });

      await expect(apiClient.get("/test")).rejects.toThrow("Session expired");
      expect(window.location.href).toBe("/login?session=expired");
    });

    it("should throw error with message from response", async () => {
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: async () => ({ error: "Bad request" }),
      });

      await expect(apiClient.get("/test")).rejects.toThrow("Bad request");
    });

    it("should handle empty response", async () => {
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        text: async () => "",
      });

      const result = await apiClient.get("/test");

      expect(result).toEqual({});
    });
  });

  describe("post", () => {
    it("should make POST request with body", async () => {
      const mockData = { id: 1, name: "Created" };
      const requestBody = { name: "Test" };
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(mockData),
      });

      const result = await apiClient.post("/items", requestBody);

      expect(fetch).toHaveBeenCalledWith("/api/items", {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(requestBody),
      });
      expect(result).toEqual(mockData);
    });

    it("should make POST request without body", async () => {
      const mockData = { success: true };
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(mockData),
      });

      await apiClient.post("/action");

      expect(fetch).toHaveBeenCalledWith("/api/action", {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
        },
        body: undefined,
      });
    });

    it("should include query parameters with POST", async () => {
      const mockData = { success: true };
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(mockData),
      });

      await apiClient.post(
        "/items",
        { name: "Test" },
        { tenant: "test-tenant" },
      );

      expect(fetch).toHaveBeenCalledWith(
        "/api/items?tenant=test-tenant",
        expect.any(Object),
      );
    });
  });

  describe("put", () => {
    it("should make PUT request with body", async () => {
      const mockData = { id: 1, name: "Updated" };
      const requestBody = { name: "Updated Name" };
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(mockData),
      });

      const result = await apiClient.put("/items/1", requestBody);

      expect(fetch).toHaveBeenCalledWith("/api/items/1", {
        method: "PUT",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(requestBody),
      });
      expect(result).toEqual(mockData);
    });
  });

  describe("delete", () => {
    it("should make DELETE request", async () => {
      const mockData = { success: true };
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(mockData),
      });

      const result = await apiClient.delete("/items/1");

      expect(fetch).toHaveBeenCalledWith("/api/items/1", {
        method: "DELETE",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
        },
      });
      expect(result).toEqual(mockData);
    });

    it("should include query parameters with DELETE", async () => {
      const mockData = { success: true };
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(mockData),
      });

      await apiClient.delete("/items/1", { force: true });

      expect(fetch).toHaveBeenCalledWith(
        "/api/items/1?force=true",
        expect.any(Object),
      );
    });
  });

  describe("getBlob", () => {
    it("should return blob response", async () => {
      const mockBlob = new Blob(["test content"], { type: "text/plain" });
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        blob: async () => mockBlob,
      });

      const result = await apiClient.getBlob("/files/test.txt");

      expect(fetch).toHaveBeenCalledWith("/api/files/test.txt", {
        method: "GET",
        credentials: "include",
      });
      expect(result).toEqual(mockBlob);
    });

    it("should include query parameters with getBlob", async () => {
      const mockBlob = new Blob(["test"], { type: "text/plain" });
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        blob: async () => mockBlob,
      });

      await apiClient.getBlob("/files/test.txt", { format: "raw" });

      expect(fetch).toHaveBeenCalledWith(
        "/api/files/test.txt?format=raw",
        expect.any(Object),
      );
    });

    it("should throw error on failed blob request", async () => {
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: false,
        status: 404,
      });

      await expect(apiClient.getBlob("/files/missing.txt")).rejects.toThrow(
        "HTTP 404",
      );
    });
  });

  describe("custom base URL", () => {
    it("should use custom base URL when provided", async () => {
      const customClient = new ApiClient("/custom-api");
      const mockData = { id: 1 };
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(mockData),
      });

      await customClient.get("/test");

      expect(fetch).toHaveBeenCalledWith(
        "/custom-api/test",
        expect.any(Object),
      );
    });
  });

  describe("error handling", () => {
    it("should handle JSON parse error in error response", async () => {
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: async () => {
          throw new Error("Invalid JSON");
        },
      });

      await expect(apiClient.get("/test")).rejects.toThrow("Request failed");
    });

    it("should use HTTP status in error when no error message", async () => {
      (
        global.fetch as unknown as ReturnType<typeof vi.fn>
      ).mockResolvedValueOnce({
        ok: false,
        status: 503,
        json: async () => ({}),
      });

      await expect(apiClient.get("/test")).rejects.toThrow("HTTP 503");
    });
  });
});
