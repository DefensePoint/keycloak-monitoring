# Features - Domain Modules

## Purpose
Each feature is an autonomous module that encapsulates all logic for a business domain.

## Feature Structure
```
feature-name/
├── claude.md       → Feature-specific directives
├── components/     → React components specific to this feature and tests
├── hooks/          → Custom hooks
├── services/       → API calls
├── context/        → Context providers (if needed)
├── types/          → TypeScript types/interfaces
├── pages/          → Page components (routes)
├── utils/          → Feature-specific utilities (optional)
└── index.ts        → Public API (barrel export)
```

## Fundamental Rules

### ✅ CAN
- Import from `@/shared/*`
- Import from `@/theme/*`
- Have local state (Context, hooks)
- Make API calls via services

### ❌ CANNOT
- Import from other features (NEVER)
- Export internal components directly
- Access state from other features

## Cross-Feature Communication
When features need to communicate:
1. **Via URL/Router** - pass data through query params or path
2. **Via Shared Context** - contexts in `app/providers`
3. **Via Events** - custom events for decoupling

## Barrel Export (index.ts)
Each feature exposes only its public API:

```tsx
// features/auth/index.ts
// Pages (for lazy loading in router)
export { LoginPage } from './pages/LoginPage'

// Public hooks
export { useAuth, usePermission } from './hooks'

// Public components (used by other features via shared)
// If a component needs to be used outside, move it to shared/

// Public types
export type { User, Permission } from './types'
```

## Conventions

### File Naming
- `components/` → PascalCase: `AlertCard.tsx` and `AlertCard.test.tsx`
- `hooks/` → camelCase: `useAlerts.ts`
- `services/` → camelCase: `alertService.ts`
- `types/` → kebab-case: `alert.types.ts`
- `pages/` → PascalCase with suffix: `AlertsPage.tsx`

### Services
```tsx
// services/alertService.ts
import { apiClient } from '@/shared/lib/apiClient'
import type { Alert } from '../types/alert.types'

export const alertService = {
  getAll: (tenantId: string) =>
    apiClient.get<Alert[]>(`/tenants/${tenantId}/alerts`),

  getById: (tenantId: string, id: string) =>
    apiClient.get<Alert>(`/tenants/${tenantId}/alerts/${id}`),
}
```

### Hooks with React Query

#### Query Hook (for fetching data)
```tsx
// hooks/useAlerts.ts
import { useQuery } from '@tanstack/react-query'
import { alertService } from '../services/alertService'

export const useAlerts = (tenantId: string) => {
  return useQuery({
    queryKey: ['alerts', tenantId],
    queryFn: () => alertService.getAll(tenantId),
    enabled: !!tenantId, // Only fetch when tenantId exists
  })
}
```

#### Mutation Hook (for creating/updating/deleting)
```tsx
// hooks/useUpdateAlert.ts
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { alertService } from '../services/alertService'

export const useUpdateAlert = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ tenantId, alertId, data }) =>
      alertService.update(tenantId, alertId, data),
    onSuccess: (_, variables) => {
      // Invalidate related queries to refetch
      queryClient.invalidateQueries({ queryKey: ['alerts'] })
      queryClient.invalidateQueries({ queryKey: ['alert', variables.alertId] })
    },
  })
}
```

#### Using Hooks in Components
```tsx
// pages/AlertsPage.tsx
export function AlertsPage() {
  const { tenantId } = useTenant()
  const { data: alerts, isLoading, error } = useAlerts(tenantId)
  const { mutate: updateAlert, isPending } = useUpdateAlert()

  // NO useState for loading/error
  // NO try/catch for API calls
  // NO direct service calls

  if (isLoading) return <Loading />
  if (error) return <Error message={error.message} />

  return <AlertsList alerts={alerts} onUpdate={updateAlert} />
}
```

### Anti-patterns to Avoid
```tsx
// ❌ WRONG: Direct API call in component
const handleSubmit = async () => {
  try {
    setLoading(true)
    await alertService.update(data)
    setLoading(false)
  } catch (err) {
    setError(err.message)
  }
}

// ✅ CORRECT: Use mutation hook
const { mutate, isPending, error } = useUpdateAlert()
const handleSubmit = () => mutate(data)
```
