# Shared Constants - Application Constants

## Purpose
Constant values used across the application.

## Rules

### ✅ CAN
- Define static values
- Define configuration constants
- Define enum-like objects
- Be imported by any feature

### ❌ CANNOT
- Contain functions
- Have runtime logic
- Import from features
- Change at runtime

## Structure
```tsx
// index.ts - Main exports
export * from './api'
export * from './routes'
export * from './time'
```

## Constant Categories

### API Constants
```tsx
// api.ts
export const API_BASE_URL = '/api'
export const AUTH_BASE_URL = '/auth'

export const API_ENDPOINTS = {
  tenants: '/tenants',
  alerts: '/alerts',
  events: '/events',
  health: '/health',
} as const
```

### Route Constants
```tsx
// routes.ts
export const ROUTES = {
  home: '/',
  login: '/login',
  dashboard: '/:tenantId/dashboard',
  alerts: '/:tenantId/alerts',
  events: '/:tenantId/events',
  realms: '/:tenantId/realms',
  settings: '/settings',
  admin: {
    users: '/admin/users',
    roles: '/admin/roles',
  },
} as const
```

### Time Constants
```tsx
// time.ts
export const TIME_RANGES = {
  LAST_HOUR: 60 * 60 * 1000,
  LAST_24H: 24 * 60 * 60 * 1000,
  LAST_7D: 7 * 24 * 60 * 60 * 1000,
  LAST_30D: 30 * 24 * 60 * 60 * 1000,
} as const

export const DATE_FORMATS = {
  display: 'MMM dd, yyyy',
  displayWithTime: 'MMM dd, yyyy HH:mm',
  iso: "yyyy-MM-dd'T'HH:mm:ss.SSSxxx",
  time: 'HH:mm:ss',
} as const
```

### UI Constants
```tsx
// ui.ts
export const PAGINATION = {
  defaultPageSize: 20,
  pageSizeOptions: [10, 20, 50, 100],
} as const

export const BREAKPOINTS = {
  sm: 640,
  md: 768,
  lg: 1024,
  xl: 1280,
} as const
```

## Conventions

### Naming
- Use SCREAMING_SNAKE_CASE for primitives
- Use PascalCase or camelCase for objects

### `as const`
Always use `as const` for object constants to get literal types:
```tsx
// This gives type: { home: string }
const ROUTES = { home: '/' }

// This gives type: { readonly home: '/' }
const ROUTES = { home: '/' } as const
```

### Organization
Group related constants in separate files, export all from index.ts.
