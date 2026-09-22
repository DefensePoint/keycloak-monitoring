// Generic API response wrapper
export interface ApiResponse<T> {
  data: T;
  message?: string;
  timestamp?: string;
}

// Pagination
export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
  hasMore: boolean;
}

// Error response
export interface ApiError {
  code: string;
  message: string;
  details?: Record<string, unknown>;
}

// Common ID types
export type UUID = string;
export type Timestamp = string; // ISO 8601

// Base entity with common fields
export interface BaseEntity {
  id: UUID;
  created_at?: Timestamp;
  updated_at?: Timestamp;
}

// Select option for dropdowns
export interface SelectOption<T = string> {
  label: string;
  value: T;
  disabled?: boolean;
}

// Key-value pair
export interface KeyValue<T = string> {
  key: string;
  value: T;
}

// Nullable helper
export type Nullable<T> = T | null;

// Optional helper
export type Optional<T> = T | undefined;
