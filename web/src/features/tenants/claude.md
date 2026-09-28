# Feature: Tenants

## Domain
Multi-tenant management and tenant context.

## Responsibilities
- List available tenants
- Tenant selection and switching
- Tenant CRUD operations (admin)
- Tenant health status
- Tenant-scoped navigation

## Structure
```
tenants/
├── components/
│   ├── TenantSelector.tsx      → Dropdown for tenant selection
│   ├── TenantCard.tsx          → Tenant display card
│   └── TenantHealthBadge.tsx   → Health status indicator
├── hooks/
│   └── useTenants.ts           → Tenant data and selection
├── services/
│   └── tenantService.ts        → Tenant API calls
├── context/
│   └── TenantContext.tsx       → Global tenant state
├── types/
│   └── tenant.types.ts         → Tenant, TenantHealth
├── pages/
│   ├── TenantsPage.tsx         → Tenant list
│   └── TenantDetailPage.tsx    → Tenant details
└── index.ts
```

## Public API (index.ts)
```tsx
export { TenantsPage, TenantDetailPage } from './pages'
export { TenantSelector } from './components'
export { useTenants, useCurrentTenant } from './hooks'
export { TenantProvider, TenantContext } from './context'
export type { Tenant, TenantHealth } from './types'
```

## Key Types
```tsx
interface Tenant {
  id: string
  name: string
  slug: string
  keycloakUrl: string
  enabled: boolean
  createdAt: string
}

interface TenantHealth {
  status: 'healthy' | 'degraded' | 'unhealthy'
  lastCheck: string
  details?: Record<string, unknown>
}
```

## URL Pattern
All tenant-scoped routes follow: `/:tenantId/...`

## Context Usage
```tsx
const { tenants, currentTenant, setCurrentTenant } = useTenants()
```

## Permissions
- `tenants:read` - View tenant list
- `tenants:write` - Create/update tenants
- `tenants:delete` - Delete tenants
