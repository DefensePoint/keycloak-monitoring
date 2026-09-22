# Shared Lib - External Library Configuration

## Purpose
Wrappers and configurations for external libraries.

## Structure
```
lib/
├── apiClient.ts      → HTTP client (Fetch/Axios wrapper)
├── queryClient.ts    → React Query configuration
└── index.ts          → Re-exports
```

## API Client

### Configuration
```tsx
// apiClient.ts
const BASE_URL = '/api'

export const apiClient = {
  get: <T>(url: string) => fetch(`${BASE_URL}${url}`, {
    credentials: 'include',
  }).then(res => res.json() as Promise<T>),

  post: <T>(url: string, data: unknown) => fetch(`${BASE_URL}${url}`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  }).then(res => res.json() as Promise<T>),

  // ... put, patch, delete
}
```

### Features
- Base URL configuration
- Credentials included (session auth)
- JSON content type by default
- Generic response typing

## Query Client

### Configuration
```tsx
// queryClient.ts
import { QueryClient } from '@tanstack/react-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 60_000, // 1 minute
    },
    mutations: {
      retry: 0,
    },
  },
})
```

## Rules

### ✅ CAN
- Configure external libraries
- Create wrapper functions
- Handle global error handling
- Set default configurations

### ❌ CANNOT
- Contain business logic
- Know about specific endpoints
- Import from features

## Adding New Libraries
When adding a new external library:
1. Create a wrapper file in `lib/`
2. Export configured instance
3. Document configuration options
4. Features import from here, not directly from the library
