# Shared Types - Global Type Definitions

## Purpose
TypeScript types and interfaces used across multiple features.

## Rules

### ✅ CAN
- Define generic API response types
- Define common utility types
- Define shared enums
- Be imported by any feature

### ❌ CANNOT
- Define domain-specific types (those go in features)
- Contain runtime code
- Import from features

## Type Categories

### API Types (api.types.ts)
```tsx
// Generic API response wrapper
interface ApiResponse<T> {
  data: T
  message?: string
  timestamp: string
}

// Pagination
interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
  hasMore: boolean
}

// Error response
interface ApiError {
  code: string
  message: string
  details?: Record<string, unknown>
}

// Request options
interface RequestOptions {
  signal?: AbortSignal
  headers?: Record<string, string>
}
```

### Common Types (common.types.ts)
```tsx
// Nullable helper
type Nullable<T> = T | null

// Optional helper
type Optional<T> = T | undefined

// ID types
type UUID = string
type Timestamp = string // ISO 8601

// Common entities
interface BaseEntity {
  id: UUID
  createdAt: Timestamp
  updatedAt?: Timestamp
}

// Select option
interface SelectOption<T = string> {
  label: string
  value: T
  disabled?: boolean
}

// Key-value pair
interface KeyValue<T = string> {
  key: string
  value: T
}
```

## Conventions

### Naming
- Interfaces: PascalCase (`UserResponse`)
- Types: PascalCase (`Nullable`)
- Enums: PascalCase with singular name (`Status`, not `Statuses`)

### Generic Types
Use descriptive generic parameter names:
```tsx
// Good
interface ApiResponse<TData>
interface PaginatedList<TItem>

// Avoid
interface ApiResponse<T>
```

### Documentation
Add JSDoc for non-obvious types:
```tsx
/**
 * Represents a time range selection
 * @property start - ISO 8601 timestamp
 * @property end - ISO 8601 timestamp
 */
interface TimeRange {
  start: Timestamp
  end: Timestamp
}
```

## Feature Types
Domain-specific types should NOT be here. They belong in their feature:
- `Alert` → `features/alerts/types/`
- `Tenant` → `features/tenants/types/`
- `User` → `features/users/types/`
