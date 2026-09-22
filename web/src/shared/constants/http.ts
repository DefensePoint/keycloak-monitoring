/**
 * HTTP Configuration Constants
 *
 * Centralized HTTP-related constants for API requests and responses.
 */

// HTTP configuration for fetch requests
export const HTTP_CONFIG = {
  CREDENTIALS: "include" as RequestCredentials,
  MODE: "cors" as RequestMode,
  CACHE: "no-cache" as RequestCache,
} as const;

// HTTP configuration for axios requests
export const AXIOS_CONFIG = {
  WITH_CREDENTIALS: true,
  TIMEOUT: 30000, // 30 seconds
} as const;

// HTTP header names
export const HTTP_HEADERS = {
  CONTENT_TYPE: "Content-Type",
  AUTHORIZATION: "Authorization",
  ACCEPT: "Accept",
  CACHE_CONTROL: "Cache-Control",
} as const;

// Content type values
export const CONTENT_TYPES = {
  JSON: "application/json",
  FORM: "application/x-www-form-urlencoded",
  FORM_DATA: "multipart/form-data",
  TEXT: "text/plain",
  HTML: "text/html",
} as const;

// HTTP methods
export const HTTP_METHODS = {
  GET: "GET",
  POST: "POST",
  PUT: "PUT",
  DELETE: "DELETE",
  PATCH: "PATCH",
  OPTIONS: "OPTIONS",
  HEAD: "HEAD",
} as const;

// HTTP status codes
export const HTTP_STATUS = {
  OK: 200,
  CREATED: 201,
  NO_CONTENT: 204,
  BAD_REQUEST: 400,
  UNAUTHORIZED: 401,
  FORBIDDEN: 403,
  NOT_FOUND: 404,
  METHOD_NOT_ALLOWED: 405,
  CONFLICT: 409,
  INTERNAL_SERVER_ERROR: 500,
  SERVICE_UNAVAILABLE: 503,
} as const;

// API base paths
export const API_PATHS = {
  BASE: "/api",
  EVENTS: "/api/events",
  ALERTS: "/api/alerts",
  TENANTS: "/api/tenants",
  USERS: "/api/users",
  ROLES: "/api/roles",
  PERMISSIONS: "/api/permissions",
  KEYCLOAK: "/api/keycloak",
  AUTH: "/api/auth",
} as const;

// Common request headers configuration
export const DEFAULT_HEADERS = {
  [HTTP_HEADERS.CONTENT_TYPE]: CONTENT_TYPES.JSON,
  [HTTP_HEADERS.ACCEPT]: CONTENT_TYPES.JSON,
} as const;

// Type exports for TypeScript
export type HttpMethod = (typeof HTTP_METHODS)[keyof typeof HTTP_METHODS];
export type ContentType = (typeof CONTENT_TYPES)[keyof typeof CONTENT_TYPES];
export type HttpStatus = (typeof HTTP_STATUS)[keyof typeof HTTP_STATUS];
export type ApiPath = (typeof API_PATHS)[keyof typeof API_PATHS];
