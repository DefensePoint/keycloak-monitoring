# Feature: Realms

## Domain
Keycloak realm monitoring and management.

## Responsibilities
- List monitored realms per tenant
- Realm selection and switching
- Realm metrics visualization
- Realm users and clients listing
- Realm-specific event filtering

## Structure
```
realms/
├── components/
│   ├── RealmSelector.tsx       → Realm dropdown
│   ├── RealmDetails.tsx        → Realm info display
│   ├── RealmCard.tsx           → Realm summary card
│   └── RealmMetrics.tsx        → Metrics visualization
├── hooks/
│   ├── useRealms.ts            → Realm list
│   ├── useRealmUsers.ts        → Users of a realm
│   └── useRealmClients.ts      → Clients of a realm
├── services/
│   └── realmService.ts         → Realm API calls
├── types/
│   └── realm.types.ts          → Realm, RealmMetrics
├── pages/
│   ├── RealmsPage.tsx          → Realm list
│   └── RealmDetailPage.tsx     → Realm details
└── index.ts
```

## Public API (index.ts)
```tsx
export { RealmsPage, RealmDetailPage } from './pages'
export { RealmSelector, RealmCard } from './components'
export { useRealms, useCurrentRealm } from './hooks'
export type { Realm, RealmUser, RealmClient } from './types'
```

## Key Types
```tsx
interface Realm {
  name: string
  displayName?: string
  enabled: boolean
  userCount?: number
  clientCount?: number
}

interface RealmMetrics {
  activeUsers: number
  totalSessions: number
  loginEvents: number
  failedLogins: number
}
```

## Realm Selection
- Stored in sessionStorage per tenant
- Persists during browser session
- Resets when switching tenants

## API Endpoints
- `GET /tenants/:tenantId/realms` - List realms
- `GET /tenants/:tenantId/realms/:realm/users` - Realm users
- `GET /tenants/:tenantId/realms/:realm/clients` - Realm clients
