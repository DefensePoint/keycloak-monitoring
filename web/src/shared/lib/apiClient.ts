const API_BASE_URL = "/api";

// Dispatched on window when the API answers 401. The auth context signs the
// user out, so the login form shows at the current URL and signing in again
// returns to the same page.
export const SESSION_EXPIRED_EVENT = "kmt:session-expired";

function sessionExpired(): Error {
  window.dispatchEvent(new Event(SESSION_EXPIRED_EVENT));
  return new Error("Session expired");
}

interface RequestConfig extends RequestInit {
  params?: Record<string, string | number | boolean | undefined>;
}

class ApiClient {
  private baseUrl: string;

  constructor(baseUrl: string = API_BASE_URL) {
    this.baseUrl = baseUrl;
  }

  private buildUrl(
    endpoint: string,
    params?: Record<string, string | number | boolean | undefined>,
  ): string {
    const url = `${this.baseUrl}${endpoint}`;
    if (!params) return url;

    const searchParams = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) {
        searchParams.append(key, String(value));
      }
    });

    const queryString = searchParams.toString();
    return queryString ? `${url}?${queryString}` : url;
  }

  private async request<T>(
    endpoint: string,
    config: RequestConfig = {},
  ): Promise<T> {
    const { params, ...fetchConfig } = config;
    const url = this.buildUrl(endpoint, params);

    const response = await fetch(url, {
      ...fetchConfig,
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        ...fetchConfig.headers,
      },
    });

    if (response.status === 401) {
      throw sessionExpired();
    }

    if (!response.ok) {
      const error = await response
        .json()
        .catch(() => ({ error: "Request failed" }));
      throw httpError(
        error.error || `HTTP ${response.status}`,
        response.status,
      );
    }

    // Handle empty responses
    const text = await response.text();
    if (!text) return {} as T;

    return JSON.parse(text);
  }

  async get<T>(
    endpoint: string,
    params?: Record<string, string | number | boolean | undefined>,
  ): Promise<T> {
    return this.request<T>(endpoint, { method: "GET", params });
  }

  async post<T>(
    endpoint: string,
    data?: unknown,
    params?: Record<string, string | number | boolean | undefined>,
  ): Promise<T> {
    return this.request<T>(endpoint, {
      method: "POST",
      body: data ? JSON.stringify(data) : undefined,
      params,
    });
  }

  async put<T>(
    endpoint: string,
    data?: unknown,
    params?: Record<string, string | number | boolean | undefined>,
  ): Promise<T> {
    return this.request<T>(endpoint, {
      method: "PUT",
      body: data ? JSON.stringify(data) : undefined,
      params,
    });
  }

  async delete<T>(
    endpoint: string,
    params?: Record<string, string | number | boolean | undefined>,
  ): Promise<T> {
    return this.request<T>(endpoint, { method: "DELETE", params });
  }

  async getBlob(
    endpoint: string,
    params?: Record<string, string | number | boolean | undefined>,
  ): Promise<Blob> {
    const url = this.buildUrl(endpoint, params);

    const response = await fetch(url, {
      method: "GET",
      credentials: "include",
    });

    if (response.status === 401) {
      throw sessionExpired();
    }

    if (!response.ok) {
      throw httpError(`HTTP ${response.status}`, response.status);
    }

    return response.blob();
  }
}

export const apiClient = new ApiClient();
export { ApiClient };

/** An Error from a request the server answered with a failing status. */
export interface HttpError extends Error {
  status: number;
}

function httpError(message: string, status: number): HttpError {
  return Object.assign(new Error(message), { status });
}

/** The HTTP status a failed request was answered with, if it got one. */
export function httpStatusOf(error: unknown): number | undefined {
  const status = (error as { status?: unknown } | null)?.status;
  return typeof status === "number" ? status : undefined;
}
